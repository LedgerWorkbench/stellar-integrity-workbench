package domain

import (
	"fmt"
)

// AssertionKind identifies the specific semantic condition being asserted.
type AssertionKind string

const (
	// AssertionKindLedgerSequenceEquals checks that the decoded ledger sequence
	// matches the expected non-negative integer.
	AssertionKindLedgerSequenceEquals AssertionKind = "ledger_sequence_equals"

	// AssertionKindTransactionCountEquals checks that the total count of
	// transactions in the ledger close meta matches the expected integer.
	AssertionKindTransactionCountEquals AssertionKind = "transaction_count_equals"

	// AssertionKindTransactionHashPresent checks that a specific transaction hash
	// (64-char hex) is present in the decoded ledger metadata.
	AssertionKindTransactionHashPresent AssertionKind = "transaction_hash_present"

	// AssertionKindDecodedTypeEquals checks that the decoded top-level XDR type
	// matches the expected type name (e.g. "LedgerCloseMeta").
	AssertionKindDecodedTypeEquals AssertionKind = "decoded_type_equals"

	// AssertionKindInputHashEquals checks that the computed SHA-256 hash of the
	// original input bytes matches the expected digest.
	AssertionKindInputHashEquals AssertionKind = "input_hash_equals"
)

// AllAssertionKinds returns all supported assertion kinds in this build slice.
func AllAssertionKinds() []AssertionKind {
	return []AssertionKind{
		AssertionKindLedgerSequenceEquals,
		AssertionKindTransactionCountEquals,
		AssertionKindTransactionHashPresent,
		AssertionKindDecodedTypeEquals,
		AssertionKindInputHashEquals,
	}
}

// IsValid reports whether the assertion kind is supported.
func (k AssertionKind) IsValid() bool {
	switch k {
	case AssertionKindLedgerSequenceEquals,
		AssertionKindTransactionCountEquals,
		AssertionKindTransactionHashPresent,
		AssertionKindDecodedTypeEquals,
		AssertionKindInputHashEquals:
		return true
	default:
		return false
	}
}

// Validate checks that the assertion kind is valid.
func (k AssertionKind) Validate() error {
	if !k.IsValid() {
		return fmt.Errorf("unsupported assertion kind %q; must be one of: %v", k, AllAssertionKinds())
	}
	return nil
}

// AssertionStatus represents the outcome of an individual assertion evaluation.
type AssertionStatus string

const (
	AssertionStatusPass         AssertionStatus = "pass"
	AssertionStatusFail         AssertionStatus = "fail"
	AssertionStatusSkipped      AssertionStatus = "skipped"
	AssertionStatusInconclusive AssertionStatus = "inconclusive"
)

// AllAssertionStatuses returns all valid assertion result statuses.
func AllAssertionStatuses() []AssertionStatus {
	return []AssertionStatus{
		AssertionStatusPass,
		AssertionStatusFail,
		AssertionStatusSkipped,
		AssertionStatusInconclusive,
	}
}

// IsValid reports whether the assertion status is valid.
func (s AssertionStatus) IsValid() bool {
	switch s {
	case AssertionStatusPass,
		AssertionStatusFail,
		AssertionStatusSkipped,
		AssertionStatusInconclusive:
		return true
	default:
		return false
	}
}

// Validate checks that the assertion status is valid.
func (s AssertionStatus) Validate() error {
	if !s.IsValid() {
		return fmt.Errorf("invalid assertion status %q; must be one of: %v", s, AllAssertionStatuses())
	}
	return nil
}

// AssertionExpectation declares an expected condition specified in a corpus manifest.
type AssertionExpectation struct {
	AssertionID string        `json:"assertion_id"`
	Kind        AssertionKind `json:"kind"`
	Expected    any           `json:"expected"`
	Description string        `json:"description"`
}

// AssertionResult records the evaluation of a single assertion expectation.
type AssertionResult struct {
	AssertionID  string          `json:"assertion_id"`
	Kind         AssertionKind   `json:"kind"`
	Expected     any             `json:"expected"`
	Observed     any             `json:"observed,omitempty"`
	Status       AssertionStatus `json:"status"`
	Explanation  string          `json:"explanation"`
	EvidenceRefs []string        `json:"evidence_refs,omitempty"`
}
