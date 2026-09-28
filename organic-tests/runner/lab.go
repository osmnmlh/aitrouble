package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

type Runner struct {
	repo, organicRoot, lab string
	runID                  string
	seed                   int64
	report                 Report
	bin                    BuildIdentity
	results                []ScenarioResult
	sequence               int
}

func newRunner(seed int64) (*Runner, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	organicRoot := filepath.Dir(cwd)
	repo := filepath.Dir(organicRoot)
	runID := fmt.Sprintf("%s-seed-%d", time.Now().UTC().Format("20060102T150405Z"), seed)
	lab := filepath.Join(organicRoot, "lab", runID)
	for _, dir := range []string{lab, filepath.Join(lab, "bin"), filepath.Join(lab, "projects"), filepath.Join(lab, "configs"), filepath.Join(lab, "services"), filepath.Join(lab, "processes"), filepath.Join(lab, "artifacts"), filepath.Join(lab, "reports"), filepath.Join(lab, "seeds"), filepath.Join(lab, "backups"), filepath.Join(organicRoot, "reports")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
	}
	r := &Runner{repo: repo, organicRoot: organicRoot, lab: lab, runID: runID, seed: seed}
	harnessSHA, _ := commandCombined(cwd, nil, "git", "rev-parse", "HEAD")
	r.report = Report{RunID: runID, MasterSeed: seed, StartedAt: time.Now().UTC().Format(time.RFC3339Nano), LabDirectory: lab, Totals: map[string]int{}, HarnessCommit: strings.TrimSpace(harnessSHA)}
	return r, nil
}

func commandOutput(dir string, env []string, name string, args ...string) (int, string, string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		if x, ok := err.(*exec.ExitError); ok {
			code = x.ExitCode()
		} else {
			code = -1
		}
	}
	return code, stdout.String(), stderr.String(), err
}

func commandCombined(dir string, env []string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}
	b, err := cmd.CombinedOutput()
	return string(b), err
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func sha256Bytes(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func (r *Runner) buildSubject() error {
	commit, err := commandCombined(r.repo, nil, "git", "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("git commit: %w", err)
	}
	describe, err := commandCombined(r.repo, nil, "git", "describe", "--always", "--dirty")
	if err != nil {
		return fmt.Errorf("git describe: %w", err)
	}
	goVersion, err := commandCombined(r.repo, nil, "go", "version")
	if err != nil {
		return fmt.Errorf("go version: %w", err)
	}
	version := "organic-" + strings.TrimSpace(describe)
	path := filepath.Join(r.lab, "bin", "aitrouble-windows-amd64.exe")
	// Match the GoReleaser build contract: trimmed paths, CGO disabled and an
	// injected non-dev version. This is the Windows release-style binary that
	// every organic scenario executes.
	releaseEnv := append(os.Environ(), "CGO_ENABLED=0", "GOOS=windows", "GOARCH=amd64")
	out, err := commandCombined(r.repo, releaseEnv, "go", "build", "-trimpath", "-o", path, "-ldflags", "-s -w -X main.version="+version, "./cmd/aitrouble")
	if err != nil {
		return fmt.Errorf("candidate build: %w: %s", err, out)
	}
	hash, err := sha256File(path)
	if err != nil {
		return err
	}
	_, versionOut, _, runErr := commandOutput(r.repo, nil, path, "--version")
	if runErr != nil {
		return fmt.Errorf("candidate version: %w", runErr)
	}
	r.bin = BuildIdentity{Path: path, SHA256: hash, Commit: strings.TrimSpace(commit), Describe: strings.TrimSpace(describe), Version: strings.TrimSpace(versionOut), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, GoVersion: strings.TrimSpace(goVersion)}
	r.report.Subject = r.bin
	return nil
}

func cleanChildEnv(home, appdata string, values map[string]*string) []string {
	remove := map[string]bool{"OPENAI_BASE_URL": true, "OPENAI_API_KEY": true, "HOME": true, "USERPROFILE": true, "HOMEDRIVE": true, "HOMEPATH": true, "APPDATA": true, "XDG_CONFIG_HOME": true}
	var out []string
	for _, pair := range os.Environ() {
		key, _, _ := strings.Cut(pair, "=")
		if !remove[strings.ToUpper(key)] {
			out = append(out, pair)
		}
	}
	homeDrive, homePath := filepath.VolumeName(home), strings.TrimPrefix(home, filepath.VolumeName(home))
	out = append(out, "HOME="+home, "USERPROFILE="+home, "HOMEDRIVE="+homeDrive, "HOMEPATH="+homePath, "APPDATA="+appdata, "XDG_CONFIG_HOME="+filepath.Join(home, ".config"))
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if values[k] != nil {
			out = append(out, k+"="+*values[k])
		}
	}
	return out
}

type doctorRun struct {
	code           int
	stdout, stderr string
	duration       time.Duration
}

func (r *Runner) doctor(ctx context.Context, binary, project, home, appdata string, values map[string]*string, extraArgs ...string) doctorRun {
	args := append([]string{"doctor"}, extraArgs...)
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = project
	cmd.Env = cleanChildEnv(home, appdata, values)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	started := time.Now()
	err := cmd.Run()
	code := 0
	if err != nil {
		if x, ok := err.(*exec.ExitError); ok {
			code = x.ExitCode()
		} else {
			code = -1
		}
	}
	return doctorRun{code: code, stdout: stdout.String(), stderr: stderr.String(), duration: time.Since(started)}
}

func directoryDigest(root string) (string, error) {
	var items []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if rel == "." {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			items = append(items, "D|"+filepath.ToSlash(rel))
			return nil
		}
		if !info.Mode().IsRegular() {
			items = append(items, "O|"+filepath.ToSlash(rel)+"|"+info.Mode().String())
			return nil
		}
		h, err := sha256File(path)
		if err != nil {
			return err
		}
		items = append(items, fmt.Sprintf("F|%s|%d|%s", filepath.ToSlash(rel), info.Size(), h))
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(items)
	return sha256Bytes([]byte(strings.Join(items, "\n"))), nil
}

func directTCP(addr string) error {
	c, err := net.DialTimeout("tcp", addr, 1200*time.Millisecond)
	if err == nil {
		_ = c.Close()
	}
	return err
}

func closedPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	p := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		return 0, err
	}
	return p, nil
}

func nullable(s string) *string { return &s }

func writeText(path, text string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(text), 0600)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
