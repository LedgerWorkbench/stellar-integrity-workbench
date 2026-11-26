package adapter

import (
	"errors"
	"fmt"
)

// Sentinel errors for the adapter protocol and process runner.
//
// All errors use fmt.Errorf with %w to preserve cause chains for
// errors.Is and errors.As inspection.
var (
	// ErrInvalidRequest indicates that the adapter request failed validation.
	ErrInvalidRequest = errors.New("invalid adapter request")

	// ErrUnsupportedProtocolVersion indicates that the request or result
	// carries an unsupported schema_version.
	ErrUnsupportedProtocolVersion = errors.New("unsupported adapter protocol version")

	// ErrInvalidResult indicates that the adapter result failed validation.
	ErrInvalidResult = errors.New("invalid adapter result")

	// ErrAdapterUnavailable indicates that the requested adapter is not
	// registered, its binary is missing, or it is not executable.
	ErrAdapterUnavailable = errors.New("adapter unavailable")

	// ErrAdapterIdentityMismatch indicates that the result's adapter_id,
	// adapter_version, or sdk_version does not match the request or registry.
	ErrAdapterIdentityMismatch = errors.New("adapter identity mismatch")

	// ErrInputHashMismatch indicates that the input SHA-256 in the result
	// does not match the original verified input hash.
	ErrInputHashMismatch = errors.New("adapter input hash mismatch")

	// ErrAdapterTimeout indicates that the adapter process exceeded the
	// configured timeout.
	ErrAdapterTimeout = errors.New("adapter process timed out")

	// ErrAdapterCancelled indicates that the adapter invocation was cancelled
	// by the caller (parent context cancellation).
	ErrAdapterCancelled = errors.New("adapter process cancelled")

	// ErrAdapterProcessFailure indicates that the adapter process exited with
	// a non-zero exit code or was terminated by a signal.
	ErrAdapterProcessFailure = errors.New("adapter process failure")

	// ErrAdapterOutputTooLarge indicates that the adapter's stdout output
	// exceeded the configured max_output_bytes limit.
	ErrAdapterOutputTooLarge = errors.New("adapter output too large")

	// ErrAdapterStderrTooLarge indicates that the adapter's stderr output
	// exceeded the configured maximum diagnostic capture limit.
	ErrAdapterStderrTooLarge = errors.New("adapter stderr too large")

	// ErrAdapterMalformedOutput indicates that the adapter's stdout was not
	// valid JSON or contained trailing data.
	ErrAdapterMalformedOutput = errors.New("adapter output malformed")

	// ErrAdapterResponseMismatch indicates that a field in the adapter's result
	// (run_id, case_id, etc.) does not match the original request.
	ErrAdapterResponseMismatch = errors.New("adapter response mismatch")

	// ErrInputTooLarge indicates that the input exceeds the configured
	// max_input_bytes before the adapter is launched.
	ErrInputTooLarge = errors.New("input exceeds maximum size for adapter")
)

// RequestValidationError wraps a validation failure with a descriptive message.
type RequestValidationError struct {
	Field   string
	Message string
	Cause   error
}

func (e *RequestValidationError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: field %q: %s: %v", ErrInvalidRequest, e.Field, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: field %q: %s", ErrInvalidRequest, e.Field, e.Message)
}

func (e *RequestValidationError) Unwrap() error {
	return ErrInvalidRequest
}

// ResultValidationError wraps a result validation failure with a descriptive message.
type ResultValidationError struct {
	Field   string
	Message string
	Cause   error
}

func (e *ResultValidationError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: field %q: %s: %v", ErrInvalidResult, e.Field, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: field %q: %s", ErrInvalidResult, e.Field, e.Message)
}

func (e *ResultValidationError) Unwrap() error {
	return ErrInvalidResult
}
