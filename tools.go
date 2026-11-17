//go:build tools

// Package tools pins module dependencies that are required by the project
// but not yet directly imported by application code in the current build
// slice.
//
// The official Stellar Go SDK is the primary ingestion and XDR dependency.
// It is imported here to keep it pinned in go.mod and go.sum while the
// command entry points are skeleton implementations. The direct import
// moves into the go-ingest adapter (cmd/go-ingest-adapter) and internal
// packages in Unit 5.
//
// This file is excluded from normal builds by the "tools" build tag.
package tools

import (
	// Pin the official Stellar Go SDK at the version selected for this phase.
	_ "github.com/stellar/go-stellar-sdk/ingest"
	_ "github.com/stellar/go-stellar-sdk/xdr"
)
