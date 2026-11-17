package corpus_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/corpus"
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

// TestValidateObjectPath_SchemaAndGoParity verifies that the JSON Schema object_path
// pattern and the Go ValidateObjectPath function reach identical validation decisions.
func TestValidateObjectPath_SchemaAndGoParity(t *testing.T) {
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
			// 1. Check Go validation
			goErr := corpus.ValidateObjectPath(tc.path)
			goValid := goErr == nil

			if goValid != tc.wantValid {
				t.Errorf("Go ValidateObjectPath(%q): got valid=%v, want valid=%v (err=%v)", tc.path, goValid, tc.wantValid, goErr)
			}

			// 2. Check JSON Schema validation via ValidateManifestBytes
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
    "object_path": %q
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
}`, tc.path)

			_, _, schemaErr := corpus.ValidateManifestBytes([]byte(manifestJSON))
			schemaValid := schemaErr == nil

			if schemaValid != tc.wantValid {
				t.Errorf("Schema ValidateManifestBytes with path %q: got valid=%v, want valid=%v (err=%v)", tc.path, schemaValid, tc.wantValid, schemaErr)
			}

			// 3. Confirm exact agreement
			if goValid != schemaValid {
				t.Fatalf("DISAGREEMENT between Go (%v) and Schema (%v) for path %q", goValid, schemaValid, tc.path)
			}
		})
	}
}
