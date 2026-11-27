package assertion

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/adapter/goingest"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/domain"
)

const (
	sampleHash1 = "142ada46a60fdaaefaeeb389a9b6869e5f205a453d5e59811db5b8aac97646e1"
	sampleHash2 = "a8e865730bf28005a031587d1e96f0660a5e5d77fa22644c354df634c176bea9"
	sampleSHA   = "0316872e732b0e341f69f92af0c589a1432309b6c4ef19860ee16423f738e9a3"
	otherSHA    = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
)

func makeProjection(seq int64, txCount int, hashes []string, truncated bool, decodedType string) *domain.SemanticProjection {
	return &domain.SemanticProjection{
		DecodedType:                decodedType,
		LedgerSequence:             &seq,
		TransactionCount:           &txCount,
		TransactionHashes:          hashes,
		TransactionHashesTruncated: truncated,
	}
}

func TestEvaluate_AllFiveKinds_Pass(t *testing.T) {
	proj := makeProjection(65695102, 2, []string{sampleHash1, sampleHash2}, false, "LedgerCloseMeta")
	expectations := []domain.AssertionExpectation{
		{AssertionID: "assertion_seq", Kind: domain.AssertionKindLedgerSequenceEquals, Expected: json.Number("65695102")},
		{AssertionID: "assertion_cnt", Kind: domain.AssertionKindTransactionCountEquals, Expected: json.Number("2")},
		{AssertionID: "assertion_tx", Kind: domain.AssertionKindTransactionHashPresent, Expected: strings.ToUpper(sampleHash2)},
		{AssertionID: "assertion_type", Kind: domain.AssertionKindDecodedTypeEquals, Expected: "LedgerCloseMeta"},
		{AssertionID: "assertion_sha", Kind: domain.AssertionKindInputHashEquals, Expected: sampleSHA},
	}

	refs := []string{"artifact://input/sha256/" + sampleSHA, "artifact://projection/v1"}
	eval, err := Evaluate(expectations, EvaluationInput{
		Projection:          proj,
		VerifiedInputSHA256: sampleSHA,
		SupportedKinds:      GoIngestSupportedKinds(),
		EvidenceRefs:        refs,
	})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if eval.Outcome != domain.RunOutcomePass {
		t.Errorf("Outcome = %q, want %q", eval.Outcome, domain.RunOutcomePass)
	}
	if len(eval.Results) != len(expectations) {
		t.Fatalf("len(Results) = %d, want %d", len(eval.Results), len(expectations))
	}

	for i, res := range eval.Results {
		if res.AssertionID != expectations[i].AssertionID {
			t.Errorf("result[%d].AssertionID = %q, want %q", i, res.AssertionID, expectations[i].AssertionID)
		}
		if res.Kind != expectations[i].Kind {
			t.Errorf("result[%d].Kind = %q, want %q", i, res.Kind, expectations[i].Kind)
		}
		if res.Status != domain.AssertionStatusPass {
			t.Errorf("result[%d].Status = %q, want %q (explanation: %s)", i, res.Status, domain.AssertionStatusPass, res.Explanation)
		}
		if len(res.EvidenceRefs) != len(refs) {
			t.Errorf("result[%d].EvidenceRefs = %v, want %v", i, res.EvidenceRefs, refs)
		}
	}

	// Ensure caller's EvidenceRefs slice was cloned and not aliased.
	refs[0] = "mutated"
	if eval.Results[0].EvidenceRefs[0] == "mutated" {
		t.Error("Evaluate aliased caller's EvidenceRefs slice")
	}
}

func TestEvaluate_FailuresYieldRegression(t *testing.T) {
	tests := []struct {
		name        string
		expectation domain.AssertionExpectation
		proj        *domain.SemanticProjection
		verifiedSHA string
	}{
		{
			name:        "ledger_sequence_mismatch",
			expectation: domain.AssertionExpectation{AssertionID: "a_seq", Kind: domain.AssertionKindLedgerSequenceEquals, Expected: int64(100)},
			proj:        makeProjection(101, 0, []string{}, false, "LedgerCloseMeta"),
			verifiedSHA: sampleSHA,
		},
		{
			name:        "transaction_count_mismatch",
			expectation: domain.AssertionExpectation{AssertionID: "a_cnt", Kind: domain.AssertionKindTransactionCountEquals, Expected: int64(5)},
			proj:        makeProjection(100, 3, []string{}, false, "LedgerCloseMeta"),
			verifiedSHA: sampleSHA,
		},
		{
			name:        "transaction_hash_absent_complete",
			expectation: domain.AssertionExpectation{AssertionID: "a_tx", Kind: domain.AssertionKindTransactionHashPresent, Expected: sampleHash2},
			proj:        makeProjection(100, 1, []string{sampleHash1}, false, "LedgerCloseMeta"),
			verifiedSHA: sampleSHA,
		},
		{
			name:        "transaction_hash_absent_empty_complete",
			expectation: domain.AssertionExpectation{AssertionID: "a_tx_empty", Kind: domain.AssertionKindTransactionHashPresent, Expected: sampleHash1},
			proj:        makeProjection(100, 0, []string{}, false, "LedgerCloseMeta"),
			verifiedSHA: sampleSHA,
		},
		{
			name:        "decoded_type_mismatch",
			expectation: domain.AssertionExpectation{AssertionID: "a_type", Kind: domain.AssertionKindDecodedTypeEquals, Expected: "LedgerCloseMeta"},
			proj:        makeProjection(100, 0, []string{}, false, "TransactionEnvelope"),
			verifiedSHA: sampleSHA,
		},
		{
			name:        "input_hash_mismatch",
			expectation: domain.AssertionExpectation{AssertionID: "a_sha", Kind: domain.AssertionKindInputHashEquals, Expected: sampleSHA},
			proj:        makeProjection(100, 0, []string{}, false, "LedgerCloseMeta"),
			verifiedSHA: otherSHA,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			eval, err := Evaluate([]domain.AssertionExpectation{tc.expectation}, EvaluationInput{
				Projection:          tc.proj,
				VerifiedInputSHA256: tc.verifiedSHA,
				SupportedKinds:      GoIngestSupportedKinds(),
			})
			if err != nil {
				t.Fatalf("Evaluate failed: %v", err)
			}
			if eval.Results[0].Status != domain.AssertionStatusFail {
				t.Errorf("Status = %q, want %q", eval.Results[0].Status, domain.AssertionStatusFail)
			}
			if eval.Outcome != domain.RunOutcomeRegression {
				t.Errorf("Outcome = %q, want %q", eval.Outcome, domain.RunOutcomeRegression)
			}
		})
	}
}

func TestEvaluate_TransactionHashTruncationBehavior(t *testing.T) {
	t.Run("found_in_truncated_list_passes", func(t *testing.T) {
		proj := makeProjection(500, 300, []string{sampleHash1}, true, "LedgerCloseMeta")
		eval, err := Evaluate([]domain.AssertionExpectation{
			{AssertionID: "a_tx", Kind: domain.AssertionKindTransactionHashPresent, Expected: sampleHash1},
		}, EvaluationInput{
			Projection:     proj,
			SupportedKinds: GoIngestSupportedKinds(),
		})
		if err != nil {
			t.Fatalf("Evaluate failed: %v", err)
		}
		if eval.Results[0].Status != domain.AssertionStatusPass {
			t.Errorf("Status = %q, want %q", eval.Results[0].Status, domain.AssertionStatusPass)
		}
		if eval.Outcome != domain.RunOutcomePass {
			t.Errorf("Outcome = %q, want %q", eval.Outcome, domain.RunOutcomePass)
		}
	})

	t.Run("absent_in_truncated_list_is_inconclusive", func(t *testing.T) {
		proj := makeProjection(500, 300, []string{sampleHash1}, true, "LedgerCloseMeta")
		eval, err := Evaluate([]domain.AssertionExpectation{
			{AssertionID: "a_tx", Kind: domain.AssertionKindTransactionHashPresent, Expected: sampleHash2},
		}, EvaluationInput{
			Projection:     proj,
			SupportedKinds: GoIngestSupportedKinds(),
		})
		if err != nil {
			t.Fatalf("Evaluate failed: %v", err)
		}
		if eval.Results[0].Status != domain.AssertionStatusInconclusive {
			t.Errorf("Status = %q, want %q", eval.Results[0].Status, domain.AssertionStatusInconclusive)
		}
		if eval.Outcome != domain.RunOutcomeInconclusive {
			t.Errorf("Outcome = %q, want %q", eval.Outcome, domain.RunOutcomeInconclusive)
		}
	})

	t.Run("nil_hash_list_is_inconclusive", func(t *testing.T) {
		proj := makeProjection(500, 0, nil, false, "LedgerCloseMeta")
		eval, err := Evaluate([]domain.AssertionExpectation{
			{AssertionID: "a_tx", Kind: domain.AssertionKindTransactionHashPresent, Expected: sampleHash1},
		}, EvaluationInput{
			Projection:     proj,
			SupportedKinds: GoIngestSupportedKinds(),
		})
		if err != nil {
			t.Fatalf("Evaluate failed: %v", err)
		}
		if eval.Results[0].Status != domain.AssertionStatusInconclusive {
			t.Errorf("Status = %q, want %q", eval.Results[0].Status, domain.AssertionStatusInconclusive)
		}
	})
}

func TestEvaluate_MissingObservationsAndNilProjection(t *testing.T) {
	expectations := []domain.AssertionExpectation{
		{AssertionID: "a_seq", Kind: domain.AssertionKindLedgerSequenceEquals, Expected: int64(1)},
		{AssertionID: "a_cnt", Kind: domain.AssertionKindTransactionCountEquals, Expected: int64(0)},
		{AssertionID: "a_tx", Kind: domain.AssertionKindTransactionHashPresent, Expected: sampleHash1},
		{AssertionID: "a_type", Kind: domain.AssertionKindDecodedTypeEquals, Expected: "LedgerCloseMeta"},
		{AssertionID: "a_sha", Kind: domain.AssertionKindInputHashEquals, Expected: sampleSHA},
	}

	eval, err := Evaluate(expectations, EvaluationInput{
		Projection:          nil,
		VerifiedInputSHA256: "",
		SupportedKinds:      GoIngestSupportedKinds(),
	})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if eval.Outcome != domain.RunOutcomeInconclusive {
		t.Errorf("Outcome = %q, want %q", eval.Outcome, domain.RunOutcomeInconclusive)
	}
	for i, res := range eval.Results {
		if res.Status != domain.AssertionStatusInconclusive {
			t.Errorf("result[%d].Status = %q, want %q", i, res.Status, domain.AssertionStatusInconclusive)
		}
		if res.Observed != nil {
			t.Errorf("result[%d].Observed = %v, want nil", i, res.Observed)
		}
	}
}

func TestEvaluate_CapabilitySkipping(t *testing.T) {
	proj := makeProjection(10, 1, []string{sampleHash1}, false, "LedgerCloseMeta")
	expectations := []domain.AssertionExpectation{
		{AssertionID: "a_seq", Kind: domain.AssertionKindLedgerSequenceEquals, Expected: int64(10)},
		{AssertionID: "a_tx", Kind: domain.AssertionKindTransactionHashPresent, Expected: sampleHash1},
	}

	t.Run("all_skipped_yields_not_applicable", func(t *testing.T) {
		eval, err := Evaluate(expectations, EvaluationInput{
			Projection:     proj,
			SupportedKinds: nil,
		})
		if err != nil {
			t.Fatalf("Evaluate failed: %v", err)
		}
		if eval.Outcome != domain.RunOutcomeNotApplicable {
			t.Errorf("Outcome = %q, want %q", eval.Outcome, domain.RunOutcomeNotApplicable)
		}
		for i, res := range eval.Results {
			if res.Status != domain.AssertionStatusSkipped {
				t.Errorf("result[%d].Status = %q, want %q", i, res.Status, domain.AssertionStatusSkipped)
			}
		}
	})

	t.Run("pass_and_skipped_yields_inconclusive", func(t *testing.T) {
		eval, err := Evaluate(expectations, EvaluationInput{
			Projection:     proj,
			SupportedKinds: []domain.AssertionKind{domain.AssertionKindLedgerSequenceEquals},
		})
		if err != nil {
			t.Fatalf("Evaluate failed: %v", err)
		}
		if eval.Results[0].Status != domain.AssertionStatusPass {
			t.Errorf("result[0].Status = %q, want %q", eval.Results[0].Status, domain.AssertionStatusPass)
		}
		if eval.Results[1].Status != domain.AssertionStatusSkipped {
			t.Errorf("result[1].Status = %q, want %q", eval.Results[1].Status, domain.AssertionStatusSkipped)
		}
		if eval.Outcome != domain.RunOutcomeInconclusive {
			t.Errorf("Outcome = %q, want %q", eval.Outcome, domain.RunOutcomeInconclusive)
		}
	})
}

func TestEvaluate_ExactIntegerBoundsAndPrecision(t *testing.T) {
	// Max uint32 boundary (4294967295) succeeds.
	maxSeq := domain.MaxStellarSequence
	proj := makeProjection(maxSeq, int(domain.MaxStellarTransactionCount), []string{}, false, "LedgerCloseMeta")
	eval, err := Evaluate([]domain.AssertionExpectation{
		{AssertionID: "a_max_seq", Kind: domain.AssertionKindLedgerSequenceEquals, Expected: json.Number("4294967295")},
	}, EvaluationInput{
		Projection:     proj,
		SupportedKinds: GoIngestSupportedKinds(),
	})
	if err != nil {
		t.Fatalf("Evaluate failed at MaxStellarSequence: %v", err)
	}
	if eval.Results[0].Status != domain.AssertionStatusPass {
		t.Errorf("Status = %q, want %q", eval.Results[0].Status, domain.AssertionStatusPass)
	}

	// Value > 2^53 (9007199254740993, which float64 cannot represent exactly) must be
	// parsed exactly and rejected as out of Stellar uint32 range, not rounded.
	_, err = Evaluate([]domain.AssertionExpectation{
		{AssertionID: "a_over_2_53", Kind: domain.AssertionKindLedgerSequenceEquals, Expected: json.Number("9007199254740993")},
	}, EvaluationInput{
		Projection:     proj,
		SupportedKinds: GoIngestSupportedKinds(),
	})
	if err == nil {
		t.Fatal("expected out-of-range error for >2^53 integer, got nil")
	}
}

func TestEvaluate_ValidationErrors(t *testing.T) {
	validProj := makeProjection(10, 1, []string{sampleHash1}, false, "LedgerCloseMeta")
	validExp := []domain.AssertionExpectation{
		{AssertionID: "a_1", Kind: domain.AssertionKindLedgerSequenceEquals, Expected: int64(10)},
	}

	tests := []struct {
		name         string
		expectations []domain.AssertionExpectation
		input        EvaluationInput
	}{
		{
			name:         "empty_expectations",
			expectations: nil,
			input:        EvaluationInput{Projection: validProj, SupportedKinds: GoIngestSupportedKinds()},
		},
		{
			name: "empty_assertion_id",
			expectations: []domain.AssertionExpectation{
				{AssertionID: "  ", Kind: domain.AssertionKindLedgerSequenceEquals, Expected: int64(10)},
			},
			input: EvaluationInput{Projection: validProj, SupportedKinds: GoIngestSupportedKinds()},
		},
		{
			name: "duplicate_assertion_id",
			expectations: []domain.AssertionExpectation{
				{AssertionID: "dup", Kind: domain.AssertionKindLedgerSequenceEquals, Expected: int64(10)},
				{AssertionID: "dup", Kind: domain.AssertionKindDecodedTypeEquals, Expected: "LedgerCloseMeta"},
			},
			input: EvaluationInput{Projection: validProj, SupportedKinds: GoIngestSupportedKinds()},
		},
		{
			name: "invalid_assertion_kind",
			expectations: []domain.AssertionExpectation{
				{AssertionID: "a_bad", Kind: domain.AssertionKind("unknown_kind"), Expected: "x"},
			},
			input: EvaluationInput{Projection: validProj, SupportedKinds: GoIngestSupportedKinds()},
		},
		{
			name: "float64_expected_integer",
			expectations: []domain.AssertionExpectation{
				{AssertionID: "a_float", Kind: domain.AssertionKindLedgerSequenceEquals, Expected: float64(10.0)},
			},
			input: EvaluationInput{Projection: validProj, SupportedKinds: GoIngestSupportedKinds()},
		},
		{
			name: "decimal_json_number",
			expectations: []domain.AssertionExpectation{
				{AssertionID: "a_dec", Kind: domain.AssertionKindTransactionCountEquals, Expected: json.Number("1.5")},
			},
			input: EvaluationInput{Projection: validProj, SupportedKinds: GoIngestSupportedKinds()},
		},
		{
			name: "negative_sequence",
			expectations: []domain.AssertionExpectation{
				{AssertionID: "a_neg", Kind: domain.AssertionKindLedgerSequenceEquals, Expected: int64(-1)},
			},
			input: EvaluationInput{Projection: validProj, SupportedKinds: GoIngestSupportedKinds()},
		},
		{
			name: "invalid_tx_hash_expected",
			expectations: []domain.AssertionExpectation{
				{AssertionID: "a_tx", Kind: domain.AssertionKindTransactionHashPresent, Expected: "not-hex"},
			},
			input: EvaluationInput{Projection: validProj, SupportedKinds: GoIngestSupportedKinds()},
		},
		{
			name: "uppercase_expected_input_hash_rejected",
			expectations: []domain.AssertionExpectation{
				{AssertionID: "a_sha", Kind: domain.AssertionKindInputHashEquals, Expected: strings.ToUpper(sampleSHA)},
			},
			input: EvaluationInput{Projection: validProj, VerifiedInputSHA256: sampleSHA, SupportedKinds: GoIngestSupportedKinds()},
		},
		{
			name: "empty_decoded_type_expected",
			expectations: []domain.AssertionExpectation{
				{AssertionID: "a_type", Kind: domain.AssertionKindDecodedTypeEquals, Expected: "   "},
			},
			input: EvaluationInput{Projection: validProj, SupportedKinds: GoIngestSupportedKinds()},
		},
		{
			name:         "invalid_supported_kind",
			expectations: validExp,
			input:        EvaluationInput{Projection: validProj, SupportedKinds: []domain.AssertionKind{"bad_kind"}},
		},
		{
			name:         "empty_evidence_ref",
			expectations: validExp,
			input:        EvaluationInput{Projection: validProj, SupportedKinds: GoIngestSupportedKinds(), EvidenceRefs: []string{"ok", ""}},
		},
		{
			name: "malformed_observed_tx_hash",
			expectations: []domain.AssertionExpectation{
				{AssertionID: "a_tx", Kind: domain.AssertionKindTransactionHashPresent, Expected: sampleHash1},
			},
			input: EvaluationInput{
				Projection:     makeProjection(10, 1, []string{"bad-observed-hash"}, false, "LedgerCloseMeta"),
				SupportedKinds: GoIngestSupportedKinds(),
			},
		},
		{
			name: "malformed_observed_input_sha",
			expectations: []domain.AssertionExpectation{
				{AssertionID: "a_sha", Kind: domain.AssertionKindInputHashEquals, Expected: sampleSHA},
			},
			input: EvaluationInput{
				Projection:          validProj,
				VerifiedInputSHA256: "not-a-sha256",
				SupportedKinds:      GoIngestSupportedKinds(),
			},
		},
		{
			name:         "negative_observed_sequence",
			expectations: validExp,
			input: EvaluationInput{
				Projection:     makeProjection(-5, 0, []string{}, false, "LedgerCloseMeta"),
				SupportedKinds: GoIngestSupportedKinds(),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Evaluate(tc.expectations, tc.input); err == nil {
				t.Fatalf("expected validation error for %s, got nil", tc.name)
			}
		})
	}
}

func TestProjectionFromGoIngest(t *testing.T) {
	gp := &goingest.SemanticProjection{
		ProjectionVersion: goingest.ProjectionVersion,
		XDRType:           "LedgerCloseMeta",
		OuterVariant:      2,
		Ledger: goingest.LedgerProjection{
			Sequence:        65695102,
			ProtocolVersion: 30,
		},
		Transactions: goingest.TransactionsProjection{
			Total:           300,
			Successful:      290,
			Failed:          10,
			Hashes:          []string{sampleHash1},
			HashesTruncated: true,
		},
	}

	dp, err := ProjectionFromGoIngest(gp)
	if err != nil {
		t.Fatalf("ProjectionFromGoIngest failed: %v", err)
	}
	if dp.DecodedType != "LedgerCloseMeta" {
		t.Errorf("DecodedType = %q, want LedgerCloseMeta", dp.DecodedType)
	}
	if dp.LedgerSequence == nil || *dp.LedgerSequence != 65695102 {
		t.Errorf("LedgerSequence = %v, want 65695102", dp.LedgerSequence)
	}
	if dp.TransactionCount == nil || *dp.TransactionCount != 300 {
		t.Errorf("TransactionCount = %v, want 300", dp.TransactionCount)
	}
	if !dp.TransactionHashesTruncated {
		t.Error("TransactionHashesTruncated = false, want true")
	}
	if len(dp.TransactionHashes) != 1 || dp.TransactionHashes[0] != sampleHash1 {
		t.Errorf("TransactionHashes = %v, want [%s]", dp.TransactionHashes, sampleHash1)
	}
}
