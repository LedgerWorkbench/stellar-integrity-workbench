package evidence

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/adapter"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/adapter/goingest"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/assertion"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/corpus"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/domain"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/source/localfile"
)

var adapterSchemaInitOnce sync.Once

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
	decodedResult, err := validateBuildInput(input)
	if err != nil {
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

	pending, err := buildPendingArtifacts(input, decodedResult)
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
		_ = root.RemoveAll(runID)
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

	manifest := buildManifest(input, decodedResult, artifactRefs)
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

func validateBuildInput(input BuildInput) (*adapter.Result, error) {
	if input.Case == nil {
		return nil, fmt.Errorf("%w: Case must not be nil", ErrInvalidBundleInput)
	}
	if input.Run == nil {
		return nil, fmt.Errorf("%w: Run must not be nil", ErrInvalidBundleInput)
	}
	if input.VerifiedInput == nil {
		return nil, fmt.Errorf("%w: VerifiedInput must not be nil", ErrInvalidBundleInput)
	}
	if err := ValidateRunID(input.Run.RunID); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidBundleInput, err)
	}
	if input.Run.CaseID != input.Case.CaseID {
		return nil, fmt.Errorf("%w: run.case_id %q does not match case.case_id %q",
			ErrInvalidBundleInput, input.Run.CaseID, input.Case.CaseID)
	}
	if input.Run.AdapterID != input.AdapterTarget.AdapterID {
		return nil, fmt.Errorf("%w: run.adapter_id %q does not match adapter target %q",
			ErrInvalidBundleInput, input.Run.AdapterID, input.AdapterTarget.AdapterID)
	}
	if !input.Run.State.IsTerminal() {
		return nil, fmt.Errorf("%w: run.state %q is not terminal", ErrInvalidBundleInput, input.Run.State)
	}
	if input.Run.StartedAt.IsZero() {
		return nil, fmt.Errorf("%w: run.started_at must not be zero", ErrInvalidBundleInput)
	}
	if input.Run.CompletedAt == nil || input.Run.CompletedAt.IsZero() {
		return nil, fmt.Errorf("%w: terminal run must have completed_at", ErrInvalidBundleInput)
	}
	if input.Run.CompletedAt.Before(input.Run.StartedAt) {
		return nil, fmt.Errorf("%w: run.completed_at precedes run.started_at", ErrInvalidBundleInput)
	}

	if strings.TrimSpace(input.Worker.Version) == "" {
		return nil, fmt.Errorf("%w: worker.version must not be empty", ErrInvalidBundleInput)
	}
	if strings.TrimSpace(input.AdapterTarget.AdapterID) == "" ||
		strings.TrimSpace(input.AdapterTarget.AdapterVersion) == "" ||
		strings.TrimSpace(input.AdapterTarget.SDKVersion) == "" {
		return nil, fmt.Errorf("%w: adapter target metadata must be non-empty", ErrInvalidBundleInput)
	}

	// Cross-check VerifiedInput against corpus Case metadata.
	if err := corpus.ValidateObjectPath(input.Case.Input.ObjectPath); err != nil {
		return nil, fmt.Errorf("%w: invalid case input.object_path: %v", ErrInvalidBundleInput, err)
	}
	if input.VerifiedInput.ObjectPath() != input.Case.Input.ObjectPath {
		return nil, fmt.Errorf("%w: verified input object_path %q does not match case %q",
			ErrInvalidBundleInput, input.VerifiedInput.ObjectPath(), input.Case.Input.ObjectPath)
	}
	if input.VerifiedInput.ByteLength() != input.Case.Input.ByteLength {
		return nil, fmt.Errorf("%w: verified input byte length %d does not match case %d",
			ErrInvalidBundleInput, input.VerifiedInput.ByteLength(), input.Case.Input.ByteLength)
	}
	if input.VerifiedInput.SHA256() != input.Case.Input.SHA256 {
		return nil, fmt.Errorf("%w: verified input SHA-256 %s does not match case %s",
			ErrInvalidBundleInput, input.VerifiedInput.SHA256(), input.Case.Input.SHA256)
	}

	var decodedResult *adapter.Result
	if input.AdapterOutput != nil {
		var err error
		decodedResult, err = validateAdapterOutputAgainstStdout(input.AdapterOutput)
		if err != nil {
			return nil, err
		}
		if decodedResult.Execution.ExitCode != input.AdapterOutput.ActualExitCode {
			return nil, fmt.Errorf("%w: adapter result exit_code %d does not match host-observed exit code %d",
				ErrInvalidBundleInput, decodedResult.Execution.ExitCode, input.AdapterOutput.ActualExitCode)
		}
		if decodedResult.RunID != input.Run.RunID {
			return nil, fmt.Errorf("%w: adapter result run_id %q does not match run %q",
				ErrInvalidBundleInput, decodedResult.RunID, input.Run.RunID)
		}
		if decodedResult.CaseID != input.Case.CaseID {
			return nil, fmt.Errorf("%w: adapter result case_id %q does not match case %q",
				ErrInvalidBundleInput, decodedResult.CaseID, input.Case.CaseID)
		}
		if decodedResult.AdapterID != input.AdapterTarget.AdapterID ||
			decodedResult.AdapterVersion != input.AdapterTarget.AdapterVersion ||
			decodedResult.SDKVersion != input.AdapterTarget.SDKVersion {
			return nil, fmt.Errorf("%w: adapter result identity does not match target", ErrInvalidBundleInput)
		}
		if decodedResult.Input.SHA256 != input.VerifiedInput.SHA256() {
			return nil, fmt.Errorf("%w: adapter result input.sha256 %s does not match verified input %s",
				ErrInvalidBundleInput, decodedResult.Input.SHA256, input.VerifiedInput.SHA256())
		}
	}

	if input.Run.State == domain.RunStateCompleted {
		if input.Run.Outcome == nil || !input.Run.Outcome.IsValid() {
			return nil, fmt.Errorf("%w: completed run must have a valid conformance outcome", ErrInvalidBundleInput)
		}
		if len(input.Case.Expectations) == 0 {
			return nil, fmt.Errorf("%w: completed run requires non-empty case expectations", ErrInvalidBundleInput)
		}
		if decodedResult == nil {
			return nil, fmt.Errorf("%w: completed run requires non-nil adapter output and result", ErrInvalidBundleInput)
		}
		if decodedResult.Execution.Status != adapter.ExecutionStatusCompleted {
			return nil, fmt.Errorf("%w: completed run requires adapter execution status %q, got %q",
				ErrInvalidBundleInput, adapter.ExecutionStatusCompleted, decodedResult.Execution.Status)
		}
		if input.AdapterOutput.ActualExitCode != 0 {
			return nil, fmt.Errorf("%w: completed run requires host-observed exit code 0, got %d",
				ErrInvalidBundleInput, input.AdapterOutput.ActualExitCode)
		}
		if err := recalculateAndVerifyCompletedAssertions(input, decodedResult); err != nil {
			return nil, err
		}
	} else {
		if input.Run.Outcome != nil {
			return nil, fmt.Errorf("%w: non-completed run (state=%s) must not have a fabricated outcome",
				ErrInvalidBundleInput, input.Run.State)
		}
		seenAssertionIDs := make(map[string]struct{}, len(input.Assertions))
		for i, a := range input.Assertions {
			if strings.TrimSpace(a.AssertionID) == "" {
				return nil, fmt.Errorf("%w: assertion[%d] has empty assertion_id", ErrInvalidBundleInput, i)
			}
			if _, dup := seenAssertionIDs[a.AssertionID]; dup {
				return nil, fmt.Errorf("%w: duplicate assertion_id %q", ErrInvalidBundleInput, a.AssertionID)
			}
			seenAssertionIDs[a.AssertionID] = struct{}{}
			if err := a.Kind.Validate(); err != nil {
				return nil, fmt.Errorf("%w: assertion %q: %v", ErrInvalidBundleInput, a.AssertionID, err)
			}
			if err := a.Status.Validate(); err != nil {
				return nil, fmt.Errorf("%w: assertion %q: %v", ErrInvalidBundleInput, a.AssertionID, err)
			}
		}
	}

	return decodedResult, nil
}

func validateAdapterOutputAgainstStdout(out *adapter.RunOutput) (*adapter.Result, error) {
	if out.Result == nil {
		return nil, fmt.Errorf("%w: adapter output Result must not be nil", ErrInvalidBundleInput)
	}
	stdoutBytes := out.StdoutCopy()
	if len(bytes.TrimSpace(stdoutBytes)) == 0 {
		return nil, fmt.Errorf("%w: adapter output requires non-empty raw stdout bytes", ErrInvalidBundleInput)
	}

	adapterSchemaInitOnce.Do(adapter.Init)

	decoded, err := adapter.DecodeResult(stdoutBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: raw adapter stdout failed strict decode: %v", ErrInvalidBundleInput, err)
	}
	if err := adapter.SchemaValidateResult(stdoutBytes); err != nil {
		return nil, fmt.Errorf("%w: raw adapter stdout failed schema validation: %v", ErrInvalidBundleInput, err)
	}
	if err := adapter.ValidateResult(decoded); err != nil {
		return nil, fmt.Errorf("%w: raw adapter stdout failed protocol validation: %v", ErrInvalidBundleInput, err)
	}
	if !reflect.DeepEqual(decoded, out.Result) {
		return nil, fmt.Errorf("%w: AdapterOutput.Result does not match decoded raw stdout bytes", ErrInvalidBundleInput)
	}
	return decoded, nil
}

func recalculateAndVerifyCompletedAssertions(input BuildInput, decodedResult *adapter.Result) error {
	if len(input.Assertions) == 0 {
		return fmt.Errorf("%w: completed run requires non-empty assertion results", ErrInvalidBundleInput)
	}
	if len(input.Assertions) != len(input.Case.Expectations) {
		return fmt.Errorf("%w: completed run has %d assertion results, expected %d matching case expectations",
			ErrInvalidBundleInput, len(input.Assertions), len(input.Case.Expectations))
	}

	// Fail closed for unknown adapter IDs until a persistent capability registry is implemented.
	if input.AdapterTarget.AdapterID != adapter.AdapterIDGoIngest {
		return fmt.Errorf("%w: unsupported adapter ID %q for Unit 7 assertion recalculation",
			ErrInvalidBundleInput, input.AdapterTarget.AdapterID)
	}

	var proj *domain.SemanticProjection
	if decodedResult.SemanticProjection != nil {
		var gp goingest.SemanticProjection
		dec := json.NewDecoder(bytes.NewReader(*decodedResult.SemanticProjection))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&gp); err != nil {
			return fmt.Errorf("%w: decode go-ingest semantic_projection: %v", ErrInvalidBundleInput, err)
		}
		var extra any
		if err := dec.Decode(&extra); err != io.EOF {
			return fmt.Errorf("%w: trailing JSON in go-ingest semantic_projection", ErrInvalidBundleInput)
		}
		var err error
		proj, err = assertion.ProjectionFromGoIngest(&gp)
		if err != nil {
			return fmt.Errorf("%w: map go-ingest semantic_projection: %v", ErrInvalidBundleInput, err)
		}
	}

	var evidenceRefs []string
	if len(input.Assertions[0].EvidenceRefs) > 0 {
		evidenceRefs = make([]string, len(input.Assertions[0].EvidenceRefs))
		copy(evidenceRefs, input.Assertions[0].EvidenceRefs)
	}

	eval, err := assertion.Evaluate(input.Case.Expectations, assertion.EvaluationInput{
		Projection:          proj,
		VerifiedInputSHA256: input.VerifiedInput.SHA256(),
		SupportedKinds:      assertion.GoIngestSupportedKinds(),
		EvidenceRefs:        evidenceRefs,
	})
	if err != nil {
		return fmt.Errorf("%w: recalculate assertions via Unit 6: %v", ErrInvalidBundleInput, err)
	}

	if eval.Outcome != *input.Run.Outcome {
		return fmt.Errorf("%w: declared run outcome %q disagrees with recalculated outcome %q",
			ErrInvalidBundleInput, *input.Run.Outcome, eval.Outcome)
	}

	for i := range eval.Results {
		recalc := &eval.Results[i]
		supplied := &input.Assertions[i]
		if supplied.AssertionID != recalc.AssertionID {
			return fmt.Errorf("%w: assertion[%d] ID %q does not match expected %q",
				ErrInvalidBundleInput, i, supplied.AssertionID, recalc.AssertionID)
		}
		if supplied.Kind != recalc.Kind {
			return fmt.Errorf("%w: assertion %q kind %q does not match expected %q",
				ErrInvalidBundleInput, supplied.AssertionID, supplied.Kind, recalc.Kind)
		}
		if err := compareExactValue(recalc.Kind, recalc.Expected, supplied.Expected); err != nil {
			return fmt.Errorf("%w: assertion %q expected value mismatch: %v",
				ErrInvalidBundleInput, supplied.AssertionID, err)
		}
		if err := compareObservedValue(recalc.Observed, supplied.Observed); err != nil {
			return fmt.Errorf("%w: assertion %q observed value mismatch: %v",
				ErrInvalidBundleInput, supplied.AssertionID, err)
		}
		if supplied.Status != recalc.Status {
			return fmt.Errorf("%w: assertion %q status %q does not match recalculated %q",
				ErrInvalidBundleInput, supplied.AssertionID, supplied.Status, recalc.Status)
		}
		if supplied.Explanation != recalc.Explanation {
			return fmt.Errorf("%w: assertion %q explanation mismatch", ErrInvalidBundleInput, supplied.AssertionID)
		}
		if !reflect.DeepEqual(supplied.EvidenceRefs, recalc.EvidenceRefs) {
			return fmt.Errorf("%w: assertion %q evidence_refs mismatch", ErrInvalidBundleInput, supplied.AssertionID)
		}
	}

	return nil
}

func compareObservedValue(recalc, supplied any) error {
	if recalc == nil && supplied == nil {
		return nil
	}
	if recalc == nil || supplied == nil {
		return fmt.Errorf("recalculated=%v supplied=%v", recalc, supplied)
	}
	if rInt, err1 := domain.ParseExactInteger(recalc); err1 == nil {
		sInt, err2 := domain.ParseExactInteger(supplied)
		if err2 != nil {
			return fmt.Errorf("expected exact integer matching %d, got %T (%v)", rInt, supplied, supplied)
		}
		if rInt != sInt {
			return fmt.Errorf("supplied %d != recalculated %d", sInt, rInt)
		}
		return nil
	}
	if !reflect.DeepEqual(recalc, supplied) {
		return fmt.Errorf("supplied %v != recalculated %v", supplied, recalc)
	}
	return nil
}

func compareExactValue(kind domain.AssertionKind, expVal, resVal any) error {
	switch kind {
	case domain.AssertionKindLedgerSequenceEquals, domain.AssertionKindTransactionCountEquals:
		eInt, err := domain.ParseExactInteger(expVal)
		if err != nil {
			return fmt.Errorf("case expectation: %w", err)
		}
		rInt, err := domain.ParseExactInteger(resVal)
		if err != nil {
			return fmt.Errorf("assertion result: %w", err)
		}
		if eInt != rInt {
			return fmt.Errorf("result %d != expectation %d", rInt, eInt)
		}
		return nil
	case domain.AssertionKindTransactionHashPresent, domain.AssertionKindDecodedTypeEquals, domain.AssertionKindInputHashEquals:
		eStr, ok1 := expVal.(string)
		rStr, ok2 := resVal.(string)
		if !ok1 || !ok2 {
			return fmt.Errorf("expected string values, got %T and %T", expVal, resVal)
		}
		if eStr != rStr {
			return fmt.Errorf("result %q != expectation %q", rStr, eStr)
		}
		return nil
	default:
		return fmt.Errorf("unsupported assertion kind %q", kind)
	}
}

func buildPendingArtifacts(input BuildInput, decodedResult *adapter.Result) ([]pendingArtifact, error) {
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

		if decodedResult != nil && decodedResult.SemanticProjection != nil {
			projRaw := *decodedResult.SemanticProjection
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

func buildManifest(input BuildInput, decodedResult *adapter.Result, artifactRefs []ArtifactRef) *Manifest {
	var retrievedAtStr *string
	if input.Case.Source.RetrievedAt != nil {
		s := input.Case.Source.RetrievedAt.UTC().Format(time.RFC3339)
		retrievedAtStr = &s
	}

	var detectedType *string
	var adapterStatus *string
	var adapterExitCode *int
	var adapterDurationMs *int64
	var hostExitCode *int
	var hostDurationMs *int64
	var adapterDiags []adapter.ResultDiagnostic
	var stderrBytesSeen *int64
	var stderrTruncated *bool

	if input.AdapterOutput != nil {
		seen := input.AdapterOutput.StderrBytesSeen
		trunc := input.AdapterOutput.StderrTruncated
		stderrBytesSeen = &seen
		stderrTruncated = &trunc

		hec := input.AdapterOutput.ActualExitCode
		hdur := input.AdapterOutput.ActualDuration.Milliseconds()
		if hdur < 0 {
			hdur = 0
		}
		hostExitCode = &hec
		hostDurationMs = &hdur

		if decodedResult != nil {
			if decodedResult.Decode.DetectedType != nil {
				dt := *decodedResult.Decode.DetectedType
				detectedType = &dt
			}
			st := decodedResult.Execution.Status
			ec := decodedResult.Execution.ExitCode
			dur := decodedResult.Execution.DurationMs
			adapterStatus = &st
			adapterExitCode = &ec
			adapterDurationMs = &dur

			adapterDiags = make([]adapter.ResultDiagnostic, len(decodedResult.Diagnostics))
			copy(adapterDiags, decodedResult.Diagnostics)
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
			HostExitCode:           hostExitCode,
			HostDurationMs:         hostDurationMs,
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
