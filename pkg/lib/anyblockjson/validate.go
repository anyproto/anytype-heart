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

// DetectFormat reports the version and $schema markers of a document without
// validating or importing it — the cheap dispatch probe for import wiring
// (§13). ok is false when data is not a JSON object carrying an integer
// version.
func DetectFormat(data []byte) (version int, schemaURL string, ok bool) {
	var probe struct {
		Schema  string      `json:"$schema"`
		Version json.Number `json:"version"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return 0, "", false
	}
	v, err := probe.Version.Int64()
	if err != nil {
		f, ferr := probe.Version.Float64()
		if ferr != nil || f != math.Trunc(f) {
			return 0, "", false
		}
		v = int64(f)
	}
	return int(v), probe.Schema, true
}

// Validate checks data against the embedded schema and the semantic rules
// without building a snapshot (§12). Validate is always strict; the lenient
// indent mode exists only on Unmarshal (Options.NormalizeIndent).
func Validate(data []byte) error {
	_, err := validateToDoc(data, false, nil)
	return err
}

// validateToDoc runs the full validation pipeline and returns the decoded
// document for the importer to consume. With lenient set, over-deep indents
// are clamped instead of rejected, each clamp reported through warn (§4).
func validateToDoc(data []byte, lenient bool, warn func(Issue)) (map[string]any, error) {
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

	if issues := semanticIssues(doc, lenient, warn); len(issues) > 0 {
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

// leafBlockTypes are the block types that cannot be parents (V2) — the same
// list as the export side's withChildren = false sites and the editor's
// leaf blocks, plus the equation input alias.
var leafBlockTypes = map[string]bool{
	"embed": true, "equation": true, "bookmark": true, "link": true,
	"divider": true, "table": true, "property": true, "dataview": true,
	"icon": true, "tableOfContents": true, "featuredProperties": true,
	"chat": true,
}

// clampIndents applies the §4 lenient rule in place: an indent more than one
// deeper than its predecessor clamps to predecessor+1 (CommonMark's "a level
// that hasn't been established cannot be opened"); the first entry's
// predecessor is base. onClamp, when non-nil, receives each clamp.
func clampIndents(indents []int, base int, onClamp func(i, from, to int)) {
	prev := base
	for i, k := range indents {
		if k > prev+1 {
			if onClamp != nil {
				onClamp(i, k, prev+1)
			}
			k = prev + 1
			indents[i] = k
		}
		prev = k
	}
}

// indentOf reads a block's indent; absent means 0. The schema guarantees an
// integer in [0, 32] (V4) when present.
func indentOf(block map[string]any) int {
	raw, ok := block["indent"]
	if !ok {
		return 0
	}
	num, ok := raw.(json.Number)
	if !ok {
		return 0
	}
	v, err := num.Int64()
	if err != nil {
		return 0
	}
	return int(v)
}

// semanticIssues runs the checks the schema cannot express: envelope
// combinations, indent monotonicity and containment over the flat blocks
// array (V1–V3), id uniqueness over the flattened tree including derived
// table cell ids, table arity, language-vs-fields.lang conflicts, and inline
// markup grammar (§12). With lenient set, V1 violations clamp (reported via
// warn) instead of erroring; V2/V3 are evaluated on the clamped indents and
// stay errors.
func semanticIssues(doc map[string]any, lenient bool, warn func(Issue)) []Issue {
	var issues []Issue
	addIssue := func(path, format string, args ...any) {
		issues = append(issues, Issue{Path: path, Message: fmt.Sprintf(format, args...)})
	}
	warnIssue := func(path, format string, args ...any) {
		if warn != nil {
			warn(Issue{Path: path, Message: fmt.Sprintf(format, args...)})
		}
	}

	if _, ok := doc["templateFor"]; ok {
		if typ, _ := doc["type"].(string); typ != "template" {
			addIssue("/templateFor", `templateFor is only valid on templates (type "template")`)
		}
	}

	if _, ok := doc["typeProperties"]; ok {
		if kind, _ := doc["kind"].(string); kind != "objectType" {
			addIssue("/typeProperties", `typeProperties is only valid on type documents (kind "objectType")`)
		}
		// typeProperties replaces the recommended-relation lists (§2a): a
		// document carrying both is ambiguous
		if props, _ := doc["properties"].(map[string]any); props != nil {
			for _, l := range recommendedListKeys {
				if _, dup := props[l.detailKey]; dup {
					addIssue("/properties/"+l.detailKey, "conflicts with typeProperties, which replaces this list")
				}
			}
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
		if _, _, err := parseInline(text); err != nil {
			addIssue(path+"/text", "inline markup: %v", err)
		}
	}

	var checkFlatRun func(blocks []any, basePath string, inCell bool)
	var walkBlock func(block map[string]any, path string)
	walkBlock = func(block map[string]any, path string) {
		typ, _ := block["type"].(string)
		if id, _ := block["id"].(string); id != "" {
			claimId(id, path+"/id")
		}
		checkText(block, path)
		if typ == "code" && codeLangConflict(block) {
			addIssue(path, "language and fields.lang are both set")
		}
		if typ == "table" {
			walkTable(block, path, claimId, addIssue, walkBlock, checkFlatRun)
		}
	}

	// checkFlatRun validates one flat pre-order run (the document's blocks
	// array, or a table cell's array form): V1 monotonicity, V2 leaf
	// containment, V3 row→column, then the per-block checks. inCell bans an
	// id on the first element (cell ids are derived, §6.1).
	checkFlatRun = func(blocks []any, basePath string, inCell bool) {
		type frame struct {
			indent int
			typ    string
		}
		prev := -1
		var stack []frame
		for i, raw := range blocks {
			block, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			path := fmt.Sprintf("%s/%d", basePath, i)
			typ, _ := block["type"].(string)
			if inCell && i == 0 {
				if _, has := block["id"]; has {
					addIssue(path+"/id", "cell blocks cannot carry an id — cell ids are derived")
				}
			}
			k := indentOf(block)
			if k > prev+1 {
				// V1: continue with the clamped value either way so one bad
				// indent does not cascade into follow-on errors
				switch {
				case lenient && prev < 0:
					warnIssue(path, "indent %d on the first block — clamped to 0", k)
				case lenient:
					warnIssue(path, "indent %d follows indent %d — clamped to %d", k, prev, prev+1)
				case prev < 0:
					addIssue(path, "indent %d on the first block — the first block must be at indent 0", k)
				default:
					addIssue(path, "indent %d follows indent %d — a block can be at most one level deeper than its predecessor", k, prev)
				}
				k = prev + 1
			}
			for len(stack) > 0 && stack[len(stack)-1].indent >= k {
				stack = stack[:len(stack)-1]
			}
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				if leafBlockTypes[parent.typ] {
					addIssue(path, "nested under a %s block — %s blocks cannot have children", parent.typ, parent.typ)
				} else if parent.typ == "row" && typ != "column" {
					addIssue(path, "a row block can only contain column blocks, got %s", typ)
				}
			}
			stack = append(stack, frame{k, typ})
			prev = k
			walkBlock(block, path)
		}
	}

	if blocks, _ := doc["blocks"].([]any); blocks != nil {
		checkFlatRun(blocks, "/blocks", false)
	}
	return issues
}

// codeLangConflict reports a code block carrying both the first-class
// language prop and the internal fields.lang it lifts (§5.1).
func codeLangConflict(block map[string]any) bool {
	if _, hasLang := block["language"]; !hasLang {
		return false
	}
	fields, _ := block["fields"].(map[string]any)
	if fields == nil {
		return false
	}
	_, conflict := fields[codeLangField]
	return conflict
}

func walkTable(block map[string]any, path string,
	claimId func(id, path string), addIssue func(path, format string, args ...any),
	walkBlock func(block map[string]any, path string),
	checkFlatRun func(blocks []any, basePath string, inCell bool)) {

	columns, _ := block["columns"].([]any)
	colIds := make([]string, 0, len(columns))
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
					if _, _, err := parseInline(cell); err != nil {
						addIssue(cellPath, "inline markup: %v", err)
					}
				}
			case map[string]any:
				// a full walk: nested tables join the id uniqueness domain
				// and get their text checked
				walkBlock(cell, cellPath)
			case []any:
				// array form (§6.1 F10): a flat run — cell block first at
				// indent 0, descendants following
				checkFlatRun(cell, cellPath, true)
			}
		}
	}
}
