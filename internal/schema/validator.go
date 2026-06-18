// Package schema validates serialized sensor envelopes against the embedded
// JSON Schema (Draft 2020-12) contracts. It is the Go equivalent of the C#
// SchemaValidator: validate the envelope schema first, then the payload schema
// matching sensor_type.
package schema

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Validator holds the compiled envelope and per-sensor-type payload schemas.
type Validator struct {
	envelope *jsonschema.Schema
	payloads map[string]*jsonschema.Schema
}

var payloadSchemaFiles = map[string]string{
	"gnss":              "payload-gnss.schema.json",
	"rainfall":          "payload-rainfall.schema.json",
	"strain":            "payload-strain.schema.json",
	"soil_pressure":     "payload-soil-pressure.schema.json",
	"deep_displacement": "payload-deep-displacement.schema.json",
}

// New compiles the embedded schemas. It panics on failure: the schemas are
// embedded build-time constants, so a compile error is a programming bug, not a
// runtime condition (mirrors the C# constructor that throws).
func New() *Validator {
	c := jsonschema.NewCompiler()
	// Note: like the C# default EvaluationOptions, format keywords (uuid,
	// date-time) are treated as annotations only — not asserted. The decoders
	// already guarantee well-formed values, and asserting here would diverge
	// from the baseline. AssertFormat is intentionally left off.

	for name, file := range schemaFiles() {
		doc, err := loadJSON(file)
		if err != nil {
			panic(fmt.Sprintf("schema: load %s: %v", file, err))
		}
		if err := c.AddResource(resourceURL(name), doc); err != nil {
			panic(fmt.Sprintf("schema: add resource %s: %v", file, err))
		}
	}

	v := &Validator{payloads: make(map[string]*jsonschema.Schema, len(payloadSchemaFiles))}
	v.envelope = mustCompile(c, "envelope.schema.json")
	for sensorType, file := range payloadSchemaFiles {
		v.payloads[sensorType] = mustCompile(c, file)
	}
	return v
}

// Validate checks envelopeJSON against the envelope schema and, if sensorType is
// known, the payload object against its schema. Returns (true, "") on success or
// (false, message) describing the first failing schema, matching C#.
func (v *Validator) Validate(envelopeJSON string, sensorType string) (bool, string) {
	var node any
	if err := json.Unmarshal([]byte(envelopeJSON), &node); err != nil {
		return false, "Failed to parse envelope JSON"
	}

	if err := v.envelope.Validate(node); err != nil {
		return false, formatErr(err)
	}

	payloadSchema, ok := v.payloads[sensorType]
	if !ok {
		return true, ""
	}

	obj, ok := node.(map[string]any)
	if !ok {
		return false, "Missing 'payload' in envelope"
	}
	payloadNode, ok := obj["payload"]
	if !ok {
		return false, "Missing 'payload' in envelope"
	}
	if err := payloadSchema.Validate(payloadNode); err != nil {
		return false, formatErr(err)
	}

	return true, ""
}

func schemaFiles() map[string]string {
	files := map[string]string{"envelope.schema.json": "envelope.schema.json"}
	for _, f := range payloadSchemaFiles {
		files[f] = f
	}
	return files
}

// resourceURL must match each schema's own $id so the compiler resolves
// resources by the canonical URI declared inside the document.
func resourceURL(file string) string {
	return "https://landslide-monitoring/schemas/" + file
}

func loadJSON(file string) (any, error) {
	raw, err := schemaFS.ReadFile("schemas/" + file)
	if err != nil {
		return nil, err
	}
	return jsonschema.UnmarshalJSON(strings.NewReader(string(raw)))
}

func mustCompile(c *jsonschema.Compiler, file string) *jsonschema.Schema {
	s, err := c.Compile(resourceURL(file))
	if err != nil {
		panic(fmt.Sprintf("schema: compile %s: %v", file, err))
	}
	return s
}

func formatErr(err error) string {
	return err.Error()
}
