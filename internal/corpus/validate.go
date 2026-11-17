package corpus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/domain"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/schemas"
)

var (
	compiledSchemaOnce sync.Once
	compiledSchema     *jsonschema.Schema
	compiledSchemaErr  error
)

// SchemaResourceName is the local schema identifier used for in-memory compilation.
const SchemaResourceName = "corpus-manifest.schema.json"

// getSchema returns the precompiled in-memory corpus manifest JSON Schema (Draft 2020-12).
// No remote network calls or external downloads are performed.
func getSchema() (*jsonschema.Schema, error) {
	compiledSchemaOnce.Do(func() {
		compiler := jsonschema.NewCompiler()
		// Enable format assertions (URI, date-time, etc.) for Draft 2020-12.
		compiler.AssertFormat()

		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemas.CorpusManifestSchemaJSON))
		if err != nil {
			compiledSchemaErr = fmt.Errorf("failed to parse embedded corpus manifest schema: %w", err)
			return
		}

		if err := compiler.AddResource(SchemaResourceName, doc); err != nil {
			compiledSchemaErr = fmt.Errorf("failed to register schema resource: %w", err)
			return
		}

		compiledSchema, compiledSchemaErr = compiler.Compile(SchemaResourceName)
		if compiledSchemaErr != nil {
			compiledSchemaErr = fmt.Errorf("failed to compile corpus manifest schema: %w", compiledSchemaErr)
		}
	})

	return compiledSchema, compiledSchemaErr
}

// FieldError represents a single deterministic validation error at a specific JSON pointer path.
type FieldError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// ValidationError aggregates one or more normalized, sorted validation errors.
type ValidationError struct {
	Errors []FieldError `json:"errors"`
}

func (v *ValidationError) Error() string {
	if len(v.Errors) == 0 {
		return "corpus manifest validation failed"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("corpus manifest validation failed with %d error(s):\n", len(v.Errors)))
	for _, e := range v.Errors {
		sb.WriteString(fmt.Sprintf("  - %s: %s\n", e.Path, e.Message))
	}
	return strings.TrimRight(sb.String(), "\n")
}

// ValidateManifestBytes validates raw manifest JSON bytes against the v1 JSON Schema,
// verifies schema version compatibility, rejects unknown fields, and deserializes
// into typed domain structures. The original input slice is not modified.
func ValidateManifestBytes(data []byte) (*domain.Case, *Manifest, error) {
	schema, err := getSchema()
	if err != nil {
		return nil, nil, fmt.Errorf("schema compiler error: %w", err)
	}

	// 1. Verify single complete JSON document and decode generic document without losing number precision
	docDecoder := json.NewDecoder(bytes.NewReader(data))
	docDecoder.UseNumber()

	var doc any
	if err := docDecoder.Decode(&doc); err != nil {
		return nil, nil, fmt.Errorf("invalid manifest JSON: %w", err)
	}

	// Reject trailing second JSON document or trailing non-whitespace data
	var trailing any
	if err := docDecoder.Decode(&trailing); err != io.EOF {
		return nil, nil, fmt.Errorf("invalid manifest JSON: expected exactly one JSON document, found trailing data")
	}

	// 2. Validate against Draft 2020-12 JSON Schema
	if valErr := schema.Validate(doc); valErr != nil {
		return nil, nil, normalizeValidationError(valErr)
	}

	// 3. Deserialize with unknown field rejection
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()

	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return nil, nil, fmt.Errorf("deserialization error: %w", err)
	}

	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, nil, fmt.Errorf("deserialization error: expected exactly one JSON document, found trailing data")
	}

	// 4. Validate major schema version
	if !strings.HasPrefix(manifest.SchemaVersion, "1.") {
		return nil, nil, fmt.Errorf("unsupported major schema version %q; only 1.x.y is supported", manifest.SchemaVersion)
	}

	// 5. Validate object path safety
	if err := ValidateObjectPath(manifest.Input.ObjectPath); err != nil {
		return nil, nil, fmt.Errorf("invalid input.object_path: %w", err)
	}

	// 6. Convert to domain case
	domainCase, err := manifest.ToDomain()
	if err != nil {
		return nil, nil, fmt.Errorf("manifest to domain conversion error: %w", err)
	}

	return domainCase, &manifest, nil
}

// normalizeValidationError extracts and flattens errors from jsonschema.ValidationError,
// removing duplicates and sorting deterministically by path and message.
func normalizeValidationError(err error) error {
	ve, ok := err.(*jsonschema.ValidationError)
	if !ok {
		return err
	}

	basic := ve.BasicOutput()
	var fieldErrors []FieldError
	seen := make(map[string]bool)

	var collect func(u jsonschema.OutputUnit)
	collect = func(u jsonschema.OutputUnit) {
		if u.Error != nil {
			path := u.InstanceLocation
			if path == "" {
				path = "/"
			}
			msg := u.Error.String()
			key := path + "::" + msg
			if !seen[key] {
				seen[key] = true
				fieldErrors = append(fieldErrors, FieldError{
					Path:    path,
					Message: msg,
				})
			}
		}
		for _, child := range u.Errors {
			collect(child)
		}
	}

	for _, unit := range basic.Errors {
		collect(unit)
	}

	if len(fieldErrors) == 0 {
		fieldErrors = append(fieldErrors, FieldError{
			Path:    "/",
			Message: ve.Error(),
		})
	}

	// Sort deterministically by Path then Message
	sort.Slice(fieldErrors, func(i, j int) bool {
		if fieldErrors[i].Path != fieldErrors[j].Path {
			return fieldErrors[i].Path < fieldErrors[j].Path
		}
		return fieldErrors[i].Message < fieldErrors[j].Message
	})

	return &ValidationError{Errors: fieldErrors}
}
