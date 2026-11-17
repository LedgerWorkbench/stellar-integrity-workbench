// Package schemas contains and embeds the canonical JSON Schema definitions
// for Stellar Integrity Workbench.
package schemas

import _ "embed"

// CorpusManifestSchemaJSON contains the canonical JSON Schema (Draft 2020-12)
// for corpus manifest validation (schema version 1.0.0).
//
//go:embed corpus-manifest.schema.json
var CorpusManifestSchemaJSON []byte
