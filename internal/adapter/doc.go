// Package adapter implements the versioned adapter process protocol for the
// Stellar Integrity Workbench. It defines typed request and result structures,
// JSON schema validation, a production adapter registry, and a bounded
// allowlisted process runner.
//
// # Protocol Version
//
// The current protocol version is "1". All requests and results must carry
// schema_version "1". Unsupported versions are rejected.
//
// # Communication Contract
//
// Adapters communicate over JSON stdin/stdout:
//   - Exactly one JSON request is written to the adapter's stdin.
//   - Exactly one JSON result is read from the adapter's stdout.
//   - Diagnostics are written to stderr.
//   - Stdout must contain no banners, progress, or log output.
//
// # Security Model
//
// The process runner enforces:
//   - Allowlisted executables only (no shell, no user-supplied paths).
//   - Per-run temporary directories with restrictive permissions.
//   - Input staging from verified bytes (not arbitrary paths).
//   - Bounded stdout and stderr capture.
//   - Hard timeout and cancellation propagation.
//   - Minimal environment (no inherited credentials).
//   - Identity and integrity verification of all responses.
//
// # Limitations
//
// The process runner does not provide OS-level container isolation, memory
// cgroups, or process-tree termination guarantees on all platforms.
// See process_runner.go for platform-specific documentation.
package adapter
