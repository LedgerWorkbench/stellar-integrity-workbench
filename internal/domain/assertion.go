package domain

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Stellar protocol bounds derived from official XDR specifications.
const (
	// MaxStellarSequence is the maximum valid Stellar ledger sequence (uint32 max: 4,294,967,295).
	MaxStellarSequence int64 = 4294967295

	// MaxStellarTransactionCount is the maximum transactions possible in a ledger close (uint32 max: 4,294,967,295).
	MaxStellarTransactionCount int64 = 4294967295
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

// ParseExactInteger extracts an exact int64 from a raw value (e.g. json.Number, int, int64),
// explicitly rejecting float64, boolean, string, or floating point decimals.
// It preserves exact values up to math.MaxInt64 without float64 precision loss (including > 2^53).
func ParseExactInteger(val any) (int64, error) {
	switch v := val.(type) {
	case json.Number:
		s := v.String()
		if strings.Contains(s, ".") || strings.Contains(s, "e") || strings.Contains(s, "E") {
			return 0, fmt.Errorf("expected exact integer, got decimal/float number: %s", s)
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("integer out of int64 range: %s", s)
		}
		return n, nil
	case int:
		return int64(v), nil
	case int64:
		return v, nil
	case int32:
		return int64(v), nil
	case uint32:
		return int64(v), nil
	case uint64:
		if v > math.MaxInt64 {
			return 0, fmt.Errorf("integer out of int64 range: %d", v)
		}
		return int64(v), nil
	case float64, float32:
		return 0, fmt.Errorf("floating point value %v cannot be used for exact integer assertion", val)
	default:
		return 0, fmt.Errorf("expected numeric integer value, got %T (%v)", val, val)
	}
}

// AssertionExpectation declares an expected condition specified in a corpus manifest.
type AssertionExpectation struct {
	AssertionID string        `json:"assertion_id"`
	Kind        AssertionKind `json:"kind"`
	Expected    any           `json:"expected"`
	Description string        `json:"description"`
}

// ExpectedInt64 extracts the expected value as an exact int64, returning an error
// if the value is not an exact integer.
func (e *AssertionExpectation) ExpectedInt64() (int64, error) {
	return ParseExactInteger(e.Expected)
}

// ExpectedString extracts the expected value as a string, returning an error
// if the value is not a string.
func (e *AssertionExpectation) ExpectedString() (string, error) {
	s, ok := e.Expected.(string)
	if !ok {
		return "", fmt.Errorf("assertion %q expected string value, got %T", e.AssertionID, e.Expected)
	}
	return s, nil
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
