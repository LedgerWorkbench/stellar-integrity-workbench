package adapter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/schemas"
)

// compiledRequestSchema and compiledResultSchema hold the compiled JSON Schemas
// for adapter request and result validation. They are initialized by Init().
var (
	compiledRequestSchema *jsonschema.Schema
	compiledResultSchema  *jsonschema.Schema
)

// Init compiles and validates the adapter request and result JSON schemas.
// It must be called before using SchemaValidateRequest or SchemaValidateResult.
// Panics if the schemas cannot be compiled (programming error).
func Init() {
	var err error
	compiledRequestSchema, err = compileSchema("adapter-request.schema.json", schemas.AdapterRequestSchemaJSON)
	if err != nil {
		panic(fmt.Sprintf("adapter: failed to compile request schema: %v", err))
	}
	compiledResultSchema, err = compileSchema("adapter-result.schema.json", schemas.AdapterResultSchemaJSON)
	if err != nil {
		panic(fmt.Sprintf("adapter: failed to compile result schema: %v", err))
	}
}

func compileSchema(name string, data []byte) (*jsonschema.Schema, error) {
	c := jsonschema.NewCompiler()

	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to parse schema %s: %w", name, err)
	}

	if err := c.AddResource(name, doc); err != nil {
		return nil, fmt.Errorf("failed to add schema resource %s: %w", name, err)
	}
	sch, err := c.Compile(name)
	if err != nil {
		return nil, fmt.Errorf("failed to compile schema %s: %w", name, err)
	}
	return sch, nil
}

// SchemaValidateRequest validates raw JSON bytes against the adapter request schema.
func SchemaValidateRequest(data []byte) error {
	if compiledRequestSchema == nil {
		return fmt.Errorf("adapter request schema not initialized; call adapter.Init() first")
	}
	return validateAgainstSchema(compiledRequestSchema, data, "request")
}

// SchemaValidateResult validates raw JSON bytes against the adapter result schema.
func SchemaValidateResult(data []byte) error {
	if compiledResultSchema == nil {
		return fmt.Errorf("adapter result schema not initialized; call adapter.Init() first")
	}
	return validateAgainstSchema(compiledResultSchema, data, "result")
}

func validateAgainstSchema(sch *jsonschema.Schema, data []byte, kind string) error {
	var v any
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return fmt.Errorf("invalid %s JSON: %w", kind, err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("invalid %s JSON: trailing JSON value after document", kind)
		}
		return fmt.Errorf("invalid %s JSON: trailing data after document: %w", kind, err)
	}
	if err := sch.Validate(v); err != nil {
		return fmt.Errorf("adapter %s schema validation failed: %w", kind, err)
	}
	return nil
}

// CompileRequestSchema compiles and returns the adapter request JSON Schema.
// This is primarily useful for tests that need to verify schema compilation.
func CompileRequestSchema() (*jsonschema.Schema, error) {
	return compileSchema("adapter-request.schema.json", schemas.AdapterRequestSchemaJSON)
}

// CompileResultSchema compiles and returns the adapter result JSON Schema.
// This is primarily useful for tests that need to verify schema compilation.
func CompileResultSchema() (*jsonschema.Schema, error) {
	return compileSchema("adapter-result.schema.json", schemas.AdapterResultSchemaJSON)
}
