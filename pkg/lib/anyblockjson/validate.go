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
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

//go:embed schema/object.schema.json
var schemaJSON []byte

// SchemaJSON returns the embedded published JSON Schema (§12). Callers must
// not mutate the returned slice; discovery surfaces (API v2 §5) serve it
// verbatim.
func SchemaJSON() []byte {
	return schemaJSON
}

const (
	// FormatVersion is the AnyBlock JSON format version this package reads
	// and writes (§10). It is a single integer with no minor axis: every
	// format change bumps it, and a reader rejects anything newer than its
	// own while migrating anything older.
	FormatVersion = 1

	// schemaBaseURL is where the published schemas live, one directory per
	// format version.
	schemaBaseURL = "https://schemas.anytype.io/anyblock/"

	// maxBlockIndent is the F4 resource bound on nesting depth, mirrored by
	// the schema's indent maximum. Export enforces it too — Marshal must
	// never emit output its own Validate rejects.
	maxBlockIndent = 32
)

// SchemaURL and IndexSchemaURL are the published schema locations written
// into exported documents. Both are derived from FormatVersion so a version
// bump carries them along and they cannot drift out of sync with it; the
// $id inside each embedded schema file is checked against them by
// TestVersionIdentity, which is the one copy the compiler cannot keep honest.
var (
	SchemaURL      = schemaBaseURL + strconv.Itoa(FormatVersion) + "/object.schema.json"
	IndexSchemaURL = schemaBaseURL + strconv.Itoa(FormatVersion) + "/index.schema.json"
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
	// NewerFormat is set when the document declares a format version newer
	// than this package reads, which a reader always rejects outright (§10).
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
	v, ok := jsonIntValue(probe.Version)
	if !ok {
		return 0, "", false
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

// ValidateWarn is Validate with a sink for warning-grade issues: things that
// do not make a document invalid but do mean part of it is dead weight — a
// groupBy on a view type that cannot group (§6.2), for instance. Validate
// discards them, so a tool that wants to show them must call this.
func ValidateWarn(data []byte, onWarning func(Issue)) error {
	_, err := validateToDoc(data, false, onWarning)
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
	// MIGRATION SEAM: an older version is migrated forward here, between the
	// version gate and schema validation. The schema pins the version to a
	// const, so it doubles as the assertion that migration ran (§10).
	sch, err := compileSchema()
	if err != nil {
		return nil, fmt.Errorf("embedded schema: %w", err)
	}
	// the key slots first: the schema states their rule but cannot say which
	// member broke it, so this pass owns the wording and schemaIssues stays
	// quiet about whatever it spoke for (see propertyNameIssues).
	spoken := propertyNameIssues(doc)
	if err := sch.Validate(doc); err != nil {
		return nil, &ValidationError{Issues: append(spoken.issues, schemaIssues(err, spoken)...)}
	}
	if len(spoken.issues) > 0 {
		// unreachable while the two statements of the rule agree; a
		// divergence must still refuse the document rather than pass it
		return nil, &ValidationError{Issues: spoken.issues}
	}

	if issues := semanticIssues(doc, lenient, warn); len(issues) > 0 {
		return nil, &ValidationError{Issues: issues}
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

func jsonPath(tokens []string) string {
	if len(tokens) == 0 {
		return ""
	}
	return "/" + strings.Join(tokens, "/")
}

// schemaIssues turns a jsonschema error tree into the flat, path-addressed
// issue list §12 promises. Flattening the tree verbatim does not produce that
// list: it produces the tree's own bookkeeping, in which two mechanics report
// problems the document does not have.
//
//   - `unevaluatedProperties: false` (the closed-set check on blocks) only
//     sees the properties that *successfully* evaluated subschemas annotated.
//     When a block's type-specific subschema fails — a bad `type`, one field
//     of the wrong shape — its annotations are discarded and every property of
//     that block is reported unevaluated, i.e. "not allowed". So a document
//     whose only fault is `"type": "bulleted_list_item"` is also told to
//     remove `type` and `text`.
//   - an `anyOf` reports every branch it tried. A table cell written as an
//     object collects the three "wrong shape" verdicts from the string, null
//     and array branches alongside the one real complaint.
//
// Both are confidently wrong rather than merely noisy, and the format's
// purpose is the generate → validate → feed-back loop: an agent told
// `property "type" is not allowed` deletes `type` and its next attempt is
// worse. So the noise is pruned here rather than explained in the spec.
func schemaIssues(err error, spoken keySlotReport) []Issue {
	verr, ok := err.(*jsonschema.ValidationError)
	if !ok {
		return []Issue{{Message: err.Error()}}
	}
	printer := message.NewPrinter(language.English)
	leaves := collectSchemaLeaves(verr, printer, spoken)

	// a leaf that is not an unevaluated-property verdict is a real fault, and
	// it makes the closed-set verdict on its enclosing objects unreliable
	realAt := map[string]bool{}
	for _, l := range leaves {
		if l.unevaluated {
			continue
		}
		for p := l.path; ; p = parentPath(p) {
			realAt[p] = true
			if p == "" {
				break
			}
		}
	}
	vocabulary := schemaPropertyNames()
	out := make([]Issue, 0, len(leaves))
	for _, l := range leaves {
		// "not allowed" is dropped only where it is unreliable: a name the
		// schema knows somewhere, inside an object that failed for another
		// reason. A name the schema never mentions is inadmissible under
		// every reading, so that verdict stands and the author gets both
		// facts in one round.
		if l.unevaluated && vocabulary[l.property] && realAt[parentPath(l.path)] {
			continue
		}
		out = append(out, Issue{Path: l.path, Message: l.message})
	}
	return out
}

// schemaLeaf is one rendered schema complaint plus what the pruning needs to
// know about where it came from.
type schemaLeaf struct {
	path        string
	message     string
	unevaluated bool   // reported by unevaluatedProperties, not by a rule
	property    string // the property name, for an unevaluated verdict
}

func collectSchemaLeaves(e *jsonschema.ValidationError, printer *message.Printer, spoken keySlotReport) []schemaLeaf {
	// a member propertyNameIssues already named: its verdict there carries a
	// pointer and this one does not, so reporting both says the same thing
	// twice, once unusably
	if k, isName := e.ErrorKind.(*kind.PropertyNames); isName && spoken.names[k.Property] {
		return nil
	}
	if len(e.Causes) == 0 {
		l := schemaLeaf{path: jsonPath(e.InstanceLocation), message: schemaIssueMessage(e, printer)}
		if spoken.values[l.path] {
			return nil // same value, already reported by name and by rule
		}
		if strings.Contains(e.SchemaURL, "/unevaluatedProperties") {
			l.unevaluated = true
			if toks := e.InstanceLocation; len(toks) > 0 {
				l.property = toks[len(toks)-1]
			}
		}
		return []schemaLeaf{l}
	}
	switch e.ErrorKind.(type) {
	case *kind.AnyOf, *kind.OneOf:
		return branchLeaves(e, printer, spoken)
	}
	var out []schemaLeaf
	for _, c := range e.Causes {
		out = append(out, collectSchemaLeaves(c, printer, spoken)...)
	}
	return out
}

// branchLeaves reports the alternatives of an anyOf/oneOf. A branch whose only
// complaint is the instance's own type never applied — the author did not write
// a string where this branch wanted a string — so reporting it says nothing
// about the document. When some branch did apply, only those are reported;
// when none did, the shape is wrong and the alternatives merge into one issue
// naming all of them, which is the whole content of a failed anyOf.
func branchLeaves(e *jsonschema.ValidationError, printer *message.Printer, spoken keySlotReport) []schemaLeaf {
	at := jsonPath(e.InstanceLocation)
	var applied []schemaLeaf
	var inapplicable []*kind.Type
	for _, c := range e.Causes {
		leaves := collectSchemaLeaves(c, printer, spoken)
		// a branch that failed only on the instance's own type is a branch
		// the instance was never a candidate for
		types := branchTypeErrors(c)
		if len(types) == len(leaves) && allAt(leaves, at) {
			inapplicable = append(inapplicable, types...)
			continue
		}
		applied = append(applied, leaves...)
	}
	if len(applied) > 0 {
		return applied
	}
	if len(inapplicable) == 0 {
		// nothing to merge and nothing applied: report the tree verbatim
		// rather than swallow the failure into an error with no issues
		var out []schemaLeaf
		for _, c := range e.Causes {
			out = append(out, collectSchemaLeaves(c, printer, spoken)...)
		}
		return out
	}
	want := make([]string, 0, len(inapplicable))
	for _, t := range inapplicable {
		want = append(want, t.Want...)
	}
	return []schemaLeaf{{
		path:    at,
		message: fmt.Sprintf("got %s, want %s", inapplicable[0].Got, strings.Join(dedupe(want), ", ")),
	}}
}

func allAt(leaves []schemaLeaf, path string) bool {
	for _, l := range leaves {
		if l.path != path {
			return false
		}
	}
	return true
}

// branchTypeErrors returns the type mismatches of one anyOf branch, and
// nothing when the branch failed for any other reason.
func branchTypeErrors(e *jsonschema.ValidationError) []*kind.Type {
	if t, isType := e.ErrorKind.(*kind.Type); isType {
		return []*kind.Type{t}
	}
	var out []*kind.Type
	for _, c := range e.Causes {
		out = append(out, branchTypeErrors(c)...)
	}
	return out
}

func dedupe(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// parentPath returns the JSON pointer of the container holding path.
func parentPath(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[:i]
	}
	return ""
}

// schemaPropertyNames is every property name the embedded schema mentions
// anywhere. It answers one question: could this name have been admitted under
// some reading of the schema? A name that is absent could not, whatever else
// failed — which is what makes the "not allowed" verdict on it trustworthy.
var schemaPropertyNames = sync.OnceValue(func() map[string]bool {
	names := map[string]bool{}
	var doc any
	if err := json.Unmarshal(schemaJSON, &doc); err != nil {
		return names
	}
	var walk func(node any)
	walk = func(node any) {
		switch n := node.(type) {
		case map[string]any:
			for key, v := range n {
				if key == "properties" {
					if props, isMap := v.(map[string]any); isMap {
						for name, sub := range props {
							names[name] = true
							walk(sub)
						}
						continue
					}
				}
				walk(v)
			}
		case []any:
			for _, v := range n {
				walk(v)
			}
		}
	}
	walk(doc)
	return names
})

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
// markup; code/embed text is literal (§8.4).
func textBearing(typ string) bool {
	switch typ {
	case "paragraph", "heading_1", "heading_2", "heading_3", "heading_4", "header_4",
		"quote", "checkbox", "bulleted_list_item", "numbered_list_item", "toggle",
		"callout", "toggle_heading_1", "toggle_heading_2", "toggle_heading_3",
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
	"icon": true, "table_of_contents": true, "featured_properties": true,
	"chat": true,
}

// LeafBlockType reports whether typ cannot be a parent (§5 leaf types, the
// V2 containment check). Exported for wiring that pre-checks edits before a
// full document validation (API v2 Phase 3).
func LeafBlockType(typ string) bool {
	return leafBlockTypes[typ]
}

// TextBlockType reports whether typ carries a `text` prop — the §5
// text-bearing styles plus the literal-text blocks (`code`, `embed` and its
// `equation` alias, §8.4). Exported for the same wiring as LeafBlockType.
func TextBlockType(typ string) bool {
	switch typ {
	case "code", "embed", "equation":
		return true
	}
	return textBearing(typ)
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

// jsonInt64 and jsonInt32 read a schema-integer field into the stored type.
// They are the decode-side half of the agreement rule: the schema admits
// integer-valued floats and bounds each field to its stored type's range, so
// these accept exactly what it accepts, and an absent field (the zero
// json.Number) reads as 0.
//
// The clamp is unreachable while the schema carries the bounds — Unmarshal
// always validates first — and is here so that a bound removed from the schema
// costs a wrong pixel width rather than a wrapped negative one.
func jsonInt64(num json.Number) int64 {
	v, _ := jsonIntValue(num)
	return v
}

func jsonInt32(num json.Number) int32 {
	v, ok := jsonIntValue(num)
	if !ok {
		return 0
	}
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < math.MinInt32 {
		return math.MinInt32
	}
	return int32(v)
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

	// Every number in the document has to land in a float64: the loose surfaces
	// (§3 properties, block fields, store, filter values) decode into a proto
	// Struct, whose numbers are doubles, and the schema cannot bound them
	// without closing surfaces the format deliberately leaves open. Left
	// unchecked, Validate accepted 1e400 and Unmarshal then failed with a bare
	// Go decode error carrying no JSON pointer — the divergence §12 rules out.
	checkNumbers(doc, "", addIssue)

	if _, ok := doc["template_for"]; ok {
		if typ, _ := doc["type"].(string); typ != "template" {
			addIssue("/template_for", `template_for is only valid on templates (type "template")`)
		}
	}

	// Every per-key check below runs on the STORED key a document spelling
	// resolves to — not on the raw spelling. The canonical document spells the
	// snake_case api slug (§3), so checks keyed off the raw spelling were dead
	// for exactly the documents this format produces: "unique_key" walked past
	// the deny rule that "uniqueKey" tripped, and the legend could rebind any
	// spelling onto any stored key, denied ones included. resolveDocKey is the
	// §3 chain with the only vocabulary Validate has: the document's own
	// legend first, then the bundled table, then the spelling verbatim — the
	// same resolution importer.propertyKey performs with default Options. A
	// caller-supplied vocabulary can resolve further than Validate can see,
	// which is why importer.build re-runs admission on ITS resolved key.
	legend, _ := doc["property_keys"].(map[string]any)
	resolveDocKey := func(term string) string {
		if v, ok := legend[term]; ok {
			if key, isStr := v.(string); isStr && key != "" {
				return key
			}
		}
		key, _ := BundledKeyVocabulary{}.PropertyKey(term)
		return key
	}

	if props, _ := doc["properties"].(map[string]any); props != nil {
		for _, term := range sortedMapKeys(props) {
			v := props[term]
			path := "/properties/" + escapeJSONPointer(term)
			// the importer lifts these two spellings into the envelope before
			// any resolution runs (§2), so the legend cannot re-purpose them
			key := term
			if term != detailKeyId && term != detailKeyType {
				key = resolveDocKey(term)
			}
			if reason, denied := deniedPropertyKey(key); denied {
				addIssue(path, "%s", reason)
				continue
			}
			// layout properties are named, not numbered (§3). A typo would
			// otherwise import as a raw string onto a number-format property:
			// no error anywhere, and every consumer reads it with an int getter
			// and silently sees "basic".
			if s, isStr := v.(string); isLayoutKey(key) && isStr {
				if !layoutNames.has(s) {
					addIssue(path, "unknown layout %q", s)
				}
				continue // a raw number is still accepted (§3)
			}
			if reason, wrong := wrongShapeForFormat(key, v); wrong {
				warnIssue(path, "%s", reason)
			}
		}
	}

	if _, ok := doc["type_properties"]; ok {
		if kind, _ := doc["kind"].(string); kind != "object_type" {
			addIssue("/type_properties", `type_properties is only valid on type documents (kind "object_type")`)
		}
		// typeProperties replaces the recommended-relation lists (§2a): a
		// document carrying both is ambiguous. The lists are named by whatever
		// spelling resolves onto them — recommendedListKeys holds stored keys,
		// and the canonical document spells recommended_relations (§3)
		if props, _ := doc["properties"].(map[string]any); props != nil {
			listKeys := make(map[string]bool, len(recommendedListKeys))
			for _, l := range recommendedListKeys {
				listKeys[l.detailKey] = true
			}
			for _, term := range sortedMapKeys(props) {
				if listKeys[resolveDocKey(term)] {
					addIssue("/properties/"+escapeJSONPointer(term),
						"conflicts with type_properties, which replaces this list")
				}
			}
		}
		// name is used only when the property has to be created (§2a); an
		// existing one keeps its own, so renaming a bundled key here reads as
		// working and silently does nothing
		if list, _ := doc["type_properties"].([]any); list != nil {
			for i, raw := range list {
				tp, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				key, _ := tp["key"].(string)
				// options declare a select's vocabulary and its display
				// order (§2a); on any other format there is nothing to
				// declare and the array would be silently dropped
				if opts, has := tp["options"].([]any); has && len(opts) > 0 {
					if f, _ := tp["format"].(string); f != "select" && f != "multi_select" {
						shown := f
						if shown == "" {
							shown = "text"
						}
						addIssue(fmt.Sprintf("/type_properties/%d/options", i),
							"options is only meaningful on select/multi_select, not %q", shown)
					}
					// an option is a bare name or an object carrying a color
					// (§2a), and the two forms name the same vocabulary: the
					// duplicate check has to read across both
					seen := map[string]bool{}
					for j, o := range opts {
						n := optionEntryName(o)
						if seen[n] {
							addIssue(fmt.Sprintf("/type_properties/%d/options/%d", i, j),
								"duplicate option %q", n)
						}
						seen[n] = true
					}
				}
				// objectTypes restricts what an object reference may point
				// at; on any other format there is nothing to restrict and
				// the array would be silently dropped
				if ots, has := tp["object_types"].([]any); has && len(ots) > 0 {
					if f, _ := tp["format"].(string); f != "objects" && f != "files" {
						shown := f
						if shown == "" {
							shown = "text"
						}
						addIssue(fmt.Sprintf("/type_properties/%d/object_types", i),
							"object_types is only meaningful on objects/files, not %q", shown)
					}
				}
				// a bundled property is used as-is: only the wiring's
				// create path reads these, and it never runs for a key that
				// already exists (§2a). The key slot spells the api slug like
				// every other (§3), so the lookup runs on the resolved key
				if key != "" {
					if rel, err := bundle.GetRelation(domain.RelationKey(resolveDocKey(key))); err == nil && rel != nil {
						if name, _ := tp["name"].(string); name != "" && name != rel.Name {
							warnIssue(fmt.Sprintf("/type_properties/%d/name", i),
								"%q is a bundled property named %q — this name is ignored; mint a custom key if the label matters",
								key, rel.Name)
						}
						if ots, has := tp["object_types"].([]any); has && len(ots) > 0 {
							warnIssue(fmt.Sprintf("/type_properties/%d/object_types", i),
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

	// checkInline parses one text string for grammar errors, and reports the
	// tag-shaped sequences the grammar does not recognize: those stay literal
	// (§10), but canonical export escapes them (§8.2), so an unescaped one
	// says the text did not come from this version's export.
	checkInline := func(text, path string) {
		_, _, notes, err := parseInlineNotes(text)
		if err != nil {
			addIssue(path, "inline markup: %v", err)
			return
		}
		for _, name := range notes.unknownTags {
			warnIssue(path, "tag-shaped %q is not markup this version recognizes — "+
				"kept as literal text; canonical output escapes the \"<\"", "<"+name)
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
		checkInline(text, path+"/text")
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
			walkTable(block, path, claimId, addIssue, checkInline, walkBlock, checkFlatRun)
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

// neverWritableProperties are the keys import must refuse even though they are
// not in bundle.LocalAndDerivedRelationKeys, so the derived half of
// strippedDetailKeys does not know about them. (They ARE bundled relations —
// bundle.HasRelation is true for both and each has an api slug — they are just
// not on the local/derived list.) They are the importer's own resolution
// vectors: existingobject.go picks which existing object in the space a
// snapshot merges into using oldAnytypeID, uniqueKey and sourceFilePath, so a
// document that sets them aims itself at an object it did not create.
var neverWritableProperties = map[string]string{
	"oldAnytypeID":   "oldAnytypeID selects which existing object a document merges into and cannot be set by a document",
	"sourceFilePath": "sourceFilePath selects which existing object a document merges into and cannot be set by a document",
}

// maxPropertyKeyLen mirrors the schema's propertyNames maxLength (§3).
const maxPropertyKeyLen = 128

// isWritablePropertyKey reports whether a key can be a property name at all,
// mirroring the schema's propertyNames rule: non-empty, no control characters,
// and inside the length bound. Both directions consult it — validation through
// the schema, export directly — because a stored detail key is not guaranteed
// to be one: an empty key and a key holding a newline both exist in real data,
// and neither survives as a JSON property name that means anything.
func isWritablePropertyKey(key string) bool {
	if key == "" || utf8.RuneCountInString(key) > maxPropertyKeyLen {
		return false
	}
	for _, r := range key {
		if r <= 0x1f || r == 0x7f {
			return false
		}
	}
	return true
}

// keySlotReport is what propertyNameIssues found, plus what it spoke for so
// the schema does not say it again: names holds the member NAMES it rejected
// (the schema's propertyNames verdict on those is redundant and pathless),
// values holds the pointers whose VALUE it rejected (the schema addresses
// those correctly but says nothing about what is wrong with the string).
type keySlotReport struct {
	issues []Issue
	names  map[string]bool
	values map[string]bool
}

// propertyNameIssues states, where the key is in hand, every rule the schema
// carries as `propertyNames`: the `properties` map and the `property_keys`
// legend take a writable property key (§3), the `refs` legend takes a label
// (§9a). A legend VALUE rides along because it is a stored key under the same
// rule and the schema's verdict on it names the bound, not the string.
//
// The rule stays in the published schema — an external validator runs that and
// nothing else (§12) — and is restated here because `propertyNames` cannot
// produce the issue §12 promises. The library validates each name as a
// standalone string instance, so its verdict carries neither the enclosing
// object's location nor, for a length bound, the name: a 200-character
// property key was reported as `maxLength: got 200, want 128` at the document
// ROOT, and an agent running the generate → validate → feed-back loop (§13)
// cannot tell from that which property to fix. The predicates are the export
// side's own (isWritablePropertyKey, isValidRefsKey), so the two directions
// cannot drift into Marshal emitting what Validate rejects (§11, I1).
func propertyNameIssues(doc map[string]any) keySlotReport {
	r := keySlotReport{names: map[string]bool{}, values: map[string]bool{}}
	rejectName := func(path, name, reason string) {
		r.issues = append(r.issues, Issue{Path: path, Message: reason})
		r.names[name] = true
	}
	rejectValue := func(path, reason string) {
		r.issues = append(r.issues, Issue{Path: path, Message: reason})
		r.values[path] = true
	}

	if refs, _ := doc["refs"].(map[string]any); refs != nil {
		for _, label := range sortedMapKeys(refs) {
			if !isValidRefsKey(label) {
				rejectName("/refs/"+escapeJSONPointer(label), label, fmt.Sprintf(
					"refs label %q is not 1-64 characters of [A-Za-z0-9_-] (§9a)", label))
			}
		}
	}
	if props, _ := doc["properties"].(map[string]any); props != nil {
		for _, term := range sortedMapKeys(props) {
			if !isWritablePropertyKey(term) {
				rejectName("/properties/"+escapeJSONPointer(term), term,
					unwritableKeyReason("property key", term))
			}
		}
	}
	if legend, _ := doc["property_keys"].(map[string]any); legend != nil {
		for _, term := range sortedMapKeys(legend) {
			path := "/property_keys/" + escapeJSONPointer(term)
			if !isWritablePropertyKey(term) {
				rejectName(path, term, unwritableKeyReason("legend spelling", term))
			}
			key, isString := legend[term].(string)
			if !isString {
				continue // the schema types the value; this pass only shapes it
			}
			if !isWritablePropertyKey(key) {
				rejectValue(path, unwritableKeyReason("legend stored key", key))
			}
		}
	}
	return r
}

// unwritableKeyReason names the string that broke the writable-key rule and
// which half of it broke. Naming it is not redundant with the pointer: a
// legend VALUE has no pointer of its own, and an over-long key makes a pointer
// no one reads.
func unwritableKeyReason(what, key string) string {
	switch n := utf8.RuneCountInString(key); {
	case key == "":
		return what + " is empty — a key slot has to name something (§3)"
	case n > maxPropertyKeyLen:
		return fmt.Sprintf("%s %q is %d characters; the bound is %d (§3)",
			what, key, n, maxPropertyKeyLen)
	default:
		return fmt.Sprintf("%s %q carries a control character (§3)", what, key)
	}
}

// deniedPropertyKey reports whether a property key may be written at all, and
// why not. The rule is a single sentence — **import refuses exactly what export
// strips** (§3, §4a) — and it is derived from the export side's own list rather
// than restated, because a restated list is how the two surfaces drifted apart
// in the first place: import used to accept isArchived, spaceId, restrictions,
// uniqueKey and the empty key, all of which export removes.
func deniedPropertyKey(key string) (string, bool) {
	if reason, never := neverWritableProperties[key]; never {
		return reason, true
	}
	if key == detailKeyId || key == detailKeyType {
		return fmt.Sprintf("%q belongs in the envelope, not in properties (§2)", key), true
	}
	if strippedDetailKeys()[key] {
		return fmt.Sprintf("%q is internal: export strips it, so import does not accept it (§3)", key), true
	}
	return "", false
}

// wrongShapeForFormat reports a property value whose JSON shape its property's
// format cannot hold — "next Friday" on a date, "yes" on a checkbox — which is
// stored verbatim and then read as the format's zero value forever, with
// nothing to show that anything went wrong.
//
// Only bundled properties can be checked: Validate takes no resolver, so a
// custom key's format is unknown here. And it is a **warning**, not an error,
// for a reason worth writing down: the same check on the export path would make
// one already-corrupt stored value enough to make an object unexportable, and
// "Marshal never emits what Validate rejects" (§11) is the stronger promise.
// Reporting it costs nothing and catches the authoring case, which is the one
// that can still be fixed.
func wrongShapeForFormat(key string, v any) (string, bool) {
	if v == nil {
		return "", false // an explicit null is a value: the key was set (§3)
	}
	rel, err := bundle.GetRelation(domain.RelationKey(key))
	if err != nil || rel == nil {
		return "", false
	}
	switch rel.Format {
	case model.RelationFormat_date:
		// a number is unix seconds — including the raw number export writes for
		// a date with no RFC 3339 form (§3)
		if _, isNum := v.(json.Number); isNum {
			return "", false
		}
		if s, isStr := v.(string); isStr {
			if _, ok := parseDate(s); ok {
				return "", false
			}
		}
		return fmt.Sprintf("%q is a date property: a value that is neither unix seconds nor an "+
			"RFC 3339 string is stored as written and reads as no date at all", key), true
	case model.RelationFormat_checkbox:
		if _, isBool := v.(bool); !isBool {
			return fmt.Sprintf("%q is a checkbox property: anything but true/false reads as false", key), true
		}
	case model.RelationFormat_number:
		if _, isNum := v.(json.Number); !isNum {
			return fmt.Sprintf("%q is a number property: a non-number reads as 0", key), true
		}
	case model.RelationFormat_longtext, model.RelationFormat_shorttext,
		model.RelationFormat_url, model.RelationFormat_email,
		model.RelationFormat_phone, model.RelationFormat_emoji:
		if _, isStr := v.(string); !isStr {
			return fmt.Sprintf("%q is a text property: a non-string reads as empty", key), true
		}
	}
	return "", false
}

// checkNumbers walks every number in the document and reports the ones no
// reader can hold. A JSON number has no range limit; float64 does, and that is
// where every number in this format ends up — so a value outside it is not a
// number this format has, whatever surface it sits on.
func checkNumbers(node any, path string, addIssue func(path, format string, args ...any)) {
	switch n := node.(type) {
	case map[string]any:
		for _, k := range sortedMapKeys(n) {
			checkNumbers(n[k], path+"/"+escapeJSONPointer(k), addIssue)
		}
	case []any:
		for i, v := range n {
			checkNumbers(v, fmt.Sprintf("%s/%d", path, i), addIssue)
		}
	case json.Number:
		if _, err := n.Float64(); err != nil {
			addIssue(path, "number %s is out of range: values must fit a 64-bit float", n.String())
		}
	}
}

func sortedMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// escapeJSONPointer escapes the two characters a JSON pointer token cannot
// carry literally (RFC 6901): a property key is author-controlled, and the
// loose surfaces accept any key at all.
func escapeJSONPointer(token string) string {
	token = strings.ReplaceAll(token, "~", "~0")
	return strings.ReplaceAll(token, "/", "~1")
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
	checkInline func(text, path string),
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
		// every row×column pair joins the id uniqueness domain (§4), whether
		// or not the cell is written: the id belongs to the table either way,
		// and the editor materializes the missing cell at exactly that id the
		// first time it is filled. Claiming only the written cells left the
		// rest of the grid free for a block to take.
		for j, colId := range colIds {
			if rowId == "" || colId == "" {
				continue
			}
			at := rowPath
			if j < len(cells) {
				at = fmt.Sprintf("%s/cells/%d", rowPath, j)
			}
			claimId(rowId+"-"+colId, at)
		}
		for j, c := range cells {
			cellPath := fmt.Sprintf("%s/cells/%d", rowPath, j)
			switch cell := c.(type) {
			case string:
				if cell != "" {
					checkInline(cell, cellPath)
				}
			case map[string]any:
				// a full walk: the cell joins the id uniqueness domain and
				// gets its text checked (tables inside cells are already
				// rejected by the schema's cellBlock definition)
				walkBlock(cell, cellPath)
			case []any:
				// array form (§6.1 F10): a flat run — cell block first at
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
	"kanban":   {"select": {}, "multi_select": {}, "checkbox": {}},
	"calendar": {"date": {}},
}

// checkDataviewViews runs the per-view semantic checks that need the
// dataview's own properties[] to know a key's format: groupBy viability and
// the date-filter empty trap. It also enforces view-id uniqueness.
//
// It reports a groupBy a view cannot honour. An impossible pair on
// a grouping view is an error: it can only come from authoring, and it
// renders as a single empty group. groupBy on a non-grouping view is only a
// warning — switching a kanban to a table in the editor leaves the stale
// groupRelationKey behind, so real exported data legitimately carries it.
//
// VIEW-ID UNIQUENESS is scoped to the dataview BLOCK, not to the document —
// the one id domain in this format that is not document-wide (§4), and
// deliberately so:
//
//   - It is the scope in which a duplicate actually breaks something. Every
//     consumer resolves a view reference within ONE dataview's views list
//     (the API's matchViewRef, the client's view tabs), and a dataview's
//     per-view editor state — groupOrders, objectOrders — is keyed by view
//     id inside that same block. Two views of one dataview sharing an id
//     make the second unaddressable forever; two views in DIFFERENT dataview
//     blocks sharing one are each reachable through their own block.
//   - Document-wide would reject data the app itself produces. The default
//     view of every set, collection and type is minted with the literal id
//     "default" (editor/template.MakeDataviewContent), and creating an
//     inline set from an existing object copies that object's views verbatim
//     into the new block (dataviewservice.CopyDataviewToBlock) — so a page
//     with two inline collections legitimately holds two views called
//     "default". A format error there would fail on real exports.
//
// Before this, `views[].id` was the one id slot in the document with no
// uniqueness check at all — invalid but unvalidated on every channel,
// create and import included, not just PATCH (§8.31).
func checkDataviewViews(block map[string]any, path string, addIssue, warnIssue func(string, string, ...any)) {
	views, _ := block["views"].([]any)
	if len(views) == 0 {
		return
	}
	seenViewIds := map[string]string{} // id -> path of first occurrence
	for i, raw := range views {
		view, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id, _ := view["id"].(string)
		if id == "" {
			continue // ids are optional on input (§9); import generates them
		}
		idPath := fmt.Sprintf("%s/views/%d/id", path, i)
		if first, dup := seenViewIds[id]; dup {
			addIssue(idPath, "duplicate view id %q in this dataview (first used at %s)", id, first)
			continue
		}
		seenViewIds[id] = idPath
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
		groupBy, _ := view["group_by"].(string)
		if groupBy == "" {
			continue
		}
		vPath := fmt.Sprintf("%s/views/%d/group_by", path, i)
		viewType, _ := view["type"].(string)
		if viewType == "" {
			viewType = "table" // §6.2: the default view type
		}
		allowed, groups := groupableFormats[viewType]
		if !groups {
			warnIssue(vPath, "%q views do not group; group_by is ignored", viewType)
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

// checkDateFilters warns about `less`/`less_or_equal` on a date property that
// is not guarded by a `not_empty`/`exists` on the same property in an
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
					(cond == "not_empty" || cond == "exists") {
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
				childScope := scope
				if op == "or" {
					// an `empty` sibling on the same property under an OR is
					// intent to INCLUDE the undated objects ("… OR dueDate IS
					// EMPTY") — warning that the comparison also matches them
					// would contradict the filter's own text
					childScope = map[string]bool{}
					for k := range scope {
						childScope[k] = true
					}
					for _, subRaw := range sub {
						leaf, isLeaf := subRaw.(map[string]any)
						if !isLeaf {
							continue
						}
						cond, _ := leaf["condition"].(string)
						if prop, _ := leaf["property"].(string); prop != "" && cond == "empty" {
							childScope[prop] = true
						}
					}
				}
				walk(sub, nPath+"/filters", op != "or", childScope)
				continue
			}
			// the day-count presets read their operand from value; without
			// one the count is 0, which quietly means "today" rather than
			// "n days ago" (pkg/lib/database.getDateRange) — but only where
			// the range is applied at all (datePresetConditions)
			if preset, _ := n["date_preset"].(string); preset != "" {
				_, counts := countingPresetNames[preset]
				leafCond, _ := n["condition"].(string)
				_, applies := datePresetConditions[leafCond]
				if counts && applies {
					if _, has := n["value"]; !has {
						addIssue(nPath, "date_preset %q needs a day count in \"value\"; without one it means 0 days, i.e. today", preset)
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
			if cond != "less" && cond != "less_or_equal" {
				continue
			}
			prop, _ := n["property"].(string)
			if formats[prop] != "date" || scope[prop] {
				continue
			}
			warnIssue(nPath, "%q on date %q also matches objects with no %s; "+
				"pair it with a %q leaf in an \"and\" group to exclude them",
				cond, prop, prop, "not_empty")
		}
	}
	walk(nodes, path+"/filters", true, map[string]bool{})
}

// filterTemplateValues returns the dynamic filter tokens (§6.2) inside a
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
