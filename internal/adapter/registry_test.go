package adapter

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func createDummyExecutable(t *testing.T, dir, name string, mode os.FileMode) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\nexit 0\n"), mode); err != nil {
		t.Fatalf("failed to write dummy executable: %v", err)
	}
	return p
}

func TestNewProductionRegistry_OnlyGoIngest(t *testing.T) {
	dir := t.TempDir()
	bin := createDummyExecutable(t, dir, "go-ingest-adapter", 0o755)

	reg, err := NewProductionRegistry(ProductionRegistryConfig{
		GoIngestBinaryPath: bin,
	})
	if err != nil {
		t.Fatalf("NewProductionRegistry failed: %v", err)
	}

	ids := reg.ListIDs()
	if len(ids) != 1 || ids[0] != AdapterIDGoIngest {
		t.Fatalf("expected only [%q], got %v", AdapterIDGoIngest, ids)
	}

	// Ensure no test adapter is registered.
	for _, forbidden := range []string{"test", "test-adapter", "mock", "js-xdr", "rust-xdr"} {
		_, err := reg.Resolve(RequestTarget{
			AdapterID:      forbidden,
			AdapterVersion: DefaultGoAdapterVersion,
			SDKVersion:     PinnedGoSDKVersion,
		}, OperationDecodeLedgerMeta)
		if !errors.Is(err, ErrAdapterUnavailable) {
			t.Fatalf("expected ErrAdapterUnavailable for %q, got %v", forbidden, err)
		}
	}
}

func TestNewProductionRegistry_EmptyPath(t *testing.T) {
	_, err := NewProductionRegistry(ProductionRegistryConfig{})
	if !errors.Is(err, ErrAdapterUnavailable) {
		t.Fatalf("expected ErrAdapterUnavailable for empty path, got %v", err)
	}
}

func TestRegistry_Resolve_Valid(t *testing.T) {
	dir := t.TempDir()
	bin := createDummyExecutable(t, dir, "go-ingest-adapter", 0o755)

	reg, err := NewProductionRegistry(ProductionRegistryConfig{
		GoIngestBinaryPath: bin,
		GoIngestVersion:    "v0.1.0",
		GoSDKVersion:       "v0.9.0",
	})
	if err != nil {
		t.Fatalf("NewProductionRegistry failed: %v", err)
	}

	spec, err := reg.Resolve(RequestTarget{
		AdapterID:      AdapterIDGoIngest,
		AdapterVersion: "v0.1.0",
		SDKVersion:     "v0.9.0",
	}, OperationDecodeLedgerMeta)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if spec.BinaryPath != bin {
		t.Fatalf("expected binary %q, got %q", bin, spec.BinaryPath)
	}
}

func TestRegistry_Resolve_UnknownAdapter(t *testing.T) {
	dir := t.TempDir()
	bin := createDummyExecutable(t, dir, "go-ingest-adapter", 0o755)

	reg, err := NewProductionRegistry(ProductionRegistryConfig{GoIngestBinaryPath: bin})
	if err != nil {
		t.Fatalf("NewProductionRegistry failed: %v", err)
	}

	_, err = reg.Resolve(RequestTarget{
		AdapterID:      "unknown-adapter",
		AdapterVersion: DefaultGoAdapterVersion,
		SDKVersion:     PinnedGoSDKVersion,
	}, OperationDecodeLedgerMeta)
	if !errors.Is(err, ErrAdapterUnavailable) {
		t.Fatalf("expected ErrAdapterUnavailable, got %v", err)
	}
}

func TestRegistry_Resolve_VersionMismatches(t *testing.T) {
	dir := t.TempDir()
	bin := createDummyExecutable(t, dir, "go-ingest-adapter", 0o755)

	reg, err := NewProductionRegistry(ProductionRegistryConfig{
		GoIngestBinaryPath: bin,
		GoIngestVersion:    "v0.1.0",
		GoSDKVersion:       PinnedGoSDKVersion,
	})
	if err != nil {
		t.Fatalf("NewProductionRegistry failed: %v", err)
	}

	t.Run("adapter version mismatch", func(t *testing.T) {
		_, err := reg.Resolve(RequestTarget{
			AdapterID:      AdapterIDGoIngest,
			AdapterVersion: "v9.9.9",
			SDKVersion:     PinnedGoSDKVersion,
		}, OperationDecodeLedgerMeta)
		if !errors.Is(err, ErrAdapterIdentityMismatch) {
			t.Fatalf("expected ErrAdapterIdentityMismatch, got %v", err)
		}
	})

	t.Run("sdk version mismatch", func(t *testing.T) {
		_, err := reg.Resolve(RequestTarget{
			AdapterID:      AdapterIDGoIngest,
			AdapterVersion: "v0.1.0",
			SDKVersion:     "v0.1.0",
		}, OperationDecodeLedgerMeta)
		if !errors.Is(err, ErrAdapterIdentityMismatch) {
			t.Fatalf("expected ErrAdapterIdentityMismatch, got %v", err)
		}
	})

	t.Run("unsupported operation", func(t *testing.T) {
		_, err := reg.Resolve(RequestTarget{
			AdapterID:      AdapterIDGoIngest,
			AdapterVersion: "v0.1.0",
			SDKVersion:     PinnedGoSDKVersion,
		}, "arbitrary_shell_exec")
		if !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("expected ErrInvalidRequest, got %v", err)
		}
	})
}

func TestRegistry_Resolve_MissingOrInvalidBinary(t *testing.T) {
	dir := t.TempDir()

	t.Run("missing binary", func(t *testing.T) {
		reg, err := NewProductionRegistry(ProductionRegistryConfig{
			GoIngestBinaryPath: filepath.Join(dir, "does-not-exist"),
		})
		if err != nil {
			t.Fatalf("NewProductionRegistry failed: %v", err)
		}
		_, err = reg.Resolve(RequestTarget{
			AdapterID:      AdapterIDGoIngest,
			AdapterVersion: DefaultGoAdapterVersion,
			SDKVersion:     PinnedGoSDKVersion,
		}, OperationDecodeLedgerMeta)
		if !errors.Is(err, ErrAdapterUnavailable) {
			t.Fatalf("expected ErrAdapterUnavailable, got %v", err)
		}
	})

	t.Run("directory instead of file", func(t *testing.T) {
		subDir := filepath.Join(dir, "subdir")
		if err := os.Mkdir(subDir, 0o755); err != nil {
			t.Fatal(err)
		}
		reg, err := NewProductionRegistry(ProductionRegistryConfig{
			GoIngestBinaryPath: subDir,
		})
		if err != nil {
			t.Fatalf("NewProductionRegistry failed: %v", err)
		}
		_, err = reg.Resolve(RequestTarget{
			AdapterID:      AdapterIDGoIngest,
			AdapterVersion: DefaultGoAdapterVersion,
			SDKVersion:     PinnedGoSDKVersion,
		}, OperationDecodeLedgerMeta)
		if !errors.Is(err, ErrAdapterUnavailable) {
			t.Fatalf("expected ErrAdapterUnavailable, got %v", err)
		}
	})

	if runtime.GOOS != "windows" {
		t.Run("non-executable file", func(t *testing.T) {
			nonExec := createDummyExecutable(t, dir, "not-exec", 0o644)
			reg, err := NewProductionRegistry(ProductionRegistryConfig{
				GoIngestBinaryPath: nonExec,
			})
			if err != nil {
				t.Fatalf("NewProductionRegistry failed: %v", err)
			}
			_, err = reg.Resolve(RequestTarget{
				AdapterID:      AdapterIDGoIngest,
				AdapterVersion: DefaultGoAdapterVersion,
				SDKVersion:     PinnedGoSDKVersion,
			}, OperationDecodeLedgerMeta)
			if !errors.Is(err, ErrAdapterUnavailable) {
				t.Fatalf("expected ErrAdapterUnavailable, got %v", err)
			}
		})
	}
}
