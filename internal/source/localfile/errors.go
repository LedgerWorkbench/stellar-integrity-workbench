package localfile

import (
	"errors"
)

var (
	// ErrUnsafePath indicates that the requested object path is invalid, absolute,
	// contains directory traversal segments, or attempts to escape the storage root.
	ErrUnsafePath = errors.New("unsafe object path")

	// ErrFileNotFound indicates that the requested object does not exist within the storage root.
	ErrFileNotFound = errors.New("corpus object not found")

	// ErrNotRegularFile indicates that the target path is not a regular file (e.g. directory, FIFO, device, socket).
	ErrNotRegularFile = errors.New("corpus object is not a regular file")

	// ErrFileTooLarge indicates that the object exceeds the configured maximum byte limit.
	ErrFileTooLarge = errors.New("corpus object exceeds maximum size limit")

	// ErrByteLengthMismatch indicates that the actual read byte length differs from the expected length.
	ErrByteLengthMismatch = errors.New("corpus object byte length mismatch")

	// ErrSHA256Mismatch indicates that the computed SHA-256 digest differs from the expected hash.
	ErrSHA256Mismatch = errors.New("corpus object SHA-256 hash mismatch")

	// ErrReadFailed indicates an unexpected I/O error occurred while reading the object bytes.
	ErrReadFailed = errors.New("failed to read corpus object")

	// ErrInvalidConfig indicates that the provided storage configuration is invalid.
	ErrInvalidConfig = errors.New("invalid storage configuration")
)
