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
		shadowedBundledKeyIssue(i, tp.Key, warn)
		// entries speak STORED keys in every key slot — `key` and
		// `object_types` alike — so there is no legend to run and no
		// vocabulary to consult: the definition is built by the same shared
		// builder both doors of the §2a array use, with the slots passed
		// through verbatim. `format` resolves per key exactly as a
		// relation_settings format does (§3): "text" on a bundled
		// short-text key stays short text, and on anything else is longtext.
		def := tp.definition(tp.Key, declaredFormatWith(Options{}, tp.Key, tp.Format), tp.ObjectTypes)
		d.Properties = append(d.Properties, def)
	}
	return d, nil
}

// installedKeys reads the `installed` list, recovering a key written in the
// label spelling and reporting every key the bundled table does not know.
//
// `installed` carries STORED keys — `dueDate`, not `due_date` — and it is the
// only slot in the format that does, which is exactly why authors write the
// other one: within a single real bundle, an object document spells the
// property `created_date` and the dictionary spells it `createdDate`.
//
// A key outside the table is TOLERATED, not refused, and that tolerance is
// deliberate: the bundled table grows independently of the format version, so
// a backup written by a newer app lists keys an older reader has never heard
// of, and refusing them would make every backup unreadable one app version
// back. What was missing is that nothing SAID so — the document read clean
// and only re-rendering it failed.
//
// When the unknown key folds to exactly one bundled key it is recovered to
// it, the forgiving layer's own rule (bundle.RelationKeysByApiFold): exact
// match first, a single fold match second, an ambiguity never resolved by
// guess. A key that folds to nothing is kept verbatim — it may be the newer
// app's.
func installedKeys(raw []string, warn func(Issue)) []string {
	if len(raw) == 0 {
		return raw
	}
	out := make([]string, 0, len(raw))
	for i, key := range raw {
		path := fmt.Sprintf("/installed/%d", i)
		if bundle.HasRelation(domain.RelationKey(key)) {
			out = append(out, key)
			continue
		}
		switch candidates := bundle.RelationKeysByApiFold(key); len(candidates) {
		case 1:
			stored := candidates[0].String()
			warnIssue(warn, path, "installed key %q is spelled the way a document spells it; "+
				"this list carries STORED keys, so it is read as %q. Write %q here — `installed` "+
				"restores from the bundled table, and %q is not in it (§2f)",
				key, stored, stored, key)
			out = append(out, stored)
		case 0:
			warnIssue(warn, path, "installed key %q is not a bundled property, so a reader "+
				"restoring this bundle installs NOTHING for it. Give it a full entry in "+
				"`properties`, where its definition travels with it — or, if it comes from a "+
				"newer app whose bundled table has it, expect this reader to skip it (§2f)", key)
			out = append(out, key)
		default:
			names := make([]string, 0, len(candidates))
			for _, c := range candidates {
				names = append(names, strconv.Quote(c.String()))
			}
			sort.Strings(names)
			warnIssue(warn, path, "installed key %q folds to more than one bundled property (%s), "+
				"so which one is meant cannot be decided here — write the stored key you mean (§2f)",
				key, strings.Join(names, ", "))
			out = append(out, key)
		}
	}
	return out
}

// shadowedBundledKeyIssue reports an entry whose key is the LABEL spelling of
// a bundled property.
//
// Nothing is recovered here, unlike `installed`: an entry carries a full
// definition, and a custom property deliberately named close to a bundled one
// is legitimate — rewriting its key would change what it defines. But an
// entry keyed `due_date` beside the bundled `dueDate` defines a SECOND
// property that merely looks like the first, and every document referring to
// the bundled one keeps referring to the bundled one, so the shadow collects
// nothing.
func shadowedBundledKeyIssue(i int, key string, warn func(Issue)) {
	if warn == nil || key == "" || bundle.HasRelation(domain.RelationKey(key)) {
		return
	}
	candidates := bundle.RelationKeysByApiFold(key)
	if len(candidates) != 1 {
		return
	}
	stored := candidates[0].String()
	warnIssue(warn, fmt.Sprintf("/properties/%d/key", i),
		"%q defines a NEW property that merely looks like the bundled %q — this list carries "+
			"STORED keys, and %q is the label spelling. Documents referring to %q go on referring "+
			"to it, so nothing ever uses this entry. Write %q to define the bundled property, or "+
			"keep this key if a distinct property is what you meant (§2f)",
		key, stored, key, stored, stored)
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

	installed := append([]string(nil), d.Installed...)
	sort.Strings(installed)
	for i, key := range installed {
		if i > 0 && installed[i-1] == key {
			return nil, fmt.Errorf("installed key %q is listed twice: the dictionary has one slot per key (§2f)", key)
		}
		if _, err := bundle.GetRelation(domain.RelationKey(key)); err != nil {
			return nil, fmt.Errorf("installed key %q is not a bundled property: `installed` restores from the "+
				"bundled table, so a key outside it tells the reader to install nothing — give it a full "+
				"entry in `properties` instead (§2f)", key)
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
// the §2e order, keys verbatim (stored keys are the dictionary's spelling,
// so there is no slug to translate and no legend to write). `format` is
// written unconditionally — required by the schema, because an entry without
// one is readable only by a reader shipping the bundled table (§2f) — and a
// stored format outside the enum is an ERROR for relationFormatName's
// reason: writing "text" for a format that is not text would be a permanent
// silent format rewrite, the disease the dictionary must not reintroduce.
func dictionaryEntryOmap(def PropertyDefinition) (*omap, error) {
	if !isWritablePropertyKey(string(def.Key)) {
		return nil, fmt.Errorf("property dictionary: %s", unwritableKeyReason("property key", string(def.Key)))
	}
	name := formatName(def.Format)
	if name == "" {
		return nil, fmt.Errorf("property %q: format %d has no §3 name: the entry cannot state "+
			"what the property holds (§2f)", def.Key, def.Format)
	}
	m := &omap{}
	m.set("key", string(def.Key))
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
