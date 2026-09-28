package main

import "time"

// Status values deliberately distinguish unexecuted required work from a pass.
type Status string

const (
	StatusPass    Status = "PASS"
	StatusFail    Status = "FAIL"
	StatusSkip    Status = "SKIP"
	StatusBlocked Status = "BLOCKED"
	StatusFlaky   Status = "FLAKY"
)

type BuildIdentity struct {
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	Commit    string `json:"commit"`
	Describe  string `json:"describe"`
	Version   string `json:"version"`
	GOOS      string `json:"goos"`
	GOARCH    string `json:"goarch"`
	GoVersion string `json:"go_version"`
}

type Expectation struct {
	ExitCode             int    `json:"exit_code"`
	FailingLayer         string `json:"failing_layer"`
	FailureKind          string `json:"failure_kind"`
	ProviderRequests     int    `json:"provider_requests"`
	ProviderRequest      string `json:"provider_request"`
	DownstreamProviderSK bool   `json:"downstream_provider_skipped"`
}

type Observation struct {
	ExitCode             int      `json:"exit_code"`
	FailingLayer         string   `json:"failing_layer"`
	FailureKind          string   `json:"failure_kind"`
	ProviderRequests     int      `json:"provider_requests"`
	ProviderRequest      string   `json:"provider_request"`
	DownstreamProviderSK bool     `json:"downstream_provider_skipped"`
	ProviderMethods      []string `json:"provider_methods,omitempty"`
	ProviderPaths        []string `json:"provider_paths,omitempty"`
	AuthorizationPresent []bool   `json:"authorization_present,omitempty"`
}

type ScenarioResult struct {
	ID             string             `json:"id"`
	Name           string             `json:"name"`
	Category       string             `json:"category"`
	Status         Status             `json:"status"`
	Triage         string             `json:"triage,omitempty"`
	Seed           int64              `json:"seed"`
	Timestamp      string             `json:"timestamp"`
	Project        string             `json:"project"`
	ArtifactDir    string             `json:"artifact_dir"`
	Precondition   string             `json:"precondition"`
	FaultInjection string             `json:"fault_injection"`
	Expected       Expectation        `json:"expected"`
	Observed       Observation        `json:"observed"`
	Assertions     map[string]bool    `json:"assertions"`
	Evidence       []string           `json:"evidence"`
	SecretScan     SecretScan         `json:"secret_scan"`
	Filesystem     Integrity          `json:"filesystem_integrity"`
	Environment    Integrity          `json:"environment_integrity"`
	Process        ProcessObservation `json:"process_observation"`
	DurationMS     int64              `json:"duration_ms"`
	Binary         BuildIdentity      `json:"binary"`
	ReplayStatus   Status             `json:"replay_status,omitempty"`
}

type Integrity struct {
	BeforeSHA256 string `json:"before_sha256,omitempty"`
	AfterSHA256  string `json:"after_sha256,omitempty"`
	Unchanged    bool   `json:"unchanged"`
	Restored     bool   `json:"restored"`
	Detail       string `json:"detail,omitempty"`
}

type SecretScan struct {
	CanarySHA256 string   `json:"canary_sha256,omitempty"`
	Leaked       bool     `json:"leaked"`
	Locations    []string `json:"locations,omitempty"`
}

type ProcessObservation struct {
	MarkerPath      string `json:"marker_path,omitempty"`
	MarkerFound     bool   `json:"marker_found"`
	UnexpectedChild bool   `json:"unexpected_child"`
	ListenerClosed  bool   `json:"listener_closed"`
}

type Report struct {
	RunID         string           `json:"run_id"`
	MasterSeed    int64            `json:"master_seed"`
	StartedAt     string           `json:"started_at"`
	FinishedAt    string           `json:"finished_at"`
	LabDirectory  string           `json:"lab_directory"`
	Subject       BuildIdentity    `json:"subject"`
	HarnessCommit string           `json:"harness_commit_sha"`
	ReleaseBuilds []ReleaseBuild   `json:"release_builds"`
	Results       []ScenarioResult `json:"results"`
	Totals        map[string]int   `json:"totals"`
	Verdict       string           `json:"verdict"`
	Notes         []string         `json:"notes"`
}

type ReleaseBuild struct {
	GOOS     string `json:"goos"`
	GOARCH   string `json:"goarch"`
	Status   Status `json:"status"`
	Artifact string `json:"artifact,omitempty"`
	SHA256   string `json:"sha256,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

type ProviderMode string

const (
	ProviderOK          ProviderMode = "ok"
	ProviderInvalidJSON ProviderMode = "invalid_json"
	ProviderMissingData ProviderMode = "missing_data"
	ProviderClose       ProviderMode = "connection_close"
	ProviderSlow        ProviderMode = "slow"
)

type ProviderRequest struct {
	Method               string
	Path                 string
	AuthorizationPresent bool
	At                   time.Time
}
