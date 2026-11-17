package corpus_test

import (
	"strings"
	"testing"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/corpus"
)

// validSyntheticFixtureJSON represents a minimal valid synthetic fixture manifest.
const validSyntheticFixtureJSON = `{
  "schema_version": "1.0.0",
  "case_id": "case_synthetic_empty_ledger",
  "title": "Synthetic Empty Ledger Close Meta",
  "summary": "Constructed synthetic test fixture used for parser testing.",
  "classification": "synthetic_fixture",
  "source": {
    "kind": "synthetic_construction",
    "url": null,
    "repository": null,
    "issue_number": null,
    "pull_request_number": null,
    "retrieved_at": null,
    "notes": "Generated in memory for unit testing."
  },
  "network": {
    "name": "Synthetic Standalone Network",
    "protocol_version": 21,
    "ledger_sequence": 100
  },
  "input": {
    "format": "LedgerCloseMeta",
    "encoding": "xdr_binary",
    "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
    "byte_length": 0,
    "object_path": "testdata/synthetic/empty.xdr"
  },
  "expectations": [
    {
      "assertion_id": "assertion_seq_check",
      "kind": "ledger_sequence_equals",
      "expected": 100,
      "description": "Verifies that the decoded ledger sequence is 100."
    },
    {
      "assertion_id": "assertion_tx_count",
      "kind": "transaction_count_equals",
      "expected": 0,
      "description": "Verifies that no transactions exist in the empty ledger."
    },
    {
      "assertion_id": "assertion_type_check",
      "kind": "decoded_type_equals",
      "expected": "LedgerCloseMeta",
      "description": "Verifies top-level decoded XDR type."
    },
    {
      "assertion_id": "assertion_hash_check",
      "kind": "input_hash_equals",
      "expected": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
      "description": "Verifies unparsed SHA-256 digest."
    }
  ],
  "implementation_context": {
    "stellar_core_version": "v21.1.0",
    "sdk_versions": {
      "stellar-go-sdk": "v0.9.0"
    },
    "indexer_version": null
  },
  "limitations": [
    "Synthetic input; not recorded on a live Stellar network."
  ]
}`

// validHistoricalRegressionJSON represents a valid historical regression manifest.
const validHistoricalRegressionJSON = `{
  "schema_version": "1.0.0",
  "case_id": "case_hist_reg_wallet_731",
  "title": "Wallet Backend SAC Trustline State Mutation",
  "summary": "Historical regression case based on stellar/wallet-backend issue #731.",
  "classification": "historical_regression",
  "source": {
    "kind": "upstream_issue",
    "url": "https://github.com/stellar/wallet-backend/issues/731",
    "repository": "stellar/wallet-backend",
    "issue_number": 731,
    "pull_request_number": 734,
    "retrieved_at": "2026-11-17T08:00:00Z",
    "notes": "Historical issue documenting missing trustline creation state changes."
  },
  "network": {
    "name": "Public Global Stellar Network",
    "protocol_version": 20,
    "ledger_sequence": 50000000
  },
  "input": {
    "format": "LedgerCloseMeta",
    "encoding": "xdr_binary",
    "sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
    "byte_length": 4096,
    "object_path": "testdata/historical/wallet_backend_731.xdr"
  },
  "expectations": [
    {
      "assertion_id": "assertion_ledger_seq",
      "kind": "ledger_sequence_equals",
      "expected": 50000000,
      "description": "Checks ledger sequence 50000000."
    },
    {
      "assertion_id": "assertion_tx_present",
      "kind": "transaction_hash_present",
      "expected": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
      "description": "Verifies transaction presence in ledger."
    }
  ],
  "implementation_context": {
    "stellar_core_version": null,
    "sdk_versions": {
      "stellar-go-sdk": "v0.9.0"
    },
    "indexer_version": "v0.1.0"
  },
  "limitations": [
    "Derived from historical issue test reproducer."
  ]
}`

// validReproducedRegressionJSON represents a valid reproduced regression manifest with structured reproduction evidence.
const validReproducedRegressionJSON = `{
  "schema_version": "1.0.0",
  "case_id": "case_repro_reg_wallet_731",
  "title": "Wallet Backend SAC Trustline State Mutation (Reproduced)",
  "summary": "Independently reproduced defect based on stellar/wallet-backend issue #731.",
  "classification": "reproduced_regression",
  "source": {
    "kind": "upstream_issue",
    "url": "https://github.com/stellar/wallet-backend/issues/731",
    "repository": "stellar/wallet-backend",
    "issue_number": 731,
    "pull_request_number": 734,
    "retrieved_at": "2026-11-17T08:00:00Z",
    "notes": "Reproduced differential behaviour between affected v0.1.0 and fixed v0.2.0."
  },
  "network": {
    "name": "Public Global Stellar Network",
    "protocol_version": 20,
    "ledger_sequence": 50000000
  },
  "input": {
    "format": "LedgerCloseMeta",
    "encoding": "xdr_binary",
    "sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
    "byte_length": 4096,
    "object_path": "testdata/historical/wallet_backend_731.xdr"
  },
  "expectations": [
    {
      "assertion_id": "assertion_ledger_seq",
      "kind": "ledger_sequence_equals",
      "expected": 50000000,
      "description": "Checks ledger sequence 50000000."
    }
  ],
  "implementation_context": {
    "stellar_core_version": null,
    "sdk_versions": {
      "stellar-go-sdk": "v0.9.0"
    },
    "indexer_version": "v0.1.0"
  },
  "limitations": [
    "Differential run evidence preserved."
  ],
  "reproduction": {
    "affected_implementation": {
      "name": "stellar-wallet-backend",
      "version": "v0.1.0"
    },
    "fixed_implementation": {
      "name": "stellar-wallet-backend",
      "version": "v0.2.0"
    },
    "differential_evidence": {
      "evidence_ref": "evidence/runs/run_repro_731_bundle.json",
      "sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
      "notes": "Verified state mismatch on trustline balance creation"
    }
  }
}`

func TestValidateManifestBytes_ValidCases(t *testing.T) {
	t.Run("valid synthetic fixture", func(t *testing.T) {
		domainCase, manifest, err := corpus.ValidateManifestBytes([]byte(validSyntheticFixtureJSON))
		if err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
		if domainCase.CaseID != "case_synthetic_empty_ledger" {
			t.Errorf("CaseID mismatch: got %q", domainCase.CaseID)
		}
		if manifest.Classification != corpus.ClassificationSyntheticFixture {
			t.Errorf("Classification mismatch: got %q", manifest.Classification)
		}
		if len(domainCase.Expectations) != 4 {
			t.Errorf("expected 4 expectations, got %d", len(domainCase.Expectations))
		}
	})

	t.Run("valid historical regression", func(t *testing.T) {
		domainCase, manifest, err := corpus.ValidateManifestBytes([]byte(validHistoricalRegressionJSON))
		if err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
		if domainCase.CaseID != "case_hist_reg_wallet_731" {
			t.Errorf("CaseID mismatch: got %q", domainCase.CaseID)
		}
		if manifest.Classification != corpus.ClassificationHistoricalRegression {
			t.Errorf("Classification mismatch: got %q", manifest.Classification)
		}
		if *domainCase.Source.IssueNumber != 731 {
			t.Errorf("expected issue number 731, got %v", domainCase.Source.IssueNumber)
		}
	})

	t.Run("valid reproduced regression", func(t *testing.T) {
		domainCase, manifest, err := corpus.ValidateManifestBytes([]byte(validReproducedRegressionJSON))
		if err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
		if domainCase.CaseID != "case_repro_reg_wallet_731" {
			t.Errorf("CaseID mismatch: got %q", domainCase.CaseID)
		}
		if manifest.Classification != corpus.ClassificationReproducedRegression {
			t.Errorf("Classification mismatch: got %q", manifest.Classification)
		}
		if domainCase.Reproduction == nil {
			t.Fatalf("expected non-nil Reproduction in domainCase")
		}
		if domainCase.Reproduction.AffectedImplementation.Version != "v0.1.0" {
			t.Errorf("expected affected version v0.1.0, got %q", domainCase.Reproduction.AffectedImplementation.Version)
		}
	})

	t.Run("valid null handling for unknown protocol and ledger values", func(t *testing.T) {
		jsonWithNulls := strings.Replace(validSyntheticFixtureJSON, `"protocol_version": 21,`, `"protocol_version": null,`, 1)
		jsonWithNulls = strings.Replace(jsonWithNulls, `"ledger_sequence": 100`, `"ledger_sequence": null`, 1)

		domainCase, _, err := corpus.ValidateManifestBytes([]byte(jsonWithNulls))
		if err != nil {
			t.Fatalf("unexpected validation error for null values: %v", err)
		}
		if domainCase.Network.ProtocolVersion != nil {
			t.Errorf("expected nil ProtocolVersion, got %v", domainCase.Network.ProtocolVersion)
		}
		if domainCase.Network.LedgerSequence != nil {
			t.Errorf("expected nil LedgerSequence, got %v", domainCase.Network.LedgerSequence)
		}
	})
}

func TestValidateManifestBytes_InvalidCases(t *testing.T) {
	tests := []struct {
		name          string
		modifyJSON    func(string) string
		expectedError string
	}{
		{
			name: "missing required field case_id",
			modifyJSON: func(s string) string {
				return strings.Replace(s, `"case_id": "case_synthetic_empty_ledger",`, "", 1)
			},
			expectedError: "case_id",
		},
		{
			name: "missing required field input",
			modifyJSON: func(s string) string {
				// remove input block
				idxStart := strings.Index(s, `"input": {`)
				idxEnd := strings.Index(s, `"expectations": [`)
				return s[:idxStart] + s[idxEnd:]
			},
			expectedError: "input",
		},
		{
			name: "unknown classification",
			modifyJSON: func(s string) string {
				return strings.Replace(s, `"classification": "synthetic_fixture"`, `"classification": "invalid_tier"`, 1)
			},
			expectedError: "classification",
		},
		{
			name: "unknown assertion kind",
			modifyJSON: func(s string) string {
				return strings.Replace(s, `"kind": "ledger_sequence_equals"`, `"kind": "unsupported_assertion_kind"`, 1)
			},
			expectedError: "kind",
		},
		{
			name: "unsupported major schema version 2.0.0",
			modifyJSON: func(s string) string {
				return strings.Replace(s, `"schema_version": "1.0.0"`, `"schema_version": "2.0.0"`, 1)
			},
			expectedError: "schema_version",
		},
		{
			name: "malformed SHA-256 uppercase letters",
			modifyJSON: func(s string) string {
				return strings.Replace(s,
					`"sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"`,
					`"sha256": "E3B0C44298FC1C149AFBF4C8996FB92427AE41E4649B934CA495991B7852B855"`, 1)
			},
			expectedError: "sha256",
		},
		{
			name: "malformed SHA-256 length 63",
			modifyJSON: func(s string) string {
				return strings.Replace(s,
					`"sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"`,
					`"sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b85"`, 1)
			},
			expectedError: "sha256",
		},
		{
			name: "invalid URI in source.url",
			modifyJSON: func(s string) string {
				return strings.Replace(s, `"url": null,`, `"url": "not a valid uri with spaces",`, 1)
			},
			expectedError: "url",
		},
		{
			name: "invalid date-time in source.retrieved_at",
			modifyJSON: func(s string) string {
				return strings.Replace(s, `"retrieved_at": null,`, `"retrieved_at": "2026-13-45 99:99:99",`, 1)
			},
			expectedError: "retrieved_at",
		},
		{
			name: "wrong JSON type for byte_length (string instead of int)",
			modifyJSON: func(s string) string {
				return strings.Replace(s, `"byte_length": 0,`, `"byte_length": "zero",`, 1)
			},
			expectedError: "byte_length",
		},
		{
			name: "negative value for byte_length",
			modifyJSON: func(s string) string {
				return strings.Replace(s, `"byte_length": 0,`, `"byte_length": -1,`, 1)
			},
			expectedError: "byte_length",
		},
		{
			name: "negative value for ledger_sequence in expectations",
			modifyJSON: func(s string) string {
				return strings.Replace(s, `"expected": 100,`, `"expected": -5,`, 1)
			},
			expectedError: "expected",
		},
		{
			name: "unexpected property at top-level",
			modifyJSON: func(s string) string {
				return strings.Replace(s, `"schema_version": "1.0.0",`, `"schema_version": "1.0.0", "unauthorized_extra_field": true,`, 1)
			},
			expectedError: "unauthorized_extra_field",
		},
		{
			name: "invalid assertion expected value type for ledger_sequence_equals (string instead of int)",
			modifyJSON: func(s string) string {
				return strings.Replace(s, `"expected": 100,`, `"expected": "one hundred",`, 1)
			},
			expectedError: "expected",
		},
		{
			name: "out of range assertion expected ledger sequence (> uint32 max / > 2^53)",
			modifyJSON: func(s string) string {
				return strings.Replace(s, `"expected": 100,`, `"expected": 9007199254740993,`, 1)
			},
			expectedError: "expected",
		},
		{
			name: "decimal float assertion expected ledger sequence",
			modifyJSON: func(s string) string {
				return strings.Replace(s, `"expected": 100,`, `"expected": 100.5,`, 1)
			},
			expectedError: "expected",
		},
		{
			name: "invalid assertion expected value type for transaction_hash_present (short hash)",
			modifyJSON: func(s string) string {
				return strings.Replace(s, `"expected": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"`, `"expected": "short_hash"`, 1)
			},
			expectedError: "expected",
		},
		{
			name: "verified_ledger_sample missing authentic source URL",
			modifyJSON: func(s string) string {
				res := strings.Replace(s, `"classification": "synthetic_fixture"`, `"classification": "verified_ledger_sample"`, 1)
				res = strings.Replace(res, `"kind": "synthetic_construction"`, `"kind": "galexie_archive"`, 1)
				// url is null
				return res
			},
			expectedError: "url",
		},
		{
			name: "historical_regression missing issue or PR reference",
			modifyJSON: func(s string) string {
				// Null out url, repository, issue_number, pull_request_number
				res := strings.Replace(s, `"url": "https://github.com/stellar/wallet-backend/issues/731",`, `"url": null,`, 1)
				res = strings.Replace(res, `"repository": "stellar/wallet-backend",`, `"repository": null,`, 1)
				res = strings.Replace(res, `"issue_number": 731,`, `"issue_number": null,`, 1)
				res = strings.Replace(res, `"pull_request_number": 734,`, `"pull_request_number": null,`, 1)
				return res
			},
			expectedError: "source",
		},
		{
			name: "object_path with absolute path",
			modifyJSON: func(s string) string {
				return strings.Replace(s, `"object_path": "testdata/synthetic/empty.xdr"`, `"object_path": "/var/data/ledger.xdr"`, 1)
			},
			expectedError: "object_path",
		},
		{
			name: "object_path with parent directory traversal",
			modifyJSON: func(s string) string {
				return strings.Replace(s, `"object_path": "testdata/synthetic/empty.xdr"`, `"object_path": "testdata/../secret.xdr"`, 1)
			},
			expectedError: "object_path",
		},
		{
			name: "object_path with windows backslash",
			modifyJSON: func(s string) string {
				return strings.Replace(s, `"object_path": "testdata/synthetic/empty.xdr"`, `"object_path": "testdata\\synthetic\\empty.xdr"`, 1)
			},
			expectedError: "object_path",
		},
		{
			name: "object_path with double slashes",
			modifyJSON: func(s string) string {
				return strings.Replace(s, `"object_path": "testdata/synthetic/empty.xdr"`, `"object_path": "testdata//empty.xdr"`, 1)
			},
			expectedError: "object_path",
		},
		{
			name: "reproduced_regression missing reproduction section",
			modifyJSON: func(s string) string {
				idx := strings.Index(s, `"reproduction":`)
				if idx != -1 {
					comma := strings.LastIndex(s[:idx], ",")
					if comma != -1 {
						return s[:comma] + "\n}"
					}
					return s[:idx] + "\n}"
				}
				return s
			},
			expectedError: "reproduction",
		},
		{
			name: "reproduced_regression missing affected_implementation",
			modifyJSON: func(s string) string {
				return strings.Replace(s, "\"affected_implementation\": {\n      \"name\": \"stellar-wallet-backend\",\n      \"version\": \"v0.1.0\"\n    },", "", 1)
			},
			expectedError: "affected_implementation",
		},
		{
			name: "reproduced_regression missing differential_evidence",
			modifyJSON: func(s string) string {
				return strings.Replace(s, `"differential_evidence": {
      "evidence_ref": "evidence/runs/run_repro_731_bundle.json",
      "sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
      "notes": "Verified state mismatch on trustline balance creation"
    }`, `"missing_diff": {}`, 1)
			},
			expectedError: "differential_evidence",
		},
		{
			name: "historical_regression with contradictory source kind synthetic_construction",
			modifyJSON: func(s string) string {
				return strings.Replace(s, `"kind": "upstream_issue"`, `"kind": "synthetic_construction"`, 1)
			},
			expectedError: "kind",
		},
		{
			name: "synthetic_fixture with non-synthetic source kind",
			modifyJSON: func(s string) string {
				return strings.Replace(s, `"kind": "synthetic_construction"`, `"kind": "upstream_issue"`, 1)
			},
			expectedError: "kind",
		},
		{
			name: "synthetic_fixture claiming Public Global Stellar Network",
			modifyJSON: func(s string) string {
				return strings.Replace(s, `"name": "Synthetic Standalone Network"`, `"name": "Public Global Stellar Network"`, 1)
			},
			expectedError: "name",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := tc.modifyJSON(validSyntheticFixtureJSON)
			if strings.Contains(tc.name, "reproduced_regression") {
				input = tc.modifyJSON(validReproducedRegressionJSON)
			} else if strings.Contains(tc.name, "historical_regression") || strings.Contains(tc.name, "transaction_hash_present") {
				input = tc.modifyJSON(validHistoricalRegressionJSON)
			}

			_, _, err := corpus.ValidateManifestBytes([]byte(input))
			if err == nil {
				t.Fatalf("expected validation error containing %q, got nil", tc.expectedError)
			}

			if !strings.Contains(err.Error(), tc.expectedError) {
				t.Errorf("expected error to contain %q, got: %v", tc.expectedError, err)
			}
		})
	}
}

func TestValidateManifestBytes_DeterministicErrorOrdering(t *testing.T) {
	// A manifest with multiple simultaneous schema errors:
	// - wrong classification
	// - wrong byte_length type
	// - unsupported schema version
	invalidMultiErrorJSON := `{
		"schema_version": "9.9.9",
		"case_id": "case_test_multi_err",
		"title": "Title",
		"summary": "Summary",
		"classification": "bad_classification",
		"source": {
			"kind": "synthetic_construction",
			"notes": "notes"
		},
		"network": {
			"name": "test",
			"protocol_version": null,
			"ledger_sequence": null
		},
		"input": {
			"format": "LedgerCloseMeta",
			"encoding": "xdr_binary",
			"sha256": "invalid_hash",
			"byte_length": "invalid_type",
			"object_path": "path"
		},
		"expectations": [
			{
				"assertion_id": "assertion_01",
				"kind": "unknown_kind",
				"expected": 1,
				"description": "desc"
			}
		],
		"implementation_context": {
			"stellar_core_version": null,
			"sdk_versions": {},
			"indexer_version": null
		},
		"limitations": []
	}`

	_, _, err1 := corpus.ValidateManifestBytes([]byte(invalidMultiErrorJSON))
	_, _, err2 := corpus.ValidateManifestBytes([]byte(invalidMultiErrorJSON))

	if err1 == nil || err2 == nil {
		t.Fatalf("expected errors for multi-error manifest")
	}

	msg1 := err1.Error()
	msg2 := err2.Error()

	if msg1 != msg2 {
		t.Errorf("non-deterministic error output:\nRun 1:\n%s\nRun 2:\n%s", msg1, msg2)
	}

	ve, ok := err1.(*corpus.ValidationError)
	if !ok {
		t.Fatalf("expected *corpus.ValidationError type, got %T", err1)
	}

	// Verify that error list is sorted by Path
	for i := 1; i < len(ve.Errors); i++ {
		prev := ve.Errors[i-1]
		curr := ve.Errors[i]
		if prev.Path > curr.Path {
			t.Errorf("errors not sorted by Path: index %d (%s) > index %d (%s)", i-1, prev.Path, i, curr.Path)
		}
	}
}

func TestValidateManifestBytes_StrictSingleDocument(t *testing.T) {
	t.Run("trailing second JSON document is rejected", func(t *testing.T) {
		multiDoc := validSyntheticFixtureJSON + `{"extra_document": true}`
		_, _, err := corpus.ValidateManifestBytes([]byte(multiDoc))
		if err == nil {
			t.Fatalf("expected error for trailing JSON document, got nil")
		}
		if !strings.Contains(err.Error(), "trailing data") {
			t.Errorf("expected error to mention trailing data, got: %v", err)
		}
	})

	t.Run("trailing non-whitespace garbage is rejected", func(t *testing.T) {
		garbageDoc := validSyntheticFixtureJSON + ` random_trailing_garbage_text`
		_, _, err := corpus.ValidateManifestBytes([]byte(garbageDoc))
		if err == nil {
			t.Fatalf("expected error for trailing garbage, got nil")
		}
		if !strings.Contains(err.Error(), "trailing data") {
			t.Errorf("expected error to mention trailing data, got: %v", err)
		}
	})

	t.Run("truncated JSON document is rejected", func(t *testing.T) {
		truncatedDoc := `{"schema_version": "1.0.0", "case_id": "case_incomplete"`
		_, _, err := corpus.ValidateManifestBytes([]byte(truncatedDoc))
		if err == nil {
			t.Fatalf("expected error for truncated JSON document, got nil")
		}
		if !strings.Contains(err.Error(), "invalid manifest JSON") {
			t.Errorf("expected error to mention invalid manifest JSON, got: %v", err)
		}
	})

	t.Run("valid document with trailing whitespace succeeds", func(t *testing.T) {
		whitespaceDoc := validSyntheticFixtureJSON + "   \n\t\r\n  "
		_, _, err := corpus.ValidateManifestBytes([]byte(whitespaceDoc))
		if err != nil {
			t.Fatalf("expected document with trailing whitespace to succeed, got: %v", err)
		}
	})
}
