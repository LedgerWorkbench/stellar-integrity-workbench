package evidence

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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
//  1. Validates manifest.json (strict JSON, no trailing data, Draft 2020-12 schema).
//  2. Verifies manifest.run_id matches runID.
//  3. Rejects symlinks, non-regular files, path traversal, and duplicate paths.
//  4. Recomputes exact byte lengths and SHA-256 hashes of all referenced artifacts.
//  5. Detects missing artifacts and any unlisted extra files or directories.
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

	manifestBytes, err := readRegularFileInRoot(runRoot, ManifestFilename)
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
	for _, art := range manifest.Artifacts {
		if err := ValidateArtifactPath(art.Path); err != nil {
			return nil, err
		}
		if _, dup := expectedFiles[art.Path]; dup {
			return nil, fmt.Errorf("%w: duplicate artifact reference %q", ErrManifestInvalid, art.Path)
		}
		expectedFiles[art.Path] = struct{}{}

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

	if err := checkNoExtraFiles(filepath.Join(cleanRoot, runID), expectedFiles); err != nil {
		return nil, err
	}

	return &VerificationResult{
		RunID:             manifest.RunID,
		ManifestSHA256:    manifestSHA256,
		ArtifactCount:     len(verifiedArts),
		VerifiedArtifacts: verifiedArts,
	}, nil
}

func readRegularFileInRoot(runRoot *os.Root, relPath string) ([]byte, error) {
	info, err := runRoot.Lstat(relPath)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%w: %q", ErrSymlinkRejected, relPath)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %q", ErrNotRegularFile, relPath)
	}
	f, err := runRoot.Open(relPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func hashRegularFileInRoot(runRoot *os.Root, relPath string) (string, int64, error) {
	info, err := runRoot.Lstat(relPath)
	if err != nil {
		return "", 0, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", 0, fmt.Errorf("%w: %q", ErrSymlinkRejected, relPath)
	}
	if !info.Mode().IsRegular() {
		return "", 0, fmt.Errorf("%w: %q", ErrNotRegularFile, relPath)
	}
	f, err := runRoot.Open(relPath)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	return HashReader(f)
}

func checkNoExtraFiles(bundleDir string, expectedFiles map[string]struct{}) error {
	topEntries, err := os.ReadDir(bundleDir)
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
			artEntries, err := os.ReadDir(filepath.Join(bundleDir, ArtifactsDirname))
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
