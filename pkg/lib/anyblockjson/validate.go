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
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

//go:embed schema/object.schema.json
var schemaJSON []byte

const (
	// FormatVersion is the AnyBlock JSON format version this package reads
	// and writes.
	FormatVersion = 1
	// SchemaURL is the published schema location written into exported
	// documents.
	SchemaURL = "https://schemas.anytype.io/anyblock/1.0/object.schema.json"

	// maxBlockIndent is the F4 resource bound on nesting depth, mirrored by
	// the schema's indent maximum. Export enforces it too — Marshal must
	// never emit output its own Validate rejects.
	maxBlockIndent = 32
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

// ValidationError aggregates every issue found in a document.
type ValidationError struct {
	Issues []Issue
	// NewerFormat is set when the document cites a newer 1.x schema, so the
	// failure likely means "produced by a newer version".
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
// validating or importing it — the cheap dispatch probe for import wiring.
// Ok is false when data is not a JSON object carrying an integer
// version.
func DetectFormat(data []byte) (version int, schemaURL string, ok bool) {
	var probe struct {
		Schema  string      `json:"$schema"`
		Version json.Number `json:"version"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return 0, "", false
	}
	v, ok := jsonIntValue(probe.Version)
	if !ok {
		return 0, "", false
	}
	return int(v), probe.Schema, true
}

// Validate checks data against the embedded schema and the semantic rules
// without building a snapshot. Validate is always strict; the lenient
// indent mode exists only on Unmarshal (Options.NormalizeIndent).
func Validate(data []byte) error {
	_, err := validateToDoc(data, false, nil)
	return err
}

// ValidateWarn is Validate with a sink for warning-grade issues: things that
// do not make a document invalid but do mean part of it is dead weight — a
// groupBy on a view type that cannot group, for instance. Validate
// discards them, so a tool that wants to show them must call this.
func ValidateWarn(data []byte, onWarning func(Issue)) error {
	_, err := validateToDoc(data, false, onWarning)
	return err
}

// validateToDoc runs the full validation pipeline and returns the decoded
// document for the importer to consume. With lenient set, over-deep indents
// are clamped instead of rejected, each clamp reported through warn.
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
					Message: schemaIssueMessage(e, printer),
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
// both versions, before schema validation gets a chance to produce a
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
	v, ok := jsonIntValue(num)
	if !ok {
		return &ValidationError{Issues: []Issue{{Path: "/version", Message: "version must be an integer"}}}
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

// schemaIssueMessage renders one schema error. Unknown properties fail
// against a `false` schema (unevaluatedProperties / removed keys), whose
// stock text "false schema" names neither the rule nor the key — rewrite it
// to name the property, with a migration hint for `children` (the error
// every nested-era generator will hit).
func schemaIssueMessage(e *jsonschema.ValidationError, printer *message.Printer) string {
	if _, isFalse := e.ErrorKind.(*kind.FalseSchema); isFalse {
		if toks := e.InstanceLocation; len(toks) > 0 {
			prop := toks[len(toks)-1]
			if prop == "children" {
				return `property "children" is not allowed — the flat format has no children; nest with indent instead (§4)`
			}
			if _, err := strconv.Atoi(prop); err != nil { // numeric = array index, not a property
				return fmt.Sprintf("property %q is not allowed", prop)
			}
		}
	}
	return e.ErrorKind.LocalizedString(printer)
}

// textBearing reports whether the block type's text is parsed for inline
// markup; code/embed text is literal.
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

// clampIndents applies the lenient rule in place: an indent more than one
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

// jsonIntValue reads a json.Number as an integer, accepting integer-valued
// floats like 2.0 and 1e0 — JSON Schema numeric equality treats them as
// integers, so every reader of a schema-integer field must too.
func jsonIntValue(num json.Number) (int64, bool) {
	v, err := num.Int64()
	if err == nil {
		return v, true
	}
	f, ferr := num.Float64()
	if ferr != nil || f != math.Trunc(f) {
		return 0, false
	}
	return int64(f), true
}

// indentOf reads a block's indent; absent means 0. The schema guarantees an
// integer in [0, 32] (V4) when present, which includes integer-valued
// floats — jsonIntValue keeps this reader in agreement with the schema and
// with Unmarshal.
func indentOf(block map[string]any) int {
	raw, ok := block["indent"]
	if !ok {
		return 0
	}
	num, ok := raw.(json.Number)
	if !ok {
		return 0
	}
	v, ok := jsonIntValue(num)
	if !ok {
		return 0
	}
	return int(v)
}

// semanticIssues runs the checks the schema cannot express: envelope
// combinations, indent monotonicity and containment over the flat blocks
// array (V1–V3), id uniqueness over the flattened tree including derived
// table cell ids, table arity, language-vs-fields.lang conflicts, and inline
// markup grammar. With lenient set, V1 violations clamp (reported via
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

	// layout properties are named, not numbered. A typo would otherwise
	// import as a raw string onto a number-format property: no error anywhere,
	// and every consumer reads it with an int getter and silently sees "basic".
	if props, _ := doc["properties"].(map[string]any); props != nil {
		for key, v := range props {
			s, isStr := v.(string)
			if !isLayoutKey(key) || !isStr {
				continue // a raw number is still accepted (§3)
			}
			if !layoutNames.has(s) {
				addIssue("/properties/"+key, "unknown layout %q", s)
			}
		}
	}

	if _, ok := doc["typeProperties"]; ok {
		if kind, _ := doc["kind"].(string); kind != "objectType" {
			addIssue("/typeProperties", `typeProperties is only valid on type documents (kind "objectType")`)
		}
		// typeProperties replaces the recommended-relation lists: a
		// document carrying both is ambiguous
		if props, _ := doc["properties"].(map[string]any); props != nil {
			for _, l := range recommendedListKeys {
				if _, dup := props[l.detailKey]; dup {
					addIssue("/properties/"+l.detailKey, "conflicts with typeProperties, which replaces this list")
				}
			}
		}
		// name is used only when the property has to be created; an
		// existing one keeps its own, so renaming a bundled key here reads as
		// working and silently does nothing
		if list, _ := doc["typeProperties"].([]any); list != nil {
			for i, raw := range list {
				tp, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				key, _ := tp["key"].(string)
				// options declare a select's vocabulary and its display
				// order; on any other format there is nothing to
				// declare and the array would be silently dropped
				if opts, has := tp["options"].([]any); has && len(opts) > 0 {
					if f, _ := tp["format"].(string); f != "select" && f != "multiSelect" {
						shown := f
						if shown == "" {
							shown = "text"
						}
						addIssue(fmt.Sprintf("/typeProperties/%d/options", i),
							"options is only meaningful on select/multiSelect, not %q", shown)
					}
					// an option is a bare name or an object carrying a color,
					// and the two forms name the same vocabulary: the
					// duplicate check has to read across both
					seen := map[string]bool{}
					for j, o := range opts {
						n := optionEntryName(o)
						if seen[n] {
							addIssue(fmt.Sprintf("/typeProperties/%d/options/%d", i, j),
								"duplicate option %q", n)
						}
						seen[n] = true
					}
				}
				// objectTypes restricts what an object reference may point
				// at; on any other format there is nothing to restrict and
				// the array would be silently dropped
				if ots, has := tp["objectTypes"].([]any); has && len(ots) > 0 {
					if f, _ := tp["format"].(string); f != "objects" && f != "files" {
						shown := f
						if shown == "" {
							shown = "text"
						}
						addIssue(fmt.Sprintf("/typeProperties/%d/objectTypes", i),
							"objectTypes is only meaningful on objects/files, not %q", shown)
					}
				}
				// a bundled property is used as-is: only the wiring's
				// create path reads these, and it never runs for a key that
				// already exists
				if key != "" {
					if rel, err := bundle.GetRelation(domain.RelationKey(key)); err == nil && rel != nil {
						if name, _ := tp["name"].(string); name != "" && name != rel.Name {
							warnIssue(fmt.Sprintf("/typeProperties/%d/name", i),
								"%q is a bundled property named %q — this name is ignored; mint a custom key if the label matters",
								key, rel.Name)
						}
						if ots, has := tp["objectTypes"].([]any); has && len(ots) > 0 {
							warnIssue(fmt.Sprintf("/typeProperties/%d/objectTypes", i),
								"%q is a bundled property; its target types are fixed by the bundle and this list is ignored — mint a custom key to target different types",
								key)
						}
					}
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
		if typ == "dataview" {
			checkDataviewViews(block, path, addIssue, warnIssue)
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
// language prop and the internal fields.lang it lifts.
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
			// derived cell ids join the uniqueness domain
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
				// a full walk: the cell joins the id uniqueness domain and
				// gets its text checked (tables inside cells are already
				// rejected by the schema's cellBlock definition)
				walkBlock(cell, cellPath)
			case []any:
				// array form: a flat run — cell block first at
				// indent 0, descendants following
				checkFlatRun(cell, cellPath, true)
			}
		}
	}
}

// groupableFormats lists, per view type, the property formats that view can
// group by. Only kanban and calendar group at all: the middleware assigns
// groupRelationKey for exactly these pairs (converter.insertGroupRelationKey,
// whose default branch is a no-op), the kanban service registers groupers for
// exactly these formats (core/kanban.Service.Init), and the client offers the
// same set (Relation.getGroupTypes). Every other view type ignores groupBy.
var groupableFormats = map[string]map[string]struct{}{
	"kanban":   {"select": {}, "multiSelect": {}, "checkbox": {}},
	"calendar": {"date": {}},
}

// checkDataviewViews runs the per-view semantic checks that need the
// dataview's own properties[] to know a key's format: groupBy viability and
// the date-filter empty trap.
//
// It reports a groupBy a view cannot honour. An impossible pair on
// a grouping view is an error: it can only come from authoring, and it
// renders as a single empty group. groupBy on a non-grouping view is only a
// warning — switching a kanban to a table in the editor leaves the stale
// groupRelationKey behind, so real exported data legitimately carries it.
func checkDataviewViews(block map[string]any, path string, addIssue, warnIssue func(string, string, ...any)) {
	views, _ := block["views"].([]any)
	if len(views) == 0 {
		return
	}
	formats := map[string]string{}
	props, _ := block["properties"].([]any)
	for _, raw := range props {
		p, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		key, _ := p["key"].(string)
		if f, isStr := p["format"].(string); isStr && key != "" {
			formats[key] = f
		} else if key != "" {
			formats[key] = "text" // §3: an omitted format is text
		}
	}
	for i, raw := range views {
		view, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		checkDateFilters(view, formats, fmt.Sprintf("%s/views/%d", path, i), addIssue, warnIssue)
		groupBy, _ := view["groupBy"].(string)
		if groupBy == "" {
			continue
		}
		vPath := fmt.Sprintf("%s/views/%d/groupBy", path, i)
		viewType, _ := view["type"].(string)
		if viewType == "" {
			viewType = "table" // §6.2: the default view type
		}
		allowed, groups := groupableFormats[viewType]
		if !groups {
			warnIssue(vPath, "%q views do not group; groupBy is ignored", viewType)
			continue
		}
		// a key absent from properties has no declared format to check
		format, declared := formats[groupBy]
		if !declared {
			continue
		}
		if _, ok := allowed[format]; !ok {
			addIssue(vPath, "%q views cannot group by %q (format %q); expected %s",
				viewType, groupBy, format, strings.Join(sortedKeys(allowed), " · "))
		}
	}
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// checkDateFilters warns about `less`/`lessOrEqual` on a date property that
// is not guarded by a `notEmpty`/`exists` on the same property in an
// enclosing AND. An object with no value for that date matches: the filter's
// value is set and the record's is not, so domain.Value.Compare returns 1,
// which is exactly what Less tests for. A freshness view written the obvious
// way ("verifiedUntil less today") therefore lists every never-verified
// object alongside the genuinely stale ones. It is a warning, not an error —
// including undated objects is a legal thing to want, and real exported data
// contains such filters.
func checkDateFilters(view map[string]any, formats map[string]string, path string, addIssue, warnIssue func(string, string, ...any)) {
	nodes, _ := view["filters"].([]any)
	if len(nodes) == 0 {
		return
	}
	var walk func(nodes []any, path string, and bool, guarded map[string]bool)
	walk = func(nodes []any, path string, and bool, guarded map[string]bool) {
		// only an AND lets a sibling notEmpty guarantee anything: under an OR
		// the comparison can be reached without it
		scope := guarded
		if and {
			scope = map[string]bool{}
			for k := range guarded {
				scope[k] = true
			}
			for _, raw := range nodes {
				n, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				cond, _ := n["condition"].(string)
				if prop, _ := n["property"].(string); prop != "" &&
					(cond == "notEmpty" || cond == "exists") {
					scope[prop] = true
				}
			}
		}
		for i, raw := range nodes {
			n, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			nPath := fmt.Sprintf("%s/%d", path, i)
			if sub, isGroup := n["filters"].([]any); isGroup {
				op, _ := n["operator"].(string)
				walk(sub, nPath+"/filters", op != "or", scope)
				continue
			}
			// the day-count presets read their operand from value; without
			// one the count is 0, which quietly means "today" rather than
			// "n days ago" (pkg/lib/database.getDateRange)
			if preset, _ := n["datePreset"].(string); preset != "" {
				if _, counts := countingPresetNames[preset]; counts {
					if _, has := n["value"]; !has {
						addIssue(nPath, "datePreset %q needs a day count in \"value\"; without one it means 0 days, i.e. today", preset)
					}
				}
			}
			// a dynamic filter token resolves to an object id, so it can
			// only match an object/file property; anywhere else it is
			// compared as a literal string and matches nothing
			if prop, _ := n["property"].(string); prop != "" {
				if f, declared := formats[prop]; declared && f != "objects" && f != "files" {
					for _, tok := range filterTemplateValues(n["value"]) {
						addIssue(nPath+"/value",
							"%q resolves to an object id and cannot match %q (format %q)", tok, prop, f)
					}
				}
			}
			cond, _ := n["condition"].(string)
			if cond != "less" && cond != "lessOrEqual" {
				continue
			}
			prop, _ := n["property"].(string)
			if formats[prop] != "date" || scope[prop] {
				continue
			}
			warnIssue(nPath, "%q on date %q also matches objects with no %s; "+
				"pair it with a %q leaf in an \"and\" group to exclude them",
				cond, prop, prop, "notEmpty")
		}
	}
	walk(nodes, path+"/filters", true, map[string]bool{})
}

// filterTemplateValues returns the dynamic filter tokens inside a
// filter value, which may be a bare string or an array of them.
func filterTemplateValues(v any) []string {
	var out []string
	switch x := v.(type) {
	case string:
		if isFilterTemplate(x) {
			out = append(out, x)
		}
	case []any:
		for _, e := range x {
			if s, ok := e.(string); ok && isFilterTemplate(s) {
				out = append(out, s)
			}
		}
	}
	return out
}
