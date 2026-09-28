package main

import (
	"encoding/base64"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	layerPattern   = regexp.MustCompile(`(?m)DIAGNOSIS\s+The chain breaks at:\s+(.+)$`)
	failurePattern = regexp.MustCompile(`(?m) \[([a-z0-9_]+)\]$`)
	sourcePattern  = regexp.MustCompile(`OPENAI_BASE_URL\s+.*source:\s+(shell|\.env|default|absent)`)
)

type labScenario struct {
	r                                *Runner
	result                           ScenarioResult
	project, home, appdata, artifact string
	started                          time.Time
	beforeFS, beforeEnv              string
	canary                           string
	wrote                            []string
	forced                           Status
}

func (r *Runner) begin(id, name, category string, seed int64) (*labScenario, error) {
	artifact := filepath.Join(r.lab, "artifacts", id)
	project := filepath.Join(r.lab, "projects", id)
	home := filepath.Join(r.lab, "configs", id, "home")
	appdata := filepath.Join(r.lab, "configs", id, "appdata")
	for _, path := range []string{artifact, project, home, appdata} {
		if err := os.MkdirAll(path, 0700); err != nil {
			return nil, err
		}
	}
	s := &labScenario{r: r, project: project, home: home, appdata: appdata, artifact: artifact, started: time.Now()}
	s.result = ScenarioResult{ID: id, Name: name, Category: category, Status: StatusFail, Seed: seed, Timestamp: time.Now().UTC().Format(time.RFC3339Nano), Project: project, ArtifactDir: artifact, Assertions: map[string]bool{}, Binary: r.bin}
	return s, nil
}

func (s *labScenario) random() *rand.Rand { return rand.New(rand.NewSource(s.result.Seed)) }

func parentEnvironmentState() string {
	keys := []string{"OPENAI_BASE_URL", "OPENAI_API_KEY", "HOME", "USERPROFILE", "HOMEDRIVE", "HOMEPATH", "APPDATA", "XDG_CONFIG_HOME"}
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		_, ok := os.LookupEnv(key)
		parts = append(parts, fmt.Sprintf("%s:%t", key, ok))
	}
	return sha256Bytes([]byte(strings.Join(parts, "|")))
}

func (s *labScenario) snapshot() error {
	parts := []string{}
	for _, root := range []string{s.project, s.home, s.appdata} {
		d, err := directoryDigest(root)
		if err != nil {
			return err
		}
		parts = append(parts, d)
	}
	s.beforeFS = sha256Bytes([]byte(strings.Join(parts, "|")))
	s.beforeEnv = parentEnvironmentState()
	return nil
}

func parseActual(run doctorRun, provider *controlledProvider) Observation {
	combined := run.stdout + "\n" + run.stderr
	o := Observation{ExitCode: run.code}
	if match := layerPattern.FindStringSubmatch(combined); len(match) == 2 {
		o.FailingLayer = strings.TrimSpace(match[1])
	} else if strings.Contains(combined, "All tested components appear healthy.") {
		o.FailingLayer = "None"
	}
	kinds := failurePattern.FindAllStringSubmatch(combined, -1)
	if len(kinds) > 0 {
		o.FailureKind = kinds[len(kinds)-1][1]
	}
	if o.FailingLayer == "None" {
		o.FailureKind = "None"
	}
	o.DownstreamProviderSK = strings.Contains(combined, "Provider Probe") && strings.Contains(combined, "skipped due to network failure")
	if provider != nil {
		po := provider.observation()
		o.ProviderRequests, o.ProviderMethods, o.ProviderPaths, o.AuthorizationPresent = po.ProviderRequests, po.ProviderMethods, po.ProviderPaths, po.AuthorizationPresent
	}
	if o.ProviderRequests == 0 {
		o.ProviderRequest = "no HTTP request observed"
	} else {
		o.ProviderRequest = strings.Join(o.ProviderMethods, ",") + " " + strings.Join(o.ProviderPaths, ",")
	}
	return o
}

func (s *labScenario) writeStage(name string, run doctorRun) {
	_ = os.WriteFile(filepath.Join(s.artifact, name+"-stdout.txt"), []byte(run.stdout), 0600)
	_ = os.WriteFile(filepath.Join(s.artifact, name+"-stderr.txt"), []byte(run.stderr), 0600)
	s.wrote = append(s.wrote, filepath.Join(s.artifact, name+"-stdout.txt"), filepath.Join(s.artifact, name+"-stderr.txt"))
}

func (s *labScenario) check(name string, value bool) { s.result.Assertions[name] = value }
func (s *labScenario) evidence(text string)          { s.result.Evidence = append(s.result.Evidence, text) }

func (s *labScenario) compare(run doctorRun, provider *controlledProvider, expected Expectation, authExpected *bool) Observation {
	actual := parseActual(run, provider)
	s.result.Expected, s.result.Observed = expected, actual
	s.check("exit_code", actual.ExitCode == expected.ExitCode)
	s.check("failing_layer", actual.FailingLayer == expected.FailingLayer)
	s.check("failure_kind", actual.FailureKind == expected.FailureKind)
	s.check("provider_request_count", actual.ProviderRequests == expected.ProviderRequests)
	s.check("downstream_provider_skip", actual.DownstreamProviderSK == expected.DownstreamProviderSK)
	if expected.ProviderRequests > 0 {
		s.check("provider_method_and_path", actual.ProviderRequest == expected.ProviderRequest)
		if authExpected != nil {
			matches := len(actual.AuthorizationPresent) == expected.ProviderRequests
			for _, present := range actual.AuthorizationPresent {
				matches = matches && present == *authExpected
			}
			s.check("authorization_header_presence", matches)
		}
	}
	return actual
}

func (s *labScenario) expectBaseSource(run doctorRun, expected string) {
	match := sourcePattern.FindStringSubmatch(run.stdout)
	actual := "absent"
	if len(match) == 2 {
		actual = match[1]
	}
	s.check("effective_base_url_source", actual == expected)
	s.evidence("effective OPENAI_BASE_URL source expected=" + expected + " observed=" + actual)
}

func (s *labScenario) secretCanary() string {
	// The canary itself is never written to reports. The seed and SHA-256 are
	// sufficient to reproduce a failed scenario without broadcasting a secret.
	v := fmt.Sprintf("ORGANIC_CANARY_%x_%x", uint64(s.result.Seed), uint64(s.random().Int63()))
	s.canary = v
	s.result.SecretScan.CanarySHA256 = sha256Bytes([]byte(v))
	return v
}

func (s *labScenario) scanSecrets() {
	if s.canary == "" {
		return
	}
	variants := []string{s.canary, base64.StdEncoding.EncodeToString([]byte(s.canary)), s.canary[:min(18, len(s.canary))]}
	for _, path := range s.wrote {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, variant := range variants {
			if strings.Contains(string(b), variant) {
				s.result.SecretScan.Leaked = true
				s.result.SecretScan.Locations = append(s.result.SecretScan.Locations, filepath.Base(path))
				break
			}
		}
	}
	s.check("secret_canary_absent_from_outputs", !s.result.SecretScan.Leaked)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (s *labScenario) finish() ScenarioResult {
	afterParts := []string{}
	for _, root := range []string{s.project, s.home, s.appdata} {
		d, err := directoryDigest(root)
		if err != nil {
			s.result.Filesystem.Detail = err.Error()
			s.result.Filesystem.Unchanged = false
			break
		}
		afterParts = append(afterParts, d)
	}
	afterFS := sha256Bytes([]byte(strings.Join(afterParts, "|")))
	s.result.Filesystem = Integrity{BeforeSHA256: s.beforeFS, AfterSHA256: afterFS, Unchanged: s.beforeFS != "" && s.beforeFS == afterFS, Restored: s.beforeFS != "" && s.beforeFS == afterFS}
	s.check("filesystem_unchanged", s.result.Filesystem.Unchanged)
	afterEnv := parentEnvironmentState()
	s.result.Environment = Integrity{BeforeSHA256: s.beforeEnv, AfterSHA256: afterEnv, Unchanged: s.beforeEnv == afterEnv, Restored: s.beforeEnv == afterEnv}
	s.check("parent_environment_unchanged", s.result.Environment.Unchanged)
	s.scanSecrets()
	passed := true
	for _, v := range s.result.Assertions {
		passed = passed && v
	}
	if s.forced != "" {
		s.result.Status = s.forced
	} else if passed {
		s.result.Status = StatusPass
	} else {
		s.result.Status = StatusFail
	}
	if s.result.Status == StatusFail && s.result.Triage == "" {
		s.result.Triage = "PRODUCT BUG OR HARNESS BUG — inspect independent fixture and artifact evidence"
	}
	s.result.DurationMS = time.Since(s.started).Milliseconds()
	s.result.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	// The observation serialisation intentionally contains only request metadata,
	// never a configured API key or MCP canary.
	_ = writeText(filepath.Join(s.artifact, "observation.txt"), fmt.Sprintf("status=%s\nexpected=%+v\nobserved=%+v\nassertions=%v\n", s.result.Status, s.result.Expected, s.result.Observed, s.result.Assertions))
	s.r.results = append(s.r.results, s.result)
	return s.result
}
