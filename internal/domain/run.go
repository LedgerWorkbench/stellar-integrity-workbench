package domain

import (
	"fmt"
	"time"
)

// RunState represents the lifecycle state of a replay execution.
//
// The state machine strictly follows:
//
//	queued -> fetching -> verifying_input -> decoding -> processing -> asserting -> reporting -> completed
//
// With terminal alternative states: failed, cancelled.
type RunState string

const (
	RunStateQueued         RunState = "queued"
	RunStateFetching       RunState = "fetching"
	RunStateVerifyingInput RunState = "verifying_input"
	RunStateDecoding       RunState = "decoding"
	RunStateProcessing     RunState = "processing"
	RunStateAsserting      RunState = "asserting"
	RunStateReporting      RunState = "reporting"
	RunStateCompleted      RunState = "completed"
	RunStateFailed         RunState = "failed"
	RunStateCancelled      RunState = "cancelled"
)

// AllRunStates returns all valid execution states.
func AllRunStates() []RunState {
	return []RunState{
		RunStateQueued,
		RunStateFetching,
		RunStateVerifyingInput,
		RunStateDecoding,
		RunStateProcessing,
		RunStateAsserting,
		RunStateReporting,
		RunStateCompleted,
		RunStateFailed,
		RunStateCancelled,
	}
}

// IsValid reports whether the state is recognized.
func (s RunState) IsValid() bool {
	switch s {
	case RunStateQueued,
		RunStateFetching,
		RunStateVerifyingInput,
		RunStateDecoding,
		RunStateProcessing,
		RunStateAsserting,
		RunStateReporting,
		RunStateCompleted,
		RunStateFailed,
		RunStateCancelled:
		return true
	default:
		return false
	}
}

// IsTerminal reports whether the state is a final state.
func (s RunState) IsTerminal() bool {
	return s == RunStateCompleted || s == RunStateFailed || s == RunStateCancelled
}

// Validate checks that the run state is valid.
func (s RunState) Validate() error {
	if !s.IsValid() {
		return fmt.Errorf("invalid run state %q; must be one of: %v", s, AllRunStates())
	}
	return nil
}

// RunOutcome represents the semantic conformance outcome of an evaluated run.
//
// This is strictly decoupled from the execution state: an execution can complete
// successfully with a regression outcome. Operational errors (timeouts, crashes)
// belong in RunState (failed), never in RunOutcome.
type RunOutcome string

const (
	// RunOutcomePass indicates all required assertions passed against expected behaviour.
	RunOutcomePass RunOutcome = "pass"

	// RunOutcomeRegression indicates one or more required assertions failed against
	// the declared expected behaviour.
	RunOutcomeRegression RunOutcome = "regression"

	// RunOutcomeInconclusive indicates the run completed but one or more required
	// assertions could not be evaluated.
	RunOutcomeInconclusive RunOutcome = "inconclusive"

	// RunOutcomeNotApplicable indicates the selected adapter does not implement
	// the required capability.
	RunOutcomeNotApplicable RunOutcome = "not_applicable"
)

// AllRunOutcomes returns all recognized run outcomes.
func AllRunOutcomes() []RunOutcome {
	return []RunOutcome{
		RunOutcomePass,
		RunOutcomeRegression,
		RunOutcomeInconclusive,
		RunOutcomeNotApplicable,
	}
}

// IsValid reports whether the run outcome is recognized.
func (o RunOutcome) IsValid() bool {
	switch o {
	case RunOutcomePass,
		RunOutcomeRegression,
		RunOutcomeInconclusive,
		RunOutcomeNotApplicable:
		return true
	default:
		return false
	}
}

// Validate checks that the run outcome is valid.
func (o RunOutcome) Validate() error {
	if !o.IsValid() {
		return fmt.Errorf("invalid run outcome %q; must be one of: %v", o, AllRunOutcomes())
	}
	return nil
}

// Run represents a single replay and conformance execution run record.
type Run struct {
	RunID       string      `json:"run_id"`
	CaseID      string      `json:"case_id"`
	AdapterID   string      `json:"adapter_id"`
	State       RunState    `json:"state"`
	Outcome     *RunOutcome `json:"outcome,omitempty"`
	StartedAt   time.Time   `json:"started_at"`
	CompletedAt *time.Time  `json:"completed_at,omitempty"`
	ErrorDetail *string     `json:"error_detail,omitempty"`
}
