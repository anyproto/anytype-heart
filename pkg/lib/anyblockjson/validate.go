package anyblockjson

// validate.go implements §12: schema validation against the embedded JSON
// Schema (draft 2020-12) plus the semantic checks the schema cannot express,
// all reported as path-addressed issues.

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

//go:embed schema/object.schema.json
var schemaJSON []byte

const (
	// FormatVersion is the AnyBlock JSON format version this package reads
	// and writes (§10).
	FormatVersion = 1
	// SchemaURL is the published schema location written into exported
	// documents.
	SchemaURL = "https://schemas.anytype.io/anyblock/1.0/object.schema.json"
)

// Issue is a single path-addressed validation problem.
type Issue struct {
	Path    string // JSON pointer into the document, "" for the root
	Message string
}

func (i Issue) String() string {
	if i.Path == "" {
		return i.Message
	}
	return i.Path + ": " + i.Message
}

// ValidationError aggregates every issue found in a document (§12).
type ValidationError struct {
	Issues []Issue
	// NewerFormat is set when the document cites a newer 1.x schema, so the
	// failure likely means "produced by a newer version" (§10).
	NewerFormat bool
}

func (e *ValidationError) Error() string {
	var b strings.Builder
	if e.NewerFormat {
		b.WriteString("document was produced by a newer version of the AnyBlock format; ")
	}
	b.WriteString("validation failed")
	for _, i := range e.Issues {
		b.WriteString("\n  ")
		b.WriteString(i.String())
	}
	return b.String()
}

var compileSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaJSON))
	if err != nil {
		return nil, fmt.Errorf("decode embedded schema: %w", err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(SchemaURL, doc); err != nil {
		return nil, fmt.Errorf("add schema resource: %w", err)
	}
	sch, err := c.Compile(SchemaURL)
	if err != nil {
		return nil, fmt.Errorf("compile schema: %w", err)
	}
	return sch, nil
})

// Validate checks data against the embedded schema and the semantic rules
// without building a snapshot (§12).
func Validate(data []byte) error {
	_, err := validateToDoc(data)
	return err
}

// validateToDoc runs the full validation pipeline and returns the decoded
// document for the importer to consume.
func validateToDoc(data []byte) (map[string]any, error) {
	raw, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, &ValidationError{Issues: []Issue{{Message: fmt.Sprintf("invalid JSON: %v", err)}}}
	}
	doc, ok := raw.(map[string]any)
	if !ok {
		return nil, &ValidationError{Issues: []Issue{{Message: "document must be a JSON object"}}}
	}
	if err := checkVersion(doc); err != nil {
		return nil, err
	}
	newer := citesNewerSchema(doc)

	sch, err := compileSchema()
	if err != nil {
		return nil, fmt.Errorf("embedded schema: %w", err)
	}
	if err := sch.Validate(doc); err != nil {
		ve := &ValidationError{NewerFormat: newer}
		var flatten func(e *jsonschema.ValidationError)
		printer := message.NewPrinter(language.English)
		flatten = func(e *jsonschema.ValidationError) {
			if len(e.Causes) == 0 {
				ve.Issues = append(ve.Issues, Issue{
					Path:    jsonPath(e.InstanceLocation),
					Message: e.ErrorKind.LocalizedString(printer),
				})
				return
			}
			for _, c := range e.Causes {
				flatten(c)
			}
		}
		if verr, ok := err.(*jsonschema.ValidationError); ok {
			flatten(verr)
		} else {
			ve.Issues = append(ve.Issues, Issue{Message: err.Error()})
		}
		return nil, ve
	}

	if issues := semanticIssues(doc); len(issues) > 0 {
		return nil, &ValidationError{Issues: issues, NewerFormat: newer}
	}
	return doc, nil
}

// checkVersion rejects unsupported versions with a dedicated error naming
// both versions (§10), before schema validation gets a chance to produce a
// generic constraint failure.
func checkVersion(doc map[string]any) error {
	raw, ok := doc["version"]
	if !ok {
		return &ValidationError{Issues: []Issue{{Path: "/version", Message: "version is required"}}}
	}
	num, ok := raw.(json.Number)
	if !ok {
		return &ValidationError{Issues: []Issue{{Path: "/version", Message: "version must be an integer"}}}
	}
	v, err := num.Int64()
	if err != nil {
		// accept integer-valued floats like 1.0, matching JSON Schema
		// numeric equality
		f, ferr := num.Float64()
		if ferr != nil || f != math.Trunc(f) {
			return &ValidationError{Issues: []Issue{{Path: "/version", Message: "version must be an integer"}}}
		}
		v = int64(f)
	}
	if v > FormatVersion {
		return &ValidationError{
			NewerFormat: true,
			Issues: []Issue{{
				Path:    "/version",
				Message: fmt.Sprintf("document version %d is newer than the supported version %d", v, FormatVersion),
			}},
		}
	}
	if v < 1 {
		return &ValidationError{Issues: []Issue{{Path: "/version", Message: fmt.Sprintf("unknown version %d", v)}}}
	}
	return nil
}

var schemaMinorRe = regexp.MustCompile(`anyblock/1\.(\d+)/object\.schema\.json$`)

func citesNewerSchema(doc map[string]any) bool {
	url, _ := doc["$schema"].(string)
	m := schemaMinorRe.FindStringSubmatch(url)
	if m == nil {
		return false
	}
	minor, err := strconv.Atoi(m[1])
	return err == nil && minor > 0
}

func jsonPath(tokens []string) string {
	if len(tokens) == 0 {
		return ""
	}
	return "/" + strings.Join(tokens, "/")
}

// textBearing reports whether the block type's text is parsed for inline
// markup; code/embed text is literal (§8.4).
func textBearing(typ string) bool {
	switch typ {
	case "paragraph", "heading1", "heading2", "heading3", "heading4", "header4",
		"quote", "checkbox", "bulletedListItem", "numberedListItem", "toggle",
		"callout", "toggleHeading1", "toggleHeading2", "toggleHeading3",
		"title", "description":
		return true
	}
	return false
}

// semanticIssues runs the checks the schema cannot express: envelope
// combinations, id uniqueness over the flattened tree including derived
// table cell ids, table arity, language-vs-fields.lang conflicts, and inline
// markup grammar (§12).
func semanticIssues(doc map[string]any) []Issue {
	var issues []Issue
	addIssue := func(path, format string, args ...any) {
		issues = append(issues, Issue{Path: path, Message: fmt.Sprintf(format, args...)})
	}

	if _, ok := doc["templateFor"]; ok {
		if typ, _ := doc["type"].(string); typ != "template" {
			addIssue("/templateFor", `templateFor is only valid on templates (type "template")`)
		}
	}

	seenIds := map[string]string{} // id -> path of first occurrence
	claimId := func(id, path string) {
		if id == "" {
			return
		}
		if first, dup := seenIds[id]; dup {
			addIssue(path, "duplicate id %q (first used at %s)", id, first)
		} else {
			seenIds[id] = path
		}
	}

	checkText := func(block map[string]any, path string) {
		typ, _ := block["type"].(string)
		if !textBearing(typ) {
			return
		}
		text, _ := block["text"].(string)
		if text == "" {
			return
		}
		if _, _, err := ParseInline(text); err != nil {
			addIssue(path+"/text", "inline markup: %v", err)
		}
	}

	var walkBlock func(block map[string]any, path string)
	walkBlock = func(block map[string]any, path string) {
		typ, _ := block["type"].(string)
		if id, _ := block["id"].(string); id != "" {
			claimId(id, path+"/id")
		}
		checkText(block, path)
		if typ == "code" {
			if _, hasLang := block["language"]; hasLang {
				if fields, _ := block["fields"].(map[string]any); fields != nil {
					if _, conflict := fields["lang"]; conflict {
						addIssue(path, "language and fields.lang are both set")
					}
				}
			}
		}
		if typ == "table" {
			walkTable(block, path, claimId, addIssue, &walkBlock)
		}
		if children, _ := block["children"].([]any); children != nil {
			for i, c := range children {
				if cb, ok := c.(map[string]any); ok {
					walkBlock(cb, fmt.Sprintf("%s/children/%d", path, i))
				}
			}
		}
	}

	if blocks, _ := doc["blocks"].([]any); blocks != nil {
		for i, b := range blocks {
			if bb, ok := b.(map[string]any); ok {
				walkBlock(bb, fmt.Sprintf("/blocks/%d", i))
			}
		}
	}
	return issues
}

func walkTable(block map[string]any, path string,
	claimId func(id, path string), addIssue func(path, format string, args ...any),
	walkBlock *func(block map[string]any, path string)) {

	columns, _ := block["columns"].([]any)
	var colIds []string
	for i, c := range columns {
		col, _ := c.(map[string]any)
		id, _ := col["id"].(string)
		colIds = append(colIds, id)
		if id != "" {
			claimId(id, fmt.Sprintf("%s/columns/%d/id", path, i))
		}
	}
	rows, _ := block["rows"].([]any)
	for i, r := range rows {
		row, _ := r.(map[string]any)
		rowPath := fmt.Sprintf("%s/rows/%d", path, i)
		rowId, _ := row["id"].(string)
		if rowId != "" {
			claimId(rowId, rowPath+"/id")
		}
		cells, _ := row["cells"].([]any)
		if len(cells) > len(columns) {
			addIssue(rowPath+"/cells", "row has %d cells but the table has %d columns", len(cells), len(columns))
		}
		for j, c := range cells {
			cellPath := fmt.Sprintf("%s/cells/%d", rowPath, j)
			// derived cell ids join the uniqueness domain (§4)
			if rowId != "" && j < len(colIds) && colIds[j] != "" {
				claimId(rowId+"-"+colIds[j], cellPath)
			}
			switch cell := c.(type) {
			case string:
				if cell != "" {
					if _, _, err := ParseInline(cell); err != nil {
						addIssue(cellPath, "inline markup: %v", err)
					}
				}
			case map[string]any:
				// a full walk: nested tables and children join the id
				// uniqueness domain and get their text checked
				(*walkBlock)(cell, cellPath)
			}
		}
	}
}
