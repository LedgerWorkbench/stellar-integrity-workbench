// Package assertion implements deterministic evaluation of typed corpus
// manifest expectations against normalized semantic projections and trusted
// input verification metadata.
package assertion

import (
	"fmt"
	"math"
	"strings"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/adapter/goingest"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/domain"
)

// EvaluationInput supplies the normalized semantic observations, trusted
// input integrity digest, declared adapter assertion capabilities, and
// logical evidence references for an assertion evaluation pass.
type EvaluationInput struct {
	Projection          *domain.SemanticProjection
	VerifiedInputSHA256 string
	SupportedKinds      []domain.AssertionKind
	EvidenceRefs        []string
}

// Evaluation holds the ordered per-assertion results and the derived
// overall conformance outcome.
type Evaluation struct {
	Results []domain.AssertionResult
	Outcome domain.RunOutcome
}

// GoIngestSupportedKinds returns the assertion kinds supported by the Unit 5
// go-ingest adapter.
func GoIngestSupportedKinds() []domain.AssertionKind {
	return domain.AllAssertionKinds()
}

// ProjectionFromGoIngest maps a Unit 5 goingest.SemanticProjection into the
// domain.SemanticProjection contract used by the assertion engine.
func ProjectionFromGoIngest(p *goingest.SemanticProjection) (*domain.SemanticProjection, error) {
	if p == nil {
		return nil, nil
	}
	if int64(p.Ledger.Sequence) > domain.MaxStellarSequence {
		return nil, fmt.Errorf("ledger sequence %d exceeds max %d", p.Ledger.Sequence, domain.MaxStellarSequence)
	}
	if int64(p.Transactions.Total) > domain.MaxStellarTransactionCount {
		return nil, fmt.Errorf("transaction count %d exceeds max %d", p.Transactions.Total, domain.MaxStellarTransactionCount)
	}
	if int64(p.Transactions.Total) > math.MaxInt {
		return nil, fmt.Errorf("transaction count %d overflows int", p.Transactions.Total)
	}

	seq := int64(p.Ledger.Sequence)
	txCount := int(p.Transactions.Total)
	proto := int(p.Ledger.ProtocolVersion)

	if p.Transactions.HashesTruncated {
		if int64(len(p.Transactions.Hashes)) >= int64(p.Transactions.Total) {
			return nil, fmt.Errorf("inconsistent Unit 5 projection: hashes_truncated is true but hash count %d >= total %d",
				len(p.Transactions.Hashes), p.Transactions.Total)
		}
	} else {
		if int64(len(p.Transactions.Hashes)) != int64(p.Transactions.Total) {
			return nil, fmt.Errorf("inconsistent Unit 5 projection: hashes_truncated is false but hash count %d != total %d",
				len(p.Transactions.Hashes), p.Transactions.Total)
		}
	}

	hashes := make([]string, len(p.Transactions.Hashes))
	copy(hashes, p.Transactions.Hashes)

	return &domain.SemanticProjection{
		DecodedType:                p.XDRType,
		LedgerSequence:             &seq,
		TransactionCount:           &txCount,
		TransactionHashes:          hashes,
		TransactionHashesTruncated: p.Transactions.HashesTruncated,
		ProtocolVersion:            &proto,
	}, nil
}

// Evaluate validates expectations and evaluation inputs, evaluates each
// expectation in manifest order, and derives the overall RunOutcome.
//
// Invalid expectations, malformed observed values, invalid capability
// declarations, or empty evidence references return an error rather than
// fabricating assertion results.
func Evaluate(expectations []domain.AssertionExpectation, input EvaluationInput) (*Evaluation, error) {
	if len(expectations) == 0 {
		return nil, fmt.Errorf("cannot evaluate assertions: expectations list is empty")
	}

	supportedSet := make(map[domain.AssertionKind]bool, len(input.SupportedKinds))
	for _, k := range input.SupportedKinds {
		if err := k.Validate(); err != nil {
			return nil, fmt.Errorf("invalid supported assertion kind: %w", err)
		}
		supportedSet[k] = true
	}

	for i, ref := range input.EvidenceRefs {
		if strings.TrimSpace(ref) == "" {
			return nil, fmt.Errorf("evidence reference at index %d must not be empty", i)
		}
	}

	seenIDs := make(map[string]struct{}, len(expectations))
	normalizedExpected := make([]any, len(expectations))
	for i := range expectations {
		exp := &expectations[i]
		if strings.TrimSpace(exp.AssertionID) == "" {
			return nil, fmt.Errorf("expectation at index %d has empty assertion_id", i)
		}
		if _, dup := seenIDs[exp.AssertionID]; dup {
			return nil, fmt.Errorf("duplicate assertion_id %q", exp.AssertionID)
		}
		seenIDs[exp.AssertionID] = struct{}{}

		if err := exp.Kind.Validate(); err != nil {
			return nil, fmt.Errorf("assertion %q: %w", exp.AssertionID, err)
		}

		val, err := validateExpectedValue(exp)
		if err != nil {
			return nil, err
		}
		normalizedExpected[i] = val
	}

	results := make([]domain.AssertionResult, len(expectations))
	for i := range expectations {
		exp := &expectations[i]
		expectedVal := normalizedExpected[i]

		if !supportedSet[exp.Kind] {
			results[i] = domain.AssertionResult{
				AssertionID:  exp.AssertionID,
				Kind:         exp.Kind,
				Expected:     expectedVal,
				Status:       domain.AssertionStatusSkipped,
				Explanation:  fmt.Sprintf("assertion kind %q is not supported by the selected adapter", exp.Kind),
				EvidenceRefs: cloneStrings(input.EvidenceRefs),
			}
			continue
		}

		res, err := evaluateSingle(exp, expectedVal, input)
		if err != nil {
			return nil, fmt.Errorf("assertion %q (%s): %w", exp.AssertionID, exp.Kind, err)
		}
		res.EvidenceRefs = cloneStrings(input.EvidenceRefs)
		results[i] = res
	}

	outcome, err := domain.DeriveOutcome(results)
	if err != nil {
		return nil, fmt.Errorf("derive outcome: %w", err)
	}

	return &Evaluation{
		Results: results,
		Outcome: outcome,
	}, nil
}

func validateExpectedValue(exp *domain.AssertionExpectation) (any, error) {
	switch exp.Kind {
	case domain.AssertionKindLedgerSequenceEquals:
		n, err := exp.ExpectedInt64()
		if err != nil {
			return nil, fmt.Errorf("assertion %q (%s): %w", exp.AssertionID, exp.Kind, err)
		}
		if n < 0 || n > domain.MaxStellarSequence {
			return nil, fmt.Errorf("assertion %q (%s): expected sequence %d out of range [0, %d]",
				exp.AssertionID, exp.Kind, n, domain.MaxStellarSequence)
		}
		return n, nil

	case domain.AssertionKindTransactionCountEquals:
		n, err := exp.ExpectedInt64()
		if err != nil {
			return nil, fmt.Errorf("assertion %q (%s): %w", exp.AssertionID, exp.Kind, err)
		}
		if n < 0 || n > domain.MaxStellarTransactionCount {
			return nil, fmt.Errorf("assertion %q (%s): expected transaction count %d out of range [0, %d]",
				exp.AssertionID, exp.Kind, n, domain.MaxStellarTransactionCount)
		}
		return n, nil

	case domain.AssertionKindTransactionHashPresent:
		s, err := exp.ExpectedString()
		if err != nil {
			return nil, fmt.Errorf("assertion %q (%s): %w", exp.AssertionID, exp.Kind, err)
		}
		if !isHex64(s, true) {
			return nil, fmt.Errorf("assertion %q (%s): expected 64-character hex transaction hash, got %q",
				exp.AssertionID, exp.Kind, s)
		}
		return s, nil

	case domain.AssertionKindDecodedTypeEquals:
		s, err := exp.ExpectedString()
		if err != nil {
			return nil, fmt.Errorf("assertion %q (%s): %w", exp.AssertionID, exp.Kind, err)
		}
		if strings.TrimSpace(s) == "" {
			return nil, fmt.Errorf("assertion %q (%s): expected decoded type must not be empty",
				exp.AssertionID, exp.Kind)
		}
		return s, nil

	case domain.AssertionKindInputHashEquals:
		s, err := exp.ExpectedString()
		if err != nil {
			return nil, fmt.Errorf("assertion %q (%s): %w", exp.AssertionID, exp.Kind, err)
		}
		if !isHex64(s, false) {
			return nil, fmt.Errorf("assertion %q (%s): expected 64-character lowercase hex SHA-256, got %q",
				exp.AssertionID, exp.Kind, s)
		}
		return s, nil

	default:
		return nil, fmt.Errorf("assertion %q: unsupported kind %q", exp.AssertionID, exp.Kind)
	}
}

func evaluateSingle(exp *domain.AssertionExpectation, expectedVal any, input EvaluationInput) (domain.AssertionResult, error) {
	base := domain.AssertionResult{
		AssertionID: exp.AssertionID,
		Kind:        exp.Kind,
		Expected:    expectedVal,
	}

	switch exp.Kind {
	case domain.AssertionKindLedgerSequenceEquals:
		expectedSeq := expectedVal.(int64)
		if input.Projection == nil || input.Projection.LedgerSequence == nil {
			base.Status = domain.AssertionStatusInconclusive
			base.Explanation = "observed ledger sequence is unavailable"
			return base, nil
		}
		obs := *input.Projection.LedgerSequence
		if obs < 0 || obs > domain.MaxStellarSequence {
			return domain.AssertionResult{}, fmt.Errorf("observed ledger sequence %d out of range [0, %d]", obs, domain.MaxStellarSequence)
		}
		base.Observed = obs
		if obs == expectedSeq {
			base.Status = domain.AssertionStatusPass
			base.Explanation = fmt.Sprintf("observed ledger sequence %d matches expected %d", obs, expectedSeq)
		} else {
			base.Status = domain.AssertionStatusFail
			base.Explanation = fmt.Sprintf("observed ledger sequence %d does not match expected %d", obs, expectedSeq)
		}
		return base, nil

	case domain.AssertionKindTransactionCountEquals:
		expectedCount := expectedVal.(int64)
		if input.Projection == nil || input.Projection.TransactionCount == nil {
			base.Status = domain.AssertionStatusInconclusive
			base.Explanation = "observed transaction count is unavailable"
			return base, nil
		}
		obs := int64(*input.Projection.TransactionCount)
		if obs < 0 || obs > domain.MaxStellarTransactionCount {
			return domain.AssertionResult{}, fmt.Errorf("observed transaction count %d out of range [0, %d]", obs, domain.MaxStellarTransactionCount)
		}
		base.Observed = obs
		if obs == expectedCount {
			base.Status = domain.AssertionStatusPass
			base.Explanation = fmt.Sprintf("observed transaction count %d matches expected %d", obs, expectedCount)
		} else {
			base.Status = domain.AssertionStatusFail
			base.Explanation = fmt.Sprintf("observed transaction count %d does not match expected %d", obs, expectedCount)
		}
		return base, nil

	case domain.AssertionKindTransactionHashPresent:
		expectedHash := expectedVal.(string)
		if input.Projection == nil || input.Projection.TransactionHashes == nil {
			base.Status = domain.AssertionStatusInconclusive
			base.Explanation = "observed transaction hash list is unavailable"
			return base, nil
		}
		matched := ""
		for i, h := range input.Projection.TransactionHashes {
			if !isHex64(h, true) {
				return domain.AssertionResult{}, fmt.Errorf("observed transaction hash at index %d is malformed: %q", i, h)
			}
			if matched == "" && strings.EqualFold(h, expectedHash) {
				matched = h
			}
		}
		if matched != "" {
			base.Observed = matched
			base.Status = domain.AssertionStatusPass
			base.Explanation = fmt.Sprintf("expected transaction hash %s is present in observed hashes", expectedHash)
			return base, nil
		}
		if input.Projection.TransactionHashesTruncated {
			base.Observed = fmt.Sprintf("absent from first %d hashes (truncated)", len(input.Projection.TransactionHashes))
			base.Status = domain.AssertionStatusInconclusive
			base.Explanation = fmt.Sprintf("expected transaction hash %s not found in first %d projected hashes and list is truncated",
				expectedHash, len(input.Projection.TransactionHashes))
			return base, nil
		}
		base.Observed = fmt.Sprintf("absent from %d hashes", len(input.Projection.TransactionHashes))
		base.Status = domain.AssertionStatusFail
		base.Explanation = fmt.Sprintf("expected transaction hash %s is not present in %d observed hashes",
			expectedHash, len(input.Projection.TransactionHashes))
		return base, nil

	case domain.AssertionKindDecodedTypeEquals:
		expectedType := expectedVal.(string)
		if input.Projection == nil || input.Projection.DecodedType == "" {
			base.Status = domain.AssertionStatusInconclusive
			base.Explanation = "observed decoded type is unavailable"
			return base, nil
		}
		if strings.TrimSpace(input.Projection.DecodedType) == "" {
			return domain.AssertionResult{}, fmt.Errorf("observed decoded type must not be whitespace-only")
		}
		obs := input.Projection.DecodedType
		base.Observed = obs
		if obs == expectedType {
			base.Status = domain.AssertionStatusPass
			base.Explanation = fmt.Sprintf("observed decoded type %q matches expected %q", obs, expectedType)
		} else {
			base.Status = domain.AssertionStatusFail
			base.Explanation = fmt.Sprintf("observed decoded type %q does not match expected %q", obs, expectedType)
		}
		return base, nil

	case domain.AssertionKindInputHashEquals:
		expectedHash := expectedVal.(string)
		if input.VerifiedInputSHA256 == "" {
			base.Status = domain.AssertionStatusInconclusive
			base.Explanation = "trusted verified input SHA-256 is unavailable"
			return base, nil
		}
		if !isHex64(input.VerifiedInputSHA256, false) {
			return domain.AssertionResult{}, fmt.Errorf("observed verified input SHA-256 is malformed: %q", input.VerifiedInputSHA256)
		}
		obs := input.VerifiedInputSHA256
		base.Observed = obs
		if obs == expectedHash {
			base.Status = domain.AssertionStatusPass
			base.Explanation = fmt.Sprintf("trusted verified input SHA-256 %s matches expected", obs)
		} else {
			base.Status = domain.AssertionStatusFail
			base.Explanation = fmt.Sprintf("trusted verified input SHA-256 %s does not match expected %s", obs, expectedHash)
		}
		return base, nil

	default:
		return domain.AssertionResult{}, fmt.Errorf("unsupported assertion kind %q", exp.Kind)
	}
}

func isHex64(s string, allowUpper bool) bool {
	if len(s) != 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		isLower := c >= 'a' && c <= 'f'
		isDigit := c >= '0' && c <= '9'
		isUpper := allowUpper && c >= 'A' && c <= 'F'
		if !isDigit && !isLower && !isUpper {
			return false
		}
	}
	return true
}

func cloneStrings(src []string) []string {
	if len(src) == 0 {
		return nil
	}
	out := make([]string, len(src))
	copy(out, src)
	return out
}
