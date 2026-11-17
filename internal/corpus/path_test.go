package corpus_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/corpus"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/schemas"
)

func TestValidateObjectPath(t *testing.T) {
	validPaths := []string{
		"test.xdr",
		"fixtures/test.xdr",
		"testdata/synthetic/empty.xdr",
		"objects/sha256/e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		"a/b/c/d/file.xdr",
		"ledger_2026-11-17.xdr",
		"nested-dir_01/sub.dir/item-1.2.3.bin",
	}

	for _, p := range validPaths {
		if err := corpus.ValidateObjectPath(p); err != nil {
			t.Errorf("expected path %q to be valid, got: %v", p, err)
		}
	}

	invalidTests := []struct {
		name        string
		path        string
		expectedErr string
	}{
		{"empty path", "", "empty"},
		{"whitespace path only", "   ", "leading or trailing whitespace"},
		{"leading space", " fixtures/test.xdr", "leading or trailing whitespace"},
		{"trailing space", "fixtures/test.xdr ", "leading or trailing whitespace"},
		{"absolute unix path", "/etc/passwd", "relative, not absolute"},
		{"parent directory traversal alone", "..", "parent directory traversal"},
		{"current directory alone", ".", "current directory segment"},
		{"parent directory traversal start", "../secret.xdr", "parent directory traversal"},
		{"parent directory traversal middle", "fixtures/../secret.xdr", "parent directory traversal"},
		{"current directory segment start", "./file.xdr", "current directory segment"},
		{"current directory segment middle", "fixtures/./file.xdr", "current directory segment"},
		{"double slash", "fixtures//file.xdr", "empty path segment"},
		{"trailing slash", "fixtures/test/", "empty path segment"},
		{"windows backslash", "fixtures\\test.xdr", "invalid path separator"},
		{"windows drive letter", "C:file.xdr", "Windows drive"},
		{"windows absolute path", "C:\\file.xdr", "invalid path separator"},
		{"control character null", "fixtures/test\x00.xdr", "unsupported or control character"},
		{"control character newline", "fixtures/test\n.xdr", "unsupported or control character"},
		{"control character tab", "fixtures/test\t.xdr", "unsupported or control character"},
		{"unsupported character semicolon", "fixtures/test;rm.xdr", "unsupported or control character"},
		{"unsupported character space inside", "fixtures/my file.xdr", "unsupported or control character"},
	}

	for _, tc := range invalidTests {
		t.Run(tc.name, func(t *testing.T) {
			err := corpus.ValidateObjectPath(tc.path)
			if err == nil {
				t.Fatalf("expected error for path %q, got nil", tc.path)
			}
			if !strings.Contains(err.Error(), tc.expectedErr) {
				t.Errorf("expected error to contain %q, got: %v", tc.expectedErr, err)
			}
		})
	}
}

func compileCorpusManifestSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()

	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemas.CorpusManifestSchemaJSON))
	if err != nil {
		t.Fatalf("failed to unmarshal embedded schema JSON: %v", err)
	}

	if err := compiler.AddResource("corpus-manifest.schema.json", doc); err != nil {
		t.Fatalf("failed to add schema resource: %v", err)
	}

	schema, err := compiler.Compile("corpus-manifest.schema.json")
	if err != nil {
		t.Fatalf("failed to compile schema: %v", err)
	}

	return schema
}

// TestCorpusManifestSchema_Draft2020MetaSchema validates that the embedded corpus manifest
// schema conforms strictly to the JSON Schema Draft 2020-12 meta-schema.
func TestCorpusManifestSchema_Draft2020MetaSchema(t *testing.T) {
	compiler := jsonschema.NewCompiler()
	metaSchema, err := compiler.Compile("https://json-schema.org/draft/2020-12/schema")
	if err != nil {
		t.Fatalf("failed to compile Draft 2020-12 meta-schema: %v", err)
	}

	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemas.CorpusManifestSchemaJSON))
	if err != nil {
		t.Fatalf("failed to unmarshal corpus manifest schema: %v", err)
	}

	if err := metaSchema.Validate(doc); err != nil {
		t.Fatalf("corpus manifest schema failed Draft 2020-12 meta-schema validation: %v", err)
	}
}

// TestValidateObjectPath_SchemaAndGoParity verifies that the JSON Schema object_path
// pattern and the Go ValidateObjectPath function reach identical validation decisions
// by evaluating both validation routes independently (without calling ValidateManifestBytes).
func TestValidateObjectPath_SchemaAndGoParity(t *testing.T) {
	schema := compileCorpusManifestSchema(t)

	testMatrix := []struct {
		path      string
		wantValid bool
	}{
		// Valid cases
		{"test.xdr", true},
		{"fixtures/test.xdr", true},
		{"testdata/synthetic/empty.xdr", true},
		{"objects/sha256/e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", true},
		{"a/b/c/file.xdr", true},
		{"ledger_2026-11-17.xdr", true},

		// Invalid cases - must be rejected by BOTH
		{"", false},
		{"   ", false},
		{" /leading/space", false},
		{"trailing/space ", false},
		{".", false},
		{"..", false},
		{"/test.xdr", false},
		{"/root/file.xdr", false},
		{"../secret.xdr", false},
		{"fixtures/../secret.xdr", false},
		{"./file.xdr", false},
		{"fixtures/./file.xdr", false},
		{"fixtures//file.xdr", false},
		{"fixtures/test/", false},
		{"fixtures\\test.xdr", false},
		{"C:file.xdr", false},
		{"C:\\file.xdr", false},
		{"file\x00.xdr", false},
		{"file\n.xdr", false},
		{"file name.xdr", false},
		{"file;rm.xdr", false},
	}

	for _, tc := range testMatrix {
		t.Run(fmt.Sprintf("parity_%s", tc.path), func(t *testing.T) {
			// 1. Check Go validation independently
			goErr := corpus.ValidateObjectPath(tc.path)
			goValid := goErr == nil

			if goValid != tc.wantValid {
				t.Errorf("Go ValidateObjectPath(%q): got valid=%v, want valid=%v (err=%v)", tc.path, goValid, tc.wantValid, goErr)
			}

			// 2. Check compiled JSON Schema validation independently (do not call ValidateManifestBytes)
			pathJSON, err := json.Marshal(tc.path)
			if err != nil {
				t.Fatalf("failed to marshal path %q: %v", tc.path, err)
			}

			manifestJSON := fmt.Sprintf(`{
  "schema_version": "1.0.0",
  "case_id": "case_parity_test",
  "title": "Parity Test",
  "summary": "Tests path parity between schema and Go",
  "classification": "synthetic_fixture",
  "source": {
    "kind": "synthetic_construction",
    "notes": "Parity test"
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
    "object_path": %s
  },
  "expectations": [
    {
      "assertion_id": "assertion_01",
      "kind": "ledger_sequence_equals",
      "expected": 100,
      "description": "desc"
    }
  ],
  "implementation_context": {
    "stellar_core_version": null,
    "sdk_versions": {},
    "indexer_version": null
  },
  "limitations": []
}`, string(pathJSON))

			doc, err := jsonschema.UnmarshalJSON(strings.NewReader(manifestJSON))
			if err != nil {
				t.Fatalf("failed to unmarshal test manifest JSON: %v", err)
			}
			schemaErr := schema.Validate(doc)
			schemaValid := schemaErr == nil

			if schemaValid != tc.wantValid {
				t.Errorf("Direct Schema Validate with path %q: got valid=%v, want valid=%v (err=%v)", tc.path, schemaValid, tc.wantValid, schemaErr)
			}

			// 3. Confirm exact agreement
			if goValid != schemaValid {
				t.Fatalf("DISAGREEMENT between Go (%v) and Schema (%v) for path %q", goValid, schemaValid, tc.path)
			}
		})
	}
}

// TestSyntheticNetworkName_DirectSchemaValidation tests the synthetic network naming
// policy directly against the compiled JSON Schema without calling Go validation helpers.
func TestSyntheticNetworkName_DirectSchemaValidation(t *testing.T) {
	schema := compileCorpusManifestSchema(t)

	// Valid cases: synthetic fixtures that positively identify as synthetic or standalone
	validSyntheticNames := []string{
		"Synthetic Standalone Network",
		"synthetic test network",
		"Standalone Test Cluster",
		"Local Synthetic Sandbox",
		"SYNTHETIC",
		"standalone",
		"My-Synthetic-Core-Node",
		"standalone_sandbox_env",
	}

	for _, name := range validSyntheticNames {
		t.Run("valid_synthetic_"+name, func(t *testing.T) {
			manifestJSON := fmt.Sprintf(`{
  "schema_version": "1.0.0",
  "case_id": "case_schema_synthetic_test",
  "title": "Synthetic Network Schema Test",
  "summary": "Tests schema validation of synthetic network names",
  "classification": "synthetic_fixture",
  "source": {
    "kind": "synthetic_construction",
    "notes": "Testing synthetic network name schema validation"
  },
  "network": {
    "name": %q,
    "protocol_version": 21,
    "ledger_sequence": 100
  },
  "input": {
    "format": "LedgerCloseMeta",
    "encoding": "xdr_binary",
    "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
    "byte_length": 0,
    "object_path": "fixtures/test.xdr"
  },
  "expectations": [
    {
      "assertion_id": "assertion_01",
      "kind": "ledger_sequence_equals",
      "expected": 100,
      "description": "desc"
    }
  ],
  "implementation_context": {
    "stellar_core_version": null,
    "sdk_versions": {},
    "indexer_version": null
  },
  "limitations": []
}`, name)

			doc, err := jsonschema.UnmarshalJSON(strings.NewReader(manifestJSON))
			if err != nil {
				t.Fatalf("failed to unmarshal JSON: %v", err)
			}
			if err := schema.Validate(doc); err != nil {
				t.Errorf("expected synthetic network name %q to pass schema validation, got error: %v", name, err)
			}
		})
	}

	// Invalid cases: synthetic fixtures with forbidden or missing naming
	invalidSyntheticNames := []struct {
		name   string
		reason string
	}{
		{"Mainnet", "forbidden mainnet"},
		{"Stellar Mainnet", "forbidden mainnet"},
		{"Public Global Stellar Network", "forbidden public network"},
		{"public  global  stellar  network", "forbidden public network"},
		{"Testnet", "forbidden testnet"},
		{"Stellar Testnet", "forbidden testnet"},
		{"Futurenet", "forbidden futurenet"},
		{"Stellar Futurenet", "forbidden futurenet"},
		{"Synthetic Mainnet", "forbidden mainnet in synthetic name"},
		{"Standalone Testnet", "forbidden testnet in standalone name"},
		{"Custom Private Network", "lacks synthetic or standalone identifier"},
		{"Stellar Local Core", "lacks synthetic or standalone identifier"},
		{"Local Devnet", "lacks synthetic or standalone identifier"},
	}

	for _, tc := range invalidSyntheticNames {
		t.Run("invalid_synthetic_"+tc.name, func(t *testing.T) {
			manifestJSON := fmt.Sprintf(`{
  "schema_version": "1.0.0",
  "case_id": "case_schema_synthetic_test",
  "title": "Synthetic Network Schema Test",
  "summary": "Tests schema rejection of misleading synthetic network names",
  "classification": "synthetic_fixture",
  "source": {
    "kind": "synthetic_construction",
    "notes": "Testing synthetic network name schema validation"
  },
  "network": {
    "name": %q,
    "protocol_version": 21,
    "ledger_sequence": 100
  },
  "input": {
    "format": "LedgerCloseMeta",
    "encoding": "xdr_binary",
    "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
    "byte_length": 0,
    "object_path": "fixtures/test.xdr"
  },
  "expectations": [
    {
      "assertion_id": "assertion_01",
      "kind": "ledger_sequence_equals",
      "expected": 100,
      "description": "desc"
    }
  ],
  "implementation_context": {
    "stellar_core_version": null,
    "sdk_versions": {},
    "indexer_version": null
  },
  "limitations": []
}`, tc.name)

			doc, err := jsonschema.UnmarshalJSON(strings.NewReader(manifestJSON))
			if err != nil {
				t.Fatalf("failed to unmarshal JSON: %v", err)
			}
			if err := schema.Validate(doc); err == nil {
				t.Errorf("expected synthetic network name %q (%s) to be rejected by schema, but validation passed", tc.name, tc.reason)
			}
		})
	}

	// Exemption verification: historical_regression and reproduced_regression MUST NOT be rejected
	// when using public or shared networks like "Public Global Stellar Network" or "Testnet".
	exemptCases := []struct {
		name           string
		classification string
		networkName    string
		sourceKind     string
		extraFields    string
	}{
		{
			name:           "historical_regression_with_public_network",
			classification: "historical_regression",
			networkName:    "Public Global Stellar Network",
			sourceKind:     "upstream_issue",
			extraFields:    `"url": "https://github.com/stellar/stellar-core/issues/123",`,
		},
		{
			name:           "historical_regression_with_testnet",
			classification: "historical_regression",
			networkName:    "Stellar Testnet",
			sourceKind:     "upstream_issue",
			extraFields:    `"url": "https://github.com/stellar/stellar-core/issues/123",`,
		},
		{
			name:           "reproduced_regression_with_public_network",
			classification: "reproduced_regression",
			networkName:    "Public Global Stellar Network",
			sourceKind:     "upstream_issue",
			extraFields:    `"url": "https://github.com/stellar/stellar-core/issues/123",`,
		},
	}

	for _, tc := range exemptCases {
		t.Run("exempt_"+tc.name, func(t *testing.T) {
			reproductionSection := ""
			if tc.classification == "reproduced_regression" {
				reproductionSection = `,
  "reproduction": {
    "affected_implementation": {"name": "stellar-core", "version": "v20.0.0"},
    "fixed_implementation": {"name": "stellar-core", "version": "v20.1.0"},
    "differential_evidence": {
      "evidence_ref": "evidence/runs/mock_diff.json",
      "sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
    }
  }`
			}

			manifestJSON := fmt.Sprintf(`{
  "schema_version": "1.0.0",
  "case_id": "case_exempt_network_test",
  "title": "Exempt Network Test",
  "summary": "Verifies historical/reproduced regressions are exempt from synthetic naming policy",
  "classification": %q,
  "source": {
    "kind": %q,
    %s
    "notes": "Testing exemption"
  },
  "network": {
    "name": %q,
    "protocol_version": 20,
    "ledger_sequence": 123456
  },
  "input": {
    "format": "LedgerCloseMeta",
    "encoding": "xdr_binary",
    "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
    "byte_length": 100,
    "object_path": "fixtures/test.xdr"
  },
  "expectations": [
    {
      "assertion_id": "assertion_01",
      "kind": "ledger_sequence_equals",
      "expected": 123456,
      "description": "desc"
    }
  ],
  "implementation_context": {
    "stellar_core_version": null,
    "sdk_versions": {},
    "indexer_version": null
  },
  "limitations": []%s
}`, tc.classification, tc.sourceKind, tc.extraFields, tc.networkName, reproductionSection)

			doc, err := jsonschema.UnmarshalJSON(strings.NewReader(manifestJSON))
			if err != nil {
				t.Fatalf("failed to unmarshal JSON: %v", err)
			}
			if err := schema.Validate(doc); err != nil {
				t.Errorf("expected %s with network %q to pass schema validation (exempt from synthetic naming rules), got error: %v", tc.classification, tc.networkName, err)
			}
		})
	}
}
