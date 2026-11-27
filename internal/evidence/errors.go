// Package evidence implements versioned, immutable evidence bundle creation
// and independent read-only verification for Stellar Integrity Workbench runs.
package evidence

import "errors"

var (
	// ErrInvalidBundleInput indicates invalid or inconsistent inputs to WriteBundle.
	ErrInvalidBundleInput = errors.New("invalid evidence bundle input")

	// ErrManifestInvalid indicates that manifest.json failed decoding, schema
	// validation, or structural validation.
	ErrManifestInvalid = errors.New("invalid evidence manifest")

	// ErrUnsupportedSchemaVersion indicates an unsupported evidence manifest schema version.
	ErrUnsupportedSchemaVersion = errors.New("unsupported evidence schema version")

	// ErrBundleExists indicates that a run bundle directory or artifact already exists.
	ErrBundleExists = errors.New("evidence bundle already exists")

	// ErrUnsafePath indicates path traversal, absolute paths, or invalid path syntax.
	ErrUnsafePath = errors.New("unsafe evidence path")

	// ErrArtifactMissing indicates a manifest-referenced artifact does not exist.
	ErrArtifactMissing = errors.New("evidence artifact missing")

	// ErrExtraArtifact indicates an unlisted file or directory inside the bundle.
	ErrExtraArtifact = errors.New("unlisted extra file in evidence bundle")

	// ErrHashMismatch indicates an artifact's recomputed SHA-256 differs from the manifest.
	ErrHashMismatch = errors.New("evidence artifact SHA-256 mismatch")

	// ErrByteLengthMismatch indicates an artifact's actual byte length differs from the manifest.
	ErrByteLengthMismatch = errors.New("evidence artifact byte length mismatch")

	// ErrSymlinkRejected indicates a symbolic link was encountered inside a bundle.
	ErrSymlinkRejected = errors.New("symbolic link rejected in evidence bundle")

	// ErrNotRegularFile indicates an expected artifact or manifest is not a regular file.
	ErrNotRegularFile = errors.New("evidence file is not a regular file")

	// ErrRunIDMismatch indicates manifest.run_id does not match the bundle directory run ID.
	ErrRunIDMismatch = errors.New("evidence run ID mismatch")
)
