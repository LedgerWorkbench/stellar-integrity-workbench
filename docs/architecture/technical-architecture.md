# Stellar Integrity Workbench

## Phase 5: Technical Architecture Specification

**Status:** Proposed architecture, subject to implementation proof  
**Date:** 17 November 2026  
**Product name:** Stellar Integrity Workbench
**GitHub organization:** `LedgerWorkbench`
**Repository:** `stellar-integrity-workbench`
**Repository URL:** https://github.com/LedgerWorkbench/stellar-integrity-workbench

## 1. Architecture decision: no Soroban contract

The product is a developer infrastructure platform for inspecting, replaying and comparing Stellar ledger data. Its core functions are off-chain data ingestion, XDR decoding, SDK/indexer conformance checks, regression reproduction and evidence reporting.

It does not hold assets, manage ownership, settle payments, escrow funds or require public on-chain state. A Soroban contract would therefore add deployment, authorization, upgrade and maintenance costs without providing a necessary product capability.

**Decision:** the initial product contains no Soroban smart contract. No token, contract storage, `require_auth`, contract event or contract deployment is required. This is an intentional architecture decision, not an unfinished contract layer.

The playbook's contract-specification discipline is applied to the software service contracts below: each service has a defined responsibility, typed inputs and outputs, permissions, lifecycle events and dependencies.

## 2. Product boundary

The Workbench tests how SDKs and indexer pipelines interpret already-produced Stellar ledger data. It does not replace Stellar Core, Galexie, Horizon, the Stellar Go Ingest SDK or general-purpose indexing services.

It does not replay Stellar Consensus Protocol or independently re-execute a closed ledger to reproduce Core's full state transition. Its replay capability is **ingestion replay**: feed an authentic ledger-close metadata input through a supported reader, SDK or indexer adapter and compare the resulting interpretation with explicit expectations.

The product must preserve original inputs and execution evidence so another developer can reproduce a finding.

## 3. Core architectural components

### 3.1 Web Workbench (`apps/workbench`)

Responsibilities:
- Present the corpus explorer, XDR inspection studio, replay workbench, SDK conformance laboratory, diagnostics and evidence reports.
- Submit jobs and display persisted job state and results.
- Provide the product's primary developer experience.
- Render animation only from real state changes, actual run progress or user navigation.

It must not decode large untrusted ledger files or execute adapter code in the browser.

### 3.2 Application API (`apps/workbench` API routes or a separately deployable API package)

Responsibilities:
- Authenticate users and CI tokens.
- Authorize corpus and run operations.
- Validate request schemas and enforce quotas.
- Create idempotent run records and queue messages.
- Return corpus metadata, run state and report references.

The API is an orchestration layer. Heavy decoding and replay happen in workers.

### 3.3 Replay and conformance worker (`workers/replay-go`)

Responsibilities:
- Pull queued jobs.
- Fetch immutable inputs from object storage and verify SHA-256 before processing.
- Use Stellar's Go Ingest SDK as the first ledger backend/reader integration where applicable.
- Dispatch work to approved SDK or indexer adapters.
- Enforce execution time, memory and output limits.
- Normalize results, evaluate assertions and publish evidence.
- Persist state transitions and handle retries safely.

The worker must be stateless between jobs. A worker restart must not lose a corpus input, run record or report.

### 3.4 Adapter runners (`adapters/`)

Adapters are language-specific, independently versioned programs. They must not be loaded as arbitrary code into the API process.

Initial adapter families:
- Go ledger ingestion adapter using the official Go Ingest SDK.
- JavaScript XDR/SDK adapter using `@stellar/stellar-sdk` and its XDR facilities.
- Rust XDR adapter using the official `stellar-xdr` crate.
- Later, explicit indexer adapters for selected real indexer implementations.

Each adapter declares capabilities, implementation version, SDK/library version, build commit and container image digest. A language is not marked supported until its adapter passes the conformance suite.

### 3.5 Corpus service (`packages/corpus-schema`, `corpus/`)

Responsibilities:
- Validate corpus manifests.
- Classify samples and enforce provenance requirements.
- Keep the original input immutable.
- Verify content hashes and schema versions.
- Separate verified, historical, synthetic and unverified cases.

### 3.6 Assertion and comparison engine (`packages/conformance`)

Responsibilities:
- Evaluate explicit structural, decoding, semantic and ingestion assertions.
- Compare equivalent semantic projections across SDK adapters.
- Preserve raw adapter outputs for inspection.
- Explain which assertion passed, failed, was skipped or could not be evaluated.

The engine must not assume different SDKs expose identical object models. It compares canonical semantic assertions, not arbitrary in-memory object equality.

### 3.7 Evidence and reporting (`packages/evidence`, `packages/reporting`)

Responsibilities:
- Retain source references, sample hashes, adapter versions, execution details and expected/actual outcomes.
- Generate JSON and Markdown reports.
- Provide a reproducibility bundle that another developer can run without relying on the original browser session.

## 4. System topology

```text
Developer / CI
      |
      v
Web Workbench (Next.js)
      |
      v
Application API
  |       |        \
  |       |         +----> PostgreSQL metadata
  |       |
  |       +--------------> Object storage (XDR, evidence, reports)
  |
  +----------------------> Durable job queue
                                  |
                                  v
                        Replay / conformance worker
                           |          |
                           |          +--> SDK adapter runners
                           |
                           +--------------> Stellar Go Ingest SDK
                                  |
                                  v
                     Assertion and comparison engine
                                  |
                                  v
                         Evidence / report storage
                                  |
                                  v
                         Workbench reads actual results
```

External Stellar data sources are accessed by the ingestion layer or imported as immutable corpus objects. The browser does not connect directly to arbitrary ledger-data URLs.

## 5. Data flow for a conformance run

1. A user selects a verified or synthetic corpus case and one or more adapter versions.
2. The API checks the caller's permissions, case visibility, adapter availability and execution quota.
3. The API creates a run record with an idempotency key and enqueues a job.
4. A worker claims the job and records `RUNNING` with a start timestamp and worker build identity.
5. The worker fetches the exact input object, checks size and SHA-256, and records the verified input hash.
6. The worker executes the selected adapters in isolated runners with fixed resource limits.
7. Each adapter returns a versioned result envelope. The worker retains raw output and derives a canonical semantic projection.
8. The assertion engine evaluates expected conditions and records each assertion independently.
9. The report builder assembles provenance, versions, run environment, outputs, assertions and limitations.
10. The worker stores the evidence bundle, updates the run record and marks execution complete.
11. The Workbench reads the persisted state and displays the result. It must not infer success from a closed connection or a finished animation.

## 6. Service contracts

### 6.1 CorpusService

**Responsibilities:** ingest, validate, classify and retrieve corpus cases.

Operations:

- `validateManifest(manifest) -> ManifestValidationResult`
- `registerCase(manifest, objectReference, actor) -> CorpusCase`
- `getCase(caseId, actor) -> CorpusCaseDetail`
- `listCases(filters, cursor, actor) -> PaginatedCorpusCases`
- `changeVerificationState(caseId, state, reviewer) -> CorpusCase`

Permissions:
- Public: read only cases explicitly published as public.
- Authenticated user: create private submissions and read their own private cases.
- Corpus reviewer: approve, reject or deprecate a case.
- CI token: read only explicitly permitted public/private cases.

Events:
- `corpus.case.submitted`
- `corpus.case.validated`
- `corpus.case.verified`
- `corpus.case.rejected`
- `corpus.case.deprecated`

### 6.2 RunService

**Responsibilities:** validate, schedule, query and cancel conformance runs.

Operations:

- `createRun(request, actor, idempotencyKey) -> RunAccepted`
- `getRun(runId, actor) -> RunDetail`
- `listRuns(filters, cursor, actor) -> PaginatedRuns`
- `cancelRun(runId, actor) -> RunDetail`
- `getRunReport(runId, actor) -> ReportReference`

Permissions:
- Public demo users: limited runs against curated public cases, subject to rate limits.
- Authenticated users: run public cases and their own private cases.
- CI token: create runs and read their own run reports within token scope.
- Administrator/reviewer: manage adapter registry and corpus verification.

Events:
- `run.queued`
- `run.started`
- `run.progress`
- `run.adapter.completed`
- `run.completed`
- `run.execution_error`
- `run.cancelled`

### 6.3 AdapterRunner

**Responsibilities:** execute an approved adapter against a bounded input.

Input:

```json
{
  "schema_version": "1",
  "run_id": "run_...",
  "case_id": "case_...",
  "operation": "decode_ledger_meta",
  "input": {
    "format": "stellar-xdr",
    "encoding": "base64",
    "sha256": "<sha256>",
    "object_ref": "<worker-only-object-reference>"
  },
  "target": {
    "adapter_id": "go-ingest",
    "adapter_version": "<pinned-version>",
    "sdk_version": "<pinned-version>"
  },
  "limits": {
    "timeout_ms": 30000,
    "memory_mb": 512,
    "max_output_bytes": 10485760
  }
}
```

Output:

```json
{
  "schema_version": "1",
  "run_id": "run_...",
  "adapter_id": "go-ingest",
  "adapter_version": "<pinned-version>",
  "sdk_version": "<pinned-version>",
  "execution": {
    "status": "completed",
    "exit_code": 0,
    "duration_ms": 0
  },
  "decode": {
    "status": "success",
    "detected_type": "LedgerCloseMeta"
  },
  "semantic_output_ref": "<object-reference>",
  "raw_output_ref": "<object-reference>",
  "diagnostics": []
}
```

The example uses schema placeholders. It is not an actual execution result or measured duration. Actual adapter versions and limits are pinned before implementation.

### 6.4 AssertionEngine

**Responsibilities:** compare adapter output against declarative assertions.

Operations:
- `evaluate(caseAssertions, adapterOutput) -> AssertionResults`
- `compareAdapters(adapterOutputs, comparisonProfile) -> ComparisonResult`

Each assertion result has:
- Stable assertion ID.
- Assertion category.
- Expected condition.
- Actual observation or an explicit unavailable marker.
- Outcome: `passed`, `failed`, `skipped` or `inconclusive`.
- Evidence reference.
- Explanation.

A parser crash, timeout or missing dependency is an execution error, not a semantic assertion failure.

### 6.5 EvidencePublisher

**Responsibilities:** create a reproducibility bundle after execution.

A bundle includes:
- Corpus manifest and immutable input hash.
- Provenance and evidence classification.
- Adapter and SDK versions.
- Adapter image digest and source commit.
- Worker version and execution timestamp.
- Assertions with expected and actual results.
- Raw and normalized output references.
- Reproduction command or CI invocation.
- Limitations, skipped checks and execution errors.

## 7. Corpus manifest model

A corpus case has a versioned manifest. The original XDR object is stored separately from relational metadata.

```json
{
  "manifest_version": 1,
  "case_id": "stellar-lcm-example-001",
  "title": "Descriptive case title",
  "classification": "synthetic",
  "status": "draft",
  "source": {
    "kind": "upstream_issue",
    "url": "https://github.com/<owner>/<repo>/issues/<number>",
    "reference": "<issue-or-pr-reference>",
    "capture_date": null
  },
  "stellar_context": {
    "network": "unknown",
    "ledger_sequence": null,
    "protocol_version": null,
    "xdr_definition_revision": null
  },
  "input": {
    "type": "ScVal",
    "encoding": "base64",
    "compression": "none",
    "sha256": "<sha256-of-original-object>",
    "object_key": "corpus/<case-id>/input.xdr"
  },
  "assertions": [],
  "reproduction": {
    "required_adapters": [],
    "steps": []
  },
  "limitations": []
}
```

Classification values:
- `verified_ledger_sample`: the input is traceable to actual Stellar ledger data and its provenance has been checked.
- `historical_regression`: a fixture reproduces a documented historical defect. This does not automatically mean the fixture came from mainnet or testnet.
- `reproduced_regression`: the team has independently reproduced the defect and recorded the exact conditions.
- `synthetic`: deliberately constructed input used to test a defined edge case.
- `unverified`: origin or expected behavior is still under review.

These are classifications, not interchangeable quality badges. A synthetic case must never be described as a real ledger sample.

## 8. Run lifecycle and result semantics

Execution status and conformance outcome are separate fields.

**Execution status:**
- `created`
- `validating`
- `queued`
- `running`
- `completed`
- `execution_error`
- `cancelled`

**Conformance outcome** (available only after a sufficient completed execution):
- `passed`
- `failed`
- `inconclusive`
- `not_applicable`

This separation prevents a timeout, worker crash or missing dependency from being reported as a failed SDK assertion or as a successful test.

## 9. Initial API surface

All endpoints use `/api/v1` and versioned request/response schemas.

| Method | Route | Purpose |
|---|---|---|
| GET | `/health` | Service readiness without exposing secrets |
| GET | `/corpus/cases` | List accessible cases |
| GET | `/corpus/cases/{caseId}` | Read case manifest and provenance |
| GET | `/adapters` | List approved adapter capabilities and versions |
| POST | `/runs` | Create an idempotent conformance run; returns HTTP 202 |
| GET | `/runs/{runId}` | Read run state and summary |
| GET | `/runs/{runId}/report` | Retrieve report metadata and authorized artifact reference |
| POST | `/uploads` | Request a bounded private upload target |
| POST | `/corpus/submissions` | Submit a private or review-pending corpus case |

The API returns references to large objects. It does not return multi-megabyte XDR or evidence bundles inline by default.

## 10. Initial data model

PostgreSQL stores metadata and queryable state. Object storage holds binary XDR, raw adapter outputs, logs and report bundles.

Core tables:
- `users`: application identity and account state.
- `api_tokens`: hashed token material, scopes, expiry and revocation state.
- `corpus_cases`: case identity, classification, visibility, manifest version, source and verification state.
- `corpus_objects`: object key, content type, byte size, SHA-256 and immutability metadata.
- `adapters`: adapter identity, supported operations, language and repository.
- `adapter_versions`: SDK/library version, adapter version, source commit, image digest and support status.
- `runs`: actor, case, requested adapter versions, execution status, conformance outcome, idempotency key and timestamps.
- `run_adapter_results`: per-adapter execution state, output references and diagnostics.
- `assertion_results`: expected condition, observed condition, outcome and evidence reference.
- `audit_events`: security-sensitive corpus, token, permission and review actions.

Do not store large XDR blobs in PostgreSQL JSON fields. Store object references, hashes and compact queryable metadata instead.

## 11. Storage and immutability rules

- Original corpus input is immutable after registration.
- A changed input is a new object and must receive a new SHA-256 hash and manifest revision.
- Every report binds to the exact input hash and adapter versions used.
- Private user uploads remain private unless the owner explicitly requests publication and a reviewer verifies the provenance and license conditions.
- Deletion and retention policies must be documented before public user uploads are opened.
- Object storage references are access-controlled and time-limited where appropriate.

## 12. Security architecture

### Untrusted inputs

XDR files and compressed objects are untrusted input. The system must enforce maximum upload size, decompressed size, parser depth/collection bounds, CPU time, memory, output size and wall-clock timeout.

### Adapter isolation

- Run adapters as prebuilt, approved executables or containers.
- Do not accept arbitrary user-supplied executable adapters in the hosted MVP.
- Disable outbound network access by default for decoding and conformance runs.
- Give each run a temporary workspace and destroy it after completion.
- Do not share mutable files or credentials between jobs.
- Apply CPU, memory, process and file-size limits.

### API and identity

- Public corpus access is read-only.
- User uploads default to private.
- CI tokens are hashed at rest, scoped, revocable and time-bounded.
- No wallet private keys or transaction signing are required.
- Never include secrets in reports, logs or adapter output.
- Enforce authorization at the API and object-download boundary.

### Supply chain

- Pin adapter source commits and container image digests.
- Generate SBOMs for worker and adapter images.
- Scan dependencies and images in CI.
- Sign releases where the available toolchain supports it.
- Publish supported SDK version matrices from test results, not assumptions.

## 13. Reliability and observability

- Queue jobs with stable IDs and idempotency keys.
- Use bounded retry policies only for transient infrastructure failures.
- Do not automatically retry deterministic parser failures as though they were network problems.
- Make workers safe to restart and safe to process duplicate deliveries.
- Persist status changes and diagnostic references.
- Record queue age, execution duration, adapter error rates, corpus object verification failures and storage use.
- Keep a full audit trail for corpus review and token changes.

## 14. Deployment recommendation

Recommended first deployment candidate:
- Vercel for the Next.js Workbench and HTTP API.
- PostgreSQL provisioned through a Vercel Marketplace integration for structured metadata.
- Object storage such as Vercel Blob for XDR files, adapter outputs and reports.
- A durable queue and a pull-based worker process.
- Vercel Daemons may be evaluated for the Go worker, because they support long-running pull-based processes and deploy alongside the application. They are currently in public beta; the initial release does not provide persistent worker filesystem storage or inbound HTTP. Therefore the worker must keep durable state in the database/object store and must not depend on local disk surviving a restart.

The Go worker must remain a portable container. Deployment must not couple ingestion correctness to a Vercel-specific runtime. If Daemons are unsuitable after workload and cost tests, the same worker image can be deployed elsewhere without redesigning the core application.

Vercel is a recommendation, not a project constraint. Hosting costs, quotas, region availability and beta limits must be checked before production rollout. No service is assumed to be free.

## 15. Initial proof sequence

The first technical proof is deliberately narrow:

1. Obtain one authentic `LedgerCloseMeta` sample from a traceable Galexie or other permitted Stellar source. Record its provenance and hash.
2. Decode it with the official Go ingestion/XDR path.
3. Inspect the same input with one independent SDK/XDR implementation.
4. Define a small set of semantic assertions grounded in the actual sample.
5. Run both implementations and retain raw outputs, normalized projections and evidence.
6. Produce a truthful report that distinguishes actual ledger data from any synthetic regression fixture used alongside it.

A second proof should reproduce one documented historical defect using the original source conditions. If the upstream issue's example is synthetic, label it synthetic or historical-regression, not verified ledger sample.

The first proof is not complete until the worker, result persistence and report retrieval work end to end. A dashboard mockup does not satisfy this proof.

## 16. Acceptance criteria for this architecture phase

- [ ] No unnecessary Soroban contract is introduced.
- [ ] Product scope clearly distinguishes ingestion replay from Stellar consensus replay.
- [ ] A canonical corpus manifest schema is versioned.
- [ ] Each fixture has an explicit evidence classification and provenance.
- [ ] Original input hashes are verified before execution.
- [ ] Worker, API and storage responsibilities are separate.
- [ ] Adapter inputs and outputs have versioned schemas.
- [ ] Execution errors are distinguishable from conformance failures.
- [ ] Adapter execution is isolated and resource-bounded.
- [ ] Private uploads are not public by default.
- [ ] Reports bind to exact input and adapter versions.
- [ ] The first end-to-end proof uses a genuine Stellar ledger sample.
- [ ] Deployment remains portable even if Vercel is selected initially.

## 17. Open decisions for implementation planning

These items must be resolved by prototype or an explicit architecture decision record, not guessed now:

1. Which first public Galexie data source and access method can be used lawfully and reliably for the sample corpus.
2. Exact supported versions of `@stellar/stellar-sdk`, `github.com/stellar/go-stellar-sdk` and `stellar-xdr` at implementation start.
3. Whether the first indexer adapter targets a public real-world indexer or a minimal reference adapter built with the official Ingest SDK.
4. Which Postgres Marketplace provider and Vercel queue/worker configuration meet the MVP's cost and execution limits.
5. Whether the initial public demo supports anonymous bounded runs or requires sign-in.

No production compatibility claim should be made until the exact adapter/library versions have been tested and recorded.
