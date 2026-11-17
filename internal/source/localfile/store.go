package localfile

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/corpus"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/domain"
)

const (
	// DefaultMaxBytes is 64 MiB (67,108,864 bytes).
	// Rationale: Stellar ledger-close metadata envelopes (LedgerCloseMeta) under current
	// network protocol limits (Protocol 20-22) are naturally bounded by transaction and operation
	// limits per ledger, rarely exceeding several megabytes even during peak network traffic.
	// 64 MiB provides generous headroom for all historical mainnet ledgers while strictly
	// bounding worker memory consumption to protect against out-of-memory denial-of-service.
	DefaultMaxBytes int64 = 64 * 1024 * 1024
)

// Config defines the configuration for the local file source store.
type Config struct {
	// RootDir is the trusted absolute or clean relative path to the local corpus storage directory.
	// Requests cannot choose an arbitrary root directory.
	RootDir string

	// MaxBytes is the strict upper bound on file sizes permitted to be read into memory.
	// Must be greater than zero.
	MaxBytes int64
}

// DefaultConfig returns a Config with DefaultMaxBytes (64 MiB) and the provided root directory.
func DefaultConfig(rootDir string) Config {
	return Config{
		RootDir:  rootDir,
		MaxBytes: DefaultMaxBytes,
	}
}

// Store provides secure, root-confined access to local corpus object files.
type Store struct {
	root     *os.Root
	rootDir  string
	maxBytes int64
}

// NewStore initializes a new Store confined to cfg.RootDir.
// It verifies that cfg.RootDir exists, is a directory, and that cfg.MaxBytes is positive.
func NewStore(cfg Config) (*Store, error) {
	if strings.TrimSpace(cfg.RootDir) == "" {
		return nil, fmt.Errorf("%w: storage root directory must not be empty", ErrInvalidConfig)
	}

	if cfg.MaxBytes <= 0 {
		return nil, fmt.Errorf("%w: max bytes %d must be greater than zero", ErrInvalidConfig, cfg.MaxBytes)
	}

	cleanRoot := filepath.Clean(cfg.RootDir)
	info, err := os.Stat(cleanRoot)
	if err != nil {
		return nil, fmt.Errorf("%w: storage root stat failed: %v", ErrInvalidConfig, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: storage root is not a directory", ErrInvalidConfig)
	}

	root, err := os.OpenRoot(cleanRoot)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to open storage root: %v", ErrInvalidConfig, err)
	}

	return &Store{
		root:     root,
		rootDir:  cleanRoot,
		maxBytes: cfg.MaxBytes,
	}, nil
}

// Close closes the underlying root handle.
func (s *Store) Close() error {
	if s.root != nil {
		return s.root.Close()
	}
	return nil
}

// MaxBytes returns the configured maximum input size limit in bytes.
func (s *Store) MaxBytes() int64 {
	return s.maxBytes
}

// LoadAndVerifyInput loads and verifies the corpus object described by domain.Input.
func (s *Store) LoadAndVerifyInput(input domain.Input) (*VerifiedInput, error) {
	return s.LoadAndVerify(input.ObjectPath, input.ByteLength, input.SHA256)
}

// LoadAndVerify loads the corpus object at objectPath relative to the storage root.
//
// Enforcement and verification steps:
// 1. Validates the relative object path using corpus.ValidateObjectPath (rejecting traversal,
//    absolute paths, dot segments, Windows drives, and control characters).
// 2. Confines the open operation to the storage root using os.Root, preventing symlink escapes.
// 3. Opens the file safely using non-blocking I/O to guarantee that special files like FIFOs
//    cannot hang the worker process.
// 4. Verifies that the file is a regular file (rejecting directories, FIFOs, sockets, and devices).
// 5. Pre-read check: Rejects files whose stat reported size exceeds the configured maxBytes limit.
// 6. Bounded streaming read: Reads file content using an io.LimitReader bounded to maxBytes+1,
//    detecting any file that grows beyond the limit after the initial stat.
// 7. Calculates SHA-256 over the exact, unnormalized, unparsed original bytes.
// 8. Verifies that actual byte length matches expectedLength.
// 9. Verifies that computed SHA-256 matches expectedSHA256.
// 10. Returns an immutable VerifiedInput encapsulating the unmutated original bytes.
func (s *Store) LoadAndVerify(objectPath string, expectedLength int64, expectedSHA256 string) (*VerifiedInput, error) {
	if err := corpus.ValidateObjectPath(objectPath); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsafePath, err)
	}

	if expectedLength < 0 {
		return nil, fmt.Errorf("%w: expected byte length must be non-negative, got %d", ErrByteLengthMismatch, expectedLength)
	}

	expectedHex := strings.ToLower(strings.TrimSpace(expectedSHA256))
	if len(expectedHex) != 64 {
		return nil, fmt.Errorf("%w: invalid expected SHA-256 length %d (must be 64 hex characters)", ErrSHA256Mismatch, len(expectedHex))
	}

	file, err := openFileSafe(s.root, objectPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %q", ErrFileNotFound, objectPath)
		}
		return nil, fmt.Errorf("%w: failed to open %q: %v", ErrUnsafePath, objectPath, err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("%w: failed to stat %q: %v", ErrReadFailed, objectPath, err)
	}

	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: object %q mode is %s (only regular files permitted)", ErrNotRegularFile, objectPath, info.Mode().Type())
	}

	// Stage 1 pre-read check: reject if reported size exceeds configured maximum
	if info.Size() > s.maxBytes {
		return nil, fmt.Errorf("%w: object %q size %d bytes exceeds configured limit of %d bytes", ErrFileTooLarge, objectPath, info.Size(), s.maxBytes)
	}

	return readAndVerify(file, info.Size(), s.maxBytes, expectedLength, expectedHex, objectPath)
}

// readAndVerify performs the bounded streaming read, hash computation, and integrity checks.
func readAndVerify(r io.Reader, reportedSize, maxBytes, expectedLength int64, expectedHex, objectPath string) (*VerifiedInput, error) {
	// Stage 2 bounded streaming read: read up to maxBytes + 1 bytes.
	// If reading encounters more than maxBytes, the file grew after the initial stat.
	limit := maxBytes
	limitedReader := io.LimitReader(r, limit+1)
	hasher := sha256.New()
	tee := io.TeeReader(limitedReader, hasher)

	var buf bytes.Buffer
	if reportedSize > 0 && reportedSize <= limit {
		buf.Grow(int(reportedSize))
	}

	chunk := make([]byte, 32*1024)
	var totalRead int64
	for {
		n, readErr := tee.Read(chunk)
		if n > 0 {
			totalRead += int64(n)
			if totalRead > limit {
				return nil, fmt.Errorf("%w: object %q exceeded maximum size limit of %d bytes while reading", ErrFileTooLarge, objectPath, limit)
			}
			buf.Write(chunk[:n])
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return nil, fmt.Errorf("%w: failed to read object %q: %v", ErrReadFailed, objectPath, readErr)
		}
	}

	// Length verification
	if totalRead != expectedLength {
		return nil, fmt.Errorf("%w: object %q actual byte length %d does not match expected %d", ErrByteLengthMismatch, objectPath, totalRead, expectedLength)
	}

	// SHA-256 verification
	computedHex := hex.EncodeToString(hasher.Sum(nil))
	if computedHex != expectedHex {
		return nil, fmt.Errorf("%w: object %q computed SHA-256 %s does not match expected %s", ErrSHA256Mismatch, objectPath, computedHex, expectedHex)
	}

	return &VerifiedInput{
		objectPath: objectPath,
		byteLength: totalRead,
		sha256Hex:  computedHex,
		rawBytes:   buf.Bytes(),
	}, nil
}
