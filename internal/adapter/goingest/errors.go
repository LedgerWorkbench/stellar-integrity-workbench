// Package goingest implements Stellar LedgerCloseMeta XDR decoding,
// local input integrity verification, and deterministic semantic projection
// using the pinned official Stellar Go SDK (v0.9.0).
package goingest

import (
	"errors"
	"fmt"
)

// Sentinel errors for go-ingest decoding and projection failure categories.
var (
	// ErrMissingInput indicates that the staged input file does not exist.
	ErrMissingInput = errors.New("missing input file")

	// ErrNotRegularFile indicates that the input path is a symlink, directory,
	// FIFO, device, or other non-regular file.
	ErrNotRegularFile = errors.New("input is not a regular file")

	// ErrInputTooLarge indicates that the input exceeds limits.max_input_bytes.
	ErrInputTooLarge = errors.New("input exceeds maximum allowed size")

	// ErrInputHashMismatch indicates that the computed SHA-256 of the input
	// bytes does not match the expected request input_sha256.
	ErrInputHashMismatch = errors.New("input SHA-256 mismatch")

	// ErrXDRDecodeFailed indicates that XDR unmarshaling failed (empty,
	// malformed, truncated, or trailing bytes).
	ErrXDRDecodeFailed = errors.New("XDR decode failed")

	// ErrUnsupportedXDRVariant indicates an unsupported LedgerCloseMeta outer
	// variant or unsupported inner XDR union variant.
	ErrUnsupportedXDRVariant = errors.New("unsupported XDR variant")

	// ErrUnsupportedProtocol indicates established unsupported protocol behaviour.
	ErrUnsupportedProtocol = errors.New("unsupported protocol behaviour")

	// ErrProjectionFailed indicates that building the semantic projection failed.
	ErrProjectionFailed = errors.New("semantic projection failed")

	// ErrInternal indicates an unexpected internal adapter error.
	ErrInternal = errors.New("internal adapter error")
)

// Error is a structured error carrying a failure category sentinel,
// machine-readable diagnostic code, processing stage, and wrapped cause.
type Error struct {
	Sentinel error
	Stage    string
	Code     string
	Message  string
	Cause    error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s (%s): %s: %v", e.Sentinel, e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s (%s): %s", e.Sentinel, e.Code, e.Message)
}

// Unwrap returns both the category sentinel and the underlying cause so
// callers can inspect either via errors.Is / errors.As.
func (e *Error) Unwrap() []error {
	if e.Cause != nil {
		return []error{e.Sentinel, e.Cause}
	}
	return []error{e.Sentinel}
}

func newError(sentinel error, stage, code, message string, cause error) *Error {
	return &Error{
		Sentinel: sentinel,
		Stage:    stage,
		Code:     code,
		Message:  message,
		Cause:    cause,
	}
}
