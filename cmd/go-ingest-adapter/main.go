// Command go-ingest-adapter implements the versioned adapter process protocol
// for decoding and inspecting Stellar ledger-close metadata using the official
// Stellar Go SDK (v0.9.0).
//
// The adapter reads one JSON request from stdin, validates it against the
// adapter protocol v1 contract, verifies and decodes the staged
// LedgerCloseMeta XDR input, builds a deterministic semantic projection, and
// writes exactly one JSON result to stdout. All diagnostics are written to
// stderr.
//
// This binary is invoked by the workbench-worker through the allowlisted
// adapter registry. It must not be invoked with user-supplied executable
// paths or as an arbitrary shell command.
//
// Usage:
//
//	echo '<request-json>' | go-ingest-adapter
//	go-ingest-adapter version
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/adapter"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/adapter/goingest"
)

// version is set at build time via -ldflags.
var version = adapter.DefaultGoAdapterVersion

// adapterID identifies this adapter in the protocol.
const adapterID = adapter.AdapterIDGoIngest

// maxRequestBytes bounds the size of the JSON request read from stdin (1 MiB).
const maxRequestBytes int64 = 1 * 1024 * 1024

// Exit codes for the adapter process.
const (
	exitOK            = 0
	exitUsageError    = 1
	exitInputError    = 2
	exitDecodeError   = 3
	exitProtocolError = 4
	exitInternalError = 5
)

func main() {
	os.Exit(run())
}

func run() int {
	// Structured logger writing diagnostics to stderr only.
	// Stdout is reserved exclusively for the protocol result envelope.
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Context that is cancelled on SIGINT or SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// When invoked with "version" or "help", print and exit.
	// All other invocations expect the adapter protocol on stdin/stdout.
	if len(os.Args) == 2 {
		switch os.Args[1] {
		case "version":
			fmt.Fprintf(os.Stdout, "go-ingest-adapter %s (protocol %s, sdk %s)\n",
				version, adapter.ProtocolVersion, adapter.PinnedGoSDKVersion)
			return exitOK
		case "help", "-h", "--help":
			printUsage()
			return exitOK
		}
	}

	// Unexpected arguments: reject without touching stdout.
	if len(os.Args) > 1 {
		logger.Error("unexpected arguments; the adapter protocol reads JSON from stdin",
			"args", os.Args[1:])
		printUsage()
		return exitUsageError
	}

	return processRequest(ctx, logger)
}

// processRequest reads a single bounded JSON request from stdin, validates it
// against adapter protocol v1, verifies and decodes the staged LedgerCloseMeta
// input, and writes a single JSON result to stdout.
func processRequest(ctx context.Context, logger *slog.Logger) int {
	startTime := time.Now()
	adapter.Init()

	logger.InfoContext(ctx, "go-ingest-adapter starting",
		"adapter_id", adapterID,
		"version", version,
		"protocol_version", adapter.ProtocolVersion,
		"sdk_version", adapter.PinnedGoSDKVersion,
	)

	// Check for early cancellation.
	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.Canceled) {
			logger.WarnContext(ctx, "adapter cancelled before reading request")
			return exitOK
		}
		logger.ErrorContext(ctx, "unexpected context error", "error", err)
		return exitInternalError
	}

	// Read bounded JSON request from stdin (maxRequestBytes + 1 to detect overflow).
	reqBytes, err := io.ReadAll(io.LimitReader(os.Stdin, maxRequestBytes+1))
	if err != nil {
		logger.ErrorContext(ctx, "failed to read request from stdin", "error", err)
		return exitProtocolError
	}
	if int64(len(reqBytes)) > maxRequestBytes {
		logger.ErrorContext(ctx, "request exceeds maximum allowed stdin size",
			"max_bytes", maxRequestBytes)
		return exitProtocolError
	}

	// Strictly decode JSON request (rejects malformed JSON, unknown fields, trailing data).
	req, err := adapter.DecodeRequest(reqBytes)
	if err != nil {
		logger.ErrorContext(ctx, "failed to decode adapter request", "error", err)
		return exitProtocolError
	}

	// Validate protocol version and typed request fields before schema validation
	// so unsupported versions produce clear diagnostics.
	if err := adapter.ValidateRequest(req); err != nil {
		logger.ErrorContext(ctx, "adapter request validation failed", "error", err)
		return exitProtocolError
	}

	// Validate against embedded JSON Schema (Draft 2020-12).
	if err := adapter.SchemaValidateRequest(reqBytes); err != nil {
		logger.ErrorContext(ctx, "adapter request schema validation failed", "error", err)
		return exitProtocolError
	}

	// Verify target identity matches this binary.
	if req.Target.AdapterID != adapterID ||
		req.Target.AdapterVersion != version ||
		req.Target.SDKVersion != adapter.PinnedGoSDKVersion {
		logger.ErrorContext(ctx, "request target does not match this adapter",
			"requested_id", req.Target.AdapterID,
			"requested_version", req.Target.AdapterVersion,
			"requested_sdk", req.Target.SDKVersion,
			"actual_id", adapterID,
			"actual_version", version,
			"actual_sdk", adapter.PinnedGoSDKVersion,
		)
		return exitProtocolError
	}

	res := executeIngest(req, startTime, logger, ctx)

	outBytes, err := adapter.EncodeResult(res)
	if err != nil {
		logger.ErrorContext(ctx, "failed to encode adapter result", "error", err)
		return exitInternalError
	}

	if _, err := fmt.Fprintf(os.Stdout, "%s\n", outBytes); err != nil {
		logger.ErrorContext(ctx, "failed to write adapter result to stdout", "error", err)
		return exitInternalError
	}

	logger.InfoContext(ctx, "go-ingest-adapter completed",
		"run_id", req.RunID,
		"case_id", req.CaseID,
		"execution_status", res.Execution.Status,
		"decode_status", res.Decode.Status,
		"exit_code", res.Execution.ExitCode,
	)
	return res.Execution.ExitCode
}

func executeIngest(req *adapter.Request, startTime time.Time, logger *slog.Logger, ctx context.Context) *adapter.Result {
	baseResult := func(execStatus string, exitCode int, decodeStatus string, detectedType *string, proj *json.RawMessage, diags []adapter.ResultDiagnostic, caps []string) *adapter.Result {
		durationMs := time.Since(startTime).Milliseconds()
		if durationMs < 0 {
			durationMs = 0
		}
		if diags == nil {
			diags = []adapter.ResultDiagnostic{}
		}
		if caps == nil {
			caps = []string{}
		}
		return &adapter.Result{
			SchemaVersion:  adapter.ProtocolVersion,
			RunID:          req.RunID,
			CaseID:         req.CaseID,
			AdapterID:      adapterID,
			AdapterVersion: version,
			SDKVersion:     adapter.PinnedGoSDKVersion,
			Execution: adapter.ResultExecution{
				Status:     execStatus,
				ExitCode:   exitCode,
				DurationMs: durationMs,
			},
			Input: adapter.ResultInput{
				SHA256: req.InputSHA256,
			},
			Decode: adapter.ResultDecode{
				Status:       decodeStatus,
				DetectedType: detectedType,
			},
			SemanticProjection: proj,
			Diagnostics:        diags,
			CapabilitiesUsed:   caps,
		}
	}

	// Step 1: Load and verify local staged input before any XDR decoding.
	loaded, err := goingest.LoadAndVerifyInput(req.InputPath, req.InputSHA256, req.Limits.MaxInputBytes)
	if err != nil {
		logger.ErrorContext(ctx, "input verification failed", "run_id", req.RunID, "error", err)
		return mapIngestErrorToResult(err, adapter.DecodeStatusNotAttempted, nil, baseResult)
	}

	// Step 2: Decode LedgerCloseMeta using the official Stellar Go SDK.
	decoded, err := goingest.DecodeLedgerCloseMeta(loaded.Data)
	if err != nil {
		logger.ErrorContext(ctx, "LedgerCloseMeta decode failed", "run_id", req.RunID, "error", err)
		return mapIngestErrorToResult(err, adapter.DecodeStatusInvalidInput, nil, baseResult)
	}

	detectedType := adapter.InputFormatLedgerCloseMeta

	// Step 3: Build deterministic semantic projection.
	proj, err := goingest.BuildSemanticProjection(decoded)
	if err != nil {
		logger.ErrorContext(ctx, "semantic projection failed", "run_id", req.RunID, "error", err)
		return mapIngestErrorToResult(err, adapter.DecodeStatusSuccess, &detectedType, baseResult)
	}

	projBytes, err := json.Marshal(proj)
	if err != nil {
		logger.ErrorContext(ctx, "failed to marshal semantic projection", "run_id", req.RunID, "error", err)
		return baseResult(
			adapter.ExecutionStatusFailed,
			exitInternalError,
			adapter.DecodeStatusSuccess,
			&detectedType,
			nil,
			[]adapter.ResultDiagnostic{
				{
					Stage:   "projection",
					Code:    "projection_marshal_failed",
					Message: "failed to serialize semantic projection",
				},
			},
			[]string{"decode_ledger_meta", "xdr_safe_unmarshal"},
		)
	}

	rawProj := json.RawMessage(projBytes)
	return baseResult(
		adapter.ExecutionStatusCompleted,
		exitOK,
		adapter.DecodeStatusSuccess,
		&detectedType,
		&rawProj,
		[]adapter.ResultDiagnostic{},
		[]string{"decode_ledger_meta", "xdr_safe_unmarshal", "semantic_projection_v1"},
	)
}

func mapIngestErrorToResult(
	err error,
	defaultDecodeStatus string,
	detectedType *string,
	build func(execStatus string, exitCode int, decodeStatus string, detectedType *string, proj *json.RawMessage, diags []adapter.ResultDiagnostic, caps []string) *adapter.Result,
) *adapter.Result {
	stage := "decode"
	code := "ingest_failed"
	var ge *goingest.Error
	if errors.As(err, &ge) {
		stage = ge.Stage
		code = ge.Code
	}

	switch {
	case errors.Is(err, goingest.ErrMissingInput):
		return build(adapter.ExecutionStatusFailed, exitInputError, adapter.DecodeStatusNotAttempted, nil, nil,
			[]adapter.ResultDiagnostic{{Stage: stage, Code: code, Message: "staged input file is missing"}}, nil)
	case errors.Is(err, goingest.ErrNotRegularFile):
		return build(adapter.ExecutionStatusFailed, exitInputError, adapter.DecodeStatusNotAttempted, nil, nil,
			[]adapter.ResultDiagnostic{{Stage: stage, Code: code, Message: "staged input is not a regular file"}}, nil)
	case errors.Is(err, goingest.ErrInputTooLarge):
		return build(adapter.ExecutionStatusFailed, exitInputError, adapter.DecodeStatusNotAttempted, nil, nil,
			[]adapter.ResultDiagnostic{{Stage: stage, Code: code, Message: "staged input exceeds max_input_bytes"}}, nil)
	case errors.Is(err, goingest.ErrInputHashMismatch):
		return build(adapter.ExecutionStatusFailed, exitInputError, adapter.DecodeStatusNotAttempted, nil, nil,
			[]adapter.ResultDiagnostic{{Stage: stage, Code: code, Message: "staged input SHA-256 does not match request input_sha256"}}, nil)
	case errors.Is(err, goingest.ErrXDRDecodeFailed):
		return build(adapter.ExecutionStatusFailed, exitDecodeError, adapter.DecodeStatusInvalidInput, nil, nil,
			[]adapter.ResultDiagnostic{{Stage: stage, Code: code, Message: "LedgerCloseMeta XDR decoding failed"}}, nil)
	case errors.Is(err, goingest.ErrUnsupportedXDRVariant):
		return build(adapter.ExecutionStatusFailed, exitDecodeError, adapter.DecodeStatusUnsupportedType, nil, nil,
			[]adapter.ResultDiagnostic{{Stage: stage, Code: code, Message: "unsupported LedgerCloseMeta XDR variant"}}, nil)
	case errors.Is(err, goingest.ErrUnsupportedProtocol):
		return build(adapter.ExecutionStatusFailed, exitDecodeError, adapter.DecodeStatusUnsupportedProtocol, nil, nil,
			[]adapter.ResultDiagnostic{{Stage: stage, Code: code, Message: "unsupported Stellar protocol behaviour"}}, nil)
	case errors.Is(err, goingest.ErrProjectionFailed):
		return build(adapter.ExecutionStatusFailed, exitInternalError, defaultDecodeStatus, detectedType, nil,
			[]adapter.ResultDiagnostic{{Stage: stage, Code: code, Message: "failed to construct semantic projection"}}, nil)
	default:
		return build(adapter.ExecutionStatusFailed, exitInternalError, defaultDecodeStatus, detectedType, nil,
			[]adapter.ResultDiagnostic{{Stage: stage, Code: code, Message: "internal adapter error"}}, nil)
	}
}

func printUsage() {
	fmt.Fprint(os.Stderr, `Usage: go-ingest-adapter [command]

When invoked without arguments, the adapter reads one JSON request from stdin
and writes one JSON result to stdout (adapter process protocol v1).

Commands:
  version   Print the adapter version
  help      Show this help message

Exit Codes:
  0  Success
  1  Usage error (invalid CLI arguments)
  2  Input error
  3  Decode error
  4  Protocol error (malformed or invalid request)
  5  Internal error

The adapter is designed to be invoked by workbench-worker through the
allowlisted adapter registry. Do not invoke it with arbitrary input paths
or as a shell command.
`)
}
