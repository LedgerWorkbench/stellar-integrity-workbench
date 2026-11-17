package localfile_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/domain"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/source/localfile"
)

// computeSHA256 returns the lowercase hex-encoded SHA-256 digest of b.
func computeSHA256(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// setupTestStore creates a temporary directory with a Store configured for testing.
func setupTestStore(t *testing.T, maxBytes int64) (string, *localfile.Store) {
	t.Helper()
	dir := t.TempDir()

	if maxBytes <= 0 {
		maxBytes = localfile.DefaultMaxBytes
	}

	cfg := localfile.Config{
		RootDir:  dir,
		MaxBytes: maxBytes,
	}

	store, err := localfile.NewStore(cfg)
	if err != nil {
		t.Fatalf("failed to create Store: %v", err)
	}

	t.Cleanup(func() {
		_ = store.Close()
	})

	return dir, store
}

func TestStore_ValidFile(t *testing.T) {
	dir, store := setupTestStore(t, 1024*1024)

	// Explicitly synthetic test bytes (not authentic ledger data)
	payload := []byte("SYNTHETIC_TEST_PAYLOAD_LEDGER_CLOSE_META_MOCK_BYTES_1234567890")
	relPath := "fixtures/synthetic_sample.bin"

	fullPath := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatalf("failed to create dirs: %v", err)
	}
	if err := os.WriteFile(fullPath, payload, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	expectedLen := int64(len(payload))
	expectedHash := computeSHA256(payload)

	verified, err := store.LoadAndVerify(relPath, expectedLen, expectedHash)
	if err != nil {
		t.Fatalf("expected successful verification, got error: %v", err)
	}

	if verified.ObjectPath() != relPath {
		t.Errorf("ObjectPath() mismatch: got %q, want %q", verified.ObjectPath(), relPath)
	}
	if verified.ByteLength() != expectedLen {
		t.Errorf("ByteLength() mismatch: got %d, want %d", verified.ByteLength(), expectedLen)
	}
	if verified.SHA256() != expectedHash {
		t.Errorf("SHA256() mismatch: got %q, want %q", verified.SHA256(), expectedHash)
	}
	if verified.Len() != len(payload) {
		t.Errorf("Len() mismatch: got %d, want %d", verified.Len(), len(payload))
	}

	// Verify reading from NewReader returns exact bytes
	readerBytes, err := io.ReadAll(verified.NewReader())
	if err != nil {
		t.Fatalf("failed to read from NewReader: %v", err)
	}
	if !bytes.Equal(readerBytes, payload) {
		t.Errorf("reader bytes do not match original payload")
	}
}

func TestStore_CorruptedFile_SHA256Mismatch(t *testing.T) {
	dir, store := setupTestStore(t, 1024*1024)

	original := []byte("SYNTHETIC_CORPUS_ORIGINAL_BYTES_FOR_CHECKSUM_TEST_9999999999")
	corrupted := make([]byte, len(original))
	copy(corrupted, original)
	corrupted[15] = 'X' // flip byte to alter hash while keeping length identical

	relPath := "fixtures/corrupted.bin"
	fullPath := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(fullPath, corrupted, 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// Supply the hash of the original file, not the corrupted file on disk
	expectedHash := computeSHA256(original)
	expectedLen := int64(len(original))

	_, err := store.LoadAndVerify(relPath, expectedLen, expectedHash)
	if err == nil {
		t.Fatalf("expected error for corrupted file, got nil")
	}

	if !errors.Is(err, localfile.ErrSHA256Mismatch) {
		t.Errorf("expected error wrapping ErrSHA256Mismatch, got: %v", err)
	}
}

func TestStore_TruncatedFile_ByteLengthMismatch(t *testing.T) {
	dir, store := setupTestStore(t, 1024*1024)

	payload := []byte("SYNTHETIC_FULL_PAYLOAD_1234567890")
	truncated := payload[:10]

	relPath := "fixtures/truncated.bin"
	fullPath := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(fullPath, truncated, 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	expectedLen := int64(len(payload))
	expectedHash := computeSHA256(payload)

	_, err := store.LoadAndVerify(relPath, expectedLen, expectedHash)
	if err == nil {
		t.Fatalf("expected error for truncated file, got nil")
	}

	if !errors.Is(err, localfile.ErrByteLengthMismatch) {
		t.Errorf("expected error wrapping ErrByteLengthMismatch, got: %v", err)
	}
}

func TestStore_AppendedFile_ByteLengthMismatch(t *testing.T) {
	dir, store := setupTestStore(t, 1024*1024)

	payload := []byte("SYNTHETIC_BASE_PAYLOAD")
	appended := append(payload, []byte("_EXTRA_APPENDED_BYTES")...)

	relPath := "fixtures/appended.bin"
	fullPath := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(fullPath, appended, 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	expectedLen := int64(len(payload))
	expectedHash := computeSHA256(payload)

	_, err := store.LoadAndVerify(relPath, expectedLen, expectedHash)
	if err == nil {
		t.Fatalf("expected error for appended file, got nil")
	}

	if !errors.Is(err, localfile.ErrByteLengthMismatch) {
		t.Errorf("expected error wrapping ErrByteLengthMismatch, got: %v", err)
	}
}

func TestStore_ExceedsConfiguredSizeLimit_PreRead(t *testing.T) {
	limit := int64(64) // strict 64-byte limit for test
	dir, store := setupTestStore(t, limit)

	// Create a 128-byte file (exceeds 64-byte limit)
	oversized := bytes.Repeat([]byte("A"), 128)

	relPath := "fixtures/oversized.bin"
	fullPath := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(fullPath, oversized, 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	expectedLen := int64(len(oversized))
	expectedHash := computeSHA256(oversized)

	_, err := store.LoadAndVerify(relPath, expectedLen, expectedHash)
	if err == nil {
		t.Fatalf("expected error for oversized file, got nil")
	}

	if !errors.Is(err, localfile.ErrFileTooLarge) {
		t.Errorf("expected error wrapping ErrFileTooLarge, got: %v", err)
	}
}

func TestStore_FileGrowsDuringRead_StreamingCheck(t *testing.T) {
	limit := int64(32) // strict 32-byte limit
	dir, store := setupTestStore(t, limit)

	// Initial file written with 16 bytes (below limit of 32)
	initialData := []byte("1234567890123456")
	relPath := "fixtures/growing.bin"
	fullPath := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(fullPath, initialData, 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// Test deterministic streaming detection by appending to file while open or via pipe
	// We append 50 extra bytes so file on disk now has 66 bytes > 32 limit
	f, err := os.OpenFile(fullPath, os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		t.Fatalf("open for append failed: %v", err)
	}
	_, _ = f.Write(bytes.Repeat([]byte("X"), 50))
	_ = f.Close()

	// Calling LoadAndVerify should reject because size on disk now exceeds limit
	_, err = store.LoadAndVerify(relPath, int64(len(initialData)), computeSHA256(initialData))
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, localfile.ErrFileTooLarge) {
		t.Errorf("expected ErrFileTooLarge, got: %v", err)
	}
}

func TestStore_EmptyFile(t *testing.T) {
	dir, store := setupTestStore(t, 1024)

	relPath := "fixtures/empty.bin"
	fullPath := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(fullPath, []byte{}, 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// Standard SHA-256 for empty input
	const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	verified, err := store.LoadAndVerify(relPath, 0, emptySHA256)
	if err != nil {
		t.Fatalf("expected successful verification of empty file, got: %v", err)
	}

	if verified.ByteLength() != 0 {
		t.Errorf("expected ByteLength 0, got %d", verified.ByteLength())
	}
	if verified.SHA256() != emptySHA256 {
		t.Errorf("expected SHA-256 %s, got %s", emptySHA256, verified.SHA256())
	}
	if verified.Len() != 0 {
		t.Errorf("expected Len 0, got %d", verified.Len())
	}
}

func TestStore_UnsafePaths(t *testing.T) {
	_, store := setupTestStore(t, 1024)

	dummyHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	unsafePaths := []struct {
		name string
		path string
	}{
		{"absolute unix", "/etc/passwd"},
		{"absolute root", "/"},
		{"parent directory alone", ".."},
		{"current directory alone", "."},
		{"parent directory start", "../secret.xdr"},
		{"parent directory middle", "fixtures/../secret.xdr"},
		{"current directory segment", "fixtures/./file.xdr"},
		{"double slash", "fixtures//file.xdr"},
		{"trailing slash", "fixtures/test/"},
		{"windows backslash", "fixtures\\test.xdr"},
		{"windows drive", "C:file.xdr"},
		{"null byte", "file\x00.xdr"},
		{"newline", "file\n.xdr"},
	}

	for _, tc := range unsafePaths {
		t.Run(tc.name, func(t *testing.T) {
			_, err := store.LoadAndVerify(tc.path, 10, dummyHash)
			if err == nil {
				t.Fatalf("expected error for path %q, got nil", tc.path)
			}
			if !errors.Is(err, localfile.ErrUnsafePath) {
				t.Errorf("expected error wrapping ErrUnsafePath for %q, got: %v", tc.path, err)
			}
		})
	}
}

func TestStore_SymlinkEscapeOutsideRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping symlink test on Windows")
	}

	// Create root directory and an external directory
	parentDir := t.TempDir()
	rootDir := filepath.Join(parentDir, "storage_root")
	outsideDir := filepath.Join(parentDir, "outside")

	if err := os.MkdirAll(rootDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.MkdirAll(outsideDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	// Create an external secret file
	outsideFile := filepath.Join(outsideDir, "secret.bin")
	secretPayload := []byte("OUTSIDE_SECRET_BYTES")
	if err := os.WriteFile(outsideFile, secretPayload, 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// Create a symlink inside rootDir pointing outside rootDir
	escapeLink := filepath.Join(rootDir, "escape_link.bin")
	if err := os.Symlink(outsideFile, escapeLink); err != nil {
		t.Fatalf("symlink failed: %v", err)
	}

	store, err := localfile.NewStore(localfile.DefaultConfig(rootDir))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	expectedHash := computeSHA256(secretPayload)
	_, err = store.LoadAndVerify("escape_link.bin", int64(len(secretPayload)), expectedHash)
	if err == nil {
		t.Fatalf("expected symlink escape to be rejected, but it succeeded")
	}

	// Confinement must reject escape (via ErrUnsafePath or ErrFileNotFound)
	if !errors.Is(err, localfile.ErrUnsafePath) && !errors.Is(err, localfile.ErrFileNotFound) {
		t.Errorf("expected error wrapping ErrUnsafePath or ErrFileNotFound, got: %v", err)
	}
}

func TestStore_SymlinkInternalValid(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping symlink test on Windows")
	}

	dir, store := setupTestStore(t, 1024*1024)

	// Create an authentic regular file inside the root
	targetRel := "fixtures/target.bin"
	targetFull := filepath.Join(dir, targetRel)
	if err := os.MkdirAll(filepath.Dir(targetFull), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	payload := []byte("INTERNAL_VALID_SYMLINK_TARGET_PAYLOAD")
	if err := os.WriteFile(targetFull, payload, 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// Create a symlink inside the root pointing to another relative path inside the root
	linkRel := "fixtures/link_to_target.bin"
	linkFull := filepath.Join(dir, linkRel)
	if err := os.Symlink("target.bin", linkFull); err != nil {
		t.Fatalf("symlink creation failed: %v", err)
	}

	expectedLen := int64(len(payload))
	expectedHash := computeSHA256(payload)

	verified, err := store.LoadAndVerify(linkRel, expectedLen, expectedHash)
	if err != nil {
		t.Fatalf("expected internal symlink to succeed, got error: %v", err)
	}

	if verified.ByteLength() != expectedLen {
		t.Errorf("length mismatch: got %d, want %d", verified.ByteLength(), expectedLen)
	}
}

func TestStore_DirectoryAsInput(t *testing.T) {
	dir, store := setupTestStore(t, 1024*1024)

	subDir := filepath.Join(dir, "fixtures/subfolder")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	dummyHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	_, err := store.LoadAndVerify("fixtures/subfolder", 0, dummyHash)
	if err == nil {
		t.Fatalf("expected error when object path is a directory, got nil")
	}

	if !errors.Is(err, localfile.ErrNotRegularFile) {
		t.Errorf("expected ErrNotRegularFile, got: %v", err)
	}
}

func TestStore_FIFO_DoesNotHang(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping FIFO test on Windows")
	}

	dir, store := setupTestStore(t, 1024*1024)

	fifoPath := filepath.Join(dir, "test.fifo")
	// Create named pipe
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

func TestStore_OriginalFileUnchanged(t *testing.T) {
	dir, store := setupTestStore(t, 1024*1024)

	relPath := "fixtures/immutability_test.bin"
	fullPath := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	originalPayload := []byte("ORIGINAL_FILE_CANNOT_BE_MUTATED_12345")
	if err := os.WriteFile(fullPath, originalPayload, 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	expectedLen := int64(len(originalPayload))
	expectedHash := computeSHA256(originalPayload)

	// Successful verification
	_, err := store.LoadAndVerify(relPath, expectedLen, expectedHash)
	if err != nil {
		t.Fatalf("successful verification failed: %v", err)
	}

	onDiskAfterSuccess, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("read after success failed: %v", err)
	}
	if !bytes.Equal(onDiskAfterSuccess, originalPayload) {
		t.Fatalf("file on disk modified after successful load")
	}

	// Failed verification
	wrongHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	_, _ = store.LoadAndVerify(relPath, expectedLen, wrongHash)

	onDiskAfterFail, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("read after failure failed: %v", err)
	}
	if !bytes.Equal(onDiskAfterFail, originalPayload) {
		t.Fatalf("file on disk modified after failed load")
	}
}

func TestStore_VerifiedInputImmutability(t *testing.T) {
	dir, store := setupTestStore(t, 1024*1024)

	relPath := "fixtures/immutable_buffer.bin"
	fullPath := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	original := []byte("IMMUTABLE_BUFFER_CONTENT_STAYS_CONSTANT")
	if err := os.WriteFile(fullPath, original, 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	verified, err := store.LoadAndVerify(relPath, int64(len(original)), computeSHA256(original))
	if err != nil {
		t.Fatalf("LoadAndVerify failed: %v", err)
	}

	// 1. Get defensive copy and mutate it
	copy1 := verified.BytesCopy()
	copy1[0] = 'Z'
	copy1[1] = 'Z'

	// 2. Read through NewReader - must remain unmodified
	readerBytes, err := io.ReadAll(verified.NewReader())
	if err != nil {
		t.Fatalf("ReadAll from NewReader failed: %v", err)
	}
	if !bytes.Equal(readerBytes, original) {
		t.Errorf("verified buffer was mutated via external slice modification!")
	}

	// 3. Second BytesCopy must still equal original
	copy2 := verified.BytesCopy()
	if !bytes.Equal(copy2, original) {
		t.Errorf("second BytesCopy does not match original bytes!")
	}
}

// mockDownstreamConsumer represents a downstream pipeline consumer (e.g. an XDR decoder
// or replay adapter) in the test harness.
type mockDownstreamConsumer struct {
	invoked       bool
	receivedBytes []byte
}

func (c *mockDownstreamConsumer) Consume(input *localfile.VerifiedInput) {
	c.invoked = true
	c.receivedBytes = input.BytesCopy()
}

func TestStore_DownstreamConsumerSeam(t *testing.T) {
	dir, store := setupTestStore(t, 1024*1024)

	relPath := "fixtures/consumer_test.bin"
	fullPath := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	payload := []byte("CONSUMER_TEST_SYNTHETIC_PAYLOAD")
	if err := os.WriteFile(fullPath, payload, 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	correctLen := int64(len(payload))
	correctHash := computeSHA256(payload)
	corruptHash := "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"

	// 1. When verification fails, downstream consumer MUST NOT be invoked
	t.Run("integrity failure does not reach consumer", func(t *testing.T) {
		consumer := &mockDownstreamConsumer{}

		verified, err := store.LoadAndVerify(relPath, correctLen, corruptHash)
		if err != nil {
			// Expected error: pipeline stops here, consumer is NOT called
		} else {
			consumer.Consume(verified)
		}

		if consumer.invoked {
			t.Fatalf("DOWNSTREAM LEAK: consumer was invoked despite integrity verification failure!")
		}
		if consumer.receivedBytes != nil {
			t.Fatalf("consumer received bytes despite integrity verification failure!")
		}
	})

	// 2. When verification succeeds, downstream consumer IS invoked with authentic bytes
	t.Run("integrity success reaches consumer", func(t *testing.T) {
		consumer := &mockDownstreamConsumer{}

		verified, err := store.LoadAndVerify(relPath, correctLen, correctHash)
		if err != nil {
			t.Fatalf("unexpected verification failure: %v", err)
		}
		consumer.Consume(verified)

		if !consumer.invoked {
			t.Fatalf("expected consumer to be invoked on valid input")
		}
		if !bytes.Equal(consumer.receivedBytes, payload) {
			t.Fatalf("consumer received corrupted bytes")
		}
	})
}

func TestStore_ConfigValidation(t *testing.T) {
	validDir := t.TempDir()

	testCases := []struct {
		name    string
		cfg     localfile.Config
		wantErr bool
	}{
		{
			name:    "empty root dir",
			cfg:     localfile.Config{RootDir: "", MaxBytes: 1024},
			wantErr: true,
		},
		{
			name:    "whitespace root dir",
			cfg:     localfile.Config{RootDir: "   ", MaxBytes: 1024},
			wantErr: true,
		},
		{
			name:    "zero max bytes",
			cfg:     localfile.Config{RootDir: validDir, MaxBytes: 0},
			wantErr: true,
		},
		{
			name:    "negative max bytes",
			cfg:     localfile.Config{RootDir: validDir, MaxBytes: -100},
			wantErr: true,
		},
		{
			name:    "non-existent root dir",
			cfg:     localfile.Config{RootDir: filepath.Join(validDir, "non_existent"), MaxBytes: 1024},
			wantErr: true,
		},
		{
			name:    "valid configuration",
			cfg:     localfile.DefaultConfig(validDir),
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := localfile.NewStore(tc.cfg)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !errors.Is(err, localfile.ErrInvalidConfig) {
					t.Errorf("expected ErrInvalidConfig, got: %v", err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				_ = s.Close()
			}
		})
	}
}

func TestStore_LoadAndVerifyInput(t *testing.T) {
	dir, store := setupTestStore(t, 1024*1024)

	relPath := "fixtures/domain_input_test.bin"
	fullPath := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	payload := []byte("TESTING_DOMAIN_INPUT_STRUCT_INTEGRATION")
	if err := os.WriteFile(fullPath, payload, 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	input := domain.Input{
		Format:     "LedgerCloseMeta",
		Encoding:   "xdr_binary",
		SHA256:     computeSHA256(payload),
		ByteLength: int64(len(payload)),
		ObjectPath: relPath,
	}

	verified, err := store.LoadAndVerifyInput(input)
	if err != nil {
		t.Fatalf("expected success with domain.Input, got: %v", err)
	}

	if verified.ByteLength() != input.ByteLength {
		t.Errorf("byte length mismatch: got %d, want %d", verified.ByteLength(), input.ByteLength)
	}
	if verified.SHA256() != input.SHA256 {
		t.Errorf("SHA-256 mismatch: got %q, want %q", verified.SHA256(), input.SHA256)
	}
}
