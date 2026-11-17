package corpus_test

import (
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
		{"whitespace path", "   ", "empty"},
		{"absolute unix path", "/etc/passwd", "relative, not absolute"},
		{"parent directory traversal start", "../secret.xdr", "traversal"},
		{"parent directory traversal middle", "fixtures/../secret.xdr", "traversal"},
		{"current directory segment start", "./file.xdr", "current directory segment"},
		{"current directory segment middle", "fixtures/./file.xdr", "current directory segment"},
		{"double slash", "fixtures//file.xdr", "empty path segment"},
		{"trailing slash", "fixtures/test/", "empty path segment"},
		{"windows backslash", "fixtures\\test.xdr", "invalid path separator"},
		{"windows drive letter", "C:file.xdr", "Windows drive"},
		{"windows absolute path", "C:\\file.xdr", "invalid path separator"},
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
