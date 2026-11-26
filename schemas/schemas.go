// Package schemas contains and embeds the canonical JSON Schema definitions
// for Stellar Integrity Workbench.
package schemas

import _ "embed"

// CorpusManifestSchemaJSON contains the canonical JSON Schema (Draft 2020-12)
// for corpus manifest validation (schema version 1.0.0).
//
//go:embed corpus-manifest.schema.json
var CorpusManifestSchemaJSON []byte

// AdapterRequestSchemaJSON contains the canonical JSON Schema (Draft 2020-12)
// for adapter process protocol v1 request validation.
//
//go:embed adapter-request.schema.json
var AdapterRequestSchemaJSON []byte

// AdapterResultSchemaJSON contains the canonical JSON Schema (Draft 2020-12)
// for adapter process protocol v1 result validation.
//
//go:embed adapter-result.schema.json
var AdapterResultSchemaJSON []byte
