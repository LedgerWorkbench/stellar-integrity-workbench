package domain_test

import (
	"encoding/json"
	"strings"
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
			name: "pass and skipped yields inconclusive",
			results: []domain.AssertionResult{
				{AssertionID: "a1", Status: domain.AssertionStatusPass},
				{AssertionID: "a2", Status: domain.AssertionStatusSkipped},
			},
			wantOutcome: domain.RunOutcomeInconclusive,
		},
		{
			name: "fail and skipped yields regression",
			results: []domain.AssertionResult{
				{AssertionID: "a1", Status: domain.AssertionStatusFail},
				{AssertionID: "a2", Status: domain.AssertionStatusSkipped},
			},
			wantOutcome: domain.RunOutcomeRegression,
		},
		{
			name: "inconclusive and skipped yields inconclusive",
			results: []domain.AssertionResult{
				{AssertionID: "a1", Status: domain.AssertionStatusInconclusive},
				{AssertionID: "a2", Status: domain.AssertionStatusSkipped},
			},
			wantOutcome: domain.RunOutcomeInconclusive,
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

func TestSemanticProjection_TransactionHashesJSONRoundTrip(t *testing.T) {
	t.Run("nil_slice_preserved_as_null", func(t *testing.T) {
		orig := domain.SemanticProjection{
			DecodedType:       "LedgerCloseMeta",
			TransactionHashes: nil,
		}
		data, err := json.Marshal(orig)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}
		if !strings.Contains(string(data), `"transaction_hashes":null`) {
			t.Errorf("expected null transaction_hashes in JSON, got %s", string(data))
		}

		var restored domain.SemanticProjection
		if err := json.Unmarshal(data, &restored); err != nil {
			t.Fatalf("json.Unmarshal failed: %v", err)
		}
		if restored.TransactionHashes != nil {
			t.Errorf("expected nil TransactionHashes after round-trip, got %#v", restored.TransactionHashes)
		}
	})

	t.Run("non_nil_empty_slice_preserved_as_empty_array", func(t *testing.T) {
		orig := domain.SemanticProjection{
			DecodedType:       "LedgerCloseMeta",
			TransactionHashes: []string{},
		}
		data, err := json.Marshal(orig)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}
		if !strings.Contains(string(data), `"transaction_hashes":[]`) {
			t.Errorf("expected [] transaction_hashes in JSON, got %s", string(data))
		}

		var restored domain.SemanticProjection
		if err := json.Unmarshal(data, &restored); err != nil {
			t.Fatalf("json.Unmarshal failed: %v", err)
		}
		if restored.TransactionHashes == nil {
			t.Fatal("expected non-nil empty TransactionHashes after round-trip, got nil")
		}
		if len(restored.TransactionHashes) != 0 {
			t.Errorf("expected 0 hashes, got %d", len(restored.TransactionHashes))
		}
	})

	t.Run("populated_and_truncated_preserved", func(t *testing.T) {
		orig := domain.SemanticProjection{
			DecodedType:                "LedgerCloseMeta",
			TransactionHashes:          []string{"aaaa", "bbbb"},
			TransactionHashesTruncated: true,
		}
		data, err := json.Marshal(orig)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}

		var restored domain.SemanticProjection
		if err := json.Unmarshal(data, &restored); err != nil {
			t.Fatalf("json.Unmarshal failed: %v", err)
		}
		if len(restored.TransactionHashes) != 2 || restored.TransactionHashes[0] != "aaaa" || restored.TransactionHashes[1] != "bbbb" {
			t.Errorf("restored TransactionHashes = %#v, want [aaaa bbbb]", restored.TransactionHashes)
		}
		if !restored.TransactionHashesTruncated {
			t.Error("expected TransactionHashesTruncated = true after round-trip")
		}
	})
}
