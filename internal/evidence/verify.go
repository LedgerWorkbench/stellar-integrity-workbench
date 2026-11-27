package evidence

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/domain"
)

// VerifiedArtifact records the verified metadata of a single stored artifact.
type VerifiedArtifact struct {
	Path       string
	Role       string
	ByteLength int64
	SHA256     string
}

// VerificationResult summarizes an independent read-only verification of an
// evidence bundle.
type VerificationResult struct {
	RunID             string
	ManifestSHA256    string
	ArtifactCount     int
	VerifiedArtifacts []VerifiedArtifact
}

// VerifyBundle performs an independent, read-only verification of the evidence
// bundle at <evidenceRoot>/<runID>:
//  1. Bounds manifest.json to MaxManifestBytes before JSON decoding.
//  2. Validates manifest.json (strict JSON, no trailing data, Draft 2020-12 schema).
//  3. Verifies manifest.run_id matches runID.
//  4. Rejects symlinks, non-regular files, path traversal, and duplicate paths/roles.
//  5. Recomputes exact byte lengths and SHA-256 hashes of all referenced artifacts.
//  6. Verifies artifacts/assertions.json matches manifest.assertions when present.
//  7. Scans the bundle directory strictly via runRoot.FS() to detect unlisted extra files.
func VerifyBundle(evidenceRoot, runID string) (*VerificationResult, error) {
	if err := ValidateRunID(runID); err != nil {
		return nil, err
	}

	cleanRoot := filepath.Clean(strings.TrimSpace(evidenceRoot))
	if cleanRoot == "" || (cleanRoot == "." && strings.TrimSpace(evidenceRoot) == "") {
		return nil, fmt.Errorf("%w: evidence root directory must not be empty", ErrInvalidBundleInput)
	}

	root, err := os.OpenRoot(cleanRoot)
	if err != nil {
		return nil, fmt.Errorf("open evidence root %q: %w", cleanRoot, err)
	}
	defer root.Close()

	dirInfo, err := root.Lstat(runID)
	if err != nil {
		return nil, fmt.Errorf("stat bundle %q: %w", runID, err)
	}
	if dirInfo.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%w: bundle directory %q is a symlink", ErrSymlinkRejected, runID)
	}
	if !dirInfo.IsDir() {
		return nil, fmt.Errorf("%w: %q is not a directory", ErrNotRegularFile, runID)
	}

	runRoot, err := root.OpenRoot(runID)
	if err != nil {
		return nil, fmt.Errorf("open bundle directory %q: %w", runID, err)
	}
	defer runRoot.Close()

	manifestBytes, err := readBoundedRegularFileInRoot(runRoot, ManifestFilename, MaxManifestBytes)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", ManifestFilename, err)
	}
	manifestSHA256, _ := ComputeSHA256(manifestBytes)

	manifest, err := DecodeAndValidateManifest(manifestBytes)
	if err != nil {
		return nil, err
	}

	if manifest.RunID != runID {
		return nil, fmt.Errorf("%w: manifest run_id %q does not match bundle directory %q",
			ErrRunIDMismatch, manifest.RunID, runID)
	}

	expectedFiles := make(map[string]struct{}, len(manifest.Artifacts)+1)
	expectedFiles[ManifestFilename] = struct{}{}

	verifiedArts := make([]VerifiedArtifact, 0, len(manifest.Artifacts))
	var assertionsArtifactPath string
	for _, art := range manifest.Artifacts {
		if err := ValidateArtifactPath(art.Path); err != nil {
			return nil, err
		}
		if _, dup := expectedFiles[art.Path]; dup {
			return nil, fmt.Errorf("%w: duplicate artifact reference %q", ErrManifestInvalid, art.Path)
		}
		expectedFiles[art.Path] = struct{}{}
		if art.Role == ArtifactRoleAssertions {
			assertionsArtifactPath = art.Path
		}

		actualSHA, actualLen, err := hashRegularFileInRoot(runRoot, art.Path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("%w: %q", ErrArtifactMissing, art.Path)
			}
			return nil, fmt.Errorf("verify artifact %q: %w", art.Path, err)
		}

		if actualLen != art.ByteLength {
			return nil, fmt.Errorf("%w: artifact %q actual byte length %d != manifest %d",
				ErrByteLengthMismatch, art.Path, actualLen, art.ByteLength)
		}
		if actualSHA != art.SHA256 {
			return nil, fmt.Errorf("%w: artifact %q actual SHA-256 %s != manifest %s",
				ErrHashMismatch, art.Path, actualSHA, art.SHA256)
		}

		verifiedArts = append(verifiedArts, VerifiedArtifact{
			Path:       art.Path,
			Role:       art.Role,
			ByteLength: actualLen,
			SHA256:     actualSHA,
		})
	}

	if assertionsArtifactPath != "" {
		if err := verifyAssertionsConsistency(runRoot, assertionsArtifactPath, manifest.Assertions); err != nil {
			return nil, err
		}
	}

	if err := checkNoExtraFilesInRoot(runRoot, expectedFiles); err != nil {
		return nil, err
	}

	return &VerificationResult{
		RunID:             manifest.RunID,
		ManifestSHA256:    manifestSHA256,
		ArtifactCount:     len(verifiedArts),
		VerifiedArtifacts: verifiedArts,
	}, nil
}

func openVerifiedRegularFileInRoot(runRoot *os.Root, relPath string) (*os.File, os.FileInfo, error) {
	linfo, err := runRoot.Lstat(relPath)
	if err != nil {
		return nil, nil, err
	}
	if linfo.Mode()&os.ModeSymlink != 0 {
		return nil, nil, fmt.Errorf("%w: %q", ErrSymlinkRejected, relPath)
	}
	if !linfo.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("%w: %q", ErrNotRegularFile, relPath)
	}
	f, err := runRoot.Open(relPath)
	if err != nil {
		return nil, nil, err
	}
	finfo, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	if !finfo.Mode().IsRegular() {
		_ = f.Close()
		return nil, nil, fmt.Errorf("%w: %q", ErrNotRegularFile, relPath)
	}
	if !os.SameFile(linfo, finfo) {
		_ = f.Close()
		return nil, nil, fmt.Errorf("%w: %q changed between lstat and open", ErrSymlinkRejected, relPath)
	}
	return f, finfo, nil
}

func readBoundedRegularFileInRoot(runRoot *os.Root, relPath string, maxBytes int64) ([]byte, error) {
	f, finfo, err := openVerifiedRegularFileInRoot(runRoot, relPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if finfo.Size() > maxBytes {
		return nil, fmt.Errorf("%w: %q size %d bytes exceeds maximum %d bytes",
			ErrManifestInvalid, relPath, finfo.Size(), maxBytes)
	}

	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("%w: %q exceeds maximum %d bytes", ErrManifestInvalid, relPath, maxBytes)
	}
	return data, nil
}

func hashRegularFileInRoot(runRoot *os.Root, relPath string) (string, int64, error) {
	f, _, err := openVerifiedRegularFileInRoot(runRoot, relPath)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	return HashReader(f)
}

func verifyAssertionsConsistency(runRoot *os.Root, relPath string, manifestAssertions []domain.AssertionResult) error {
	data, err := readBoundedRegularFileInRoot(runRoot, relPath, MaxManifestBytes)
	if err != nil {
		return fmt.Errorf("read assertions artifact %q: %w", relPath, err)
	}
	var artifactAssertions []domain.AssertionResult
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	if err := dec.Decode(&artifactAssertions); err != nil {
		return fmt.Errorf("%w: malformed assertions artifact %q: %v", ErrManifestInvalid, relPath, err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("%w: trailing JSON in assertions artifact %q", ErrManifestInvalid, relPath)
	}

	if !reflect.DeepEqual(artifactAssertions, manifestAssertions) {
		return fmt.Errorf("%w: assertions artifact %q disagrees with manifest.assertions", ErrManifestInvalid, relPath)
	}
	return nil
}

func checkNoExtraFilesInRoot(runRoot *os.Root, expectedFiles map[string]struct{}) error {
	rootFS := runRoot.FS()
	topEntries, err := fs.ReadDir(rootFS, ".")
	if err != nil {
		return fmt.Errorf("read bundle directory: %w", err)
	}

	for _, entry := range topEntries {
		name := entry.Name()
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: unexpected symlink %q in bundle root", ErrSymlinkRejected, name)
		}
		if name == ManifestFilename {
			continue
		}
		if name == ArtifactsDirname {
			if !entry.IsDir() {
				return fmt.Errorf("%w: %q must be a directory", ErrNotRegularFile, ArtifactsDirname)
			}
			artEntries, err := fs.ReadDir(rootFS, ArtifactsDirname)
			if err != nil {
				return fmt.Errorf("read artifacts directory: %w", err)
			}
			for _, ae := range artEntries {
				rel := ArtifactsDirname + "/" + ae.Name()
				if ae.Type()&os.ModeSymlink != 0 {
					return fmt.Errorf("%w: unexpected symlink %q", ErrSymlinkRejected, rel)
				}
				if ae.IsDir() {
					return fmt.Errorf("%w: unexpected subdirectory %q", ErrExtraArtifact, rel)
				}
				if _, ok := expectedFiles[rel]; !ok {
					return fmt.Errorf("%w: unlisted file %q", ErrExtraArtifact, rel)
				}
			}
			continue
		}
		return fmt.Errorf("%w: unlisted entry %q in bundle root", ErrExtraArtifact, name)
	}
	return nil
}
