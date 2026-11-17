// Command go-ingest-adapter implements the versioned adapter process protocol
// for decoding and inspecting Stellar ledger-close metadata using the official
// Stellar Go SDK.
//
// The adapter reads one JSON request from stdin, processes the specified input,
// and writes exactly one JSON result to stdout. All diagnostics are written to
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
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

// version is set at build time via -ldflags.
var version = "dev"

// adapterID identifies this adapter in the protocol.
const adapterID = "go-ingest"

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

	// When invoked with "version", print version and exit.
	// All other invocations expect the adapter protocol on stdin/stdout.
	if len(os.Args) == 2 {
		switch os.Args[1] {
		case "version":
			fmt.Fprintf(os.Stdout, "go-ingest-adapter %s\n", version)
			return exitOK
		case "help", "-h", "--help":
			printUsage()
			return exitOK
		}
	}

	// No arguments or unexpected arguments: run the adapter protocol.
	if len(os.Args) > 1 {
		logger.Error("unexpected arguments; the adapter protocol reads JSON from stdin",
			"args", os.Args[1:])
		printUsage()
		return exitUsageError
	}

	return processRequest(ctx, logger)
}

// processRequest reads a single JSON request from stdin and writes a single
// JSON result to stdout.
//
// The full adapter protocol, XDR decoding using the official Stellar Go SDK
// and semantic projection are implemented in subsequent units (Units 4–5).
// Until then, this function reports that the adapter is not yet operational.
func processRequest(ctx context.Context, logger *slog.Logger) int {
	logger.InfoContext(ctx, "go-ingest-adapter starting",
		"adapter_id", adapterID,
		"version", version,
	)

	// Check for early cancellation.
	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.Canceled) {
			logger.WarnContext(ctx, "adapter cancelled before processing")
			return exitOK
		}
		logger.ErrorContext(ctx, "unexpected context error", "error", err)
		return exitInternalError
	}

	// The adapter protocol (JSON request/response over stdin/stdout), XDR
	// decoding via the official Stellar Go SDK v0.9.0 and semantic projection
	// will be added in Units 4 and 5. This placeholder does not emit a fake
	// result or claim processing succeeded.
	logger.ErrorContext(ctx, "adapter protocol is not yet implemented; "+
		"JSON request parsing, XDR decoding and result emission "+
		"will be added in subsequent implementation units")
	fmt.Fprintln(os.Stderr, "error: adapter protocol is not yet implemented")
	return exitProtocolError
}

func printUsage() {
	fmt.Fprint(os.Stderr, `Usage: go-ingest-adapter [command]

When invoked without arguments, the adapter reads one JSON request from stdin
and writes one JSON result to stdout (adapter process protocol v1).

Commands:
  version   Print the adapter version
  help      Show this help message

The adapter is designed to be invoked by workbench-worker through the
allowlisted adapter registry. Do not invoke it with arbitrary input paths
or as a shell command.
`)
}
