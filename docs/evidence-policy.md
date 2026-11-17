# Stellar Integrity Workbench — Evidence Classification and Provenance Policy

**Version:** 1.1.0
**Status:** Authoritative Policy
**Applies to:** Corpus Manifests, Replay Jobs, Evidence Bundles, and Reports

---

## 1. Purpose and Philosophy

Stellar Integrity Workbench tests how SDKs and indexer pipelines interpret already-produced Stellar ledger data. The integrity of findings depends entirely on the veracity and provenance of the test inputs.

Evidence classifications are **not interchangeable quality badges** or cosmetic tags. Each classification represents an exact, verifiable evidentiary standard. Under no circumstance may an input be misrepresented to claim a higher standard of authenticity than its verified provenance warrants.

---

## 2. Evidence Classification Tiers

Every corpus case manifest must declare exactly one `classification` from the following recognized tiers:

### 2.1 `verified_ledger_sample`
- **Definition:** Authentic Stellar ledger-close metadata obtained directly from a traceable, verifiable public source (such as Galexie archives, Captive Core instances, or Horizon/RPC archive storage) whose cryptographic integrity and provenance have been independently verified.
- **Mandatory Requirements:**
  - `source.kind` must be one of: `galexie_archive`, `captive_core`, `rpc_endpoint`.
  - `source.url` must be a valid, reachable URI pointing to the archive source or official object reference.
  - `source.retrieved_at` must record the exact RFC 3339 timestamp when the original input was obtained.
  - `network.ledger_sequence` must be a positive integer matching the authentic closed ledger (range `1` to `4,294,967,295`).
  - `network.protocol_version` must match the authentic ledger's protocol version (range `0` to `4,294,967,295`).
  - `input.sha256` must match the exact binary SHA-256 digest of the unparsed, unnormalized object.
- **Strict Prohibition:** Synthetic fixtures, constructed test cases, or unverified historical bug reproducers must **never** be classified as `verified_ledger_sample`.

### 2.2 `historical_regression`
- **Definition:** A test case associated with a documented historical defect in an official Stellar SDK, indexer, or client library.
- **Mandatory Requirements:**
  - `source.kind` must be one of: `upstream_issue`, `upstream_pr`, `advisory`, `other`.
  - Must cite the documented upstream issue, pull request, or repository advisory (via `source.url`, or `source.repository` with `source.issue_number` / `source.pull_request_number`).
  - Must record the source of the expected behaviour (e.g. the merged PR, test suite patch, or protocol specification).
  - If the historical issue provides a constructed or partial reproducer rather than authentic captured ledger bytes, it must be noted factually in `source.notes`.
- **Contradiction Prohibition:** `historical_regression` must **never** declare `source.kind` as `synthetic_construction`. A historical defect case requires traceable external defect documentation.

### 2.3 `reproduced_regression`
- **Definition:** A historical or newly discovered defect that has been independently executed, reproduced, and verified by this Workbench, with captured deterministic execution evidence.
- **Mandatory Requirements:**
  - Must satisfy all requirements of `historical_regression` (including traceable upstream defect reference).
  - Must include the structured `reproduction` section:
    - `reproduction.affected_implementation`: exact implementation name and affected version.
    - `reproduction.fixed_implementation`: exact implementation name and resolved/fixed version.
    - `reproduction.differential_evidence`: evidence reference URI/path and SHA-256 hash of the captured differential execution evidence artifact.
- **Strict Prohibition:** An issue URL alone is **never** sufficient to qualify a case as `reproduced_regression`. Documented differential execution evidence is mandatory.

### 2.4 `synthetic_fixture`
- **Definition:** Manually constructed, generated, or mutated test input designed to exercise specific parser paths, boundary conditions, malformed byte sequences, or schema corner cases.
- **Mandatory Requirements:**
  - Must declare `source.kind` strictly as `synthetic_construction`.
  - Must describe the construction methodology in `summary` and `source.notes`.
  - `network.name` must describe a synthetic or standalone test network (e.g. `Synthetic Standalone Network`).
- **Strict Prohibition:** A synthetic fixture must **never** claim or imply that it was captured from a public Stellar network (`Public Global Stellar Network`, `mainnet`).

### 2.5 `unverified`
- **Definition:** A candidate or submitted input whose provenance, retrieval chain, or expected behaviour is incomplete or still undergoing review.
- **Policy:** Unverified cases may be retained for diagnostic inspection but cannot be used as authoritative conformance benchmarks.
- **Automated Promotion Prohibition:** An `unverified` case **cannot** be promoted to `verified_ledger_sample` or `reproduced_regression` merely because an automated test or replay job ran without errors. Promotion requires explicit manual review of the origin, hashes, and source records.

---

## 3. Provenance and Immutability Rules

1. **Original Bytes are Immutable:**
   The original binary object is hashed (SHA-256) and stored in its authentic, byte-for-byte representation before any parsing, normalization, or decoding occurs.
2. **Hash Integrity:**
   The SHA-256 digest is always recomputed from the original bytes. A supplied hash in a manifest is treated as an assertion to be verified, never as an authoritative truth.
3. **Exact Numeric Representation:**
   Ledger sequences, transaction counts, and protocol versions are bounded integers (`uint32` max `4,294,967,295`). They must never undergo floating-point coercion or precision loss.
4. **Safe Canonical Object Paths:**
   `input.object_path` must be a canonical relative path without parent directory traversals (`..`), backslashes, or absolute filesystem prefixes.
5. **No Coercion or Silent Defaulting:**
   Unknown ledger sequences, protocol versions, or software versions must be recorded as `null`, never guessed or populated with synthetic defaults.
6. **Execution Status vs Conformance Outcome:**
   An operational failure (such as an adapter crash, timeout, or missing file) is an execution error (`failed`), not a conformance regression (`regression`). Conversely, a run that successfully completes and discovers an SDK mismatch completes with status `completed` and outcome `regression`.

---

## 4. Promotion and Review Workflow

| Current State | Target State | Permitted By | Requirements |
|---|---|---|---|
| `unverified` | `synthetic_fixture` | Reviewer | Documented construction and purpose; `synthetic_construction` kind |
| `unverified` | `historical_regression` | Reviewer | Documented issue/PR citations and expected behaviour |
| `historical_regression` | `reproduced_regression` | Workbench execution | Captured differential evidence bundle with affected/fixed version records and SHA-256 hashes |
| `unverified` | `verified_ledger_sample` | Reviewer | Independent verification of source archive, cryptographic hashes, and closed ledger sequence |
| Any | `unverified` | Reviewer / Author | Source contested, corrupted, or under investigation |
