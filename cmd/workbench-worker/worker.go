package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/adapter"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/adapter/goingest"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/assertion"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/corpus"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/domain"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/evidence"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/reporting"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/source/localfile"
)

type runConfig struct {
	ManifestPath   string
	InputRoot      string
	InputObject    string
	OutputDir      string
	JSONOutput     bool
	TimeoutMs      int64
	MaxInputBytes  int64
	MaxOutputBytes int64

	// adapterBinaryPath is an unexported override used only by in-process unit
	// tests; CLI invocations always resolve go-ingest-adapter from os.Executable().
	adapterBinaryPath string
}

type workerError struct {
	ExitCode int
	Code     string
	Message  string
	Cause    error
}

func (e *workerError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *workerError) Unwrap() error {
	return e.Cause
}

type jsonErrorEnvelope struct {
	Status  string `json:"status"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func executeRun(ctx context.Context, cfg runConfig, logger *slog.Logger) ([]byte, int, error) {
	if err := validateRunConfig(&cfg); err != nil {
		return nil, exitUsageError, err
	}

	if err := ctx.Err(); err != nil {
		return nil, exitCancelled, &workerError{
			ExitCode: exitCancelled,
			Code:     "run_cancelled",
			Message:  "run cancelled before execution started",
			Cause:    err,
		}
	}

	runID, err := generateRunID()
	if err != nil {
		return nil, exitInternalError, &workerError{
			ExitCode: exitInternalError,
			Code:     "run_id_generation_failed",
			Message:  "failed to generate run ID",
			Cause:    err,
		}
	}

	startedAt := time.Now().UTC()
	run := &domain.Run{
		RunID:     runID,
		AdapterID: adapter.AdapterIDGoIngest,
		State:     domain.RunStateQueued,
		StartedAt: startedAt,
	}

	// Verify output-dir exists and is a directory before doing work.
	outInfo, err := os.Stat(cfg.OutputDir)
	if err != nil {
		return nil, exitInputError, &workerError{
			ExitCode: exitInputError,
			Code:     "output_dir_unavailable",
			Message:  "output directory does not exist or cannot be accessed",
			Cause:    err,
		}
	}
	if !outInfo.IsDir() {
		return nil, exitInputError, &workerError{
			ExitCode: exitInputError,
			Code:     "output_dir_not_directory",
			Message:  "output-dir path must be a directory",
		}
	}

	// Step 2: Load and validate corpus manifest.
	run.State = domain.RunStateFetching
	manifestBytes, err := readBoundedRegularFile(cfg.ManifestPath, evidence.MaxManifestBytes)
	if err != nil {
		return nil, exitInputError, &workerError{
			ExitCode: exitInputError,
			Code:     "manifest_load_failed",
			Message:  "failed to load bounded corpus manifest",
			Cause:    err,
		}
	}

	domainCase, _, err := corpus.ValidateManifestBytes(manifestBytes)
	if err != nil {
		return nil, exitInputError, &workerError{
			ExitCode: exitInputError,
			Code:     "manifest_validation_failed",
			Message:  "corpus manifest validation failed",
			Cause:    err,
		}
	}
	run.CaseID = domainCase.CaseID

	if err := corpus.ValidateObjectPath(cfg.InputObject); err != nil {
		return nil, exitInputError, &workerError{
			ExitCode: exitInputError,
			Code:     "invalid_input_path",
			Message:  "input object path is invalid",
			Cause:    err,
		}
	}
	if cfg.InputObject != domainCase.Input.ObjectPath {
		return nil, exitInputError, &workerError{
			ExitCode: exitInputError,
			Code:     "input_path_mismatch",
			Message:  "supplied --input does not match manifest input.object_path",
		}
	}
	if domainCase.Input.Format != adapter.InputFormatLedgerCloseMeta {
		return nil, exitAdapterError, &workerError{
			ExitCode: exitAdapterError,
			Code:     "unsupported_input_format",
			Message:  fmt.Sprintf("unsupported input format %q", domainCase.Input.Format),
		}
	}

	// Step 3: Verify original input via root-confined localfile.Store.
	run.State = domain.RunStateVerifyingInput
	store, err := localfile.NewStore(localfile.Config{
		RootDir:  cfg.InputRoot,
		MaxBytes: cfg.MaxInputBytes,
	})
	if err != nil {
		return nil, exitInputError, &workerError{
			ExitCode: exitInputError,
			Code:     "input_store_init_failed",
			Message:  "failed to initialize local corpus input store",
			Cause:    err,
		}
	}
	defer store.Close()

	verifiedInput, err := store.LoadAndVerifyInput(domainCase.Input)
	if err != nil {
		return nil, exitInputError, &workerError{
			ExitCode: exitInputError,
			Code:     "input_verification_failed",
			Message:  "corpus input integrity verification failed",
			Cause:    err,
		}
	}

	// Step 4: Execute allowlisted go-ingest adapter via ProcessRunner.
	run.State = domain.RunStateDecoding
	adapter.Init()
	adapterBin := cfg.adapterBinaryPath
	if adapterBin == "" {
		adapterBin, err = resolveCoLocatedAdapterBinary()
		if err != nil {
			return nil, exitAdapterError, &workerError{
				ExitCode: exitAdapterError,
				Code:     "adapter_unavailable",
				Message:  "failed to locate allowlisted go-ingest-adapter binary",
				Cause:    err,
			}
		}
	}

	reg, err := adapter.NewProductionRegistry(adapter.ProductionRegistryConfig{
		GoIngestBinaryPath: adapterBin,
		GoIngestVersion:    adapter.DefaultGoAdapterVersion,
		GoSDKVersion:       adapter.PinnedGoSDKVersion,
	})
	if err != nil {
		return nil, exitAdapterError, &workerError{
			ExitCode: exitAdapterError,
			Code:     "adapter_registry_failed",
			Message:  "failed to initialize production adapter registry",
			Cause:    err,
		}
	}

	runner, err := adapter.NewProcessRunner(adapter.ProcessRunnerConfig{
		Registry: reg,
	})
	if err != nil {
		return nil, exitAdapterError, &workerError{
			ExitCode: exitAdapterError,
			Code:     "adapter_runner_failed",
			Message:  "failed to initialize adapter process runner",
			Cause:    err,
		}
	}

	target := adapter.RequestTarget{
		AdapterID:      adapter.AdapterIDGoIngest,
		AdapterVersion: adapter.DefaultGoAdapterVersion,
		SDKVersion:     adapter.PinnedGoSDKVersion,
	}

	run.State = domain.RunStateProcessing
	adapterOut, runErr := runner.Run(ctx, adapter.RunParams{
		RunID:       runID,
		CaseID:      domainCase.CaseID,
		Operation:   adapter.OperationDecodeLedgerMeta,
		InputFormat: domainCase.Input.Format,
		Target:      target,
		Limits: adapter.RequestLimits{
			TimeoutMs:      cfg.TimeoutMs,
			MaxInputBytes:  cfg.MaxInputBytes,
			MaxOutputBytes: cfg.MaxOutputBytes,
		},
	}, verifiedInput)
	if runErr != nil {
		exitCode := exitAdapterError
		errCode := "adapter_execution_failed"
		if errors.Is(runErr, adapter.ErrAdapterCancelled) || errors.Is(ctx.Err(), context.Canceled) {
			exitCode = exitCancelled
			errCode = "run_cancelled"
			run.State = domain.RunStateCancelled
		} else {
			run.State = domain.RunStateFailed
		}
		completedAt := time.Now().UTC()
		run.CompletedAt = &completedAt
		_, _ = persistBundleAndReports(cfg.OutputDir, evidence.BuildInput{
			Case:          domainCase,
			Run:           run,
			VerifiedInput: verifiedInput,
			AdapterTarget: target,
			Worker:        workerBuildMetadata(),
		})
		return nil, exitCode, &workerError{
			ExitCode: exitCode,
			Code:     errCode,
			Message:  "adapter execution did not complete",
			Cause:    runErr,
		}
	}

	if adapterOut.Result.Execution.Status != adapter.ExecutionStatusCompleted || adapterOut.ActualExitCode != 0 {
		completedAt := time.Now().UTC()
		run.State = domain.RunStateFailed
		run.CompletedAt = &completedAt
		_, _ = persistBundleAndReports(cfg.OutputDir, evidence.BuildInput{
			Case:          domainCase,
			Run:           run,
			VerifiedInput: verifiedInput,
			AdapterTarget: target,
			AdapterOutput: adapterOut,
			Worker:        workerBuildMetadata(),
		})
		return nil, exitAdapterError, &workerError{
			ExitCode: exitAdapterError,
			Code:     "adapter_decode_failed",
			Message:  fmt.Sprintf("adapter reported execution status %q (exit code %d)", adapterOut.Result.Execution.Status, adapterOut.ActualExitCode),
		}
	}

	// Step 5: Strictly decode semantic projection when non-null.
	var domainProj *domain.SemanticProjection
	if adapterOut.Result.SemanticProjection != nil {
		var gp goingest.SemanticProjection
		dec := json.NewDecoder(bytes.NewReader(*adapterOut.Result.SemanticProjection))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&gp); err != nil {
			return nil, exitAdapterError, &workerError{
				ExitCode: exitAdapterError,
				Code:     "invalid_semantic_projection",
				Message:  "failed to decode adapter semantic_projection",
				Cause:    err,
			}
		}
		var extra any
		if err := dec.Decode(&extra); err != io.EOF {
			return nil, exitAdapterError, &workerError{
				ExitCode: exitAdapterError,
				Code:     "invalid_semantic_projection",
				Message:  "trailing data in adapter semantic_projection",
			}
		}
		domainProj, err = assertion.ProjectionFromGoIngest(&gp)
		if err != nil {
			return nil, exitAdapterError, &workerError{
				ExitCode: exitAdapterError,
				Code:     "invalid_semantic_projection",
				Message:  "adapter semantic_projection failed domain mapping",
				Cause:    err,
			}
		}
	}

	// Step 6: Evaluate assertions using Unit 6.
	run.State = domain.RunStateAsserting
	evidenceRefs := []string{
		"logical://input/sha256/" + verifiedInput.SHA256(),
		"logical://adapter/go-ingest/semantic-projection/v1",
	}
	eval, err := assertion.Evaluate(domainCase.Expectations, assertion.EvaluationInput{
		Projection:          domainProj,
		VerifiedInputSHA256: verifiedInput.SHA256(),
		SupportedKinds:      assertion.GoIngestSupportedKinds(),
		EvidenceRefs:        evidenceRefs,
	})
	if err != nil {
		return nil, exitInternalError, &workerError{
			ExitCode: exitInternalError,
			Code:     "assertion_evaluation_failed",
			Message:  "assertion evaluation failed",
			Cause:    err,
		}
	}

	// Step 7 & 8 & 9: Complete run, write/verify evidence bundle, render/write reports.
	run.State = domain.RunStateReporting
	completedAt := time.Now().UTC()
	if completedAt.Before(startedAt) {
		completedAt = startedAt
	}
	outcome := eval.Outcome
	run.State = domain.RunStateCompleted
	run.Outcome = &outcome
	run.CompletedAt = &completedAt

	jsonReport, err := persistBundleAndReports(cfg.OutputDir, evidence.BuildInput{
		Case:          domainCase,
		Run:           run,
		VerifiedInput: verifiedInput,
		AdapterTarget: target,
		AdapterOutput: adapterOut,
		Assertions:    eval.Results,
		Worker:        workerBuildMetadata(),
	})
	if err != nil {
		return nil, exitInternalError, &workerError{
			ExitCode: exitInternalError,
			Code:     "evidence_or_report_failed",
			Message:  "failed to persist verified evidence bundle and reports",
			Cause:    err,
		}
	}

	exitCode := outcomeToExitCode(outcome)
	logger.InfoContext(ctx, "workbench-worker completed",
		"run_id", runID,
		"case_id", domainCase.CaseID,
		"state", run.State,
		"outcome", outcome,
		"exit_code", exitCode,
	)
	return jsonReport, exitCode, nil
}

func persistBundleAndReports(outputDir string, input evidence.BuildInput) ([]byte, error) {
	writeRes, err := evidence.WriteBundle(outputDir, input)
	if err != nil {
		return nil, err
	}

	jsonBytes, err := reporting.RenderJSON(writeRes.Manifest)
	if err != nil {
		return nil, fmt.Errorf("render JSON report: %w", err)
	}
	mdBytes, err := reporting.RenderMarkdown(writeRes.Manifest)
	if err != nil {
		return nil, fmt.Errorf("render Markdown report: %w", err)
	}

	outRoot, err := os.OpenRoot(filepath.Clean(outputDir))
	if err != nil {
		return nil, fmt.Errorf("open output directory: %w", err)
	}
	defer outRoot.Close()

	jsonName := writeRes.Manifest.RunID + ".json"
	mdName := writeRes.Manifest.RunID + ".md"

	if err := writeExclusiveReport(outRoot, jsonName, jsonBytes); err != nil {
		return nil, err
	}
	if err := writeExclusiveReport(outRoot, mdName, mdBytes); err != nil {
		_ = outRoot.Remove(jsonName)
		return nil, err
	}

	return jsonBytes, nil
}

func writeExclusiveReport(outRoot *os.Root, name string, data []byte) error {
	f, err := outRoot.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o400)
	if err != nil {
		return fmt.Errorf("create report %q: %w", name, err)
	}
	written := 0
	for written < len(data) {
		n, wErr := f.Write(data[written:])
		written += n
		if wErr != nil {
			_ = f.Close()
			_ = outRoot.Remove(name)
			return fmt.Errorf("write report %q: %w", name, wErr)
		}
		if n == 0 {
			_ = f.Close()
			_ = outRoot.Remove(name)
			return fmt.Errorf("write report %q: %w", name, io.ErrShortWrite)
		}
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = outRoot.Remove(name)
		return fmt.Errorf("sync report %q: %w", name, err)
	}
	if err := f.Close(); err != nil {
		_ = outRoot.Remove(name)
		return fmt.Errorf("close report %q: %w", name, err)
	}
	return nil
}

func outcomeToExitCode(outcome domain.RunOutcome) int {
	switch outcome {
	case domain.RunOutcomePass:
		return exitOK
	case domain.RunOutcomeRegression:
		return exitAssertionFailure
	case domain.RunOutcomeInconclusive:
		return exitInconclusive
	case domain.RunOutcomeNotApplicable:
		return exitNotApplicable
	default:
		return exitInternalError
	}
}

func validateRunConfig(cfg *runConfig) error {
	if strings.TrimSpace(cfg.ManifestPath) == "" {
		return &workerError{ExitCode: exitUsageError, Code: "missing_flag", Message: "--manifest is required"}
	}
	if strings.TrimSpace(cfg.InputRoot) == "" {
		return &workerError{ExitCode: exitUsageError, Code: "missing_flag", Message: "--input-root is required"}
	}
	if strings.TrimSpace(cfg.InputObject) == "" {
		return &workerError{ExitCode: exitUsageError, Code: "missing_flag", Message: "--input is required"}
	}
	if strings.TrimSpace(cfg.OutputDir) == "" {
		return &workerError{ExitCode: exitUsageError, Code: "missing_flag", Message: "--output-dir is required"}
	}
	if cfg.TimeoutMs < 1 || cfg.TimeoutMs > adapter.MaxTimeoutMs {
		return &workerError{ExitCode: exitUsageError, Code: "invalid_limit",
			Message: fmt.Sprintf("--timeout-ms must be between 1 and %d", adapter.MaxTimeoutMs)}
	}
	if cfg.MaxInputBytes < 1 || cfg.MaxInputBytes > adapter.MaxInputBytes {
		return &workerError{ExitCode: exitUsageError, Code: "invalid_limit",
			Message: fmt.Sprintf("--max-input-bytes must be between 1 and %d", adapter.MaxInputBytes)}
	}
	if cfg.MaxOutputBytes < 1 || cfg.MaxOutputBytes > adapter.MaxOutputBytes {
		return &workerError{ExitCode: exitUsageError, Code: "invalid_limit",
			Message: fmt.Sprintf("--max-output-bytes must be between 1 and %d", adapter.MaxOutputBytes)}
	}
	return nil
}

func generateRunID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return "run_" + hex.EncodeToString(raw[:]), nil
}

func resolveCoLocatedAdapterBinary() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	binName := "go-ingest-adapter"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	return filepath.Join(filepath.Dir(exePath), binName), nil
}

func readBoundedRegularFile(path string, maxBytes int64) ([]byte, error) {
	linfo, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if linfo.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("manifest path must not be a symbolic link")
	}
	if !linfo.Mode().IsRegular() {
		return nil, fmt.Errorf("manifest path is not a regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	finfo, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !finfo.Mode().IsRegular() {
		return nil, fmt.Errorf("manifest path is not a regular file")
	}
	if finfo.Size() > maxBytes {
		return nil, fmt.Errorf("manifest size %d bytes exceeds limit of %d bytes", finfo.Size(), maxBytes)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("manifest exceeds limit of %d bytes", maxBytes)
	}
	return data, nil
}

func workerBuildMetadata() evidence.WorkerMetadata {
	var commitSHA *string
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && strings.TrimSpace(s.Value) != "" {
				v := strings.TrimSpace(s.Value)
				commitSHA = &v
				break
			}
		}
	}
	return evidence.WorkerMetadata{
		Version:   version,
		CommitSHA: commitSHA,
	}
}
