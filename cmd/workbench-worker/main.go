// Command workbench-worker validates corpus manifests, verifies local
// ledger-close metadata inputs, executes the allowlisted Go ingestion adapter,
// evaluates assertions, writes and verifies immutable evidence bundles, and
// generates JSON and Markdown reports.
//
// Usage:
//
//	workbench-worker run --manifest <path> --input-root <dir> --input <relpath> --output-dir <dir> [--json]
//	workbench-worker version
//	workbench-worker help
//
// Diagnostics are written to stderr using structured logging. Machine-readable
// output is written to stdout only when --json is specified.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/adapter"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/source/localfile"
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
	exitInconclusive     = 6
	exitCancelled        = 7
	exitNotApplicable    = 8
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	logger := slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if len(args) < 1 {
		printUsage(stderr)
		return exitUsageError
	}

	switch args[0] {
	case "version":
		fmt.Fprintf(stdout, "workbench-worker %s (sdk %s)\n", version, adapter.PinnedGoSDKVersion)
		return exitOK

	case "run":
		return runCommand(ctx, args[1:], stdout, stderr, logger)

	case "help", "-h", "--help":
		printUsage(stdout)
		return exitOK

	default:
		logger.Error("unknown command", "command", args[0])
		printUsage(stderr)
		return exitUsageError
	}
}

func runCommand(ctx context.Context, args []string, stdout, stderr io.Writer, logger *slog.Logger) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var cfg runConfig
	fs.StringVar(&cfg.ManifestPath, "manifest", "", "path to the local corpus manifest JSON file")
	fs.StringVar(&cfg.InputRoot, "input-root", "", "trusted local corpus-object root directory")
	fs.StringVar(&cfg.InputObject, "input", "", "canonical corpus-relative object path")
	fs.StringVar(&cfg.OutputDir, "output-dir", "", "existing output directory for evidence and reports")
	fs.BoolVar(&cfg.JSONOutput, "json", false, "emit machine-readable JSON output to stdout")
	fs.Int64Var(&cfg.TimeoutMs, "timeout-ms", adapter.DefaultTimeoutMs, "adapter execution timeout in milliseconds")
	fs.Int64Var(&cfg.MaxInputBytes, "max-input-bytes", localfile.DefaultMaxBytes, "maximum allowed input bytes")
	fs.Int64Var(&cfg.MaxOutputBytes, "max-output-bytes", adapter.DefaultMaxOutputBytes, "maximum allowed adapter stdout bytes")

	if err := fs.Parse(args); err != nil {
		if cfg.JSONOutput {
			emitJSONError(stdout, "invalid_flags", "invalid command flags")
		} else {
			fmt.Fprintf(stderr, "error: %v\n", err)
			printUsage(stderr)
		}
		return exitUsageError
	}
	if fs.NArg() > 0 {
		if cfg.JSONOutput {
			emitJSONError(stdout, "unexpected_arguments", "unexpected positional arguments")
		} else {
			fmt.Fprintf(stderr, "error: unexpected positional arguments: %v\n", fs.Args())
			printUsage(stderr)
		}
		return exitUsageError
	}

	jsonReport, exitCode, err := executeRun(ctx, cfg, logger)
	if err != nil {
		var we *workerError
		code := "internal_error"
		msg := "worker execution failed"
		if errors.As(err, &we) {
			code = we.Code
			msg = we.Message
		}
		logger.ErrorContext(ctx, "workbench-worker failed", "code", code, "exit_code", exitCode, "error", err)
		if cfg.JSONOutput {
			emitJSONError(stdout, code, msg)
		} else {
			fmt.Fprintf(stderr, "error (%s): %s\n", code, msg)
		}
		return exitCode
	}

	if cfg.JSONOutput {
		_, _ = stdout.Write(jsonReport)
	}
	return exitCode
}

func emitJSONError(w io.Writer, code, message string) {
	env := jsonErrorEnvelope{
		Status:  "error",
		Code:    code,
		Message: message,
	}
	b, _ := json.Marshal(env)
	_, _ = fmt.Fprintf(w, "%s\n", b)
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `Usage: workbench-worker <command> [flags]

Commands:
  run       Execute a local conformance or regression replay job
  version   Print the worker version
  help      Show this help message

Run flags:
  --manifest          Path to the local corpus manifest JSON file (required)
  --input-root        Trusted local corpus-object root directory (required)
  --input             Canonical corpus-relative object path (required)
  --output-dir        Existing output directory for evidence and reports (required)
  --json              Emit machine-readable JSON output to stdout
  --timeout-ms        Adapter timeout in milliseconds (default: 30000)
  --max-input-bytes   Maximum allowed input size in bytes (default: 67108864)
  --max-output-bytes  Maximum allowed adapter stdout bytes (default: 10485760)

Exit Codes:
  0  Completed run with pass
  1  Invalid command, missing required flags, or usage error
  2  Input or manifest validation error
  3  Adapter unavailable, timeout, process failure, or invalid adapter response
  4  Completed run with regression
  5  Internal, evidence, or report generation failure
  6  Completed run with inconclusive outcome
  7  Cancelled run
  8  Completed run with not_applicable outcome
`)
}
