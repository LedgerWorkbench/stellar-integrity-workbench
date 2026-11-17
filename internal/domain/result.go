package domain

import (
	"fmt"
)

// SemanticProjection contains the bounded, canonical semantic fields extracted
// from a decoded Stellar input.
//
// These fields represent the normalized interpretation produced by an adapter
// for cross-SDK and cross-implementation comparison.
type SemanticProjection struct {
	DecodedType       string   `json:"decoded_type"`
	LedgerSequence    *int64   `json:"ledger_sequence,omitempty"`
	TransactionCount  *int     `json:"transaction_count,omitempty"`
	TransactionHashes []string `json:"transaction_hashes,omitempty"`
	ProtocolVersion   *int     `json:"protocol_version,omitempty"`
}

// DeriveOutcome computes the overall RunOutcome from a list of assertion results
// according to the Phase 6 assertion policy (Section 11):
//
//  1. If any assertion failed -> RunOutcomeRegression
//  2. If any assertion was inconclusive (and none failed) -> RunOutcomeInconclusive
//  3. If all assertions were skipped -> RunOutcomeNotApplicable
//  4. If all assertions passed (or passed + skipped) -> RunOutcomePass
func DeriveOutcome(results []AssertionResult) (RunOutcome, error) {
	if len(results) == 0 {
		return "", fmt.Errorf("cannot derive outcome: assertion results list is empty")
	}

	hasFail := false
	hasInconclusive := false
	hasPass := false
	allSkipped := true

	for _, r := range results {
		switch r.Status {
		case AssertionStatusFail:
			hasFail = true
			allSkipped = false
		case AssertionStatusInconclusive:
			hasInconclusive = true
			allSkipped = false
		case AssertionStatusPass:
			hasPass = true
			allSkipped = false
		case AssertionStatusSkipped:
			// skipped does not clear allSkipped
		default:
			return "", fmt.Errorf("unexpected assertion status %q in result for assertion %q", r.Status, r.AssertionID)
		}
	}

	if hasFail {
		return RunOutcomeRegression, nil
	}
	if hasInconclusive {
		return RunOutcomeInconclusive, nil
	}
	if allSkipped {
		return RunOutcomeNotApplicable, nil
	}
	if hasPass {
		return RunOutcomePass, nil
	}

	return RunOutcomeInconclusive, nil
}
