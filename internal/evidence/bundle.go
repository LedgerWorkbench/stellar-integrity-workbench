package evidence

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/adapter"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/corpus"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/domain"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/source/localfile"
)

// BuildInput supplies all trusted inputs required to build and persist an
// immutable evidence bundle for a terminal replay run.
type BuildInput struct {
	Case          *domain.Case
	Run           *domain.Run
	VerifiedInput *localfile.VerifiedInput
	AdapterTarget adapter.RequestTarget
	AdapterOutput *adapter.RunOutput
	Assertions    []domain.AssertionResult
	Worker        WorkerMetadata
}

// WriteResult describes the persisted and verified evidence bundle.
type WriteResult struct {
	BundleDir      string
	ManifestPath   string
	ManifestSHA256 string
	Manifest       *Manifest
	Verification   *VerificationResult
}

type pendingArtifact struct {
	path      string
	role      string
	mediaType string
	data      []byte
}

// WriteBundle validates all input relationships, writes artifacts and manifest.json
// immutably under <evidenceRoot>/<run-id>/ using exclusive file creation, and
// verifies the completed bundle before returning.
func WriteBundle(evidenceRoot string, input BuildInput) (*WriteResult, error) {
	if err := validateBuildInput(input); err != nil {
		return nil, err
	}

	cleanRoot := filepath.Clean(strings.TrimSpace(evidenceRoot))
	if cleanRoot == "" || cleanRoot == "." && strings.TrimSpace(evidenceRoot) == "" {
		return nil, fmt.Errorf("%w: evidence root directory must not be empty", ErrInvalidBundleInput)
	}
	rootInfo, err := os.Stat(cleanRoot)
	if err != nil {
		return nil, fmt.Errorf("%w: stat evidence root %q: %v", ErrInvalidBundleInput, cleanRoot, err)
	}
	if !rootInfo.IsDir() {
		return nil, fmt.Errorf("%w: evidence root %q is not a directory", ErrInvalidBundleInput, cleanRoot)
	}

	root, err := os.OpenRoot(cleanRoot)
	if err != nil {
		return nil, fmt.Errorf("%w: open evidence root %q: %v", ErrInvalidBundleInput, cleanRoot, err)
	}
	defer root.Close()

	runID := input.Run.RunID
	if _, err := root.Lstat(runID); err == nil {
		return nil, fmt.Errorf("%w: run bundle %q already exists", ErrBundleExists, runID)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("check existing bundle %q: %w", runID, err)
	}

	pending, err := buildPendingArtifacts(input)
	if err != nil {
		return nil, err
	}

	if err := root.Mkdir(runID, 0o700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("%w: run bundle %q already exists", ErrBundleExists, runID)
		}
		return nil, fmt.Errorf("create bundle directory %q: %w", runID, err)
	}

	bundleDir := filepath.Join(cleanRoot, runID)
	cleanupPartial := func() {
		_ = os.RemoveAll(bundleDir)
	}

	if err := root.Mkdir(filepath.Join(runID, ArtifactsDirname), 0o700); err != nil {
		cleanupPartial()
		return nil, fmt.Errorf("create artifacts directory: %w", err)
	}

	runRoot, err := root.OpenRoot(runID)
	if err != nil {
		cleanupPartial()
		return nil, fmt.Errorf("open run bundle root %q: %w", runID, err)
	}

	artifactRefs := make([]ArtifactRef, 0, len(pending))
	for _, art := range pending {
		if err := writeExclusiveInRoot(runRoot, art.path, art.data); err != nil {
			_ = runRoot.Close()
			cleanupPartial()
			return nil, err
		}
		sumHex, size := ComputeSHA256(art.data)
		artifactRefs = append(artifactRefs, ArtifactRef{
			Path:       art.path,
			Role:       art.role,
			MediaType:  art.mediaType,
			ByteLength: size,
			SHA256:     sumHex,
		})
	}

	manifest := buildManifest(input, artifactRefs)
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		_ = runRoot.Close()
		cleanupPartial()
		return nil, fmt.Errorf("marshal evidence manifest: %w", err)
	}
	manifestBytes = append(manifestBytes, '\n')

	if _, err := DecodeAndValidateManifest(manifestBytes); err != nil {
		_ = runRoot.Close()
		cleanupPartial()
		return nil, fmt.Errorf("self-check manifest failed: %w", err)
	}

	if err := writeExclusiveInRoot(runRoot, ManifestFilename, manifestBytes); err != nil {
		_ = runRoot.Close()
		cleanupPartial()
		return nil, err
	}

	if err := runRoot.Close(); err != nil {
		cleanupPartial()
		return nil, fmt.Errorf("close run bundle root: %w", err)
	}

	verification, err := VerifyBundle(cleanRoot, runID)
	if err != nil {
		cleanupPartial()
		return nil, fmt.Errorf("post-write bundle verification failed: %w", err)
	}

	return &WriteResult{
		BundleDir:      bundleDir,
		ManifestPath:   filepath.Join(bundleDir, ManifestFilename),
		ManifestSHA256: verification.ManifestSHA256,
		Manifest:       manifest,
		Verification:   verification,
	}, nil
}

func validateBuildInput(input BuildInput) error {
	if input.Case == nil {
		return fmt.Errorf("%w: Case must not be nil", ErrInvalidBundleInput)
	}
	if input.Run == nil {
		return fmt.Errorf("%w: Run must not be nil", ErrInvalidBundleInput)
	}
	if input.VerifiedInput == nil {
		return fmt.Errorf("%w: VerifiedInput must not be nil", ErrInvalidBundleInput)
	}
	if err := ValidateRunID(input.Run.RunID); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidBundleInput, err)
	}
	if input.Run.CaseID != input.Case.CaseID {
		return fmt.Errorf("%w: run.case_id %q does not match case.case_id %q",
			ErrInvalidBundleInput, input.Run.CaseID, input.Case.CaseID)
	}
	if input.Run.AdapterID != input.AdapterTarget.AdapterID {
		return fmt.Errorf("%w: run.adapter_id %q does not match adapter target %q",
			ErrInvalidBundleInput, input.Run.AdapterID, input.AdapterTarget.AdapterID)
	}
	if !input.Run.State.IsTerminal() {
		return fmt.Errorf("%w: run.state %q is not terminal", ErrInvalidBundleInput, input.Run.State)
	}
	if input.Run.StartedAt.IsZero() {
		return fmt.Errorf("%w: run.started_at must not be zero", ErrInvalidBundleInput)
	}
	if input.Run.CompletedAt == nil || input.Run.CompletedAt.IsZero() {
		return fmt.Errorf("%w: terminal run must have completed_at", ErrInvalidBundleInput)
	}
	if input.Run.CompletedAt.Before(input.Run.StartedAt) {
		return fmt.Errorf("%w: run.completed_at precedes run.started_at", ErrInvalidBundleInput)
	}

	if input.Run.State == domain.RunStateCompleted {
		if input.Run.Outcome == nil || !input.Run.Outcome.IsValid() {
			return fmt.Errorf("%w: completed run must have a valid conformance outcome", ErrInvalidBundleInput)
		}
	} else if input.Run.Outcome != nil {
		return fmt.Errorf("%w: non-completed run (state=%s) must not have a fabricated outcome",
			ErrInvalidBundleInput, input.Run.State)
	}

	if strings.TrimSpace(input.Worker.Version) == "" {
		return fmt.Errorf("%w: worker.version must not be empty", ErrInvalidBundleInput)
	}
	if strings.TrimSpace(input.AdapterTarget.AdapterID) == "" ||
		strings.TrimSpace(input.AdapterTarget.AdapterVersion) == "" ||
		strings.TrimSpace(input.AdapterTarget.SDKVersion) == "" {
		return fmt.Errorf("%w: adapter target metadata must be non-empty", ErrInvalidBundleInput)
	}

	// Cross-check VerifiedInput against corpus Case metadata.
	if err := corpus.ValidateObjectPath(input.Case.Input.ObjectPath); err != nil {
		return fmt.Errorf("%w: invalid case input.object_path: %v", ErrInvalidBundleInput, err)
	}
	if input.VerifiedInput.ObjectPath() != input.Case.Input.ObjectPath {
		return fmt.Errorf("%w: verified input object_path %q does not match case %q",
			ErrInvalidBundleInput, input.VerifiedInput.ObjectPath(), input.Case.Input.ObjectPath)
	}
	if input.VerifiedInput.ByteLength() != input.Case.Input.ByteLength {
		return fmt.Errorf("%w: verified input byte length %d does not match case %d",
			ErrInvalidBundleInput, input.VerifiedInput.ByteLength(), input.Case.Input.ByteLength)
	}
	if input.VerifiedInput.SHA256() != input.Case.Input.SHA256 {
		return fmt.Errorf("%w: verified input SHA-256 %s does not match case %s",
			ErrInvalidBundleInput, input.VerifiedInput.SHA256(), input.Case.Input.SHA256)
	}

	if input.AdapterOutput != nil {
		if input.AdapterOutput.Result == nil {
			return fmt.Errorf("%w: adapter output Result must not be nil", ErrInvalidBundleInput)
		}
		res := input.AdapterOutput.Result
		if res.RunID != input.Run.RunID {
			return fmt.Errorf("%w: adapter result run_id %q does not match run %q",
				ErrInvalidBundleInput, res.RunID, input.Run.RunID)
		}
		if res.CaseID != input.Case.CaseID {
			return fmt.Errorf("%w: adapter result case_id %q does not match case %q",
				ErrInvalidBundleInput, res.CaseID, input.Case.CaseID)
		}
		if res.AdapterID != input.AdapterTarget.AdapterID ||
			res.AdapterVersion != input.AdapterTarget.AdapterVersion ||
			res.SDKVersion != input.AdapterTarget.SDKVersion {
			return fmt.Errorf("%w: adapter result identity does not match target", ErrInvalidBundleInput)
		}
		if res.Input.SHA256 != input.VerifiedInput.SHA256() {
			return fmt.Errorf("%w: adapter result input.sha256 %s does not match verified input %s",
				ErrInvalidBundleInput, res.Input.SHA256, input.VerifiedInput.SHA256())
		}
	}

	for i, a := range input.Assertions {
		if strings.TrimSpace(a.AssertionID) == "" {
			return fmt.Errorf("%w: assertion[%d] has empty assertion_id", ErrInvalidBundleInput, i)
		}
		if err := a.Kind.Validate(); err != nil {
			return fmt.Errorf("%w: assertion %q: %v", ErrInvalidBundleInput, a.AssertionID, err)
		}
		if err := a.Status.Validate(); err != nil {
			return fmt.Errorf("%w: assertion %q: %v", ErrInvalidBundleInput, a.AssertionID, err)
		}
	}

	return nil
}

func buildPendingArtifacts(input BuildInput) ([]pendingArtifact, error) {
	var artifacts []pendingArtifact

	if input.AdapterOutput != nil {
		stdoutBytes := input.AdapterOutput.StdoutCopy()
		if len(stdoutBytes) > 0 {
			artifacts = append(artifacts, pendingArtifact{
				path:      ArtifactsDirname + "/adapter-stdout.json",
				role:      ArtifactRoleAdapterStdout,
				mediaType: "application/json",
				data:      stdoutBytes,
			})
		}

		if input.AdapterOutput.Result != nil && input.AdapterOutput.Result.SemanticProjection != nil {
			projRaw := *input.AdapterOutput.Result.SemanticProjection
			if err := validateJSONObjectBytes(projRaw); err != nil {
				return nil, fmt.Errorf("%w: semantic_projection is not a valid JSON object: %v", ErrInvalidBundleInput, err)
			}
			projCopy := make([]byte, len(projRaw))
			copy(projCopy, projRaw)
			artifacts = append(artifacts, pendingArtifact{
				path:      ArtifactsDirname + "/semantic-projection.json",
				role:      ArtifactRoleSemanticProjection,
				mediaType: "application/json",
				data:      projCopy,
			})
		}
	}

	if len(input.Assertions) > 0 {
		assertionsBytes, err := json.MarshalIndent(input.Assertions, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshal assertions artifact: %w", err)
		}
		assertionsBytes = append(assertionsBytes, '\n')
		artifacts = append(artifacts, pendingArtifact{
			path:      ArtifactsDirname + "/assertions.json",
			role:      ArtifactRoleAssertions,
			mediaType: "application/json",
			data:      assertionsBytes,
		})
	}

	return artifacts, nil
}

func validateJSONObjectBytes(data []byte) error {
	var obj map[string]any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&obj); err != nil {
		return err
	}
	if obj == nil {
		return fmt.Errorf("JSON value is null, expected object")
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("trailing JSON data after object")
	}
	return nil
}

func buildManifest(input BuildInput, artifactRefs []ArtifactRef) *Manifest {
	var retrievedAtStr *string
	if input.Case.Source.RetrievedAt != nil {
		s := input.Case.Source.RetrievedAt.UTC().Format(time.RFC3339)
		retrievedAtStr = &s
	}

	var detectedType *string
	var adapterStatus *string
	var adapterExitCode *int
	var adapterDurationMs *int64
	var adapterDiags []adapter.ResultDiagnostic
	var stderrBytesSeen *int64
	var stderrTruncated *bool

	if input.AdapterOutput != nil {
		seen := input.AdapterOutput.StderrBytesSeen
		trunc := input.AdapterOutput.StderrTruncated
		stderrBytesSeen = &seen
		stderrTruncated = &trunc

		if input.AdapterOutput.Result != nil {
			res := input.AdapterOutput.Result
			if res.Decode.DetectedType != nil {
				dt := *res.Decode.DetectedType
				detectedType = &dt
			}
			st := res.Execution.Status
			ec := res.Execution.ExitCode
			dur := res.Execution.DurationMs
			adapterStatus = &st
			adapterExitCode = &ec
			adapterDurationMs = &dur

			adapterDiags = make([]adapter.ResultDiagnostic, len(res.Diagnostics))
			copy(adapterDiags, res.Diagnostics)
		}
	}
	if adapterDiags == nil {
		adapterDiags = []adapter.ResultDiagnostic{}
	}

	limitations := make([]string, len(input.Case.Limitations))
	copy(limitations, input.Case.Limitations)

	assertions := make([]domain.AssertionResult, len(input.Assertions))
	for i, a := range input.Assertions {
		cp := a
		if len(a.EvidenceRefs) > 0 {
			cp.EvidenceRefs = make([]string, len(a.EvidenceRefs))
			copy(cp.EvidenceRefs, a.EvidenceRefs)
		}
		assertions[i] = cp
	}

	var outcomePtr *domain.RunOutcome
	if input.Run.Outcome != nil {
		o := *input.Run.Outcome
		outcomePtr = &o
	}

	return &Manifest{
		SchemaVersion:  SchemaVersion,
		RunID:          input.Run.RunID,
		CaseID:         input.Case.CaseID,
		Classification: input.Case.Classification,
		Provenance: Provenance{
			Kind:              input.Case.Source.Kind,
			URL:               input.Case.Source.URL,
			Repository:        input.Case.Source.Repository,
			IssueNumber:       input.Case.Source.IssueNumber,
			PullRequestNumber: input.Case.Source.PullRequestNumber,
			RetrievedAt:       retrievedAtStr,
			Notes:             input.Case.Source.Notes,
		},
		Input: InputEvidence{
			SHA256:          input.VerifiedInput.SHA256(),
			ByteLength:      input.VerifiedInput.ByteLength(),
			Format:          input.Case.Input.Format,
			Encoding:        input.Case.Input.Encoding,
			ObjectPath:      input.VerifiedInput.ObjectPath(),
			DetectedType:    detectedType,
			NetworkName:     input.Case.Network.Name,
			LedgerSequence:  input.Case.Network.LedgerSequence,
			ProtocolVersion: input.Case.Network.ProtocolVersion,
		},
		Worker: WorkerMetadata{
			Version:   input.Worker.Version,
			CommitSHA: input.Worker.CommitSHA,
		},
		Adapter: AdapterMetadata{
			AdapterID:      input.AdapterTarget.AdapterID,
			AdapterVersion: input.AdapterTarget.AdapterVersion,
			SDKVersion:     input.AdapterTarget.SDKVersion,
		},
		Run: RunMetadata{
			StartedAt:              input.Run.StartedAt.UTC().Format(time.RFC3339),
			CompletedAt:            input.Run.CompletedAt.UTC().Format(time.RFC3339),
			State:                  input.Run.State,
			AdapterExecutionStatus: adapterStatus,
			AdapterExitCode:        adapterExitCode,
			AdapterDurationMs:      adapterDurationMs,
			Outcome:                outcomePtr,
		},
		Assertions: assertions,
		Diagnostics: DiagnosticsEvidence{
			AdapterDiagnostics:     adapterDiags,
			Limitations:            limitations,
			AdapterStderrBytesSeen: stderrBytesSeen,
			AdapterStderrTruncated: stderrTruncated,
		},
		Artifacts: artifactRefs,
	}
}

func writeExclusiveInRoot(runRoot *os.Root, relPath string, data []byte) error {
	f, err := runRoot.OpenFile(relPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o400)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%w: %q already exists", ErrBundleExists, relPath)
		}
		return fmt.Errorf("create %q: %w", relPath, err)
	}

	written := 0
	for written < len(data) {
		n, wErr := f.Write(data[written:])
		written += n
		if wErr != nil {
			_ = f.Close()
			return fmt.Errorf("write %q: %w", relPath, wErr)
		}
		if n == 0 {
			_ = f.Close()
			return fmt.Errorf("write %q: %w", relPath, io.ErrShortWrite)
		}
	}

	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync %q: %w", relPath, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %q: %w", relPath, err)
	}
	return nil
}
