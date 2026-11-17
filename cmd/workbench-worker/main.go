// Command workbench-worker validates corpus manifests, orchestrates adapter
// execution, evaluates assertions and produces evidence bundles and reports.
//
// This is the primary local command for running conformance and regression
// jobs against immutable Stellar ledger-close metadata inputs.
//
// Usage:
//
//	workbench-worker run --manifest <path> --input <path> --output-dir <path>
//	workbench-worker version
//
// Diagnostics are written to stderr using structured logging. Machine-readable
// output is written to stdout only when explicitly requested.
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

// Exit codes used by the worker command.
const (
	exitOK               = 0
	exitUsageError       = 1
	exitInputError       = 2
	exitAdapterError     = 3
	exitAssertionFailure = 4
	exitInternalError    = 5
)

func main() {
	os.Exit(run())
}

func run() int {
	// Structured logger writing diagnostics to stderr only.
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Context that is cancelled on SIGINT or SIGTERM for graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if len(os.Args) < 2 {
		printUsage()
		return exitUsageError
	}

	switch os.Args[1] {
	case "version":
		fmt.Fprintf(os.Stdout, "workbench-worker %s\n", version)
		return exitOK

	case "run":
		return runJob(ctx, logger)

	case "help", "-h", "--help":
		printUsage()
		return exitOK

	default:
		logger.Error("unknown command", "command", os.Args[1])
		printUsage()
		return exitUsageError
	}
}

// runJob is the entry point for a conformance or regression job.
//
// This skeleton validates that the command is reachable and reports that the
// full processing pipeline is not yet implemented. It does not fake a
// successful run.
func runJob(ctx context.Context, logger *slog.Logger) int {
	logger.InfoContext(ctx, "workbench-worker starting", "version", version)

	// Check for early cancellation before proceeding.
	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.Canceled) {
			logger.WarnContext(ctx, "worker cancelled before job started")
			return exitOK
		}
		logger.ErrorContext(ctx, "unexpected context error", "error", err)
		return exitInternalError
	}

	// The manifest validation, input verification, adapter execution,
	// assertion evaluation and evidence generation pipeline is implemented
	// in subsequent units (Units 2–9). This command will report an error
	// until the full pipeline is wired.
	logger.ErrorContext(ctx, "run command is not yet implemented; "+
		"manifest validation, adapter execution and assertion evaluation "+
		"will be added in subsequent implementation units")
	fmt.Fprintln(os.Stderr, "error: run command is not yet implemented")
	return exitUsageError
}

func printUsage() {
	fmt.Fprint(os.Stderr, `Usage: workbench-worker <command> [flags]

Commands:
  run       Execute a conformance or regression job (not yet implemented)
  version   Print the worker version
  help      Show this help message

Run flags (planned):
  --manifest    Path to the corpus case manifest (JSON)
  --input       Path to the immutable ledger-close metadata input
  --output-dir  Directory for evidence bundles and reports
  --json        Emit machine-readable JSON result to stdout
`)
}
