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

// SchemaURL, IndexSchemaURL and PropertiesSchemaURL are the published schema
// locations written into exported documents. All are derived from
// FormatVersion so a version bump carries them along and they cannot drift
// out of sync with it; the
// $id inside each embedded schema file is checked against them by
// TestVersionIdentity, which is the one copy the compiler cannot keep honest.
var (
	SchemaURL           = schemaBaseURL + strconv.Itoa(FormatVersion) + "/object.schema.json"
	IndexSchemaURL      = schemaBaseURL + strconv.Itoa(FormatVersion) + "/index.schema.json"
	PropertiesSchemaURL = schemaBaseURL + strconv.Itoa(FormatVersion) + "/properties.schema.json"
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
	// a bundle index or a property dictionary is a different grammar, and
	// walking one through this grammar produces errors about the very members
	// that make it what it is (§2c, §2f). After the version gate, which every
	// grammar shares.
	if issues := misroutedIssues(data, KindObject); len(issues) > 0 {
		return nil, &ValidationError{Issues: issues}
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
	// the typed envelope fields' discriminator, for the same reason: the
	// schema can say `format` is missing but not that it is a CHOICE, and
	// naming the alternatives at the moment the author is wrong is the whole
	// reason those fields are typed rather than flat (§2b)
	iconFormatIssues(doc, &spoken)
	// a relation document's required `format` (§2d), same trade again: the
	// schema's `required` verdict cannot list the names, and the author most
	// likely to be missing it — one holding a pre-v0.31 document that spelled
	// `relation_format` in properties — needs the vocabulary, not the bound
	propertyFormatSlotIssue(doc, &spoken)
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

// jsonPath renders a schema error's instance location as the JSON pointer §12
// promises. The library hands back RAW tokens — a member name exactly as the
// document spells it — so each is escaped before it is joined (RFC 6901), the
// same way the restated key-slot checks build theirs (propertyNameIssues).
// Joining them verbatim addressed the wrong place for any key carrying `/` or
// `~`, and cost §12's one fault, one issue on top: the schema's verdict is
// suppressed for the members those checks spoke for, and that ledger is keyed
// by pointer — so an unescaped pointer missed the escaped entry and one empty
// legend value came back three times, once as `/property_internal_keys/a~1b` and twice
// more at `/property_internal_keys/a/b`, a location the document does not have.
func jsonPath(tokens []string) string {
	if len(tokens) == 0 {
		return ""
	}
	escaped := make([]string, len(tokens))
	for i, token := range tokens {
		escaped[i] = escapeJSONPointer(token)
	}
	return "/" + strings.Join(escaped, "/")
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
	markReal := func(path string) {
		for p := path; ; p = parentPath(p) {
			realAt[p] = true
			if p == "" {
				break
			}
		}
	}
	// a fault ANOTHER pass spoke for is still a fault at that location, and
	// the closed-set verdicts around it are just as unreliable. Suppressing
	// the schema's own leaf without recording this made a callout whose icon
	// lacks its `format` report the icon (once, well) and then also demand
	// `text` and `type` be deleted — the exact confidently-wrong advice this
	// pruning exists to remove.
	for path := range spoken.values {
		markReal(path)
	}
	for _, l := range leaves {
		if l.unevaluated {
			continue
		}
		markReal(l.path)
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
	// `additionalProperties: false` — the closed-set check on the ENVELOPE and
	// on the fixed-shape definitions — reports every unknown member of one
	// object in a single verdict carried at the OBJECT's location. Flattened
	// verbatim that is `additional properties 'refs' not allowed` at path ""
	// for a document whose fault is one named member, which is the pathless
	// verdict §12 rules out ("an issue names the member it is about"): inside a
	// block the same fault comes back correctly addressed, because blocks close
	// with `unevaluatedProperties`, which the library reports per member. So
	// the verdict is split into one leaf per member here, each at its own
	// pointer, and the names are sorted because the library collects them by
	// ranging over the instance's map — two unknown members otherwise came back
	// in a different order run to run.
	//
	// Unlike an unevaluated-property verdict these are never pruned, and that
	// is not an oversight: `additionalProperties` consults `properties` and
	// `patternProperties` of the SAME schema object, which always evaluate, so
	// its verdict does not depend on a sibling subschema having succeeded — the
	// unreliability the pruning exists for cannot arise here.
	if k, isAdditional := e.ErrorKind.(*kind.AdditionalProperties); isAdditional {
		at := jsonPath(e.InstanceLocation)
		props := append([]string(nil), k.Properties...)
		sort.Strings(props)
		out := make([]schemaLeaf, 0, len(props))
		for _, prop := range props {
			msg := unknownPropertyMessage(prop)
			// the pre-v0.32 relation-definition spellings, at the ROOT only
			// — anywhere else (a view, a sort) the same names are ordinary
			// unknown members and the hint would mislead. Same reasoning as
			// `refs` (§10): told only "not allowed", the obvious wrong
			// repair is to delete the definition rather than regroup it.
			if at == "" {
				switch prop {
				case "format", "include_time", "object_types":
					msg = fmt.Sprintf("property %q moved off the root: a property document "+
						"states its definition in the \"property_settings\" group (§2d) — "+
						"move it (and its two siblings, if present) in there", prop)
				case "type_properties":
					msg = `property "type_properties" moved: a type document states its ` +
						`definitions in "type_settings" and this array is its ` +
						`"property_definitions" member (§2a) — move it in there`
				}
			}
			out = append(out, schemaLeaf{
				path:    at + "/" + escapeJSONPointer(prop),
				message: msg,
			})
		}
		return out
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

// schemaFormatEnum is the list of variants a typed field's `format` member
// admits, read out of the published schema by definition name. Reading it
// rather than restating it is what makes the extension seam (§2b) free: a
// layer that appends a variant to the schema gets it named in the reader's
// own diagnostics without touching this package.
//
// Every `enum` the definition carries at a `properties/format` position is
// intersected, so a definition that narrows another one by $ref plus a
// second enum (plainIcon) answers with the narrowed set.
func schemaFormatEnum(def string) []string {
	return schemaFormatEnums()[def]
}

var schemaFormatEnums = sync.OnceValue(func() map[string][]string {
	out := map[string][]string{}
	var doc struct {
		Defs map[string]any `json:"$defs"`
	}
	if err := json.Unmarshal(schemaJSON, &doc); err != nil {
		return out
	}
	for name, def := range doc.Defs {
		var found [][]string
		var walk func(node any, inFormat bool)
		walk = func(node any, inFormat bool) {
			switch n := node.(type) {
			case map[string]any:
				for key, v := range n {
					switch {
					case inFormat && key == "enum":
						if list, isList := v.([]any); isList {
							names := make([]string, 0, len(list))
							for _, e := range list {
								if s, isStr := e.(string); isStr {
									names = append(names, s)
								}
							}
							if len(names) > 0 {
								found = append(found, names)
							}
						}
					case key == "properties":
						if props, isMap := v.(map[string]any); isMap {
							for prop, sub := range props {
								walk(sub, prop == "format")
							}
							continue
						}
						walk(v, false)
					default:
						walk(v, false)
					}
				}
			case []any:
				for _, v := range n {
					walk(v, inFormat)
				}
			}
		}
		walk(def, false)
		if len(found) == 0 {
			continue
		}
		// the intersection, in the order of the first list found. Map
		// iteration decides which that is when a definition carries two, so
		// the lists are sorted by length first: the narrowest is the answer,
		// and narrowing is the only reason a second one exists.
		sort.SliceStable(found, func(i, j int) bool { return len(found[i]) < len(found[j]) })
		keep := map[string]int{}
		for _, list := range found {
			for _, n := range list {
				keep[n]++
			}
		}
		var names []string
		for _, n := range found[0] {
			if keep[n] == len(found) {
				names = append(names, n)
			}
		}
		out[name] = names
	}
	return out
})

// propertyFormatEnum is the §3 format-name vocabulary as the published
// schema states it ($defs/propertyFormat) — read rather than restated, the
// schemaFormatEnums rule: the schema is the one statement an external
// validator also sees, so the reader's diagnostics quote it instead of a
// second list that can drift.
var propertyFormatEnum = sync.OnceValue(func() []string {
	var doc struct {
		Defs map[string]struct {
			Enum []any `json:"enum"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(schemaJSON, &doc); err != nil {
		return nil
	}
	var names []string
	for _, e := range doc.Defs["propertyFormat"].Enum {
		if s, isStr := e.(string); isStr {
			names = append(names, s)
		}
	}
	return names
})

// schemaIssueMessage renders one schema error. Unknown properties fail
// against a `false` schema (unevaluatedProperties / removed keys), whose
// stock text "false schema" names neither the rule nor the key — rewrite it
// to name the property, through the same renderer the envelope's closed-set
// verdict uses, so a removed key reads the same wherever it is written.
func schemaIssueMessage(e *jsonschema.ValidationError, printer *message.Printer) string {
	if _, isFalse := e.ErrorKind.(*kind.FalseSchema); isFalse {
		if toks := e.InstanceLocation; len(toks) > 0 {
			prop := toks[len(toks)-1]
			if _, err := strconv.Atoi(prop); err != nil { // numeric = array index, not a property
				// the §2d group is declared at the root and gated by kind, so
				// its false schema fires only OFF a relation document — and
				// only at the root (len == 1), where the member name is
				// unambiguous. "not allowed" would send the author toward
				// deleting the group; the actual repair is the kind.
				if len(toks) == 1 && prop == memberPropertySettings {
					return fmt.Sprintf("property %q is only valid on property documents "+
						`(kind "property", §2d)`, prop)
				}
				if len(toks) == 1 && prop == "type_settings" {
					return fmt.Sprintf("property %q is only valid on type documents "+
						`(kind "object_type", §2a)`, prop)
				}
				// inside the group, the refused members each have a home
				// already (§2d): telling the author only "not allowed" sends
				// them toward deleting the fact instead of moving it.
				if len(toks) == 2 && toks[0] == memberPropertySettings {
					if home, owned := propertySettingsMemberHomes[prop]; owned {
						return fmt.Sprintf("property_settings does not carry %q — %s (§2d)", prop, home)
					}
				}
				return unknownPropertyMessage(prop)
			}
		}
	}
	return e.ErrorKind.LocalizedString(printer)
}

// unknownPropertyMessage names a member no reading of the schema admits, and
// carries a migration hint for the three names a document written against an
// older grammar brings. The hints exist because the bare verdict sends the
// reader the wrong way, and the format's purpose is the generate → validate →
// feed-back loop (§13):
//
//   - `key` is the pre-v0.41 spelling of the property-naming slot in a
//     dataview's `properties[]` and the `property` block — 95,842 slots of a
//     28,599-document export spell it, so an agent prompted on old exports
//     WILL write it. Told only "not allowed", the obvious wrong repair is to
//     delete the member, which costs the block the one thing it says.
//   - `children` is what every nested-era generator writes; told only that it
//     is not allowed, the obvious repair is to drop the subtree rather than to
//     flatten it into `indent` (§4).
//   - `refs` is the object-reference legend this format used to carry (§9a).
//     Told only that it is not allowed, the obvious repair is to delete the
//     legend — which leaves behind exactly the short labels the legend was the
//     only means of inverting, now addressing nothing. The version integer
//     cannot say this either: the grammar changed under a version this reader
//     still accepts, so `refs` is the one marker a pre-v0.20 document carries,
//     and the message is where the reader is told what happened.
//
// propertySettingsMemberHomes names, for each propertyDefinition member the
// §2d group refuses, where the fact it spells already lives — the repair the
// bare "not allowed" cannot point at.
var propertySettingsMemberHomes = map[string]string{
	"internal_key":  "the envelope `internal_key` is the property's stored key",
	"property":      "a property document is addressed by its envelope `internal_key`; its spelling is derived, never stated here",
	"name":          "the property's name is the `name` property",
	"description":   "the property's description is the `description` property",
	"options":       "a property's options are property_option documents of their own",
	"max_count":     "it still travels in `properties` as `property_max_count`",
	"readonly":      "it still travels in `properties` as `property_readonly_value`",
	"default_value": "it still travels in `properties` as `property_default_value`",
}

func unknownPropertyMessage(prop string) string {
	switch prop {
	case "key":
		return `property "key" is not allowed — the member that names a property is spelled "property" in every structure (§5, §6.2): a dataview's properties[] entry and the property block spelled it "key" before v0.41, twelve lines from view columns, sorts and filters that spelled "property". Rename the member and keep its value`
	case "children":
		return `property "children" is not allowed — the flat format has no children; nest with indent instead (§4)`
	case "refs":
		return `property "refs" is not allowed — the object-reference legend was removed: every object id is now written in full, on every shape, with no legend (§9a). This document was written by an older exporter; replace each short label it uses with the id "refs" maps that label to, then drop "refs". Dropping it alone leaves labels that address nothing`
	}
	return fmt.Sprintf("property %q is not allowed", prop)
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

	// The template gate reads `kind`, and nothing else (§2). It used to
	// resolve the `type` spelling through the document's own chain — legend,
	// bundled table, verbatim — a private copy of §3 written so that Validate,
	// which has no vocabulary (§13), reached the same verdict as the
	// importer's kind derivation (§12). Both sides now read a field no chain
	// touches, so the copy is gone and so is the class of disagreement it
	// managed.
	//
	// The pre-v0.22 spelling of a template is `{"type": "template"}` with no
	// `kind`, and it is REFUSED rather than migrated (§10). It has to be
	// refused loudly, because it is the one shape that would otherwise fail
	// silently: a template exported yesterday whose target type happened to be
	// absent carries nothing to trip the template_for gate, so it would import
	// as an ordinary Page and nothing anywhere would say so. The comparison is
	// on the RAW spelling — no legend, no bundled table — because it is
	// identifying a byte sequence a previous version of this format wrote, not
	// resolving a type.
	kind, _ := doc["kind"].(string)
	typeTerm, _ := doc["type"].(string)
	// The refusal reads the document's OWN type_internal_keys legend beside the raw
	// spelling, because that is what the deleted chain read. Two documents
	// need it, in opposite directions: `{"type_internal_keys":{"tpl":"template"},
	// "type":"tpl"}` WAS a template under v0.21 (the legend resolves tpl to
	// the template key) and would otherwise import as a silent Page, and
	// `{"type_internal_keys":{"template":"custom1"},"type":"template"}` was NEVER one
	// (the legend rebinds the spelling away) and would otherwise be told to
	// add a kind that changes what it is. The clause resolves nothing beyond
	// the document itself — the same thing the refusal already does with
	// `type` — and dies with the rest of the refusal at the next version
	// bump (§15.9). The bundled table needs no equivalent: probing
	// bundle.ListTypesKeys(), the only bundled spelling that resolves to the
	// stored key `template` is the literal `template`, which the raw
	// comparison already covers.
	templateKey := typeTerm == typeKeyTemplate
	if legend, ok := doc[memberTypeInternalKeys].(map[string]any); ok {
		if bound, ok := legend[typeTerm].(string); ok {
			templateKey = bound == typeKeyTemplate
		}
	}
	legacyTemplate := kind == "" && templateKey
	if legacyTemplate {
		addIssue("/kind", `a template must say so: add "kind": "template". `+
			`Until v0.22 the type "template" carried that meaning on its own, and it no longer does `+
			`— without the kind this document imports as an ordinary page (§2)`)
	}
	if _, ok := doc["template_for"]; ok && !legacyTemplate {
		switch {
		case kind != kindNames.name(model.SmartBlockType_Template):
			addIssue("/template_for", `template_for is only valid on templates (kind "template")`)
		case typeTerm == "":
			// the target type is object_types[1] and there is no [1] without a
			// [0]: import reads template_for only inside the branch that read
			// `type`, so without one the field is silently discarded. The old
			// gate refused this shape as a side effect of resolving `type`;
			// reading `kind` instead, it has to be said outright.
			addIssue("/template_for",
				`template_for names the type this template is FOR, and needs the template's own type beside it: add "type"`)
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
	legend, _ := doc[memberPropertyInternalKeys].(map[string]any)
	resolveDocKey := func(term string) string {
		if v, ok := legend[term]; ok {
			if key, isStr := v.(string); isStr && key != "" {
				return key
			}
		}
		key, _ := BundledKeyVocabulary{}.PropertyKey(term)
		return key
	}

	// A legend VALUE is a stored key, and admission judges it as one: the
	// deny rule runs on the value itself, whether or not any member spells
	// the entry. Checked only where a /properties member resolved through it,
	// {"sneaky": "uniqueKey"} sat in the legend unchallenged — a laundering
	// primitive for any key slot outside /properties, and one export never
	// writes (a denied key never takes a slug — writableSlug).
	for _, term := range sortedMapKeys(legend) {
		key, isStr := legend[term].(string)
		if !isStr {
			continue
		}
		if reason, denied := deniedPropertyKey(key); denied {
			addIssue("/"+memberPropertyInternalKeys+"/"+escapeJSONPointer(term), "legend value: %s", reason)
		}
	}

	// An `option_ids` outer key is a PROPERTY SPELLING (§9a), and one naming a
	// property the document never spells qualifies nothing: import indexes the
	// legend by the spelling the slot it is resolving wrote, so such an entry
	// is unreachable and the values under it resolve by name as if the legend
	// were absent.
	//
	// A warning, not an error, because a legend is allowed to carry more than
	// one document needs. But ignored in SILENCE is how a legend filed under
	// `priorty` validates clean and then quietly loses the identity it was
	// written to carry, which is the degradation this format reports
	// everywhere else.
	//
	// A KEY-SET COMPARISON, not a parse. The flat spelling this replaced had
	// to split each key at its last separator before it could ask the census
	// anything, and that split was defined only for keys the shape rule
	// admitted; here the property spelling IS the key, so the census answers
	// it directly.
	if legend, _ := doc["option_ids"].(map[string]any); len(legend) > 0 {
		spellings := rawPropertySpellings(doc)
		for _, slug := range sortedMapKeys(legend) {
			if spellings[slug] {
				continue
			}
			warnIssue("/option_ids/"+escapeJSONPointer(slug),
				"no property in this document spells %q — this legend entry can "+
					"never be consulted, and the option names under it resolve by "+
					"name (§3, §9a)", slug)
		}
	}

	// The loop below is the MIRROR of the importer's details seam
	// (importer.build), refusal for refusal — denied resolved key, unwritable
	// resolved key, two spellings binding one key — in the same sorted order,
	// so with default Options the two verdicts cannot differ (§12, I2). The
	// duplicate-binding refusal used to live in the seam alone, and a
	// hand-written {"iconEmoji": …, "icon_emoji": …} validated clean and then
	// failed to import.
	if props, _ := doc["properties"].(map[string]any); props != nil {
		boundBy := make(map[string]string, len(props))
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
			// the §2a type-settings lift, kind-scoped like the import seam it
			// mirrors (typesettings.go): on a TYPE document the five stored
			// keys live in the group, and the flat spelling is refused with
			// the repair named; on every other kind the same keys are
			// ordinary properties
			if isTypeKind(doc) && typeSettingsLiftedDetailKeys()[key] {
				addIssue(path, "%q is written on a type document as %s in type_settings (§2a), "+
					"not as a property", key, typeSettingsLiftedKeyRepair(key))
				continue
			}
			// the document's own chain can hardly resolve a shape-checked
			// term onto an unwritable key — legend values and spellings were
			// vetted before this runs — but the seam refuses one however it
			// arrives, and this pass mirrors the seam, not an argument about
			// reachability
			if !isWritablePropertyKey(key) {
				addIssue(path, "%s", unwritableKeyReason("resolved property key", key))
				continue
			}
			if first, dup := boundBy[key]; dup {
				addIssue(path, "%q and %q both address property %q — keep one", first, term, key)
				continue
			}
			boundBy[key] = term
			// name-over-number properties are named, not numbered (§3). A
			// typo would otherwise import as a raw string onto a
			// number-format property: no error anywhere, and every consumer
			// reads it with an int getter and silently sees the enum's zero.
			// The refusal states the vocabulary, because no schema slot can:
			// a property SPELLING is not fixed to its stored key (a legend
			// may rebind it), so this semantic pass — which runs on the
			// RESOLVED key — is the vocabulary's only enforceable statement.
			if vocab, named := namedEnumProperty(key); named {
				if s, isStr := v.(string); isStr {
					if !vocab.has(s) {
						addIssue(path, "unknown %s %q — one of %s; a raw stored number is also accepted",
							vocab.what, s, vocab.quotedNames())
					}
					continue // a known name, or a raw number: both accepted (§3)
				}
			}
			if reason, wrong := wrongShapeForFormat(key, v); wrong {
				warnIssue(path, "%s", reason)
			}
		}
	}

	// the §2d relation-definition fields: a meaningful value against a format
	// that cannot use it is a WARNING, never an error — the reasoning lives
	// with the check (relationformat.go), beside the export surface it must
	// not contradict (I1)
	propertySettingsIssues(doc, warnIssue)
	// a definition with no identity stays a WARNING, and the reason is I1
	// rather than judgement: Marshal can emit a keyless type document — a
	// snapshot whose type carries no unique key produces one — so refusing it
	// here would make this package reject its own output. Real data never
	// does it (0 of 2,975 corpus definitions), but the invariant is stated
	// over every snapshot, not the likely ones. The cost is measured and
	// real: 8 of 36 type documents authored against the schema shipped with
	// no key and were told they were fine (§15).
	definitionIdentityIssue(doc, warnIssue)

	// the type_settings name-over-number members carry the layout rule (§2a,
	// §3): a typo'd NAME is refused — it would import as a raw string onto a
	// number-format detail, which every consumer reads with an int getter and
	// silently sees as the zero — while a raw number still passes, because a
	// stored value outside the vocabulary round-trips as its number.
	if group, _ := typeSettingsOf(doc); group != nil {
		if s, isStr := group["layout"].(string); isStr && !layoutNames.has(s) {
			addIssue("/type_settings/layout", "unknown layout %q", s)
		}
		if s, isStr := group["default_view"].(string); isStr && !viewTypeNames.has(s) {
			addIssue("/type_settings/default_view", "unknown view type %q", s)
		}
	}

	if defs, hasDefs := typePropertyDefinitionsOf(doc); hasDefs {
		// property_definitions replaces the recommended-relation lists (§2a):
		// a document carrying both is ambiguous. The lists are named by
		// whatever spelling resolves onto them — recommendedListKeys holds
		// stored keys, and the canonical document spells
		// recommended_properties (§3, the v0.38 alias)
		if props, _ := doc["properties"].(map[string]any); props != nil {
			listKeys := make(map[string]bool, len(recommendedListKeys))
			for _, l := range recommendedListKeys {
				listKeys[l.detailKey] = true
			}
			for _, term := range sortedMapKeys(props) {
				if listKeys[resolveDocKey(term)] {
					addIssue("/properties/"+escapeJSONPointer(term),
						"conflicts with type_settings.property_definitions, which replaces this list")
				}
			}
		}
		// name is used only when the property has to be created (§2a); an
		// existing one keeps its own, so renaming a bundled key here reads as
		// working and silently does nothing
		if list := defs; list != nil {
			for i, raw := range list {
				tp, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				// the identity an entry states: its `property` spelling
				// (resolved through the document's own legend below), else
				// its `internal_key`, which IS a stored key and resolves to
				// itself (§2e)
				key, _ := tp[memberProperty].(string)
				resolvedEntryKey := ""
				if key != "" {
					resolvedEntryKey = resolveDocKey(key)
				} else if ik, _ := tp[memberInternalKey].(string); ik != "" {
					key, resolvedEntryKey = ik, ik
				}
				// options declare a select's vocabulary and its display
				// order (§2a); on any other format there is nothing to
				// declare and the array would be silently dropped
				if opts, has := tp["options"].([]any); has && len(opts) > 0 {
					if f, _ := tp["format"].(string); f != "select" && f != "multi_select" {
						shown := f
						if shown == "" {
							shown = "text"
						}
						addIssue(fmt.Sprintf(typePropertyDefinitionsPath+"/%d/options", i),
							"options is only meaningful on select/multi_select, not %q", shown)
					}
					// an option is a bare name or an object carrying a color
					// (§2a), and the two forms name the same vocabulary: the
					// duplicate check has to read across both
					seen := map[string]bool{}
					for j, o := range opts {
						n := optionEntryName(o)
						if seen[n] {
							addIssue(fmt.Sprintf(typePropertyDefinitionsPath+"/%d/options/%d", i, j),
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
						addIssue(fmt.Sprintf(typePropertyDefinitionsPath+"/%d/object_types", i),
							"object_types is only meaningful on objects/files, not %q", shown)
					}
				}
				// a bundled property is used as-is: only the wiring's
				// create path reads these, and it never runs for a key that
				// already exists (§2a). The key slot spells the api slug like
				// every other (§3), so the lookup runs on the resolved key
				if key != "" {
					if rel, err := bundle.GetRelation(domain.RelationKey(resolvedEntryKey)); err == nil && rel != nil {
						if name, _ := tp["name"].(string); name != "" && name != rel.Name {
							warnIssue(fmt.Sprintf(typePropertyDefinitionsPath+"/%d/name", i),
								"%q is a bundled property named %q — this name is ignored; mint a custom key if the label matters",
								key, rel.Name)
						}
						if ots, has := tp["object_types"].([]any); has && len(ots) > 0 &&
							!restatesBundledTargets(ots, rel) {
							warnIssue(fmt.Sprintf(typePropertyDefinitionsPath+"/%d/object_types", i),
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
			checkDataviewViews(block, path, resolveDocKey, addIssue, warnIssue)
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
				if transparentBlockTypes[typ] {
					// §7a: everywhere else a container is lifted and its
					// children take its place. A cell is a position, not a
					// run — there is nowhere to lift to — so this is the one
					// spelling of a container the format cannot read back.
					addIssue(path+"/type", "a cell block cannot be a %s: a transparent container contributes no block of its own, and a cell is a position rather than a run", typ)
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
			// §7a: containment is judged against the LIFTED tree, because
			// that is the tree import builds — `row > group > column` says
			// `row > column`, which is exactly what it becomes. So the
			// effective parent is the nearest ancestor that survives the
			// lift, and a container is itself exempt: it becomes nothing, so
			// there is nothing to place. The message names the effective
			// parent AND the container between, or it reads as wrong to
			// whoever wrote the group.
			if !transparentBlockTypes[typ] {
				j := len(stack) - 1
				for j >= 0 && transparentBlockTypes[stack[j].typ] {
					j--
				}
				if j >= 0 {
					parent := stack[j]
					viaGroup := j != len(stack)-1
					switch {
					case leafBlockTypes[parent.typ] && viaGroup:
						addIssue(path, "nested under a group inside a %s block — %s blocks cannot have children", parent.typ, parent.typ)
					case leafBlockTypes[parent.typ]:
						addIssue(path, "nested under a %s block — %s blocks cannot have children", parent.typ, parent.typ)
					case parent.typ == "row" && typ != "column" && viaGroup:
						addIssue(path, "nested under a group inside a row — a row block can only contain column blocks, got %s", typ)
					case parent.typ == "row" && typ != "column":
						addIssue(path, "a row block can only contain column blocks, got %s", typ)
					}
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

// transientProperties are stored details that describe a MOMENT rather than
// the object: state the app keeps for its own use, which means nothing once
// the object is out of the space it was written in. Export drops them and
// import ignores them, silently and in both directions — they are noise, not
// input, so a document carrying one is not wrong, merely stale.
//
// This is deliberately NOT neverWritableProperties. Those are refused with an
// error because setting one aims a document at an object it did not create;
// setting one of these achieves nothing at all, and refusing it would turn a
// stale export into an unimportable file for no gain.
//
// Nor is it the local/derived list: a transient key can be an ordinary stored
// detail that survives a restart. What puts it here is that its MEANING does
// not survive the trip.
//
// The list is expected to grow. Each entry needs the same two answers as
// internalFlags: what it means in the app, and why nothing downstream of an
// import can act on it.
//
//   - internalFlags — editor UI state (editorDeleteEmpty, editorSelectType,
//     editorSelectTemplate: "this object was just created, offer the type
//     picker"). Measured across 36,967 real objects it is the single largest
//     source of exported noise, present on 18,647 of them and EMPTY on all
//     of those; a restored object is never mid-creation, so the flags have
//     nothing to say.
var transientProperties = map[string]string{
	"internalFlags": "editor state for an object being created, which a restored object never is",
	// The client's ANALYTICS context, persisted onto the object instead of
	// only being sent as an event. `route` is anytype-ts's analytics-route
	// concept (`analytics.route.shortcut`, `.header`, `.menuSystem`), and
	// `SettingsSpace` names the screen the "create type" click came from.
	// 35 type objects across 7 spaces carry the identical triple
	// — data {"route":"SettingsSpace"}, isNew true, layoutFormat 0 — on
	// ordinary user types (News, Bug report, Meeting, Issue).
	//
	// None of the three is a relation: not bundled, and no relation document
	// defines them anywhere in 38,061 documents. So they are orphan details
	// that no reader can name, give a format to, or act on — and the
	// exhaustive legend rule dutifully pins all three, spending three
	// entries to preserve the identity of something that describes nothing.
	//
	// `isNew` is `internalFlags`' idea exactly: a flag saying the object was
	// just created, which a restored object never was.
	"data":         "the client's analytics route context, recorded on the object rather than sent as an event",
	"isNew":        "a just-created flag, true of the moment and never of the object",
	"layoutFormat": "client layout state written beside the analytics context, defined by no relation",

	// The SOURCE SPACE'S LIVE SESSION — its invite credentials, its invite
	// state and its analytics identity. Every one of these describes the
	// space a bundle was exported FROM, and a space restored from that
	// bundle regenerates all of them; not one is a fact about any object in
	// it.
	//
	// Three of them are secrets. `spaceInviteFileKey` and
	// `spaceInviteGuestFileKey` are, in the bundled table's own words, the
	// "encoded encryption key of invite file" — and a bundle is a SHAREABLE
	// artifact: a use case, a template, a backup someone sends on. Measured
	// before this rule: 74 of 77 exported spaces carried at least one of
	// these, 35 carried the invite key, and `analyticsSpaceId` — a stable
	// per-space tracking identifier — travelled in 50.
	//
	// All ten occur on the space's own document and nowhere else in 38,070
	// corpus documents, so stripping them reaches nothing that wanted them.
	"spaceInviteFileKey":         "the invite file's ENCRYPTION KEY; a bundle is shareable and a restored space mints its own",
	"spaceInviteGuestFileKey":    "the guest invite file's ENCRYPTION KEY; same",
	"oneToOneRequestMetadataKey": "a participant's request-metadata KEY; belongs to the source space's session",
	"spaceInviteFileCid":         "addresses the invite file the key above opens; useless and unwanted once the key is gone",
	"spaceInviteGuestFileCid":    "same, for the guest invite",
	"spaceInvitePermissions":     "the source space's live invite configuration, remade with the new space's own invite",
	"spaceInviteType":            "same",
	"spaceInviteHeldByOwner":     "same",
	"oneToOneInboxSentStatus":    "the source space's inbox session state",
	"analyticsSpaceId":           "an anonymous per-space TRACKING id; it identifies the space it left, not the one being made",

	// DEPRECATED space details. `spaceDashboardId` is `homepage`'s
	// predecessor and its `object` format never told the truth — 46 of the
	// 54 documents carrying both disagree, and its values are the sentinels
	// `chat` and `lastOpened` rather than object ids at all. `homepage`
	// (longtext, "could handle either object id or a sentinel") is the live
	// one and index.json already carries it.
	"spaceDashboardId": "deprecated: homepage's predecessor, and its `object` format holds sentinels, not ids",
	"spaceUxType":      "deprecated",
	"hasChat":          "deprecated",

	// DEPRECATED, and the clearest case of the three: an object's own
	// featured list. The TYPE owns which properties an instance features —
	// `section: "featured"` in its property_definitions — and the clients
	// read it from there, ignoring whatever the object stores.
	//
	// heart is actively migrating the stored ones away. layout/syncer.go
	// rewrites an object's list to empty, keeping `description` if it was
	// there, and the corpus is that migration caught in flight: of 16,927
	// values across 12,603 documents, 6,135 are exactly `["description"]`
	// and 1,285 are exactly `[]` — 59% carrying the syncer's own signature.
	// There is no UI that sets a per-object featured list, so the remaining
	// 41% are not user intent either; they are objects the syncer has not
	// reached, still holding the defaults their type had at creation.
	//
	// Dropping it also retires a lie the format could not otherwise fix: the
	// key is declared `format: "objects"` — an array of object ids — in the
	// bundled table and in all 77 dictionaries, while holding zero object ids
	// in all 16,927 real values. It holds property spellings, camelCase
	// stored keys and bson keys, sometimes mixed inside one array. Nothing
	// resolves it as declared, and writing a real object id there validated,
	// imported and round-tripped with no warning at all.
	"featuredRelations": "deprecated: the type's `section: \"featured\"` owns this, and the clients read it from there",

	// A FILE's variant machinery, and the first of them is a SECRET: the
	// per-variant encryption keys. This package's own API layer already
	// refuses to emit all seven, in its words "so a future change to either
	// the bundle or the cache subscription cannot accidentally leak file keys
	// / CIDs" (core/api/service/property.go) — and the export was shipping
	// every one of them in a bundle built to be shared.
	//
	// Nothing needs them. They are read by `core/files/queries.go` and the
	// file editor, which run in a space that already HOLDS the file; no
	// import path reads any of them, and neither does this format or its
	// tools. A bundle carries the file itself: imported into another space
	// the content matches an existing file and is reused, and imported into
	// another ACCOUNT it becomes a new file with a new encryption key and is
	// uploaded afresh. The old key describes a blob the new account cannot
	// and should not open.
	//
	// They were also 93% of the format's entire warning channel — 71,736
	// warnings, six keys declared `text` and one `number` while every stored
	// value is a list. Not travelling is a better answer than not warning.
	"fileVariantKeys":      "a secret: the per-variant file ENCRYPTION keys, which a shared bundle must not carry",
	"fileVariantIds":       "file variant machinery: regenerated when the file is indexed, and never read on import",
	"fileVariantChecksums": "file variant machinery: regenerated when the file is indexed, and never read on import",
	"fileVariantMills":     "file variant machinery: regenerated when the file is indexed, and never read on import",
	"fileVariantOptions":   "file variant machinery: regenerated when the file is indexed, and never read on import",
	"fileVariantPaths":     "file variant machinery: regenerated when the file is indexed, and never read on import",
	"fileVariantWidths":    "file variant machinery: regenerated when the file is indexed, and never read on import",

	// THE FILE MACHINERY'S per-device answers, stamped on every file object
	// and meaning nothing off the device that stamped them. Their sibling
	// `fileSyncStatus` is in bundle.LocalAndDerivedRelationKeys and has never
	// exported; these two say the same kind of thing and leaked only because
	// the bundle lists them as ordinary relations.
	//
	// `fileBackupStatus` is filesyncstatus.Status — which node-sync state THIS
	// device last observed for the file (core/files/filesync writes it, the
	// reconciler and the sync-status updater read it). All 10,248 file objects
	// in a 28,604-document corpus carry it: 10,246 say Synced(4), 2 say
	// Queued(5). A restored file's backup status is the destination's filesync
	// machinery's to determine — importing "Synced" claims the new space's
	// filenode holds blocks it has never seen. The enum's own comment says
	// even the stored history is untrustworthy: SyncedLegacy exists because a
	// migration "accidentally set FileBackupStatus to Synced for all files,
	// even not synced".
	//
	// `fileIndexingStatus` is the sharpest entry on this list: ONE distinct
	// value — Indexed(1) — across all 10,248 occurrences, which is the
	// definition of saying nothing. And on import it is worse than nothing:
	// the file indexer's queue query selects file objects whose
	// fileIndexingStatus != Indexed (core/files/fileobject/fileindex.go), so
	// an imported "Indexed" tells the destination's indexer the restored file
	// needs no indexing — the one thing downstream that could act on the
	// value acts on it wrongly.
	"fileBackupStatus":   "this device's file-sync answer; the destination's filesync machinery determines its own",
	"fileIndexingStatus": "one distinct value in all real data, and importing it suppresses the destination's file indexer",
}

// isTransientProperty reports whether a stored key describes a moment rather
// than the object. Export skips it; import drops it.
func isTransientProperty(key string) bool {
	_, ok := transientProperties[key]
	return ok
}

// derivedAttributionProperties are the two properties that say WHO — the
// member who created the object and the member who last changed it. They are
// dropped on import for the same reason a transient key is (nothing
// downstream can act on the value), and they are a separate list because
// export treats them differently: a transient key is not written at all,
// while these are written as `<id>#<name>` — the folded participant id with
// the member's name as the informative suffix (§3, §9, buildProperties).
//
// Why nothing downstream can act on the value, which is the entry price for
// this list: both are `source: derived, maxCount: 1, readonly: true`
// (bundle/relations.json). Their value is not stored input — it is recovered
// from the object tree root's cryptographic signature on every rebuild
// (`treeSource.GetCreationInfo` → `NewParticipantId(spaceId, identity)`), and
// four independent seams already discard whatever a document supplies:
// `state.StructCutKeys(details, LocalAndDerivedRelationKeys)`, the pb
// importer's preserve-list (which names neither), `changeBlockDetailsSet`,
// and the API's "cannot be set directly". A document that carries one is
// telling a reader who wrote the object; it is not, and never was, setting
// anything.
//
// The asymmetry this closes: `creator` used to be ACCEPTED on import (it sat
// in propertiesKeptOnExport, so the deny rule never saw it) while
// `lastModifiedBy` — an identical relation definition, one word apart in the
// bundle — was REFUSED. Neither had any effect. 71 documents in a
// 36,966-object corpus carry `lastModifiedBy`, and every one of them was
// unimportable for it.
var derivedAttributionProperties = map[string]string{
	"creator":        "the member who created the object, recovered from the tree root's signature on every rebuild",
	"lastModifiedBy": "the member who last changed the object, derived the same way as creator",
}

// isDroppedOnImport reports whether a stored key is ignored rather than
// refused when a document carries it: the transient keys, whose meaning does
// not survive the trip, and the derived attribution keys, whose value is
// re-derived from the tree and which no write path could honour anyway. Both families
// are stripped from a document's own VALUES like every other internal key —
// what they share is that a stale or hand-written document carrying one is
// still importable (§3).
func isDroppedOnImport(key string) bool {
	if isTransientProperty(key) {
		return true
	}
	_, ok := derivedAttributionProperties[key]
	return ok
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

// rejectValueAt records an issue at a pointer the schema addresses correctly
// but words badly, and silences the schema's own verdict there. It is the
// same trade propertyNameIssues makes for a key slot: one fault, one issue
// (§12), worded by whichever pass can say what is actually wrong.
func (r *keySlotReport) rejectValueAt(path, message string) {
	r.issues = append(r.issues, Issue{Path: path, Message: message})
	r.values[path] = true
}

// propertyNameIssues states, where the key is in hand, every rule the schema
// carries as `propertyNames`: the `properties` map and the `property_internal_keys` /
// `type_internal_keys` legends take a writable key (§3), and `option_ids` takes one at
// its OUTER level with a merely non-empty option name at its inner level
// (§9a). A legend VALUE rides along because it is a stored key under the same
// rule and the schema's verdict on it names the bound, not the string — and so
// does a property-definition entry's `property` (and its `internal_key`), key
// slots the schema can only reach as ordinary string values.
//
// The rule stays in the published schema — an external validator runs that and
// nothing else (§12) — and is restated here because `propertyNames` cannot
// produce the issue §12 promises. The library validates each name as a
// standalone string instance, so its verdict carries neither the enclosing
// object's location nor, for a length bound, the name: a 200-character
// property key was reported as `maxLength: got 200, want 128` at the document
// ROOT, and an agent running the generate → validate → feed-back loop (§13)
// cannot tell from that which property to fix. The predicate is the export
// side's own (isWritablePropertyKey), so the two directions cannot drift into
// Marshal emitting what Validate rejects (§11, I1).
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

	// `option_ids` carries a `propertyNames` at BOTH levels (§9a), and each
	// needs its own case here or the schema's unaddressable root-level verdict
	// comes back for it. The two rules differ on purpose: an outer key is a
	// property spelling like any other, an inner key is an option NAME bounded
	// only by being non-empty — it is the same string the value slot already
	// holds, so any charset rule on it would refuse a legend entry for a value
	// the document itself carries.
	if legend, _ := doc["option_ids"].(map[string]any); legend != nil {
		for _, slug := range sortedMapKeys(legend) {
			path := "/option_ids/" + escapeJSONPointer(slug)
			if !isWritablePropertyKey(slug) {
				rejectName(path, slug, unwritableKeyReason("option_ids property spelling", slug))
			}
			names, isObject := legend[slug].(map[string]any)
			if !isObject {
				continue // the schema types the level; this pass only shapes it
			}
			for _, name := range sortedMapKeys(names) {
				if name != "" {
					continue
				}
				rejectName(path+"/", name,
					"option name is empty — an option_ids entry has to name an "+
						"option the document spells (§9a)")
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
	// a property definition's `property` is a property key slot too (§2a), and
	// the only one that is a JSON string VALUE rather than a member name: the
	// schema carries the rule, but `propertyNames` never sees this slot, so
	// its verdict names a bound or prints a regex instead of saying what is
	// wrong with the string. Same rule, same wording as the members above —
	// and the same reason it is a rule at all: the import seam refuses a key
	// export could not write back, so a document carrying one validated clean
	// and then failed to import (I2).
	if list, _ := typePropertyDefinitionsOf(doc); list != nil {
		for i, raw := range list {
			tp, _ := raw.(map[string]any)
			if key, isString := tp[memberProperty].(string); isString && !isWritablePropertyKey(key) {
				rejectValue(fmt.Sprintf(typePropertyDefinitionsPath+"/%d/"+memberProperty, i),
					unwritableKeyReason("property key", key))
			}
			// internal_key is a stored key under the same writable rule the
			// legend VALUES carry (§3) — the import seam refuses a key export
			// could not write back, whichever member states it
			if key, isString := tp[memberInternalKey].(string); isString && !isWritablePropertyKey(key) {
				rejectValue(fmt.Sprintf(typePropertyDefinitionsPath+"/%d/"+memberInternalKey, i),
					unwritableKeyReason("property internal key", key))
			}
		}
	}
	// A dataview FILTER has to name a property, like the sort and the column
	// beside it (§6). The schema says so — `required: ["property"]` on the
	// leaf branch — but it says it through a `oneOf`, so a filter missing the
	// member collects the OTHER branch's whole verdict as well: "missing
	// properties 'operator', 'filters'" plus one "not allowed" per member it
	// does carry. That is four confidently wrong instructions for one fault,
	// which is the failure mode §12's one fault, one issue rule exists for.
	// So this pass owns the wording and mutes the branch's noise.
	//
	// The rule is not decorative. A filter with no property filters on
	// nothing: import stored it with an empty relation key, the view silently
	// stopped meaning what it said, and export re-emitted the same nameless
	// node forever.
	for i, raw := range blocksOf(doc) {
		block, _ := raw.(map[string]any)
		if block == nil {
			continue
		}
		views, _ := block["views"].([]any)
		for j, rawView := range views {
			view, _ := rawView.(map[string]any)
			if view == nil {
				continue
			}
			nodes, _ := view["filters"].([]any)
			checkFilterProperties(nodes,
				fmt.Sprintf("/blocks/%d/views/%d/filters", i, j), rejectValue)
		}
	}
	for _, field := range []string{memberPropertyInternalKeys, memberTypeInternalKeys} {
		legend, _ := doc[field].(map[string]any)
		for _, term := range sortedMapKeys(legend) {
			path := "/" + field + "/" + escapeJSONPointer(term)
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

// checkFilterProperties walks a view's filter tree and reports every LEAF
// node that names no property. A group node (`operator` + `filters`) names
// none by design, so the walk descends rather than judging it.
func checkFilterProperties(nodes []any, path string, reject func(string, string)) {
	for i, raw := range nodes {
		node, _ := raw.(map[string]any)
		if node == nil {
			continue
		}
		nPath := fmt.Sprintf("%s/%d", path, i)
		if sub, isGroup := node["filters"].([]any); isGroup {
			checkFilterProperties(sub, nPath+"/filters", reject)
			continue
		}
		raw, named := node[memberProperty]
		if !named {
			reject(nPath, "a filter has to name the property it filters on (§6)")
			continue
		}
		if prop, isString := raw.(string); isString && prop == "" {
			reject(nPath+"/property", unwritableKeyReason("property key", prop))
		}
	}
}

// blocksOf is the document's flat block list, or nothing when it has none or
// the member is not a list — the schema types it; this pass only shapes it.
func blocksOf(doc map[string]any) []any {
	blocks, _ := doc["blocks"].([]any)
	return blocks
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
	if isDroppedOnImport(key) {
		// its VALUE is stripped on export like the rest, but the key is
		// DROPPED on import rather than refused: a document carrying transient
		// state or an attribution line is stale, not wrong, and an old export
		// should still import (§3)
		return "", false
	}
	if strippedDetailKeys()[key] {
		return fmt.Sprintf("%q is internal: export strips it, so import does not accept it (§3)", key), true
	}
	// the icon/cover lift (§2b). Unlike the internal keys these DO have a
	// written form, so the refusal names it: the same fact — this key is not
	// where the value lives any more — is worth twice as much said as a
	// repair. The set is the export side's own, never a restatement.
	if liftedDetailKeys()[key] {
		return fmt.Sprintf("%q is written as %s (§2b), not as a property", key, liftedKeyRepair(key)), true
	}
	// the relation-definition lift (§2d), same rule and same derivation. This
	// arm is also the whole of the legacy-input decision (§10): a document
	// written before v0.31 spells `relation_format` here, and it is REFUSED
	// with the repair named rather than read with a warning — the format is a
	// pre-release draft with no external consumers, and a second legal
	// spelling for a relation's format is exactly the ambiguity the lift
	// deletes.
	if propertySettingsLiftedDetailKeys()[key] {
		return fmt.Sprintf("%q is written on a property document's envelope as %s in property_settings (§2d), "+
			"not as a property", key, propertySettingsLiftedKeyRepair(key)), true
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
// listValuedDespiteDeclaration are the keys whose bundled declaration
// disagrees with every value the store has ever held, so the shape warning
// below is about the TABLE rather than the document.
//
// All seven describe a file's variants, all seven sit on every file object,
// and all seven hold a list: six are declared `text` and one (`widths`)
// `number`. In a 77-space export that is 71,736 warnings — 93% of the entire
// warning channel, against 371 warnings that tell a reader something. A
// channel that is 99% noise is a channel nobody reads, which costs the format
// the warnings it actually needs: the unguarded date filter that silently
// widens a view, the group_by a view type cannot honour.
//
// The right repair is in the bundled table, which is not this package's to
// change (§15). Until it happens, the format declines to report a mismatch
// that is universal, expected, and about a declaration no document chose.
var listValuedDespiteDeclaration = map[string]bool{
	"fileVariantChecksums": true,
	"fileVariantIds":       true,
	"fileVariantKeys":      true,
	"fileVariantMills":     true,
	"fileVariantOptions":   true,
	"fileVariantPaths":     true,
	"fileVariantWidths":    true,
}

// restatesBundledTargets reports an `object_types` list that says exactly
// what the bundled table already says for this property.
//
// The warning beside it exists to tell an author their list is IGNORED. That
// is worth saying when the list asks for something the bundle will not
// honour; it is worth nothing when the list is the bundle's own answer
// written out again — and export writes it out again on every bundled
// property that has targets, which was 5,336 warnings in a 77-space export,
// 93% of what remained of the channel. `type` restating `object_type`,
// `creator` restating `participant`, `picture` restating `image`.
//
// A list that DIFFERS still warns, because then something really is being
// discarded.
func restatesBundledTargets(stated []any, rel *model.Relation) bool {
	bundled := map[string]bool{}
	for _, u := range rel.GetObjectTypes() {
		if k, err := bundle.TypeKeyFromUrl(u); err == nil {
			bundled[string(k)] = true
			bundled[TypeKeySpelling(string(k))] = true
		}
	}
	if len(bundled) == 0 {
		return false
	}
	for _, raw := range stated {
		t, _ := raw.(string)
		if !bundled[t] {
			return false
		}
	}
	return true
}

func wrongShapeForFormat(key string, v any) (string, bool) {
	if v == nil {
		return "", false // an explicit null is a value: the key was set (§3)
	}
	if listValuedDespiteDeclaration[key] {
		return "", false
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
	case model.RelationFormat_object, model.RelationFormat_file:
		// a reference is an id, optionally followed by `#name` (§9). A value
		// that BEGINS at the separator has no id half, so it addresses
		// nothing — and the reader will not repair it: splitRefName refuses
		// to split at index 0 precisely so import never invents an empty id,
		// which means the value is stored exactly as written and dangles
		// forever. It is the shape a writer produces copying only the
		// readable half of `id#name`.
		for _, ref := range stringsOf(v) {
			if strings.HasPrefix(ref, refNameSep) {
				return fmt.Sprintf("%q is an object property: %q has no id before its %q, "+
					"so it names no object — a reference is an id, optionally followed by %q (§9)",
					key, ref, refNameSep, refNameSep+"name"), true
			}
		}
	}
	return "", false
}

// stringsOf collects the strings a property value carries, whether it holds
// one or a list of them (§3: a single value and a one-element list are the
// same value).
func stringsOf(v any) []string {
	switch x := v.(type) {
	case string:
		return []string{x}
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
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

// maxDayCount bounds a counting preset's operand — the same bound the compact
// filter grammar puts on `daysAgo(n)` (filterstring.maxDayCount, §6.2.1), and
// for the same reason: past ~100 years the day arithmetic wraps and the range
// stops meaning anything. The two forms of one filter language must admit the
// same filters, so the structured form carries it too.
const maxDayCount = 36500

// dayCountFault reports why a counting preset's operand is not a day count,
// or "" when it is one. Numbers arrive as json.Number here (the document is
// decoded with UseNumber), and as float64 through the fragment surfaces.
func dayCountFault(v any) string {
	var n float64
	switch num := v.(type) {
	case json.Number:
		f, err := num.Float64()
		if err != nil {
			// a number no float64 can hold is checkNumbers' fault to report,
			// at this very pointer; saying it twice is the one-fault-one-issue
			// rule broken (§12)
			return ""
		}
		n = f
	case float64:
		n = num
	default:
		return fmt.Sprintf("%s counts as 0 days, i.e. today — the engine reads the operand as a number or not at all", jsonKindName(v))
	}
	if n != math.Trunc(n) || n < 0 || n > maxDayCount {
		return fmt.Sprintf("%v is not a whole day count between 0 and %d", n, maxDayCount)
	}
	return ""
}

// jsonKindName names a decoded JSON value's kind for a diagnostic, in the
// grammatical form the messages above read it in.
func jsonKindName(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "a boolean"
	case string:
		return "a string"
	case []any:
		return "an array"
	case map[string]any:
		return "an object"
	}
	return "a non-numeric value"
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
				// §7a: the same refusal the array form applies at index 0.
				// A cell is a position, not a run, so a container has nowhere
				// to lift to and import refuses it (import.go's blockFromJSON)
				// — Validate has to refuse it here or the two disagree, which
				// is I2. The array form reaches this through checkFlatRun's
				// `inCell` branch; the object form has no run to walk, so it
				// needs its own.
				if typ, _ := cell["type"].(string); transparentBlockTypes[typ] {
					addIssue(cellPath+"/type", "a cell block cannot be a %s: a transparent container contributes no block of its own, and a cell is a position rather than a run", typ)
					continue
				}
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
func checkDataviewViews(block map[string]any, path string, resolveKey func(string) string,
	addIssue, warnIssue func(string, string, ...any)) {
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
		key, _ := p[memberProperty].(string)
		if f, isStr := p["format"].(string); isStr && key != "" {
			formats[key] = f
		} else if key != "" {
			formats[key] = "text" // §3: an omitted format is text
		}
	}
	// The date rules read the format the IMPORTER will attach, which is not
	// always the one this block declares: impDvFormat rehydrates a filter's
	// format from the dataview's properties list first and the bundled table
	// second (§6.2), and a hand-written dataview usually carries no properties
	// list at all — `due_date` is a date filter there all the same. Checking
	// the declaration alone would leave the rules below silent on exactly the
	// documents an agent writes.
	isDate := func(prop string) bool {
		if declared, listed := formats[prop]; listed {
			return declared == "date"
		}
		f, err := bundle.GetRelationFormat(domain.RelationKey(resolveKey(prop)))
		return err == nil && FormatName(f) == "date"
	}
	for i, raw := range views {
		view, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		checkDateFilters(view, formats, isDate, fmt.Sprintf("%s/views/%d", path, i), addIssue, warnIssue)
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
func checkDateFilters(view map[string]any, formats map[string]string, isDate func(string) bool,
	path string, addIssue, warnIssue func(string, string, ...any)) {
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
				if prop, _ := n[memberProperty].(string); prop != "" &&
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
						if prop, _ := leaf[memberProperty].(string); prop != "" && cond == "empty" {
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
			// the preset's range is applied at all, which takes BOTH halves
			// of transformDateFilter's own gate: a date property (it returns
			// a filter of any other format untouched, before any range is
			// computed) and one of the six conditions that substitute the
			// range (datePresetConditions). A count nothing reads is not
			// missing, and rejecting the document for it refused one the app
			// runs exactly as written.
			if preset, _ := n["date_preset"].(string); preset != "" {
				_, counts := countingPresetNames[preset]
				leafCond, _ := n["condition"].(string)
				_, applies := datePresetConditions[leafCond]
				prop, _ := n[memberProperty].(string)
				switch {
				case !applies:
					// the condition in front of us settles it: this preset
					// decides nothing, and a view written as "edited in the
					// last week" matches on the condition alone. A WARNING
					// and not an error, because export must stay lossless —
					// stored filters carry these pairs, the app keeps them as
					// UI state, and refusing them would make one unexportable
					// object out of every one that has one (§11, I1).
					//
					// The format half of the same gate stays silent on
					// purpose: on the document path a filter's format usually
					// comes from outside the document, so "not a date" there
					// is as often "not known here", and a warning that fires
					// on a correct filter is what makes every warning cheaper
					// to ignore (§12).
					under := "a leaf with no condition"
					if leafCond != "" {
						under = fmt.Sprintf("condition %q", leafCond)
					}
					warnIssue(nPath, "date_preset %q is ignored under %s; a preset's range is applied on %s",
						preset, under, strings.Join(sortedKeys(datePresetConditions), " · "))
				case counts && isDate(prop):
					// the operand has to BE a day count, not merely be
					// present: the engine reads it with domain.Value.Int64,
					// which answers 0 for a null, a string or anything else
					// that is not a number — the very reading this message
					// warns about. A presence-only rule refused the missing
					// operand and admitted `"value": null`, which means the
					// same thing and says it less honestly.
					v, has := n["value"]
					switch {
					case !has:
						addIssue(nPath, "date_preset %q needs a day count in \"value\"; without one it means 0 days, i.e. today", preset)
					default:
						if fault := dayCountFault(v); fault != "" {
							addIssue(nPath+"/value", "date_preset %q needs a day count in \"value\": %s", preset, fault)
						}
					}
				}
			}
			// a dynamic filter token resolves to an object id, so it can
			// only match an object/file property; anywhere else it is
			// compared as a literal string and matches nothing
			if prop, _ := n[memberProperty].(string); prop != "" {
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
			prop, _ := n[memberProperty].(string)
			if !isDate(prop) || scope[prop] {
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
