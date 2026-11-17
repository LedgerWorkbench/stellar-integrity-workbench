package localfile

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestStore_FileGrowsDuringRead_StreamingCheck exercises the bounded streaming read path
// where a file initially appears below the configured limit, but additional bytes are appended
// after opening the read handle, triggering rejection during the streaming read.
func TestStore_FileGrowsDuringRead_StreamingCheck(t *testing.T) {
	dir := t.TempDir()

	// Configured per-input limit of 32 bytes
	limit := int64(32)

	// Step 1: Create a regular file with 16 bytes (well below limit of 32)
	initialData := []byte("1234567890123456")
	relPath := "fixtures/growing.bin"
	fullPath := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(fullPath, initialData, 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// Step 2: Open a read handle on the file
	readHandle, err := os.Open(fullPath)
	if err != nil {
		t.Fatalf("open read handle failed: %v", err)
	}
	defer readHandle.Close()

	// Step 3: Record its initial reported size
	initialStat, err := readHandle.Stat()
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	initialSize := initialStat.Size()
	if initialSize != int64(len(initialData)) {
		t.Fatalf("unexpected initial size: got %d, want %d", initialSize, len(initialData))
	}

	// Step 4: Append 50 bytes using a separate file handle after readHandle is already open.
	// The file on disk now contains 16 + 50 = 66 bytes (exceeding limit of 32 bytes).
	appendHandle, err := os.OpenFile(fullPath, os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		t.Fatalf("open append handle failed: %v", err)
	}
	if _, err := appendHandle.Write(bytes.Repeat([]byte("X"), 50)); err != nil {
		_ = appendHandle.Close()
		t.Fatalf("append write failed: %v", err)
	}
	if err := appendHandle.Close(); err != nil {
		t.Fatalf("close append handle failed: %v", err)
	}

	// Step 5: Exercise the streaming verification path passing the original reported size (16 bytes).
	// Because initialSize <= limit, any pre-read check passes.
	// The failure must occur during the streaming read as soon as totalRead exceeds limit (32 bytes).
	h := sha256.Sum256(initialData)
	initialHash := hex.EncodeToString(h[:])

	_, err = readAndVerify(readHandle, initialSize, limit, initialSize, initialHash, relPath)

	// Step 6: Assert that the result wraps ErrFileTooLarge
	if err == nil {
		t.Fatalf("expected ErrFileTooLarge from streaming read, got nil")
	}
	if !errors.Is(err, ErrFileTooLarge) {
		t.Errorf("expected error wrapping ErrFileTooLarge, got: %v", err)
	}
}
