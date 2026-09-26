package core

import "time"

type CheckStatus string

const (
	StatusPass CheckStatus = "pass"
	StatusFail CheckStatus = "fail"
	StatusSkip CheckStatus = "skip"
)

type ProbeResult struct {
	Name        string
	Status      CheckStatus
	FailureKind string
	Latency     time.Duration
	Evidence    []string
}

type Candidate struct {
	Cause      string
	Confidence string
	Evidence   []string
}

type Diagnosis struct {
	FailingLayer string
	Summary      string
	FixHint      string
	Candidates   []Candidate
}
