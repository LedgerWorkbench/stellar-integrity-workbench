package adapter

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func init() {
	Init()
}

// --- Helper functions ---

func validRequest() *Request {
	return &Request{
		SchemaVersion: ProtocolVersion,
		RunID:         "run_test_001",
		CaseID:        "case_test_001",
		Operation:     OperationDecodeLedgerMeta,
		InputPath:     "/tmp/workbench/run_test_001/input.xdr",
		InputFormat:   InputFormatLedgerCloseMeta,
		InputSHA256:   "a" + strings.Repeat("0", 63),
		Target: RequestTarget{
			AdapterID:      "go-ingest",
			AdapterVersion: "dev",
			SDKVersion:     "v0.9.0",
		},
		Limits: RequestLimits{
			TimeoutMs:      30000,
			MaxInputBytes:  67108864,
			MaxOutputBytes: 10485760,
		},
	}
}

func validResult() *Result {
	return &Result{
		SchemaVersion:  ProtocolVersion,
		RunID:          "run_test_001",
		CaseID:         "case_test_001",
		AdapterID:      "go-ingest",
		AdapterVersion: "dev",
		SDKVersion:     "v0.9.0",
		Execution: ResultExecution{
			Status:     ExecutionStatusCompleted,
			ExitCode:   0,
			DurationMs: 15,
		},
		Input: ResultInput{
			SHA256: "a" + strings.Repeat("0", 63),
		},
		Decode: ResultDecode{
			Status:       DecodeStatusNotAttempted,
			DetectedType: nil,
		},
		SemanticProjection: nullJSON(),
		Diagnostics:        []ResultDiagnostic{},
		CapabilitiesUsed:   []string{},
	}
}

func nullJSON() *json.RawMessage {
	raw := json.RawMessage("null")
	return &raw
}

// --- Request Validation Tests ---

func TestValidateRequest_Valid(t *testing.T) {
	req := validRequest()
	if err := ValidateRequest(req); err != nil {
		t.Fatalf("expected valid request, got error: %v", err)
	}
}

func TestValidateRequest_Nil(t *testing.T) {
	err := ValidateRequest(nil)
	if err == nil {
		t.Fatal("expected error for nil request")
	}
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got: %v", err)
	}
}

func TestValidateRequest_UnsupportedVersion(t *testing.T) {
	req := validRequest()
	req.SchemaVersion = "2"
	err := ValidateRequest(req)
	if err == nil {
		t.Fatal("expected error for unsupported version")
	}
	if !errors.Is(err, ErrUnsupportedProtocolVersion) {
		t.Fatalf("expected ErrUnsupportedProtocolVersion, got: %v", err)
	}
}

func TestValidateRequest_EmptyRunID(t *testing.T) {
	req := validRequest()
	req.RunID = ""
	err := ValidateRequest(req)
	if err == nil {
		t.Fatal("expected error for empty run_id")
	}
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got: %v", err)
	}
}

func TestValidateRequest_EmptyCaseID(t *testing.T) {
	req := validRequest()
	req.CaseID = "   "
	err := ValidateRequest(req)
	if err == nil {
		t.Fatal("expected error for whitespace-only case_id")
	}
}

func TestValidateRequest_UnsupportedOperation(t *testing.T) {
	req := validRequest()
	req.Operation = "execute_arbitrary"
	err := ValidateRequest(req)
	if err == nil {
		t.Fatal("expected error for unsupported operation")
	}
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got: %v", err)
	}
}

func TestValidateRequest_InvalidSHA256(t *testing.T) {
	tests := []struct {
		name string
		sha  string
	}{
		{"too short", "abcdef"},
		{"too long", strings.Repeat("a", 65)},
		{"uppercase", strings.Repeat("A", 64)},
		{"special chars", strings.Repeat("g", 64)},
		{"empty", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validRequest()
			req.InputSHA256 = tt.sha
			err := ValidateRequest(req)
			if err == nil {
				t.Fatalf("expected error for SHA-256 %q", tt.sha)
			}
		})
	}
}

func TestValidateRequest_InvalidLimits(t *testing.T) {
	tests := []struct {
		name   string
		modify func(r *Request)
	}{
		{"zero timeout", func(r *Request) { r.Limits.TimeoutMs = 0 }},
		{"negative timeout", func(r *Request) { r.Limits.TimeoutMs = -1 }},
		{"excessive timeout", func(r *Request) { r.Limits.TimeoutMs = MaxTimeoutMs + 1 }},
		{"zero max_input", func(r *Request) { r.Limits.MaxInputBytes = 0 }},
		{"excessive max_input", func(r *Request) { r.Limits.MaxInputBytes = MaxInputBytes + 1 }},
		{"zero max_output", func(r *Request) { r.Limits.MaxOutputBytes = 0 }},
		{"excessive max_output", func(r *Request) { r.Limits.MaxOutputBytes = MaxOutputBytes + 1 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validRequest()
			tt.modify(req)
			err := ValidateRequest(req)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("expected ErrInvalidRequest, got: %v", err)
			}
		})
	}
}

func TestValidateRequest_EmptyAdapterID(t *testing.T) {
	req := validRequest()
	req.Target.AdapterID = ""
	err := ValidateRequest(req)
	if err == nil {
		t.Fatal("expected error for empty adapter_id")
	}
}

func TestValidateRequest_EmptyAdapterVersion(t *testing.T) {
	req := validRequest()
	req.Target.AdapterVersion = ""
	err := ValidateRequest(req)
	if err == nil {
		t.Fatal("expected error for empty adapter_version")
	}
}

func TestValidateRequest_EmptySDKVersion(t *testing.T) {
	req := validRequest()
	req.Target.SDKVersion = ""
	err := ValidateRequest(req)
	if err == nil {
		t.Fatal("expected error for empty sdk_version")
	}
}

func TestValidateRequest_InvalidInputFormat(t *testing.T) {
	req := validRequest()
	req.InputFormat = "ScVal"
	err := ValidateRequest(req)
	if err == nil {
		t.Fatal("expected error for invalid input_format")
	}
}

func TestValidateRequest_EmptyInputPath(t *testing.T) {
	req := validRequest()
	req.InputPath = ""
	err := ValidateRequest(req)
	if err == nil {
		t.Fatal("expected error for empty input_path")
	}
}

// --- Result Validation Tests ---

func TestValidateResult_Valid(t *testing.T) {
	res := validResult()
	if err := ValidateResult(res); err != nil {
		t.Fatalf("expected valid result, got error: %v", err)
	}
}

func TestValidateResult_Nil(t *testing.T) {
	err := ValidateResult(nil)
	if err == nil {
		t.Fatal("expected error for nil result")
	}
	if !errors.Is(err, ErrInvalidResult) {
		t.Fatalf("expected ErrInvalidResult, got: %v", err)
	}
}

func TestValidateResult_UnsupportedVersion(t *testing.T) {
	res := validResult()
	res.SchemaVersion = "99"
	err := ValidateResult(res)
	if err == nil {
		t.Fatal("expected error for unsupported version")
	}
	if !errors.Is(err, ErrUnsupportedProtocolVersion) {
		t.Fatalf("expected ErrUnsupportedProtocolVersion, got: %v", err)
	}
}

func TestValidateResult_EmptyRunID(t *testing.T) {
	res := validResult()
	res.RunID = ""
	err := ValidateResult(res)
	if err == nil {
		t.Fatal("expected error for empty run_id")
	}
}

func TestValidateResult_EmptyAdapterID(t *testing.T) {
	res := validResult()
	res.AdapterID = ""
	err := ValidateResult(res)
	if err == nil {
		t.Fatal("expected error for empty adapter_id")
	}
}

func TestValidateResult_InvalidExecutionStatus(t *testing.T) {
	res := validResult()
	res.Execution.Status = "running"
	err := ValidateResult(res)
	if err == nil {
		t.Fatal("expected error for invalid execution status")
	}
}

func TestValidateResult_InvalidDecodeStatus(t *testing.T) {
	res := validResult()
	res.Decode.Status = "magic"
	err := ValidateResult(res)
	if err == nil {
		t.Fatal("expected error for invalid decode status")
	}
}

func TestValidateResult_InvalidInputHash(t *testing.T) {
	res := validResult()
	res.Input.SHA256 = "invalid"
	err := ValidateResult(res)
	if err == nil {
		t.Fatal("expected error for invalid input hash")
	}
}

func TestValidateResult_InvalidExitCode(t *testing.T) {
	res := validResult()
	res.Execution.ExitCode = 300
	err := ValidateResult(res)
	if err == nil {
		t.Fatal("expected error for exit code > 255")
	}
}

func TestValidateResult_NegativeDuration(t *testing.T) {
	res := validResult()
	res.Execution.DurationMs = -1
	err := ValidateResult(res)
	if err == nil {
		t.Fatal("expected error for negative duration")
	}
}

func TestValidateResult_EmptyDiagnosticStage(t *testing.T) {
	res := validResult()
	res.Diagnostics = []ResultDiagnostic{
		{Stage: "", Code: "test", Message: "test"},
	}
	err := ValidateResult(res)
	if err == nil {
		t.Fatal("expected error for empty diagnostic stage")
	}
}

func TestValidateResult_EmptyDiagnosticCode(t *testing.T) {
	res := validResult()
	res.Diagnostics = []ResultDiagnostic{
		{Stage: "test", Code: "", Message: "test"},
	}
	err := ValidateResult(res)
	if err == nil {
		t.Fatal("expected error for empty diagnostic code")
	}
}

// --- Cross-Validation Tests ---

func TestValidateResultAgainstRequest_Valid(t *testing.T) {
	req := validRequest()
	res := validResult()
	if err := ValidateResultAgainstRequest(res, req); err != nil {
		t.Fatalf("expected valid cross-validation, got error: %v", err)
	}
}

func TestValidateResultAgainstRequest_MismatchedRunID(t *testing.T) {
	req := validRequest()
	res := validResult()
	res.RunID = "run_other"
	err := ValidateResultAgainstRequest(res, req)
	if err == nil {
		t.Fatal("expected error for mismatched run_id")
	}
	if !errors.Is(err, ErrAdapterResponseMismatch) {
		t.Fatalf("expected ErrAdapterResponseMismatch, got: %v", err)
	}
}

func TestValidateResultAgainstRequest_MismatchedCaseID(t *testing.T) {
	req := validRequest()
	res := validResult()
	res.CaseID = "case_other"
	err := ValidateResultAgainstRequest(res, req)
	if err == nil {
		t.Fatal("expected error for mismatched case_id")
	}
	if !errors.Is(err, ErrAdapterResponseMismatch) {
		t.Fatalf("expected ErrAdapterResponseMismatch, got: %v", err)
	}
}

func TestValidateResultAgainstRequest_MismatchedAdapterID(t *testing.T) {
	req := validRequest()
	res := validResult()
	res.AdapterID = "rust-xdr"
	err := ValidateResultAgainstRequest(res, req)
	if err == nil {
		t.Fatal("expected error for mismatched adapter_id")
	}
	if !errors.Is(err, ErrAdapterIdentityMismatch) {
		t.Fatalf("expected ErrAdapterIdentityMismatch, got: %v", err)
	}
}

func TestValidateResultAgainstRequest_MismatchedAdapterVersion(t *testing.T) {
	req := validRequest()
	res := validResult()
	res.AdapterVersion = "v2.0.0"
	err := ValidateResultAgainstRequest(res, req)
	if err == nil {
		t.Fatal("expected error for mismatched adapter_version")
	}
	if !errors.Is(err, ErrAdapterIdentityMismatch) {
		t.Fatalf("expected ErrAdapterIdentityMismatch, got: %v", err)
	}
}

func TestValidateResultAgainstRequest_MismatchedSDKVersion(t *testing.T) {
	req := validRequest()
	res := validResult()
	res.SDKVersion = "v1.0.0"
	err := ValidateResultAgainstRequest(res, req)
	if err == nil {
		t.Fatal("expected error for mismatched sdk_version")
	}
	if !errors.Is(err, ErrAdapterIdentityMismatch) {
		t.Fatalf("expected ErrAdapterIdentityMismatch, got: %v", err)
	}
}

func TestValidateResultAgainstRequest_MismatchedInputHash(t *testing.T) {
	req := validRequest()
	res := validResult()
	res.Input.SHA256 = strings.Repeat("b", 64)
	err := ValidateResultAgainstRequest(res, req)
	if err == nil {
		t.Fatal("expected error for mismatched input hash")
	}
	if !errors.Is(err, ErrInputHashMismatch) {
		t.Fatalf("expected ErrInputHashMismatch, got: %v", err)
	}
}

// --- JSON Encoding/Decoding Tests ---

func TestEncodeDecodeRequest_Roundtrip(t *testing.T) {
	req := validRequest()
	data, err := EncodeRequest(req)
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	decoded, err := DecodeRequest(data)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if decoded.RunID != req.RunID {
		t.Fatalf("run_id mismatch: got %q, want %q", decoded.RunID, req.RunID)
	}
	if decoded.SchemaVersion != req.SchemaVersion {
		t.Fatalf("schema_version mismatch: got %q, want %q", decoded.SchemaVersion, req.SchemaVersion)
	}
	if decoded.Target.AdapterID != req.Target.AdapterID {
		t.Fatalf("target.adapter_id mismatch")
	}
}

func TestEncodeDecodeResult_Roundtrip(t *testing.T) {
	res := validResult()
	data, err := EncodeResult(res)
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	decoded, err := DecodeResult(data)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if decoded.RunID != res.RunID {
		t.Fatalf("run_id mismatch: got %q, want %q", decoded.RunID, res.RunID)
	}
	if decoded.Execution.Status != res.Execution.Status {
		t.Fatalf("execution.status mismatch")
	}
}

func TestDecodeRequest_MalformedJSON(t *testing.T) {
	_, err := DecodeRequest([]byte(`{invalid json`))
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got: %v", err)
	}
}

func TestDecodeRequest_UnknownField(t *testing.T) {
	data := []byte(`{"schema_version":"1","run_id":"r","case_id":"c","operation":"decode_ledger_meta","input_path":"/tmp/x","input_format":"LedgerCloseMeta","input_sha256":"` +
		strings.Repeat("a", 64) + `","target":{"adapter_id":"go-ingest","adapter_version":"dev","sdk_version":"v0.9.0"},"limits":{"timeout_ms":30000,"max_input_bytes":67108864,"max_output_bytes":10485760},"unknown_field":"value"}`)
	_, err := DecodeRequest(data)
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got: %v", err)
	}
}

func TestDecodeRequest_TrailingJSON(t *testing.T) {
	req := validRequest()
	first, _ := EncodeRequest(req)
	data := append(first, []byte(`{"extra": true}`)...)
	_, err := DecodeRequest(data)
	if err == nil {
		t.Fatal("expected error for trailing JSON document")
	}
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got: %v", err)
	}
}

func TestDecodeResult_MalformedJSON(t *testing.T) {
	_, err := DecodeResult([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !errors.Is(err, ErrInvalidResult) {
		t.Fatalf("expected ErrInvalidResult, got: %v", err)
	}
}

func TestDecodeResult_UnknownField(t *testing.T) {
	res := validResult()
	data, _ := EncodeResult(res)
	// Inject unknown field
	modified := strings.Replace(string(data), `"schema_version"`, `"bogus":"x","schema_version"`, 1)
	_, err := DecodeResult([]byte(modified))
	if err == nil {
		t.Fatal("expected error for unknown field in result")
	}
	if !errors.Is(err, ErrInvalidResult) {
		t.Fatalf("expected ErrInvalidResult, got: %v", err)
	}
}

func TestDecodeResult_TrailingJSON(t *testing.T) {
	res := validResult()
	first, _ := EncodeResult(res)
	data := append(first, []byte(`{"extra": true}`)...)
	_, err := DecodeResult(data)
	if err == nil {
		t.Fatal("expected error for trailing JSON document")
	}
	if !errors.Is(err, ErrInvalidResult) {
		t.Fatalf("expected ErrInvalidResult, got: %v", err)
	}
}

// --- Schema Validation Tests ---

func TestSchemaValidateRequest_ValidJSON(t *testing.T) {
	req := validRequest()
	data, _ := EncodeRequest(req)
	if err := SchemaValidateRequest(data); err != nil {
		t.Fatalf("expected schema validation to pass, got: %v", err)
	}
}

func TestSchemaValidateRequest_MissingRequiredField(t *testing.T) {
	// Missing "operation" field
	data := []byte(`{"schema_version":"1","run_id":"r","case_id":"c","input_path":"/tmp/x","input_format":"LedgerCloseMeta","input_sha256":"` +
		strings.Repeat("a", 64) + `","target":{"adapter_id":"go-ingest","adapter_version":"dev","sdk_version":"v0.9.0"},"limits":{"timeout_ms":30000,"max_input_bytes":67108864,"max_output_bytes":10485760}}`)
	err := SchemaValidateRequest(data)
	if err == nil {
		t.Fatal("expected schema validation error for missing field")
	}
}

func TestSchemaValidateRequest_UnsupportedVersion(t *testing.T) {
	req := validRequest()
	req.SchemaVersion = "99"
	data, _ := EncodeRequest(req)
	err := SchemaValidateRequest(data)
	if err == nil {
		t.Fatal("expected schema validation error for unsupported version")
	}
}

func TestSchemaValidateRequest_InvalidSHA256(t *testing.T) {
	req := validRequest()
	req.InputSHA256 = "ZZZZ" + strings.Repeat("0", 60)
	data, _ := json.Marshal(req) // bypass Go validation to test schema
	err := SchemaValidateRequest(data)
	if err == nil {
		t.Fatal("expected schema validation error for invalid SHA-256")
	}
}

func TestSchemaValidateResult_ValidJSON(t *testing.T) {
	res := validResult()
	data, _ := EncodeResult(res)
	if err := SchemaValidateResult(data); err != nil {
		t.Fatalf("expected schema validation to pass, got: %v", err)
	}
}

func TestSchemaValidateResult_MissingRequiredField(t *testing.T) {
	// Missing "execution" field
	data := []byte(`{"schema_version":"1","run_id":"r","case_id":"c","adapter_id":"go-ingest","adapter_version":"dev","sdk_version":"v0.9.0","input":{"sha256":"` +
		strings.Repeat("a", 64) + `"},"decode":{"status":"not_attempted","detected_type":null},"semantic_projection":null,"diagnostics":[],"capabilities_used":[]}`)
	err := SchemaValidateResult(data)
	if err == nil {
		t.Fatal("expected schema validation error for missing field")
	}
}

func TestSchemaValidateResult_InvalidExecutionStatus(t *testing.T) {
	res := validResult()
	res.Execution.Status = "magic"
	data, _ := json.Marshal(res)
	err := SchemaValidateResult(data)
	if err == nil {
		t.Fatal("expected schema validation error for invalid execution status")
	}
}

func TestSchemaCompilation(t *testing.T) {
	t.Run("request schema compiles", func(t *testing.T) {
		sch, err := CompileRequestSchema()
		if err != nil {
			t.Fatalf("request schema compilation failed: %v", err)
		}
		if sch == nil {
			t.Fatal("compiled request schema is nil")
		}
	})

	t.Run("result schema compiles", func(t *testing.T) {
		sch, err := CompileResultSchema()
		if err != nil {
			t.Fatalf("result schema compilation failed: %v", err)
		}
		if sch == nil {
			t.Fatal("compiled result schema is nil")
		}
	})
}

// --- Error Type Tests ---

func TestRequestValidationError_Unwrap(t *testing.T) {
	err := &RequestValidationError{Field: "test", Message: "bad"}
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatal("RequestValidationError should unwrap to ErrInvalidRequest")
	}
	if err.Error() == "" {
		t.Fatal("error string should not be empty")
	}
}

func TestResultValidationError_Unwrap(t *testing.T) {
	err := &ResultValidationError{Field: "test", Message: "bad"}
	if !errors.Is(err, ErrInvalidResult) {
		t.Fatal("ResultValidationError should unwrap to ErrInvalidResult")
	}
}

func TestRequestValidationError_WithCause(t *testing.T) {
	cause := errors.New("underlying cause")
	err := &RequestValidationError{Field: "test", Message: "bad", Cause: cause}
	if err.Error() == "" {
		t.Fatal("error string should not be empty")
	}
	if !strings.Contains(err.Error(), "underlying cause") {
		t.Fatal("error string should contain the cause")
	}
}

func TestResultValidationError_WithCause(t *testing.T) {
	cause := errors.New("underlying cause")
	err := &ResultValidationError{Field: "test", Message: "bad", Cause: cause}
	if err.Error() == "" {
		t.Fatal("error string should not be empty")
	}
	if !strings.Contains(err.Error(), "underlying cause") {
		t.Fatal("error string should contain the cause")
	}
}

// --- Execution/Decode Status Consistency Tests ---

func TestValidateResult_AllExecutionStatuses(t *testing.T) {
	for _, status := range []string{ExecutionStatusCompleted, ExecutionStatusFailed, ExecutionStatusCancelled} {
		t.Run(status, func(t *testing.T) {
			res := validResult()
			res.Execution.Status = status
			if err := ValidateResult(res); err != nil {
				t.Fatalf("valid execution status %q rejected: %v", status, err)
			}
		})
	}
}

func TestValidateResult_AllDecodeStatuses(t *testing.T) {
	for _, status := range []string{
		DecodeStatusNotAttempted, DecodeStatusSuccess, DecodeStatusInvalidInput,
		DecodeStatusUnsupportedType, DecodeStatusUnsupportedProtocol,
	} {
		t.Run(status, func(t *testing.T) {
			res := validResult()
			res.Decode.Status = status
			if err := ValidateResult(res); err != nil {
				t.Fatalf("valid decode status %q rejected: %v", status, err)
			}
		})
	}
}

// --- Schema validation for unknown fields ---

func TestSchemaValidateRequest_UnknownField(t *testing.T) {
	req := validRequest()
	data, _ := EncodeRequest(req)
	modified := strings.Replace(string(data), `"schema_version"`, `"unknown_extra":"x","schema_version"`, 1)
	err := SchemaValidateRequest([]byte(modified))
	if err == nil {
		t.Fatal("expected schema validation error for unknown field")
	}
}

func TestSchemaValidateResult_UnknownField(t *testing.T) {
	res := validResult()
	data, _ := EncodeResult(res)
	modified := strings.Replace(string(data), `"schema_version"`, `"unknown_extra":"x","schema_version"`, 1)
	err := SchemaValidateResult([]byte(modified))
	if err == nil {
		t.Fatal("expected schema validation error for unknown field")
	}
}

// --- Single-Document Trailing Data Regression Tests ---

func TestDecodeRequest_SingleDocumentEnforcement(t *testing.T) {
	base, err := EncodeRequest(validRequest())
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	t.Run("accepts trailing whitespace", func(t *testing.T) {
		data := append(append([]byte{}, base...), []byte(" \n\t\r ")...)
		decoded, err := DecodeRequest(data)
		if err != nil {
			t.Fatalf("expected trailing whitespace to be accepted, got: %v", err)
		}
		if decoded.RunID != "run_test_001" {
			t.Fatalf("unexpected run_id: %q", decoded.RunID)
		}
	})

	rejectCases := []struct {
		name     string
		trailing string
	}{
		{"second JSON object", `{"extra":true}`},
		{"arbitrary trailing text", `trailing garbage`},
		{"extra closing brace", `}`},
		{"extra closing bracket", `]`},
	}

	for _, tc := range rejectCases {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			data := append(append([]byte{}, base...), []byte(tc.trailing)...)
			_, err := DecodeRequest(data)
			if err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
			if !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("expected ErrInvalidRequest, got: %v", err)
			}
		})
	}
}

func TestDecodeResult_SingleDocumentEnforcement(t *testing.T) {
	base, err := EncodeResult(validResult())
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	t.Run("accepts trailing whitespace", func(t *testing.T) {
		data := append(append([]byte{}, base...), []byte(" \n\t\r ")...)
		decoded, err := DecodeResult(data)
		if err != nil {
			t.Fatalf("expected trailing whitespace to be accepted, got: %v", err)
		}
		if decoded.RunID != "run_test_001" {
			t.Fatalf("unexpected run_id: %q", decoded.RunID)
		}
	})

	rejectCases := []struct {
		name     string
		trailing string
	}{
		{"second JSON object", `{"extra":true}`},
		{"arbitrary trailing text", `trailing garbage`},
		{"extra closing brace", `}`},
		{"extra closing bracket", `]`},
	}

	for _, tc := range rejectCases {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			data := append(append([]byte{}, base...), []byte(tc.trailing)...)
			_, err := DecodeResult(data)
			if err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
			if !errors.Is(err, ErrInvalidResult) {
				t.Fatalf("expected ErrInvalidResult, got: %v", err)
			}
		})
	}
}

func TestSchemaValidateRequest_SingleDocumentEnforcement(t *testing.T) {
	base, err := EncodeRequest(validRequest())
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	t.Run("accepts trailing whitespace", func(t *testing.T) {
		data := append(append([]byte{}, base...), []byte(" \n\t\r ")...)
		if err := SchemaValidateRequest(data); err != nil {
			t.Fatalf("expected trailing whitespace to be accepted, got: %v", err)
		}
	})

	for _, tc := range []struct {
		name     string
		trailing string
	}{
		{"second JSON object", `{"extra":true}`},
		{"arbitrary trailing text", `trailing garbage`},
		{"extra closing brace", `}`},
		{"extra closing bracket", `]`},
	} {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			data := append(append([]byte{}, base...), []byte(tc.trailing)...)
			if err := SchemaValidateRequest(data); err == nil {
				t.Fatalf("expected schema validation error for %s", tc.name)
			}
		})
	}
}

func TestSchemaValidateResult_SingleDocumentEnforcement(t *testing.T) {
	base, err := EncodeResult(validResult())
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	t.Run("accepts trailing whitespace", func(t *testing.T) {
		data := append(append([]byte{}, base...), []byte(" \n\t\r ")...)
		if err := SchemaValidateResult(data); err != nil {
			t.Fatalf("expected trailing whitespace to be accepted, got: %v", err)
		}
	})

	for _, tc := range []struct {
		name     string
		trailing string
	}{
		{"second JSON object", `{"extra":true}`},
		{"arbitrary trailing text", `trailing garbage`},
		{"extra closing brace", `}`},
		{"extra closing bracket", `]`},
	} {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			data := append(append([]byte{}, base...), []byte(tc.trailing)...)
			if err := SchemaValidateResult(data); err == nil {
				t.Fatalf("expected schema validation error for %s", tc.name)
			}
		})
	}
}
