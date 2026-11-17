package domain_test

import (
	"testing"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/domain"
)

func TestRunState_Validation(t *testing.T) {
	validStates := domain.AllRunStates()
	if len(validStates) != 10 {
		t.Fatalf("expected 10 run states, got %d", len(validStates))
	}

	for _, s := range validStates {
		if !s.IsValid() {
			t.Errorf("expected state %q to be valid", s)
		}
		if err := s.Validate(); err != nil {
			t.Errorf("expected Validate() nil for %q, got: %v", s, err)
		}
	}

	// Check terminal states
	terminals := map[domain.RunState]bool{
		domain.RunStateCompleted: true,
		domain.RunStateFailed:    true,
		domain.RunStateCancelled: true,
	}

	for _, s := range validStates {
		expectedTerminal := terminals[s]
		if s.IsTerminal() != expectedTerminal {
			t.Errorf("state %q terminal mismatch: got %v, want %v", s, s.IsTerminal(), expectedTerminal)
		}
	}

	// Invalid state
	invalid := domain.RunState("executing")
	if invalid.IsValid() {
		t.Errorf("expected %q to be invalid", invalid)
	}
	if err := invalid.Validate(); err == nil {
		t.Errorf("expected Validate() error for %q", invalid)
	}
}

func TestRunOutcome_Validation(t *testing.T) {
	validOutcomes := domain.AllRunOutcomes()
	if len(validOutcomes) != 4 {
		t.Fatalf("expected 4 run outcomes, got %d", len(validOutcomes))
	}

	for _, o := range validOutcomes {
		if !o.IsValid() {
			t.Errorf("expected outcome %q to be valid", o)
		}
		if err := o.Validate(); err != nil {
			t.Errorf("expected Validate() nil for %q, got: %v", o, err)
		}
	}

	invalid := domain.RunOutcome("unknown_outcome")
	if invalid.IsValid() {
		t.Errorf("expected %q to be invalid", invalid)
	}
	if err := invalid.Validate(); err == nil {
		t.Errorf("expected Validate() error for %q", invalid)
	}
}
