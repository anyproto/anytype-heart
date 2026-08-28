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
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
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
	// the semantic rules run BEFORE the schema: they are stated on the
	// resolved property key and say which key was written and why an author
	// does not write it, where the schema's literal list can only say that
	// some member matched a `not`. The schema still catches everything the
	// semantic rules do not.
	if err := authoringSemantics(data); err != nil {
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

// authoringSemantics is the authoring subset's rules that cannot be written
// as a JSON Schema keyword, and the reason is one thing: **the format
// addresses a property by its display NAME, and resolution is case- and
// separator-insensitive.** "Name", "name" and "NAME" are one key; so are
// "Creation date", "created_date" and "createdDate". JSON Schema's
// `required` and `not/enum` are literal member matches, so a rule written
// there holds for exactly one spelling of a key the codec accepts in many —
// which is not a narrower rule, it is a rule with holes in it.
//
// Two rules used to live in the schema and were losing:
//
//   - a type document must name itself. Written as
//     `properties: {required: ["name"]}`, it REFUSED the canonical
//     `{"Name": "Habit"}` and accepted only the retired lowercase spelling.
//     Swapping the literal would have moved the hole, not closed it.
//   - the subset refuses the app's own derived keys in `properties`. The
//     schema's literal list still bans the pre-raw-name spellings, so the
//     nine keys the FULL format does not also refuse — attribution,
//     timestamps, revision, internal flags, featured properties, archived —
//     lost their authoring refusal the moment their canonical spelling
//     became a display name. They are dropped at import instead, silently.
//
// Both are enforced here on the RESOLVED key, through the same chain
// Validate's own admission loop uses, so every spelling of a key gets one
// verdict. The schema keeps its literal list as documentation and as a fast
// front door — an agent reads the schema, not this file — and
// TestAuthoringDeniedKeysMatchTheSchema pins the two together so the list
// cannot rot again.
func authoringSemantics(data []byte) error {
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return &ValidationError{Issues: []Issue{{Message: fmt.Sprintf("invalid JSON: %v", err)}}}
	}
	var issues []Issue
	add := func(path, format string, args ...any) {
		issues = append(issues, Issue{Path: path, Message: fmt.Sprintf(format, args...)})
	}

	props, _ := doc["properties"].(map[string]any)
	legend, _ := doc[memberPropertyInternalKeys].(map[string]any)
	resolve := func(term string) string {
		if v, ok := legend[term]; ok {
			if key, isStr := v.(string); isStr && key != "" {
				return key
			}
		}
		key, _ := BundledKeyVocabulary{}.PropertyKey(term)
		return key
	}

	namesItself := false
	terms := make([]string, 0, len(props))
	for term := range props {
		terms = append(terms, term)
	}
	sort.Strings(terms)
	for _, term := range terms {
		key := resolve(term)
		path := "/properties/" + escapeJSONPointer(term)
		if key == bundle.RelationKeyName.String() {
			namesItself = true
		}
		if reason, denied := authoringDeniedPropertyKeys[key]; denied {
			add(path, "%q is %s — the app derives it, so an author does not write it. "+
				"Every spelling of that key is refused here, not just this one", key, reason)
			continue
		}
		// the subset narrows a name-over-number key to the NAME. The full
		// format also accepts the stored number, because export writes what
		// a space stores; an author has no number to carry over and a bare
		// integer is unreadable, so the subset takes the name only. Keyed
		// off the resolved key for the reason everything here is: the
		// canonical spelling of `layoutAlign` is "Layout align", and a rule
		// written against the member name `layout_align` stopped covering it.
		if vocab, named := namedEnumProperty(key); named {
			if _, isStr := props[term].(string); !isStr {
				add(path, "%q takes a %s NAME here, one of %v — the raw stored number is "+
					"the full format's pass-through, which the subset removes", key, vocab.what, vocab.names())
			}
		}
	}

	// a type document must name itself: the type's display name is the one
	// thing nothing else in the document can supply, and a nameless type
	// arrives in a space as an untitled row
	if isTypeKind(doc) && !namesItself {
		add("/properties", "a type document states its own display name here — "+
			"any spelling that resolves to the name property (\"Name\") will do")
	}

	if len(issues) == 0 {
		return nil
	}
	preamble := Issue{Message: "valid AnyBlock JSON, but outside the authoring subset (§2g) — " +
		"the rules below are stated on the RESOLVED property key, so they hold for every spelling of it"}
	return &ValidationError{Issues: append([]Issue{preamble}, issues...)}
}

// authoringDeniedPropertyKeys are the stored keys an author never writes in
// `properties` and the FULL format does not already refuse. Everything else
// the authoring schema's literal list names is refused by Validate before
// this pass runs — the deny rule (import refuses exactly what export
// strips) covers ids, icons, covers, spaceId, uniqueKey, snippet,
// backlinks, links, mentions, origin, importType, restrictions and the
// rest. These nine are the remainder: import DROPS them rather than
// refusing them, so without a rule here an author's value disappears
// without a word.
var authoringDeniedPropertyKeys = map[string]string{
	"creator":          "attribution the app stamps from the acting identity",
	"lastModifiedBy":   "attribution the app stamps from the acting identity",
	"createdDate":      "a timestamp the app stamps",
	"lastModifiedDate": "a timestamp the app stamps",
	"addedDate":        "a timestamp the app stamps",
	"revision":         "the bundled revision the app records",
	"internalFlags":    "the app's own creation-flow state",
	"featuredRelations": "a per-object featured list no UI sets: the layout syncer owns it, " +
		"and a type's featured properties belong in that type's recommended lists",
	"isArchived": "the app's bin membership, moved by archiving an object rather than by writing a property",
}
