package domain_test

import (
	"testing"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/domain"
)

func TestDeriveOutcome(t *testing.T) {
	tests := []struct {
		name        string
		results     []domain.AssertionResult
		wantOutcome domain.RunOutcome
		wantErr     bool
	}{
		{
			name:    "empty results",
			results: []domain.AssertionResult{},
			wantErr: true,
		},
		{
			name: "all pass",
			results: []domain.AssertionResult{
				{AssertionID: "a1", Status: domain.AssertionStatusPass},
				{AssertionID: "a2", Status: domain.AssertionStatusPass},
			},
			wantOutcome: domain.RunOutcomePass,
		},
		{
			name: "single fail yields regression",
			results: []domain.AssertionResult{
				{AssertionID: "a1", Status: domain.AssertionStatusPass},
				{AssertionID: "a2", Status: domain.AssertionStatusFail},
			},
			wantOutcome: domain.RunOutcomeRegression,
		},
		{
			name: "inconclusive without fail yields inconclusive",
			results: []domain.AssertionResult{
				{AssertionID: "a1", Status: domain.AssertionStatusPass},
				{AssertionID: "a2", Status: domain.AssertionStatusInconclusive},
			},
			wantOutcome: domain.RunOutcomeInconclusive,
		},
		{
			name: "fail takes precedence over inconclusive",
			results: []domain.AssertionResult{
				{AssertionID: "a1", Status: domain.AssertionStatusInconclusive},
				{AssertionID: "a2", Status: domain.AssertionStatusFail},
			},
			wantOutcome: domain.RunOutcomeRegression,
		},
		{
			name: "all skipped yields not applicable",
			results: []domain.AssertionResult{
				{AssertionID: "a1", Status: domain.AssertionStatusSkipped},
				{AssertionID: "a2", Status: domain.AssertionStatusSkipped},
			},
			wantOutcome: domain.RunOutcomeNotApplicable,
		},
		{
			name: "pass and skipped yields pass",
			results: []domain.AssertionResult{
				{AssertionID: "a1", Status: domain.AssertionStatusPass},
				{AssertionID: "a2", Status: domain.AssertionStatusSkipped},
			},
			wantOutcome: domain.RunOutcomePass,
		},
		{
			name: "invalid status returns error",
			results: []domain.AssertionResult{
				{AssertionID: "a1", Status: domain.AssertionStatus("invalid_status")},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			outcome, err := domain.DeriveOutcome(tc.results)
			if (err != nil) != tc.wantErr {
				t.Fatalf("DeriveOutcome() error = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && outcome != tc.wantOutcome {
				t.Errorf("DeriveOutcome() = %v, want %v", outcome, tc.wantOutcome)
			}
		})
	}
}
