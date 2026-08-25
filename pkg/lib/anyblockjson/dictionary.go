package anyblockjson

// dictionary.go implements §2f: the bundle-level property dictionary,
// properties.json. Every other document in this format describes one object
// and index.json describes the set; the dictionary says what the set's
// PROPERTIES mean — one file naming every property the bundle's objects use,
// in place of the ~9,500 relation documents per account that restated the
// bundled table field for field (measured: 9,675 of 10,617 relation
// documents are installed copies of the 194 bundled relations, and 98% of
// those are field-identical to bundle/relations.json).
//
// It is a sibling of index.json, not a section inside it, deliberately: an
// index says WHERE things are, a dictionary says WHAT THEY MEAN (§2f). And
// it is the third home of $defs/propertyDefinition (§2e) — a dictionary
// entry, a type's property-definition entry and a relation document's
// relation_settings are one shape in three places, which is why the Go
// surface here is []PropertyDefinition rather than a fourth field list.
//
// Self-sufficiency is the constraint that shapes it: a third-party reader
// must be able to interpret a backup WITHOUT shipping bundle/relations.json,
// so every entry carries its `format`. Dropping bundled relation documents
// with no dictionary was considered and rejected for exactly this reason —
// the reader could no longer tell a date from a string, which is the same
// "stands alone" property that keeps a space id off the envelope.

import (
	"bytes"
	_ "embed"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

//go:embed schema/properties.schema.json
var propertiesSchemaJSON []byte

// PropertiesFileName is the name a bundle's property dictionary must have,
// at the bundle root beside index.json — IndexFileName's rule (§2f).
const PropertiesFileName = "properties.json"

// PropertyDictionary is a bundle's properties.json (§2f).
type PropertyDictionary struct {
	// Installed lists the BUNDLED property keys present in the space, as
	// stored keys only — presence, not definition: 98% of installed copies
	// are field-identical to the bundled table, so the key is the whole of
	// what a restore needs. A key that also appears in Properties is
	// installed AND divergent: the entry overrides the table.
	Installed []string
	// Properties carries one definition per property the bundle's objects
	// actually reference — used-only (§2f) — plus a full entry for every
	// installed copy that diverges from the bundled table. Keys are STORED
	// keys, never document spellings: a document's property_keys legend
	// binds its labels to stored keys, and the stored key is what the
	// dictionary answers for.
	Properties []PropertyDefinition
}

var compilePropertiesSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(propertiesSchemaJSON))
	if err != nil {
		return nil, fmt.Errorf("decode embedded properties schema: %w", err)
	}
	c := jsonschema.NewCompiler()
	// the object schema is added alongside because a dictionary entry is a
	// $ref into it (§2e): the three homes of propertyDefinition share one
	// $defs rather than a copy in each that drifts — the same wiring the
	// index schema uses for its icon.
	objectDoc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaJSON))
	if err != nil {
		return nil, fmt.Errorf("decode embedded schema: %w", err)
	}
	if err := c.AddResource(SchemaURL, objectDoc); err != nil {
		return nil, fmt.Errorf("add schema resource: %w", err)
	}
	if err := c.AddResource(PropertiesSchemaURL, doc); err != nil {
		return nil, fmt.Errorf("add properties schema resource: %w", err)
	}
	sch, err := c.Compile(PropertiesSchemaURL)
	if err != nil {
		return nil, fmt.Errorf("compile properties schema: %w", err)
	}
	return sch, nil
})

// jsonDictionary is the decoded properties.json. Entries decode through the
// same JSON layer as a type document's property-definition entries
// (TypeProperty) so the two doors cannot disagree about which members travel
// — `section` never arrives, because the schema refuses it on a dictionary
// entry before this decode runs.
type jsonDictionary struct {
	Installed  []string       `json:"installed"`
	Properties []TypeProperty `json:"properties"`
}

// UnmarshalPropertyDictionary validates data against the properties schema
// and decodes it (§2f). Errors wrap *ValidationError with path-addressed
// issues, like Unmarshal and UnmarshalIndex.
//
// An `installed` key the bundled table does not know is TOLERATED, not
// refused, and the asymmetry with MarshalPropertyDictionary is deliberate:
// the bundled table grows independently of the format version, so a backup
// written by a newer app lists keys an older reader has never heard of —
// refusing them would make every backup unreadable one app version back.
// The reader installs the keys it knows and skips the rest; a WRITER, which
// checks against its own table, has no such excuse.
func UnmarshalPropertyDictionary(data []byte) (*PropertyDictionary, error) {
	return UnmarshalPropertyDictionaryWarn(data, nil)
}

// UnmarshalPropertyDictionaryWarn is UnmarshalPropertyDictionary with a sink
// for warning-grade issues, the way ValidateWarn is for object documents.
//
// The dictionary had no such sink, and it is the one file in the format whose
// keys are STORED keys while every other slot spells the snake_case label —
// so the likeliest authoring mistake, writing the label, produced NOTHING on
// the way in. An `installed` key outside the bundled table read clean and
// failed only on the way back out; a `properties` entry keyed by the label
// read clean and quietly minted a second property beside the bundled one.
func UnmarshalPropertyDictionaryWarn(data []byte, onWarning func(Issue)) (*PropertyDictionary, error) {
	return unmarshalPropertyDictionary(data, onWarning)
}

func unmarshalPropertyDictionary(data []byte, warn func(Issue)) (*PropertyDictionary, error) {
	raw, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, &ValidationError{Issues: []Issue{{Message: fmt.Sprintf("invalid JSON: %v", err)}}}
	}
	doc, ok := raw.(map[string]any)
	if !ok {
		return nil, &ValidationError{Issues: []Issue{{Message: "property dictionary must be a JSON object"}}}
	}
	// the dictionary shares the format version and its rules with object
	// documents (§10): gate on it here, before the schema can turn a newer
	// version into a generic "value must be 1" that says nothing about why
	if err := checkVersion(doc); err != nil {
		return nil, err
	}
	if issues := misroutedIssues(data, KindPropertyDictionary); len(issues) > 0 {
		return nil, &ValidationError{Issues: issues}
	}
	sch, err := compilePropertiesSchema()
	if err != nil {
		return nil, fmt.Errorf("embedded properties schema: %w", err)
	}
	if err := sch.Validate(raw); err != nil {
		return nil, &ValidationError{Issues: schemaIssues(err, keySlotReport{})}
	}
	if issues := dictionaryDuplicateIssues(doc); len(issues) > 0 {
		return nil, &ValidationError{Issues: issues}
	}

	var jd jsonDictionary
	if err := jsonUnmarshal(data, &jd); err != nil {
		return nil, fmt.Errorf("decode property dictionary: %w", err)
	}
	d := &PropertyDictionary{Installed: installedKeys(jd.Installed, warn)}
	for i, tp := range jd.Properties {
		storedKey := dictionaryEntryKey(i, tp.Key, warn)
		// entries speak STORED keys in every key slot — `key` and
		// `object_types` alike — so there is no legend to run and no
		// vocabulary to consult: the definition is built by the same shared
		// builder both doors of the §2a array use, with the slots passed
		// through verbatim. `format` resolves per key exactly as a
		// relation_settings format does (§3): "text" on a bundled
		// short-text key stays short text, and on anything else is longtext.
		def := tp.definition(storedKey, declaredFormatWith(Options{}, storedKey, tp.Format), tp.ObjectTypes)
		d.Properties = append(d.Properties, def)
	}
	return d, nil
}

// dictionaryKeySpelling renders an ENTRY's stored key the way the dictionary
// spells it: the api slug for a bundled property, the stored key verbatim for
// anything else (§2f).
//
// Only `properties` needs the condition. `installed` admits bundled keys and
// nothing else — it names rows to restore from the bundled table — so it
// slugs unconditionally. An ENTRY, by contrast, is how a bundle declares a
// property the bundled table does NOT have, so its key population is mixed:
// of 6,426 entries in a 77-space export, 515 are space-minted bson ids.
//
// For those the condition is load-bearing rather than cosmetic.
// `bundle.ApiSlug` is `strcase.ToSnake`, and
// `ApiSlug("6a32d4856761631534b22f85")` is
// `"6_a_32_d_4856761631534_b_22_f_85"` — a key that names nothing. Slugging
// unconditionally would corrupt one entry key in twelve.
func dictionaryKeySpelling(storedKey string) string {
	if bundle.HasRelation(domain.RelationKey(storedKey)) {
		return bundle.ApiSlug(storedKey)
	}
	return storedKey
}

// dictionaryStoredKey resolves a dictionary spelling back to the stored key
// it names, following the same ladder every other slot in the format follows:
// an exact stored key wins, then a single fold match, and an ambiguity is
// never resolved by guess (bundle.RelationKeysByApiFold).
//
// ok is false only when the spelling folds onto more than one bundled
// property, which cannot happen for a slug this package wrote —
// TestDictionaryKeys_TheBundledTableStaysUnambiguous pins that — but can for
// one an author invents.
func dictionaryStoredKey(spelling string) (stored string, ambiguous []string) {
	if bundle.HasRelation(domain.RelationKey(spelling)) {
		return spelling, nil // a stored key names itself
	}
	candidates := bundle.RelationKeysByApiFold(spelling)
	switch len(candidates) {
	case 1:
		return candidates[0].String(), nil
	case 0:
		return spelling, nil // a space-minted key, or a newer app's
	default:
		names := make([]string, 0, len(candidates))
		for _, c := range candidates {
			names = append(names, c.String())
		}
		sort.Strings(names)
		return spelling, names
	}
}

// installedKeys reads the `installed` list into stored keys, reporting a key
// that names no bundled property.
//
// Every key here is meant to be bundled — `installed` names rows to restore
// from the bundled table, and a key outside it tells a reader to install
// nothing. Such a key is TOLERATED rather than refused, and the tolerance is
// about VERSION SKEW rather than custom properties: the bundled table grows independently of the
// format version, so a backup written by a newer app lists keys an older
// reader has never heard of, and refusing them would make every backup
// unreadable one app version back. What was missing is that nothing SAID so —
// the document read clean and only re-rendering it failed.
func installedKeys(raw []string, warn func(Issue)) []string {
	if len(raw) == 0 {
		return raw
	}
	out := make([]string, 0, len(raw))
	for i, spelling := range raw {
		path := fmt.Sprintf("/installed/%d", i)
		stored, ambiguous := dictionaryStoredKey(spelling)
		switch {
		case len(ambiguous) > 0:
			warnIssue(warn, path, "installed key %q folds onto more than one bundled property (%s), "+
				"so which is meant cannot be decided here — write one of them (§2f)",
				spelling, strings.Join(quoteAll(ambiguous), ", "))
		case !bundle.HasRelation(domain.RelationKey(stored)):
			warnIssue(warn, path, "installed key %q is not a bundled property, so a reader "+
				"restoring this bundle installs NOTHING for it. Give it a full entry in "+
				"`properties`, where its definition travels with it — or, if it comes from a "+
				"newer app whose bundled table has it, expect this reader to skip it (§2f)", spelling)
		}
		out = append(out, stored)
	}
	return out
}

// dictionaryEntryKey resolves an entry's key, reporting an ambiguity.
func dictionaryEntryKey(i int, spelling string, warn func(Issue)) string {
	stored, ambiguous := dictionaryStoredKey(spelling)
	if len(ambiguous) > 0 {
		warnIssue(warn, fmt.Sprintf("/properties/%d/key", i),
			"%q folds onto more than one bundled property (%s), so which is meant cannot be "+
				"decided here — write one of them (§2f)",
			spelling, strings.Join(quoteAll(ambiguous), ", "))
	}
	return stored
}

func quoteAll(names []string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, strconv.Quote(n))
	}
	return out
}

func warnIssue(warn func(Issue), path, format string, args ...any) {
	if warn != nil {
		warn(Issue{Path: path, Message: fmt.Sprintf(format, args...)})
	}
}

// dictionaryDuplicateIssues refuses a key stated twice, in either list. Two
// entries for one key are two definitions of one property with no rule for
// which wins — the canonical form has exactly one slot per key, the way a
// document's `properties` object structurally has, and Marshal refuses the
// same input (§11 I1: the two sides owe the same answer).
func dictionaryDuplicateIssues(doc map[string]any) []Issue {
	var issues []Issue
	seenInstalled := map[string]int{}
	installed, _ := doc["installed"].([]any)
	for i, raw := range installed {
		key, _ := raw.(string)
		if first, dup := seenInstalled[key]; dup {
			issues = append(issues, Issue{
				Path: fmt.Sprintf("/installed/%d", i),
				Message: fmt.Sprintf("%q is already listed at /installed/%d — the dictionary has one slot per key",
					key, first),
			})
			continue
		}
		seenInstalled[key] = i
	}
	seenEntries := map[string]int{}
	entries, _ := doc["properties"].([]any)
	for i, raw := range entries {
		entry, _ := raw.(map[string]any)
		key, _ := entry["key"].(string)
		if key == "" {
			continue // the schema's required/minLength verdict already stands
		}
		if first, dup := seenEntries[key]; dup {
			issues = append(issues, Issue{
				Path: fmt.Sprintf("/properties/%d/key", i),
				Message: fmt.Sprintf("%q is already defined at /properties/%d — one property, one definition (§2e)",
					key, first),
			})
			continue
		}
		seenEntries[key] = i
	}
	return issues
}

// MarshalPropertyDictionary renders a dictionary in the canonical byte form
// (§4): `installed` and `properties` each sorted by key, one slot per key.
// It refuses what UnmarshalPropertyDictionary refuses — a duplicated key —
// and two things only a writer can check: an entry whose key has no written
// form, and an `installed` key its own bundled table does not know, which
// would tell the reader to install nothing (the repair is a full entry in
// `properties`, where the format travels with it).
func MarshalPropertyDictionary(d *PropertyDictionary) ([]byte, error) {
	if d == nil {
		return nil, fmt.Errorf("nil property dictionary")
	}
	doc := &omap{}
	doc.set("$schema", PropertiesSchemaURL)
	doc.set("version", FormatVersion)

	// stored keys in, SLUGS out (§2f): every key here is a bundled property,
	// and a bundled property's written spelling is its api slug everywhere
	// else in the format. The dictionary used to be the one file that spelled
	// a property `dueDate` while every document beside it said `due_date`.
	installed := make([]string, 0, len(d.Installed))
	for _, key := range d.Installed {
		if _, err := bundle.GetRelation(domain.RelationKey(key)); err != nil {
			return nil, fmt.Errorf("installed key %q is not a bundled property: `installed` restores from the "+
				"bundled table, so a key outside it tells the reader to install nothing — give it a full "+
				"entry in `properties` instead (§2f)", key)
		}
		// ApiSlug unconditionally, not dictionaryKeySpelling: the check above
		// has already established this key is bundled, and `installed` admits
		// nothing else — it is a list of rows to restore from the bundled
		// table, so a space-minted key has no meaning in it.
		installed = append(installed, bundle.ApiSlug(key))
	}
	sort.Strings(installed)
	for i, key := range installed {
		if i > 0 && installed[i-1] == key {
			return nil, fmt.Errorf("installed key %q is listed twice: the dictionary has one slot per key (§2f)", key)
		}
	}
	doc.setNonEmpty("installed", stringsToAny(installed))

	defs := append([]PropertyDefinition(nil), d.Properties...)
	sort.Slice(defs, func(i, j int) bool { return defs[i].Key < defs[j].Key })
	var entries []any
	for i, def := range defs {
		if i > 0 && defs[i-1].Key == def.Key {
			return nil, fmt.Errorf("property %q is defined twice: one property, one definition (§2e)", def.Key)
		}
		entry, err := dictionaryEntryOmap(def)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	doc.setNonEmpty("properties", entries)
	return marshalCanonical(doc)
}

// dictionaryEntryOmap renders one entry: the propertyDefinition members in
// the §2e order, its key in the dictionary's spelling — the api slug for a
// bundled property, the stored key verbatim for a space-minted one, which is
// the ladder every other slot in the format follows (§2f). There is still no
// legend to write: the spelling is a pure function of the key, so a reader
// inverts it without one. `format` is
// written unconditionally — required by the schema, because an entry without
// one is readable only by a reader shipping the bundled table (§2f) — and a
// stored format outside the enum is an ERROR for relationFormatName's
// reason: writing "text" for a format that is not text would be a permanent
// silent format rewrite, the disease the dictionary must not reintroduce.
func dictionaryEntryOmap(def PropertyDefinition) (*omap, error) {
	if !isWritablePropertyKey(string(def.Key)) {
		return nil, fmt.Errorf("property dictionary: %s", unwritableKeyReason("property key", string(def.Key)))
	}
	spelling := dictionaryKeySpelling(string(def.Key))
	name := formatName(def.Format)
	if name == "" {
		return nil, fmt.Errorf("property %q: format %d has no §3 name: the entry cannot state "+
			"what the property holds (§2f)", def.Key, def.Format)
	}
	m := &omap{}
	m.set("key", spelling)
	m.setNonEmpty("name", def.Name)
	m.set("format", name)
	m.setNonEmpty("options", optionsToAny(def.Options))
	m.setNonEmpty("object_types", stringsToAny(def.ObjectTypes))
	m.setNonEmpty("description", def.Description)
	if def.IncludeTime != nil {
		// a pointer false is a declaration, not an absence — the same
		// distinction the §2a array preserves through both its doors
		m.set("include_time", *def.IncludeTime)
	}
	// the schema bounds max_count to a non-negative int32, and setNonEmpty
	// would have written whatever the caller held: a negative or
	// out-of-range value produced bytes this file's own Unmarshal refuses,
	// which is §11 I1 broken on the shortest possible path. Refused by name,
	// the same treatment `format` gets above — an entry that cannot be read
	// back is not an entry.
	if def.MaxCount < 0 || def.MaxCount > math.MaxInt32 {
		return nil, fmt.Errorf("property %q: max_count %d is outside the range an entry can "+
			"state (0..%d) (§2f)", def.Key, def.MaxCount, math.MaxInt32)
	}
	m.setNonEmpty("max_count", def.MaxCount)
	m.setNonEmpty("readonly", def.Readonly)
	if def.DefaultValue != nil {
		// canonicalize through the same value pipeline every property value
		// takes (§3), so a map-shaped default has sorted members and the
		// output is byte-stable
		m.set("default_value", protoValueToJSON(jsonToProtoValue(def.DefaultValue)))
	}
	return m, nil
}
