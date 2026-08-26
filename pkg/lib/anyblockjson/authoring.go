package anyblockjson

// authoring.go implements the authoring subset (§2g): three schemas under
// schema/authoring/ that narrow the full grammar to exactly what an author —
// increasingly an LLM agent — composes when generating a use case from
// nothing. The subset is the same format at the same version: every document
// valid under an authoring schema is valid under the corresponding full
// schema, an invariant the tests enforce rather than claim.

import (
	"bytes"
	_ "embed"
	"fmt"
	"strconv"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed schema/authoring/object.schema.json
var authoringSchemaJSON []byte

//go:embed schema/authoring/index.schema.json
var authoringIndexSchemaJSON []byte

//go:embed schema/authoring/properties.schema.json
var authoringPropertiesSchemaJSON []byte

// AuthoringSchemaJSON returns the embedded authoring object schema (§2g).
// Callers must not mutate the returned slice; discovery surfaces serve it
// verbatim, the way SchemaJSON is served.
func AuthoringSchemaJSON() []byte { return authoringSchemaJSON }

// AuthoringIndexSchemaJSON returns the embedded authoring index schema (§2g).
func AuthoringIndexSchemaJSON() []byte { return authoringIndexSchemaJSON }

// AuthoringPropertiesSchemaJSON returns the embedded authoring property
// dictionary schema (§2g).
func AuthoringPropertiesSchemaJSON() []byte { return authoringPropertiesSchemaJSON }

// The published authoring schema locations (§2g): the full schemas'
// directory plus an authoring/ segment, so the version travels with
// FormatVersion exactly as the full URLs do — and the trailing file names
// stay object|index|properties.schema.json, which is what DocumentKind
// dispatches on, so an authored document declaring one routes to the same
// reader an exported one does.
var (
	AuthoringSchemaURL           = schemaBaseURL + strconv.Itoa(FormatVersion) + "/authoring/object.schema.json"
	AuthoringIndexSchemaURL      = schemaBaseURL + strconv.Itoa(FormatVersion) + "/authoring/index.schema.json"
	AuthoringPropertiesSchemaURL = schemaBaseURL + strconv.Itoa(FormatVersion) + "/authoring/properties.schema.json"
)

// The three authoring schemas are self-contained on purpose — no $ref
// crosses a file — so each compiles from its own bytes alone and an agent
// handed one file has the whole grammar for that surface.
var compileAuthoringSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileStandalone(AuthoringSchemaURL, authoringSchemaJSON)
})

var compileAuthoringIndexSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileStandalone(AuthoringIndexSchemaURL, authoringIndexSchemaJSON)
})

var compileAuthoringPropertiesSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileStandalone(AuthoringPropertiesSchemaURL, authoringPropertiesSchemaJSON)
})

func compileStandalone(url string, schemaBytes []byte) (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaBytes))
	if err != nil {
		return nil, fmt.Errorf("decode embedded schema %s: %w", url, err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(url, doc); err != nil {
		return nil, fmt.Errorf("add schema resource %s: %w", url, err)
	}
	sch, err := c.Compile(url)
	if err != nil {
		return nil, fmt.Errorf("compile schema %s: %w", url, err)
	}
	return sch, nil
}

// ValidateAuthoring checks an object document against the authoring subset
// (§2g): the FULL validation first — schema and semantic rules, whose
// refusals carry the curated §12 wording — and then the authoring schema, so
// a valid document that reaches outside the subset is told so as a subset
// verdict rather than a format one. A nil return therefore means the
// document is valid AnyBlock JSON, not merely subset-shaped.
func ValidateAuthoring(data []byte) error {
	if err := Validate(data); err != nil {
		return err
	}
	return validateAuthoringSubset(data, compileAuthoringSchema,
		"valid AnyBlock JSON, but outside the authoring subset (§2g) — the members below are export's, not an author's")
}

// ValidateAuthoringIndex is ValidateAuthoring for a bundle index (§2c, §2g).
func ValidateAuthoringIndex(data []byte) error {
	if _, err := UnmarshalIndex(data); err != nil {
		return err
	}
	return validateAuthoringSubset(data, compileAuthoringIndexSchema,
		"a valid bundle index, but outside the authoring subset (§2g)")
}

// ValidateAuthoringPropertyDictionary is ValidateAuthoring for a property
// dictionary (§2f, §2g).
func ValidateAuthoringPropertyDictionary(data []byte) error {
	if _, err := UnmarshalPropertyDictionary(data); err != nil {
		return err
	}
	return validateAuthoringSubset(data, compileAuthoringPropertiesSchema,
		"a valid property dictionary, but outside the authoring subset (§2g)")
}

// validateAuthoringSubset runs one authoring schema over a document the full
// reader has already accepted. The preamble issue names the verdict's
// nature: everything below it is the SUBSET refusing a member or a value,
// never the format — the full validation already passed.
func validateAuthoringSubset(data []byte, compile func() (*jsonschema.Schema, error), preamble string) error {
	raw, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return &ValidationError{Issues: []Issue{{Message: fmt.Sprintf("invalid JSON: %v", err)}}}
	}
	sch, err := compile()
	if err != nil {
		return fmt.Errorf("embedded authoring schema: %w", err)
	}
	if err := sch.Validate(raw); err != nil {
		issues := append([]Issue{{Message: preamble}}, schemaIssues(err, keySlotReport{})...)
		return &ValidationError{Issues: issues}
	}
	return nil
}
