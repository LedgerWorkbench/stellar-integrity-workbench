//go:build unix || linux || darwin

package localfile_test

import (
	"errors"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/source/localfile"
)

func TestStore_FIFO_DoesNotHang(t *testing.T) {
	dir, store := setupTestStore(t, 1024*1024)

	fifoPath := filepath.Join(dir, "test.fifo")
	// Create named pipe on Unix platforms
	if err := syscall.Mkfifo(fifoPath, 0666); err != nil {
		t.Fatalf("mkfifo failed: %v", err)
	}

	dummyHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	// Run LoadAndVerify with a timeout to prove it does not block waiting for a writer
	done := make(chan error, 1)
	go func() {
		_, err := store.LoadAndVerify("test.fifo", 0, dummyHash)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("expected error for FIFO, got nil")
		}
		if !errors.Is(err, localfile.ErrNotRegularFile) {
			t.Errorf("expected ErrNotRegularFile for FIFO, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("DEADLOCK: store.LoadAndVerify blocked indefinitely on opening FIFO")
	}
}
