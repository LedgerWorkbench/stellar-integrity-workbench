# Reporting Architecture (Unit 8)

Package: `internal/reporting` (`json.go`, `markdown.go`)

## 1. Purpose and Canonical Input Contract

`internal/reporting` renders deterministic JSON and Markdown reports solely from a validated `*evidence.Manifest` (Unit 7). Renderers do not accept separate `domain.Run`, `adapter.Result`, or `BuildInput` values, and they do not write files or create directories.

Public API:

```go
func RenderJSON(manifest *evidence.Manifest) ([]byte, error)
func RenderMarkdown(manifest *evidence.Manifest) ([]byte, error)
```

## 2. JSON Report Format

`RenderJSON` serializes the complete `*evidence.Manifest` using deterministic 2-space indentation with a trailing newline and verifies the encoded bytes via `evidence.DecodeAndValidateManifest` against `schemas/evidence-bundle.schema.json`. Reusing the versioned evidence manifest schema (`1.0.0`) guarantees that JSON reports cannot drift from `manifest.json`, preserves exact integer values (including `4294967295`), preserves `null` vs omitted fields, and avoids nondeterministic report-generation timestamps.

## 3. Markdown Report Sections

`RenderMarkdown` validates the manifest through both `evidence.ValidateManifest` and `evidence.DecodeAndValidateManifest` before rendering the following sections in fixed order:

1. **Report Identity** (schema version, run ID, case ID, classification, complete source provenance: kind, URL, repository, issue number, pull request number, retrieval timestamp, notes — missing optional fields rendered as `not recorded`)
2. **Execution** (run state, UTC start/completion timestamps, host-observed exit code/duration, adapter-reported status/exit code/duration)
3. **Conformance** (conformance outcome and explicit interpretation)
4. **Input Integrity** (format, encoding, SHA-256, byte length, corpus-relative `object_path`, detected type, network name, ledger sequence, protocol version)
5. **Adapter and Worker** (adapter ID/version, SDK version, worker version, commit SHA)
6. **Assertions** (ordered table of ID, kind, expected, observed, status, explanation, evidence references)
7. **Diagnostics and Limitations** (stderr byte count/truncation, adapter diagnostics, limitations)
8. **Evidence Artifacts** (relative path, role, media type, byte length, SHA-256)

## 4. Execution State vs Conformance Outcome

| Execution State | Conformance Outcome | Rendered Interpretation |
|---|---|---|
| `completed` | `pass` | Execution completed and assertions passed. |
| `completed` | `regression` | Execution completed and one or more assertions failed. |
| `completed` | `inconclusive` | Execution completed but required assertions could not all be evaluated. |
| `completed` | `not_applicable` | The required capability was not applicable. |
| `failed` | `null` | Execution failed; no completed conformance outcome is recorded. |
| `cancelled` | `null` | Execution was cancelled; no completed conformance outcome is recorded. |

For `failed` or `cancelled` runs, any recorded assertions are labeled `Partial Results — No Conformance Outcome Derived`, and no conformance outcome is derived from them.

## 5. Missing Values, Untrusted Input Escaping, Sensitive-Content Protection, and Determinism

- Optional nil fields render explicitly as `not recorded` (or `unavailable` for missing observed assertion values); no placeholder metrics or fake transaction counts are invented.
- All manifest strings are treated as untrusted input: `escapeMarkdown` replaces newlines/tabs/control characters and backslash-escapes Markdown-sensitive characters (`\`, `` ` ``, `*`, `_`, `{`, `}`, `[`, `]`, `(`, `)`, `#`, `+`, `-`, `!`, `|`, `<`, `>`, `~`). Provenance URLs are rendered as escaped text rather than active links.
- **Shared sensitive-content validation (`validateReportSafety`):** Both `RenderJSON` and `RenderMarkdown` run the same pre-render safety check *before* `evidence.ValidateManifest` (so invalid fields containing sensitive values—such as a local path inside `run.started_at`—fail closed with `ErrUnsafeReportContent` rather than echoing the offending value in a manifest-validation error). `validateReportSafety` marshals the manifest to its canonical JSON representation, decodes it with `json.Decoder.UseNumber()`, and recursively inspects every object key, nested array, nested object, and string value (covering arbitrary Go containers in assertion `expected`/`observed` fields) as well as credential-like object properties with non-empty values. It fails closed with `ErrUnsafeReportContent` (reporting only the field path and category, never the detected secret value) without mutating the manifest or silently masking/rewriting JSON fields. Rejected categories:
  - Unix absolute local filesystem paths (`/home/...`, `/tmp/...`, `/var/...`, `/etc/...`, `/Users/...`, `/mnt/...`, `/workspace/...`, etc., and `file://` URIs), while keeping normal public `https://` URLs and corpus-relative `object_path` values valid.
  - Windows drive-letter absolute paths (`C:\...`) and UNC paths (`\\server\share\...`).
  - Bearer authorization credentials (`Bearer ...`).
  - Common credential assignments (`api_key`, `access_token`, `refresh_token`, `password`, `passwd`, `secret`, `private_key`, `client_secret`, `auth_token`).
  - PEM private-key headers (`-----BEGIN ... PRIVATE KEY-----`).
  - Stellar secret-key encodings (`S[A-Z2-7]{55}`).
  - HTTP(S) URLs containing userinfo credentials (`user:pass@host`) or sensitive query parameters (`access_token`, `api_key`, `password`, `sig`, `X-Amz-Signature`, `X-Amz-Credential`, `X-Amz-Security-Token`, etc.).
- **Detection limitations:** Pattern-based detection is a defense-in-depth safeguard for recognizable sensitive content, not a substitute for upstream secret management or a guarantee that every possible secret format can be identified.
- Both `RenderJSON` and `RenderMarkdown` are pure, deterministic functions over `*evidence.Manifest` and never mutate their input.

## 6. Deferred Work (Unit 9)

- Connecting `RenderJSON` and `RenderMarkdown` to `workbench-worker` CLI flags and output file writing belongs to Unit 9.
