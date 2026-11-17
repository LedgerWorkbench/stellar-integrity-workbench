package domain_test

import (
	"testing"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/domain"
)

func TestAssertionKind_Validation(t *testing.T) {
	kinds := domain.AllAssertionKinds()
	if len(kinds) != 5 {
		t.Fatalf("expected 5 assertion kinds, got %d", len(kinds))
	}

	for _, k := range kinds {
		if !k.IsValid() {
			t.Errorf("expected kind %q to be valid", k)
		}
		if err := k.Validate(); err != nil {
			t.Errorf("expected Validate() nil for %q, got: %v", k, err)
		}
	}

	invalid := domain.AssertionKind("custom_expression")
	if invalid.IsValid() {
		t.Errorf("expected %q to be invalid", invalid)
	}
	if err := invalid.Validate(); err == nil {
		t.Errorf("expected Validate() error for %q", invalid)
	}
}

func TestAssertionStatus_Validation(t *testing.T) {
	statuses := domain.AllAssertionStatuses()
	if len(statuses) != 4 {
		t.Fatalf("expected 4 assertion statuses, got %d", len(statuses))
	}

	for _, s := range statuses {
		if !s.IsValid() {
			t.Errorf("expected status %q to be valid", s)
		}
		if err := s.Validate(); err != nil {
			t.Errorf("expected Validate() nil for %q, got: %v", s, err)
		}
	}

	invalid := domain.AssertionStatus("errored")
	if invalid.IsValid() {
		t.Errorf("expected %q to be invalid", invalid)
	}
	if err := invalid.Validate(); err == nil {
		t.Errorf("expected Validate() error for %q", invalid)
	}
}
