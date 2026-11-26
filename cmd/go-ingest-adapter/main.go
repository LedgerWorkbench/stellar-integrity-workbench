// Command go-ingest-adapter implements the versioned adapter process protocol
// for decoding and inspecting Stellar ledger-close metadata using the official
// Stellar Go SDK.
//
// The adapter reads one JSON request from stdin, validates it against the
// adapter protocol v1 contract, processes the specified operation, and writes
// exactly one JSON result to stdout. All diagnostics are written to stderr.
//
// In Unit 4, the adapter implements the full protocol v1 request/response
// contract. Actual Stellar XDR decoding and semantic projection are
// implemented in Unit 5; until then, valid decode_ledger_meta requests
// receive a truthful non-success result with decode.status = "not_attempted".
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
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/adapter"
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
// against adapter protocol v1, and writes a single JSON result to stdout.
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

	// Unit 5 implements actual Stellar XDR decoding. Until Unit 5 begins,
	// return an explicit, truthful non-success result marking decode as
	// not_attempted with a structured diagnostic.
	durationMs := time.Since(startTime).Milliseconds()
	if durationMs < 0 {
		durationMs = 0
	}

	res := &adapter.Result{
		SchemaVersion:  adapter.ProtocolVersion,
		RunID:          req.RunID,
		CaseID:         req.CaseID,
		AdapterID:      adapterID,
		AdapterVersion: version,
		SDKVersion:     adapter.PinnedGoSDKVersion,
		Execution: adapter.ResultExecution{
			Status:     adapter.ExecutionStatusFailed,
			ExitCode:   exitDecodeError,
			DurationMs: durationMs,
		},
		Input: adapter.ResultInput{
			SHA256: req.InputSHA256,
		},
		Decode: adapter.ResultDecode{
			Status:       adapter.DecodeStatusNotAttempted,
			DetectedType: nil,
		},
		SemanticProjection: nil,
		Diagnostics: []adapter.ResultDiagnostic{
			{
				Stage:   "decode",
				Code:    "decoder_not_implemented",
				Message: "Stellar XDR decoding for decode_ledger_meta is not yet implemented; pending Unit 5",
			},
		},
		CapabilitiesUsed: []string{},
	}

	outBytes, err := adapter.EncodeResult(res)
	if err != nil {
		logger.ErrorContext(ctx, "failed to encode adapter result", "error", err)
		return exitInternalError
	}

	if _, err := fmt.Fprintf(os.Stdout, "%s\n", outBytes); err != nil {
		logger.ErrorContext(ctx, "failed to write adapter result to stdout", "error", err)
		return exitInternalError
	}

	logger.InfoContext(ctx, "go-ingest-adapter completed protocol response (decoder pending Unit 5)",
		"run_id", req.RunID,
		"case_id", req.CaseID,
		"exit_code", exitDecodeError,
	)
	return exitDecodeError
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
  3  Decode error / decoder not implemented
  4  Protocol error (malformed or invalid request)
  5  Internal error

The adapter is designed to be invoked by workbench-worker through the
allowlisted adapter registry. Do not invoke it with arbitrary input paths
or as a shell command.
`)
}
