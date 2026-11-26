package adapter

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// ProtocolVersion is the current adapter protocol version string.
const ProtocolVersion = "1"

// Supported operations.
const (
	OperationDecodeLedgerMeta = "decode_ledger_meta"
)

// Supported input formats.
const (
	InputFormatLedgerCloseMeta = "LedgerCloseMeta"
)

// ExecutionStatus values for the result execution block.
const (
	ExecutionStatusCompleted = "completed"
	ExecutionStatusFailed    = "failed"
	ExecutionStatusCancelled = "cancelled"
)

// DecodeStatus values for the result decode block.
const (
	DecodeStatusNotAttempted        = "not_attempted"
	DecodeStatusSuccess             = "success"
	DecodeStatusInvalidInput        = "invalid_input"
	DecodeStatusUnsupportedType     = "unsupported_type"
	DecodeStatusUnsupportedProtocol = "unsupported_protocol"
)

// Limits for numeric bounds.
const (
	MaxTimeoutMs     = 3600000    // 1 hour
	MaxInputBytes    = 1073741824 // 1 GiB
	MaxOutputBytes   = 1073741824 // 1 GiB
	MaxIDLength      = 256
	MaxVersionLength = 128
)

// sha256Pattern validates lowercase hex SHA-256 hashes.
var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Request represents a versioned adapter process protocol request.
// It is the typed Go model for the JSON contract delivered to adapter stdin.
type Request struct {
	SchemaVersion string        `json:"schema_version"`
	RunID         string        `json:"run_id"`
	CaseID        string        `json:"case_id"`
	Operation     string        `json:"operation"`
	InputPath     string        `json:"input_path"`
	InputFormat   string        `json:"input_format"`
	InputSHA256   string        `json:"input_sha256"`
	Target        RequestTarget `json:"target"`
	Limits        RequestLimits `json:"limits"`
}

// RequestTarget identifies the adapter and SDK versions.
type RequestTarget struct {
	AdapterID      string `json:"adapter_id"`
	AdapterVersion string `json:"adapter_version"`
	SDKVersion     string `json:"sdk_version"`
}

// RequestLimits defines execution resource bounds.
type RequestLimits struct {
	TimeoutMs      int64 `json:"timeout_ms"`
	MaxInputBytes  int64 `json:"max_input_bytes"`
	MaxOutputBytes int64 `json:"max_output_bytes"`
}

// Result represents a versioned adapter process protocol result.
// It is the typed Go model for the JSON contract written to adapter stdout.
type Result struct {
	SchemaVersion      string             `json:"schema_version"`
	RunID              string             `json:"run_id"`
	CaseID             string             `json:"case_id"`
	AdapterID          string             `json:"adapter_id"`
	AdapterVersion     string             `json:"adapter_version"`
	SDKVersion         string             `json:"sdk_version"`
	Execution          ResultExecution    `json:"execution"`
	Input              ResultInput        `json:"input"`
	Decode             ResultDecode       `json:"decode"`
	SemanticProjection *json.RawMessage   `json:"semantic_projection"`
	Diagnostics        []ResultDiagnostic `json:"diagnostics"`
	CapabilitiesUsed   []string           `json:"capabilities_used"`
}

// ResultExecution records process-level execution outcome.
type ResultExecution struct {
	Status     string `json:"status"`
	ExitCode   int    `json:"exit_code"`
	DurationMs int64  `json:"duration_ms"`
}

// ResultInput echoes the input verification data.
type ResultInput struct {
	SHA256 string `json:"sha256"`
}

// ResultDecode records the decoding outcome.
type ResultDecode struct {
	Status       string  `json:"status"`
	DetectedType *string `json:"detected_type"`
}

// ResultDiagnostic is a structured diagnostic entry.
type ResultDiagnostic struct {
	Stage   string `json:"stage"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ValidateRequest performs comprehensive validation of an adapter request.
func ValidateRequest(req *Request) error {
	if req == nil {
		return fmt.Errorf("%w: request is nil", ErrInvalidRequest)
	}

	// Protocol version
	if req.SchemaVersion != ProtocolVersion {
		return fmt.Errorf("%w: schema_version %q is not supported (expected %q)",
			ErrUnsupportedProtocolVersion, req.SchemaVersion, ProtocolVersion)
	}

	// Run ID
	if strings.TrimSpace(req.RunID) == "" {
		return &RequestValidationError{Field: "run_id", Message: "must not be empty"}
	}
	if len(req.RunID) > MaxIDLength {
		return &RequestValidationError{Field: "run_id", Message: fmt.Sprintf("exceeds maximum length %d", MaxIDLength)}
	}

	// Case ID
	if strings.TrimSpace(req.CaseID) == "" {
		return &RequestValidationError{Field: "case_id", Message: "must not be empty"}
	}
	if len(req.CaseID) > MaxIDLength {
		return &RequestValidationError{Field: "case_id", Message: fmt.Sprintf("exceeds maximum length %d", MaxIDLength)}
	}

	// Operation
	if req.Operation != OperationDecodeLedgerMeta {
		return &RequestValidationError{Field: "operation",
			Message: fmt.Sprintf("unsupported operation %q (supported: %q)", req.Operation, OperationDecodeLedgerMeta)}
	}

	// Input path
	if strings.TrimSpace(req.InputPath) == "" {
		return &RequestValidationError{Field: "input_path", Message: "must not be empty"}
	}

	// Input format
	if req.InputFormat != InputFormatLedgerCloseMeta {
		return &RequestValidationError{Field: "input_format",
			Message: fmt.Sprintf("unsupported input format %q (supported: %q)", req.InputFormat, InputFormatLedgerCloseMeta)}
	}

	// Input SHA-256
	if !sha256Pattern.MatchString(req.InputSHA256) {
		return &RequestValidationError{Field: "input_sha256",
			Message: "must be exactly 64 lowercase hex characters"}
	}

	// Target
	if err := validateRequestTarget(&req.Target); err != nil {
		return err
	}

	// Limits
	if err := validateRequestLimits(&req.Limits); err != nil {
		return err
	}

	return nil
}

func validateRequestTarget(t *RequestTarget) error {
	if strings.TrimSpace(t.AdapterID) == "" {
		return &RequestValidationError{Field: "target.adapter_id", Message: "must not be empty"}
	}
	if len(t.AdapterID) > MaxVersionLength {
		return &RequestValidationError{Field: "target.adapter_id",
			Message: fmt.Sprintf("exceeds maximum length %d", MaxVersionLength)}
	}
	if strings.TrimSpace(t.AdapterVersion) == "" {
		return &RequestValidationError{Field: "target.adapter_version", Message: "must not be empty"}
	}
	if len(t.AdapterVersion) > MaxVersionLength {
		return &RequestValidationError{Field: "target.adapter_version",
			Message: fmt.Sprintf("exceeds maximum length %d", MaxVersionLength)}
	}
	if strings.TrimSpace(t.SDKVersion) == "" {
		return &RequestValidationError{Field: "target.sdk_version", Message: "must not be empty"}
	}
	if len(t.SDKVersion) > MaxVersionLength {
		return &RequestValidationError{Field: "target.sdk_version",
			Message: fmt.Sprintf("exceeds maximum length %d", MaxVersionLength)}
	}
	return nil
}

func validateRequestLimits(l *RequestLimits) error {
	if l.TimeoutMs < 1 {
		return &RequestValidationError{Field: "limits.timeout_ms", Message: "must be positive"}
	}
	if l.TimeoutMs > MaxTimeoutMs {
		return &RequestValidationError{Field: "limits.timeout_ms",
			Message: fmt.Sprintf("exceeds maximum %d", MaxTimeoutMs)}
	}
	if l.MaxInputBytes < 1 {
		return &RequestValidationError{Field: "limits.max_input_bytes", Message: "must be positive"}
	}
	if l.MaxInputBytes > MaxInputBytes {
		return &RequestValidationError{Field: "limits.max_input_bytes",
			Message: fmt.Sprintf("exceeds maximum %d", MaxInputBytes)}
	}
	if l.MaxOutputBytes < 1 {
		return &RequestValidationError{Field: "limits.max_output_bytes", Message: "must be positive"}
	}
	if l.MaxOutputBytes > MaxOutputBytes {
		return &RequestValidationError{Field: "limits.max_output_bytes",
			Message: fmt.Sprintf("exceeds maximum %d", MaxOutputBytes)}
	}
	return nil
}

// ValidateResult performs comprehensive validation of an adapter result.
func ValidateResult(res *Result) error {
	if res == nil {
		return fmt.Errorf("%w: result is nil", ErrInvalidResult)
	}

	// Protocol version
	if res.SchemaVersion != ProtocolVersion {
		return fmt.Errorf("%w: schema_version %q is not supported (expected %q)",
			ErrUnsupportedProtocolVersion, res.SchemaVersion, ProtocolVersion)
	}

	// Run ID
	if strings.TrimSpace(res.RunID) == "" {
		return &ResultValidationError{Field: "run_id", Message: "must not be empty"}
	}

	// Case ID
	if strings.TrimSpace(res.CaseID) == "" {
		return &ResultValidationError{Field: "case_id", Message: "must not be empty"}
	}

	// Adapter identity
	if strings.TrimSpace(res.AdapterID) == "" {
		return &ResultValidationError{Field: "adapter_id", Message: "must not be empty"}
	}
	if strings.TrimSpace(res.AdapterVersion) == "" {
		return &ResultValidationError{Field: "adapter_version", Message: "must not be empty"}
	}
	if strings.TrimSpace(res.SDKVersion) == "" {
		return &ResultValidationError{Field: "sdk_version", Message: "must not be empty"}
	}

	// Execution
	if err := validateExecutionStatus(res.Execution.Status); err != nil {
		return err
	}
	if res.Execution.ExitCode < 0 || res.Execution.ExitCode > 255 {
		return &ResultValidationError{Field: "execution.exit_code",
			Message: fmt.Sprintf("must be between 0 and 255, got %d", res.Execution.ExitCode)}
	}
	if res.Execution.DurationMs < 0 {
		return &ResultValidationError{Field: "execution.duration_ms", Message: "must be non-negative"}
	}

	// Input SHA-256
	if !sha256Pattern.MatchString(res.Input.SHA256) {
		return &ResultValidationError{Field: "input.sha256",
			Message: "must be exactly 64 lowercase hex characters"}
	}

	// Decode status
	if err := validateDecodeStatus(res.Decode.Status); err != nil {
		return err
	}

	// Diagnostics
	for i, d := range res.Diagnostics {
		if strings.TrimSpace(d.Stage) == "" {
			return &ResultValidationError{
				Field:   fmt.Sprintf("diagnostics[%d].stage", i),
				Message: "must not be empty",
			}
		}
		if strings.TrimSpace(d.Code) == "" {
			return &ResultValidationError{
				Field:   fmt.Sprintf("diagnostics[%d].code", i),
				Message: "must not be empty",
			}
		}
	}

	return nil
}

func validateExecutionStatus(status string) error {
	switch status {
	case ExecutionStatusCompleted, ExecutionStatusFailed, ExecutionStatusCancelled:
		return nil
	default:
		return &ResultValidationError{Field: "execution.status",
			Message: fmt.Sprintf("unsupported execution status %q", status)}
	}
}

func validateDecodeStatus(status string) error {
	switch status {
	case DecodeStatusNotAttempted, DecodeStatusSuccess, DecodeStatusInvalidInput,
		DecodeStatusUnsupportedType, DecodeStatusUnsupportedProtocol:
		return nil
	default:
		return &ResultValidationError{Field: "decode.status",
			Message: fmt.Sprintf("unsupported decode status %q", status)}
	}
}

// ValidateResultAgainstRequest checks that the result is consistent with the
// original request: matching run_id, case_id, adapter identity, and input hash.
func ValidateResultAgainstRequest(res *Result, req *Request) error {
	if res.RunID != req.RunID {
		return fmt.Errorf("%w: run_id %q does not match request %q",
			ErrAdapterResponseMismatch, res.RunID, req.RunID)
	}
	if res.CaseID != req.CaseID {
		return fmt.Errorf("%w: case_id %q does not match request %q",
			ErrAdapterResponseMismatch, res.CaseID, req.CaseID)
	}
	if res.AdapterID != req.Target.AdapterID {
		return fmt.Errorf("%w: adapter_id %q does not match request target %q",
			ErrAdapterIdentityMismatch, res.AdapterID, req.Target.AdapterID)
	}
	if res.AdapterVersion != req.Target.AdapterVersion {
		return fmt.Errorf("%w: adapter_version %q does not match request target %q",
			ErrAdapterIdentityMismatch, res.AdapterVersion, req.Target.AdapterVersion)
	}
	if res.SDKVersion != req.Target.SDKVersion {
		return fmt.Errorf("%w: sdk_version %q does not match request target %q",
			ErrAdapterIdentityMismatch, res.SDKVersion, req.Target.SDKVersion)
	}
	if res.Input.SHA256 != req.InputSHA256 {
		return fmt.Errorf("%w: input sha256 %q does not match request %q",
			ErrInputHashMismatch, res.Input.SHA256, req.InputSHA256)
	}
	return nil
}

// EncodeRequest marshals a request to deterministic JSON.
func EncodeRequest(req *Request) ([]byte, error) {
	return json.Marshal(req)
}

// DecodeRequest strictly decodes a JSON request, rejecting unknown fields
// and any trailing JSON values or non-whitespace bytes.
func DecodeRequest(data []byte) (*Request, error) {
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()

	var req Request
	if err := dec.Decode(&req); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}

	if err := ensureDecoderEOF(dec, ErrInvalidRequest); err != nil {
		return nil, err
	}

	return &req, nil
}

// EncodeResult marshals a result to deterministic JSON.
func EncodeResult(res *Result) ([]byte, error) {
	return json.Marshal(res)
}

// DecodeResult strictly decodes a JSON result, rejecting unknown fields
// and any trailing JSON values or non-whitespace bytes.
func DecodeResult(data []byte) (*Result, error) {
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()

	var res Result
	if err := dec.Decode(&res); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidResult, err)
	}

	if err := ensureDecoderEOF(dec, ErrInvalidResult); err != nil {
		return nil, err
	}

	return &res, nil
}

// ensureDecoderEOF verifies that no additional JSON values or non-whitespace
// trailing bytes remain in the decoder stream.
func ensureDecoderEOF(dec *json.Decoder, sentinel error) error {
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("%w: trailing JSON value after document", sentinel)
		}
		return fmt.Errorf("%w: trailing data after JSON document: %v", sentinel, err)
	}
	return nil
}
