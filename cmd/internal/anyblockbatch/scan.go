// Package anyblockbatch holds the cross-document concerns of an AnyBlock JSON
// bundle. pkg/lib/anyblockjson is deliberately one-document-at-a-time and
// leaves these to "the import wiring" (SPEC.md §3); both anyblockconvert and
// anyblockvalidate are that wiring, so the batch-wide property-format
// registry and the checks over it live here rather than in either command.
package anyblockbatch

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// FormatByName is the SPEC.md §3 format vocabulary anyblockjson accepts in
// typeProperties entries and property values. It's small and spec-fixed but
// unexported inside pkg/lib/anyblockjson, so it's replicated here.
// "text" is the only text format (§3): shorttext has no name of its own, so
// a property declared here as text is minted as longtext. Properties whose
// stored format is shorttext are the bundled ones, which this tool never
// mints — anyblockjson resolves those by key.
var FormatByName = map[string]model.RelationFormat{
	"text":         model.RelationFormat_longtext,
	"number":       model.RelationFormat_number,
	"select":       model.RelationFormat_status,
	"multi_select": model.RelationFormat_tag,
	"date":         model.RelationFormat_date,
	"files":        model.RelationFormat_file,
	"checkbox":     model.RelationFormat_checkbox,
	"url":          model.RelationFormat_url,
	"email":        model.RelationFormat_email,
	"phone":        model.RelationFormat_phone,
	"objects":      model.RelationFormat_object,
}

// FormatInfo is what the batch knows about a custom property key, gathered
// from every objectType document's typeProperties (§2a) before any document
// is actually converted. The map that holds it is keyed by the STORED
// property key each entry's identity term resolves to (propertyterm.go), which
// is what the converter reads it by.
type FormatInfo struct {
	Format     model.RelationFormat
	FormatName string
	Name       string
	// Options is the declared select vocabulary, in display order (§2a),
	// each entry carrying the color it declares (empty = the batch picks).
	Options []anyblockjson.OptionDefinition
}

type typePropRaw struct {
	// Property is the entry's document-facing spelling, InternalKey the
	// stored id export writes beside it (SPEC.md §2e). The prescan names an
	// entry by whichever it states, spelling first — the same order the
	// codec's authoredKey runs.
	Property    string `json:"property"`
	InternalKey string `json:"internal_key"`
	Name        string `json:"name"`
	Format      string `json:"format"`
	// OptionDefinition decodes both §2a forms (a bare name, or an object with
	// a color), so the prescan shares one decoder with anyblockjson rather
	// than restating the union.
	Options     []anyblockjson.OptionDefinition `json:"options"`
	ObjectTypes []string                        `json:"object_types"`
}

// term is the identity this entry states: its `property` spelling, else its
// `internal_key` (SPEC.md §2e). Empty for a name-only entry, which the
// prescans skip — the codec derives the spelling from the name at import.
func (tp typePropRaw) term() string {
	if tp.Property != "" {
		return tp.Property
	}
	return tp.InternalKey
}

// resolvedKey is the stored key the entry's identity names, by the codec's
// own rule (SPEC.md §2e): an `internal_key` verbatim — a stored id is its
// own address and never re-enters the slug ladder — and a `property`
// spelling through the legend-then-bundled-table ladder like every slot.
func (tp typePropRaw) resolvedKey(legend propertyLegend) string {
	if tp.Property == "" && tp.InternalKey != "" {
		return tp.InternalKey
	}
	return resolvePropertyTerm(legend, tp.term())
}

// typeSettingsRaw is the slice of the §2a group the batch scans read: the
// property definitions moved off the document root into
// `type_settings.property_definitions` in v0.32, and a scanner still reading
// the root would silently see no declarations at all.
type typeSettingsRaw struct {
	PropertyDefinitions *[]typePropRaw `json:"property_definitions"`
}

func (ts *typeSettingsRaw) definitions() *[]typePropRaw {
	if ts == nil {
		return nil
	}
	return ts.PropertyDefinitions
}

// typeSettingsDefs is the nil-safe value form for scanners that only range.
func typeSettingsDefs(ts *typeSettingsRaw) []typePropRaw {
	if defs := ts.definitions(); defs != nil {
		return *defs
	}
	return nil
}

type prescanDoc struct {
	PropertyKeys propertyLegend   `json:"property_internal_keys"`
	TypeSettings *typeSettingsRaw `json:"type_settings"`
}

// ScanFormats reads every document's typeProperties (§2a) once, up front, to
// build a single batch-wide property-key -> format table. typeProperties is
// the only place a custom property's format is declared in the AnyBlock JSON
// format: plain §3 property values don't self-describe their format, so a
// "person" object referencing "team" only resolves correctly if some type
// document's typeProperties already declared "team"'s format — regardless of
// which file the directory walk visits first.
//
// The table is keyed by the STORED key each entry's identity term resolves to,
// because that is what the converter reads it by: anyblockjson hands
// Options.ResolveFormat the output of importer.propertyKey, never the
// spelling. `type_settings.property_definitions[].property` is a translated slot (§3), so it runs the
// chain — this document's own property_internal_keys legend, the bundled table,
// verbatim — first; see propertyterm.go for what keying it raw costs.
func ScanFormats(files []string) (map[string]FormatInfo, error) {
	out := map[string]FormatInfo{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f, err)
		}
		var doc prescanDoc
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("parse %s: %w", f, err)
		}
		if doc.TypeSettings.definitions() == nil {
			continue
		}
		for _, tp := range *doc.TypeSettings.definitions() {
			if tp.term() == "" {
				continue
			}
			key := tp.resolvedKey(doc.PropertyKeys)
			format, ok := FormatByName[tp.Format]
			if !ok {
				// unrecognized or absent format: leave unresolved so the
				// property value passes through raw (degrades the same way
				// an unresolved format does everywhere else in this format,
				// SPEC.md §3).
				continue
			}
			// the fallback display name is the SPELLING, not the resolved
			// key: a legend exists precisely because the stored key is a
			// minted bson nobody wants to read, and this name is what
			// mintRelation writes when the entry declares none.
			name := tp.Name
			if name == "" {
				name = tp.term()
			}
			if existing, seen := out[key]; seen && len(tp.Options) == 0 && len(existing.Options) > 0 {
				// a second type referencing the same property need not
				// repeat its vocabulary
				continue
			}
			if existing, seen := out[key]; seen && existing.Format != format {
				fmt.Fprintf(os.Stderr, "warning: %s: property %q declared with conflicting formats (%s vs %s) — keeping the first seen\n",
					f, tp.term()+resolvedPropertyNote(tp.term(), key), existing.FormatName, tp.Format)
				continue
			}
			out[key] = FormatInfo{Format: format, FormatName: tp.Format, Name: name, Options: tp.Options}
		}
	}
	return out, nil
}

// Undeclared is a property value whose format nothing in the batch
// declares.
type Undeclared struct {
	File string
	// Key is the term as spelled in the document, so the author can find it.
	Key string
	// Resolved is the stored key that term binds to (§3) — what the
	// converter will actually look up, and what a property-definition entry has
	// to end up naming for the finding to go away. Equal to Key whenever the
	// document spells the stored key itself.
	Resolved string
}

// CheckPropertyFormats finds property values whose format cannot be resolved.
// Formats do not travel with values in this format (§3): a value is decoded
// against the format declared for its key, and the only declaration site is
// some type document's typeProperties (§2a) — a dataview's properties[] is a
// per-view cache the converter never reads. When nothing declares a key, the
// value passes through as raw JSON and every format-driven conversion is
// silently skipped: a date stays an RFC-3339 string instead of unix seconds,
// a select mints no RelationOption, an objects value keeps an unresolved id,
// and no Relation object is created for the property at all.
//
// Nothing downstream reports this — the document validates and converts — so
// the batch has to catch it.
//
// A `properties` key is a translated slot (§3): it spells the api slug, and
// binds to a stored key through this document's own property_internal_keys legend,
// then the bundled table, then verbatim (propertyterm.go). Both lookups below
// take the RESOLVED key — the bundled one because `bundle` is keyed by stored
// keys and knows nothing of `due_date`, the batch one because the converter
// reads that table by the resolved key too. Comparing spellings instead was
// wrong both ways: it reported every canonically-spelled bundled property as
// undeclared (a hard error in anyblockconvert), and waved through a
// legend-backed slug whose stored key nothing declares.
func CheckPropertyFormats(files []string, formats map[string]FormatInfo) ([]Undeclared, error) {
	var out []Undeclared
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f, err)
		}
		var doc struct {
			PropertyKeys propertyLegend             `json:"property_internal_keys"`
			Properties   map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("parse %s: %w", f, err)
		}
		keys := make([]string, 0, len(doc.Properties))
		for k := range doc.Properties {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			// id and type are lifted into the envelope, not property values —
			// and the codec skips them on the SPELLING, before any
			// resolution (importer.build), so this must too
			if k == "id" || k == "type" {
				continue
			}
			key := resolvePropertyTerm(doc.PropertyKeys, k)
			if _, err := bundle.GetRelationFormat(domain.RelationKey(key)); err == nil {
				continue
			}
			if _, declared := formats[key]; declared {
				continue
			}
			out = append(out, Undeclared{File: f, Key: k, Resolved: key})
		}
	}
	return out, nil
}

// DiscoverJSONFiles walks root and returns every .json object document,
// sorted so a batch is deterministic regardless of directory order. The two
// bundle-level documents are excluded: the index (§2c) and the property
// dictionary (§2f) describe the bundle rather than an object, have their own
// schemas, and would fail every object-level check.
func DiscoverJSONFiles(root string) ([]string, error) {
	var files []string
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(p, ".json") &&
			filepath.Base(p) != anyblockjson.IndexFileName &&
			filepath.Base(p) != anyblockjson.PropertiesFileName {
			files = append(files, p)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

// Report renders undeclared properties as one line each, most useful first.
// A term whose legend moves it names the stored key too: the entry that fixes
// the finding has to resolve to THAT key, which the spelling alone does not
// say.
func Report(us []Undeclared) string {
	var b strings.Builder
	for _, u := range us {
		// BOTH homes, deliberately: §2f gave a format two places it can be
		// declared, and naming only one sends an author who wrote the other
		// to undo it.
		fmt.Fprintf(&b, "  %s: property %q has no declared format%s — declare it in properties.json, "+
			"or in some type's type_settings.property_definitions\n",
			u.File, u.Key, resolvedPropertyNote(u.Key, u.Resolved))
	}
	return b.String()
}

// TypeIds maps each type key the bundle defines to the id its document
// carries. A property targeting one of those types must reference it by that
// id, so the importer relinks it with every other reference in the batch.
func TypeIds(files []string) (map[string]string, error) {
	out := map[string]string{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f, err)
		}
		var probe struct {
			Kind string `json:"kind"`
			Key  string `json:"internal_key"`
			Id   string `json:"id"`
		}
		if err := json.Unmarshal(data, &probe); err != nil {
			return nil, fmt.Errorf("parse %s: %w", f, err)
		}
		if probe.Kind == "object_type" && probe.Key != "" {
			// an id-less type still has to be registered: without it,
			// objectTypeIds cannot tell "defined here, but unaddressable"
			// from "bundled", and silently emits a bundled url for a type
			// that is not bundled
			out[probe.Key] = probe.Id
		}
	}
	return out, nil
}

// OrderTypesFirst puts type documents ahead of everything else, preserving
// relative order within each group. A bundle's types declare the schema its
// objects reference — property formats, select vocabularies, the relations
// that must exist — so converting them first means every declaration is in
// place before the first usage of it. Alphabetically the walk yields
// chats/, objects/, pages/, types/, i.e. exactly backwards.
func OrderTypesFirst(files []string) ([]string, error) {
	var types, rest []string
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f, err)
		}
		var probe struct {
			Kind string `json:"kind"`
		}
		if err := json.Unmarshal(data, &probe); err != nil {
			return nil, fmt.Errorf("parse %s: %w", f, err)
		}
		if probe.Kind == "object_type" {
			types = append(types, f)
		} else {
			rest = append(rest, f)
		}
	}
	return append(types, rest...), nil
}

// SharedSelect is a select property declared by more than one type.
type SharedSelect struct {
	// Key is the STORED property key the declarations share — what the space
	// keys the one option pool by, and what makes them the same property even
	// when two documents spell it differently (§3).
	Key   string
	Types []string // type keys, in declaration order
}

// CheckSharedSelects finds select/multiSelect properties declared by more
// than one type. Properties are space-wide, not per-type: two types sharing
// one select share one option pool, so their vocabularies merge into a
// single dropdown and each type's board grows the other's empty columns.
//
// That is right for a property whose value set is genuinely common — `tag`
// exists to be shared — and wrong for the lifecycle selects every schema
// reaches for, where "Status" on a Task and "Status" on a Project name
// different things. Splitting them (taskStatus / projectStatus, labelled
// "Task status") keeps each vocabulary clean.
//
// Reported rather than rejected: only the author knows whether the union is
// the point.
//
// Grouped by the STORED key each `key` term resolves to (§3, propertyterm.go),
// not by its spelling: what merges two vocabularies is naming one property,
// and two documents naming it two ways — one through its property_internal_keys
// legend, one verbatim — merge exactly as hard as two spelling it alike.
// Grouping by spelling missed precisely the collision an author cannot see by
// reading the files side by side.
func CheckSharedSelects(files []string) ([]SharedSelect, error) {
	type decl struct {
		types []string
		seen  map[string]bool
	}
	byKey := map[string]*decl{}
	var order []string

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f, err)
		}
		var doc struct {
			Kind         string           `json:"kind"`
			Key          string           `json:"internal_key"`
			PropertyKeys propertyLegend   `json:"property_internal_keys"`
			TypeSettings *typeSettingsRaw `json:"type_settings"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("parse %s: %w", f, err)
		}
		if doc.Kind != "object_type" {
			continue
		}
		// the envelope `internal_key` is the raw stored key and is never translated
		// (§2), so it is the right label for the type as it stands
		typeName := doc.Key
		if typeName == "" {
			typeName = filepath.Base(f)
		}
		for _, tp := range typeSettingsDefs(doc.TypeSettings) {
			if tp.Format != "select" && tp.Format != "multi_select" {
				continue
			}
			key := tp.resolvedKey(doc.PropertyKeys)
			d, ok := byKey[key]
			if !ok {
				d = &decl{seen: map[string]bool{}}
				byKey[key] = d
				order = append(order, key)
			}
			if !d.seen[typeName] {
				d.seen[typeName] = true
				d.types = append(d.types, typeName)
			}
		}
	}

	var out []SharedSelect
	for _, key := range order {
		if d := byKey[key]; len(d.types) > 1 {
			out = append(out, SharedSelect{Key: key, Types: d.types})
		}
	}
	return out, nil
}

// ReportSharedSelects renders shared selects as one line each.
func ReportSharedSelects(ss []SharedSelect) string {
	var b strings.Builder
	for _, s := range ss {
		fmt.Fprintf(&b, "  property %q is a select shared by %d types (%s) — one option pool space-wide, so their vocabularies merge; split per type unless the union is the point\n",
			s.Key, len(s.Types), strings.Join(s.Types, ", "))
	}
	return b.String()
}

// CheckTargetTypes finds objectTypes entries that cannot resolve to anything.
// A target key must name either a bundled type or a type this bundle defines
// *and* gives an id — a document without an id has nothing for a reference to
// point at, and the reference would otherwise be emitted as a bundled url for
// a type that is not bundled: valid, converted, and dangling.
//
// object_types is a translated type-key slot (§2a), so each entry runs the §3
// chain — this document's own type_internal_keys legend, the bundled table, verbatim —
// before anything is looked up. typeIds is keyed by the untranslated envelope
// key (§2), and matching a term against it raw is both a fail-closed and a
// fail-open bug; see typeterm.go.
//
// The arms are ordered the way batch.objectTypeIds orders them — LOCAL first,
// bundled only as the fallthrough — because a lint that asks the questions in
// a different order than the code it lints answers a different question.
// Checking bundled first short-circuited a bundle that defines an
// `object_type` document with a bundled key and no `id`: the converter takes
// the local arm, finds the empty id, and appends an EMPTY STRING to
// relationFormatObjectTypes, which names nothing and is invisible in every
// UI — while the lint saw `page` in the bundle table and reported clean.
func CheckTargetTypes(files []string, typeIds map[string]string) ([]BadTarget, error) {
	var out []BadTarget
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f, err)
		}
		var doc struct {
			TypeKeys     typeLegend       `json:"type_internal_keys"`
			TypeSettings *typeSettingsRaw `json:"type_settings"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("parse %s: %w", f, err)
		}
		for _, tp := range typeSettingsDefs(doc.TypeSettings) {
			for _, target := range tp.ObjectTypes {
				key := resolveTypeTerm(doc.TypeKeys, target)
				id, defined := typeIds[key]
				note := resolvedNote(target, key)
				shadows := ""
				if bundle.HasObjectTypeByKey(domain.TypeKey(key)) {
					shadows = " (this bundle defines a document with that key, and the converter prefers it over the bundled type of the same name)"
				}
				switch {
				case defined && id == "":
					out = append(out, BadTarget{File: f, Property: tp.term(), Target: target, Reason: "that type is defined here but its document carries no \"id\", so nothing can reference it" + shadows + note})
				case defined:
					// a document in this bundle, with an id to point at
				case bundle.HasObjectTypeByKey(domain.TypeKey(key)):
					// bundled, and this bundle does not shadow it
				default:
					out = append(out, BadTarget{File: f, Property: tp.term(), Target: target, Reason: "no such type: not bundled, and not defined by this bundle" + note})
				}
			}
		}
	}
	return out, nil
}

// BadTarget is an objectTypes entry that cannot resolve to a type.
type BadTarget struct {
	File     string
	Property string
	Target   string
	Reason   string
}

// BadTemplateTarget is a template whose target type cannot be wired.
type BadTemplateTarget struct {
	File   string
	Target string // the templateFor key, empty when the document has none
	Reason string
}

// CheckTemplateTargets finds templates that would import belonging to no type.
// A type's templates are found by querying the targetObjectType detail
// (core/block/template/templateimpl.queryTemplatesByType), and that detail
// holds the target type's *object id* — so templateFor has to name a type this
// bundle defines and gives an id, exactly like an objectTypes target.
//
// Unlike an objectTypes target, a bundled type key is not good enough: a
// bundled url in an object-format detail is passed through untouched on import
// (common.UpdateObjectIDsInRelations -> isBundledObjects), so "_otpage" would
// survive as a literal and match no type in the space. Real exports have a
// type document for every type their templates target, bundled ones included
// (util/builtinobjects/data/*.zip), and so must a bundle here.
//
// Nothing downstream reports any of this — the document validates, converts
// and imports; the template simply never appears under a type — so the batch
// has to catch it.
//
// The gate is `kind`, and only `kind` (§2, v0.22). Whether a document IS a
// template is not a fact about its type term: it used to be read off the
// stored key that term resolved to, through this document's own legend and the
// bundled table, and that made the same field answer two unrelated questions.
// `template_for` is still translated (§3, typeterm.go), because it names a
// type and has to be matched against the bundle's type ids.
func CheckTemplateTargets(files []string, typeIds map[string]string) ([]BadTemplateTarget, error) {
	var out []BadTemplateTarget
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f, err)
		}
		var doc struct {
			Kind         string                     `json:"kind"`
			Type         string                     `json:"type"`
			TemplateFor  string                     `json:"template_for"`
			TypeKeys     typeLegend                 `json:"type_internal_keys"`
			PropertyKeys propertyLegend             `json:"property_internal_keys"`
			Properties   map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("parse %s: %w", f, err)
		}
		if doc.Kind != templateKind {
			continue
		}
		if authoredTargetObjectType(doc.PropertyKeys, doc.Properties) {
			// an explicit id (what a round-tripped export carries) is what the
			// converter keeps, whatever templateFor says
			continue
		}
		if doc.TemplateFor == "" {
			out = append(out, BadTemplateTarget{File: f,
				Reason: `no "template_for": the template would belong to no type, and no type would list it`})
			continue
		}
		key := resolveTypeTerm(doc.TypeKeys, doc.TemplateFor)
		id, defined := typeIds[key]
		note := resolvedNote(doc.TemplateFor, key)
		switch {
		case !defined && bundle.HasObjectTypeByKey(domain.TypeKey(key)):
			out = append(out, BadTemplateTarget{File: f, Target: doc.TemplateFor,
				Reason: `that type is bundled, but a template's target must be a document in this bundle: a bundled url is never relinked on import, so it would match no type — add an object_type document with this key and an "id"` + note})
		case !defined:
			out = append(out, BadTemplateTarget{File: f, Target: doc.TemplateFor,
				Reason: "no such type: not bundled, and not defined by this bundle" + note})
		case id == "":
			out = append(out, BadTemplateTarget{File: f, Target: doc.TemplateFor,
				Reason: `that type is defined here but its document carries no "id", so nothing can reference it` + note})
		}
	}
	return out, nil
}

// authoredTargetObjectType reports whether the document writes the
// targetObjectType detail itself — the value patchTemplateTarget keeps
// whatever template_for says.
//
// The converter reads that detail off the CONVERTED snapshot, i.e. under the
// stored key, so the question here is which SPELLING lands on it — a
// translated property slot, resolved through this document's own
// property_internal_keys legend, then the bundled table, then verbatim (§3). Probing
// the map for the stored key alone was wrong both ways: `target_object_type`
// is the canonical api-slug spelling (§3) and reached the detail while this
// check missed it, reporting a template the converter wires perfectly (a hard
// error in anyblockconvert); and a legend rebinding the `targetObjectType`
// spelling onto some other key means the detail is NOT written, which this
// check took as authored and skipped — the template then imports belonging to
// no type, unreported, which is the whole point of the check.
func authoredTargetObjectType(legend propertyLegend, props map[string]json.RawMessage) bool {
	for slug := range props {
		if resolvePropertyTerm(legend, slug) == string(bundle.RelationKeyTargetObjectType) {
			return true
		}
	}
	return false
}

// ReportTemplateTargets renders unwirable template targets, one per line.
func ReportTemplateTargets(bs []BadTemplateTarget) string {
	var b strings.Builder
	for _, t := range bs {
		if t.Target == "" {
			fmt.Fprintf(&b, "  %s: %s\n", t.File, t.Reason)
			continue
		}
		fmt.Fprintf(&b, "  %s: template_for %q — %s\n", t.File, t.Target, t.Reason)
	}
	return b.String()
}

// ReportTargets renders unresolvable target types, one per line.
func ReportTargets(bs []BadTarget) string {
	var b strings.Builder
	for _, t := range bs {
		fmt.Fprintf(&b, "  %s: property %q targets %q — %s\n", t.File, t.Property, t.Target, t.Reason)
	}
	return b.String()
}

// IndexPath returns the bundle index's path, and whether it exists.
func IndexPath(root string) (string, bool) {
	p := filepath.Join(root, anyblockjson.IndexFileName)
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		return p, true
	}
	return "", false
}

// PropertiesPath returns the bundle's property dictionary path (§2f), and
// whether it exists — IndexPath's rule, at IndexPath's location: both
// bundle-level documents live at the bundle root.
func PropertiesPath(root string) (string, bool) {
	p := filepath.Join(root, anyblockjson.PropertiesFileName)
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		return p, true
	}
	return "", false
}

// CheckBundleIds finds documents that claim an id in the platform's reserved
// `_` namespace (§1). Nothing a bundle ships may live there.
//
// It is not a tidiness rule. The pb importer resolves a link target through
// the bundle's own id map FIRST (common.UpdateLinksToObjects) and only then
// asks widget.IsPredefinedWidgetTargetId, so an object whose id equals a
// reserved listing captures every widget that meant the listing — silently,
// with no finding from any check and no error at import. Keeping the two
// namespaces disjoint by a prefix is what makes that unrepresentable, and it
// stays true as listings are added, which a per-word reservation does not.
//
// The same prefix also covers the bundled objects (`_otpage`, `_brdue_date`)
// and the platform's other addresses (`_missing_object`, `_participant_…`): a
// bundle minting one of those ids would collide with the object the space
// already has.
//
// It also covers the four listings' and two screens' BARE spellings, which is
// the half a prefix rule alone would miss — see IsReservedBundleId.
func CheckBundleIds(files []string) ([]BadTarget, error) {
	var out []BadTarget
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f, err)
		}
		var probe struct {
			Id string `json:"id"`
		}
		if err := json.Unmarshal(data, &probe); err != nil {
			return nil, fmt.Errorf("parse %s: %w", f, err)
		}
		if !anyblockjson.IsReservedBundleId(probe.Id) {
			continue
		}
		var reason string
		switch {
		case anyblockjson.IsReservedWidgetTarget(probe.Id):
			reason = fmt.Sprintf("this is a reserved index.json widget listing (the inventory is %s), "+
				"not an id a bundle may mint — the importer resolves a widget target through the bundle's "+
				"own ids first, so this object would silently capture every widget naming the listing",
				strings.Join(anyblockjson.ReservedWidgetTargets(), ", "))
		case anyblockjson.IsReservedHomepage(probe.Id):
			reason = "this is a reserved index.json homepage screen, not an id a bundle may mint"
		case anyblockjson.IsPlatformId(probe.Id):
			reason = "an id may not begin with \"_\": that prefix is the platform's own address space " +
				"(bundled types and relations, participants, _missing_object) and the reserved index.json " +
				"listings and screens — an object minting one of those ids shadows it, and a widget or " +
				"homepage naming it silently gets this object instead of the built-in"
		default:
			reason = "this is what a reserved index.json listing or screen is called on the wire " +
				"(widget.IsPredefinedWidgetTargetId, setWorkspaceSettings), which is where a widget " +
				"target lands after the format's leading \"_\" is translated off — so an object with " +
				"this id silently captures the built-in that the whole bundle, not just this document, " +
				"may want to name"
		}
		out = append(out, BadTarget{File: f, Property: "id", Target: probe.Id, Reason: reason})
	}
	return out, nil
}

// CheckIndexTargets finds index.json references that name nothing the bundle
// defines. Reserved homepages and reserved widget targets name built-in
// screens and listings, so they are not expected to resolve.
//
// A widget target is checked harder than the others, because it is the only
// reference in the format whose failure is silent: an unresolvable link target
// becomes addr.MissingObject (common.handleLinkBlock), and WidgetObject.Init
// then removes the link and its wrapper. No error reaches the import result —
// the widget simply is not there.
func CheckIndexTargets(idx *anyblockjson.Index, files []string) []BadTarget {
	ids := map[string]bool{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var probe struct {
			Id string `json:"id"`
		}
		if json.Unmarshal(data, &probe) == nil && probe.Id != "" {
			ids[probe.Id] = true
		}
	}

	var out []BadTarget
	if e := idx.Entrypoint; e != "" && !ids[e] {
		out = append(out, BadTarget{
			File: anyblockjson.IndexFileName, Property: "entrypoint", Target: e,
			Reason: "no object with that id in the bundle — the install would open nothing",
		})
	}
	if h := idx.Homepage; h != "" && !anyblockjson.IsReservedHomepage(h) && !ids[h] {
		out = append(out, BadTarget{
			File: anyblockjson.IndexFileName, Property: "homepage", Target: h,
			Reason: "no object with that id in the bundle (and it is not a reserved homepage)",
		})
	}
	for i, w := range idx.Widgets {
		switch {
		case anyblockjson.IsReservedWidgetTarget(w.Target):
			if anyblockjson.IsImportableWidgetTarget(w.Target) {
				continue
			}
			// unreachable while the two inventories agree — every reserved
			// listing is importable today — but the day one is added to the
			// format before the importer learns it, this is the check that
			// keeps the failure loud instead of a widget silently gone
			out = append(out, BadTarget{
				File: anyblockjson.IndexFileName, Property: fmt.Sprintf("widgets[%d]", i), Target: w.Target,
				Reason: "a reserved listing the importer does not recognise " +
					"(widget.IsPredefinedWidgetTargetId), so this link is rewritten to " +
					"_missing_object and the widget is dropped without an error",
			})
		case !ids[w.Target]:
			out = append(out, BadTarget{
				File: anyblockjson.IndexFileName, Property: fmt.Sprintf("widgets[%d]", i), Target: w.Target,
				Reason: "no object with that id in the bundle (and it is not a reserved widget target)",
			})
		}
	}
	// the auto-widget ledger's entries are target-shaped references too; an
	// entry naming nothing is not silent-lossy like a widget (nothing renders
	// it), but it points a restored client at an object that is not there
	for i, target := range idx.AutoWidgetTargets {
		if anyblockjson.IsReservedWidgetTarget(target) || ids[target] {
			continue
		}
		out = append(out, BadTarget{
			File: anyblockjson.IndexFileName, Property: fmt.Sprintf("auto_widget_targets[%d]", i), Target: target,
			Reason: "no object with that id in the bundle (and it is not a reserved widget target)",
		})
	}
	return out
}

// ObjectNames maps every id the bundle defines to that object's name. The
// installer resolves a space icon by image name rather than by id
// (builtinobjects.getNewAvatarId), so the wiring needs this to turn an
// index.json iconImage reference into the name the profile carries.
func ObjectNames(files []string) (map[string]string, error) {
	out := map[string]string{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f, err)
		}
		var probe struct {
			Id         string `json:"id"`
			Properties struct {
				Name string `json:"name"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(data, &probe); err != nil {
			return nil, fmt.Errorf("parse %s: %w", f, err)
		}
		if probe.Id != "" {
			out[probe.Id] = probe.Properties.Name
		}
	}
	return out, nil
}

// DictionaryFormats reads the bundle's property dictionary (§2f) into the
// same batch-wide table ScanFormats builds, plus the full definitions for
// pre-minting. The dictionary is where an author declares a property WITHOUT
// writing a relation document — the same vocabulary as a type's
// property-definition entry, one file for the whole bundle — so its entries
// join the format registry exactly as type-declared ones do, and the caller
// merges with the dictionary as the authority: the dictionary is the
// property's one home (§2e), a type entry its per-type use.
//
// Keys arrive as STORED keys (§2f), so unlike ScanFormats there is no legend
// chain to run: the table is keyed by what the file spells.
func DictionaryFormats(path string) (map[string]FormatInfo, []anyblockjson.PropertyDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}
	dict, err := anyblockjson.UnmarshalPropertyDictionary(data)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", anyblockjson.PropertiesFileName, err)
	}
	out := map[string]FormatInfo{}
	for _, def := range dict.Properties {
		out[string(def.Key)] = FormatInfo{
			Format:     def.Format,
			FormatName: anyblockjson.FormatName(def.Format),
			Name:       def.Name,
			Options:    def.Options,
		}
	}
	return out, dict.Properties, nil
}

// MergeDictionaryFormats folds the dictionary's table into the type-scanned
// one, dictionary winning on a conflict — with the conflict SAID, because a
// type entry disagreeing with the dictionary means the bundle contradicts
// itself and silence would let whichever file loaded last decide. A type
// entry that declares a vocabulary the dictionary entry omits keeps that
// vocabulary: the dictionary defines the property, the type may still be the
// place its options were spelled out.
func MergeDictionaryFormats(scanned, dict map[string]FormatInfo, warn func(format string, args ...any)) map[string]FormatInfo {
	for key, d := range dict {
		// a BUNDLED key's definition is the code table's, in every space and
		// offline (§7.5a-1) — the dictionary cannot override it, and the
		// tools do not pretend to: the format table below is consulted only
		// for keys the bundled table does not answer for. Said out loud,
		// because the entry is otherwise accepted in silence and the author
		// is left believing a redefinition took effect. The same run then
		// warns from the bundled table about a value the entry declared
		// legal, which reads as the tool contradicting itself.
		if rel, err := bundle.GetRelation(domain.RelationKey(key)); err == nil && rel != nil {
			if d.Format != rel.Format {
				warn("property %q is BUNDLED: the dictionary says %s, the bundled table says %s, "+
					"and the bundled table wins here and in every reader (§7.5a-1) — "+
					"the entry documents the property, it cannot redefine it",
					key, d.FormatName, anyblockjson.FormatName(rel.Format))
			}
			continue
		}
		existing, seen := scanned[key]
		if seen && existing.Format != d.Format {
			warn("property %q: a type declares %s but the dictionary says %s — the dictionary wins (§2f)",
				key, existing.FormatName, d.FormatName)
		}
		if seen && len(d.Options) == 0 && len(existing.Options) > 0 {
			d.Options = existing.Options
		}
		scanned[key] = d
	}
	return scanned
}

// UsedPropertyKeys reports every STORED property key the bundle's documents
// reference — the population the dictionary's `properties` list names (§2f,
// used-only). Two slots count as a reference, resolved through the same
// chain every scan here runs (a document's own property_internal_keys legend, the
// bundled table, verbatim): a `properties` member on any document, and a
// `type_settings.property_definitions[].property`. A dataview's column list is
// deliberately NOT one — it is a per-view cache carrying its own inline
// format (§6.2), so a key that appears there and nowhere else gives a
// reader nothing to look up.
func UsedPropertyKeys(files []string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f, err)
		}
		var doc struct {
			PropertyKeys propertyLegend             `json:"property_internal_keys"`
			Properties   map[string]json.RawMessage `json:"properties"`
			TypeSettings *typeSettingsRaw           `json:"type_settings"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("parse %s: %w", f, err)
		}
		for k := range doc.Properties {
			// id and type are envelope facts, skipped on the SPELLING the
			// way the codec skips them (importer.build)
			if k == "id" || k == "type" {
				continue
			}
			out[resolvePropertyTerm(doc.PropertyKeys, k)] = true
		}
		for _, tp := range typeSettingsDefs(doc.TypeSettings) {
			if tp.term() == "" {
				continue
			}
			out[tp.resolvedKey(doc.PropertyKeys)] = true
		}
	}
	return out, nil
}

// CheckViewProperties reports a view slot — a filter leaf, a sort or a
// column — naming a property nothing in the bundle can resolve.
//
// This is the silent class, and the one a per-document reader cannot judge.
// A filter on a property that exists nowhere does not fail: it matches
// nothing, so the view shows everything it was meant to narrow. Asked to
// "also exclude archived items", six of nine agents produced exactly that —
// a document that validates, imports and round-trips byte-stably, with a new
// filter that is a no-op.
//
// The codec cannot raise it. A custom property whose stored key is already a
// legal spelling — `aroma_notes`, and 112 more in a 77-space corpus — binds
// no legend entry, because the spelling IS the key. Inside one document that
// is indistinguishable from a typo. Only a reader holding the whole bundle
// can tell them apart, and that is this function: `declared` carries every
// key the bundle declares anywhere — the property dictionary, each type's
// property definitions, and every document's legend.
//
// No real export trips it: 1,517 filter leaves across the corpus, none
// unresolved.
func CheckViewProperties(files []string, declared map[string]bool) ([]BadViewProperty, error) {
	var out []BadViewProperty
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f, err)
		}
		var doc struct {
			PropertyKeys propertyLegend `json:"property_internal_keys"`
			Blocks       []struct {
				Views []struct {
					Id      string            `json:"id"`
					Filters []json.RawMessage `json:"filters"`
					Sorts   []viewPropSlot    `json:"sorts"`
					Columns []viewPropSlot    `json:"columns"`
				} `json:"views"`
				Properties []struct {
					Property string `json:"property"`
				} `json:"properties"`
			} `json:"blocks"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("parse %s: %w", f, err)
		}
		for _, b := range doc.Blocks {
			// a dataview's own properties list declares what its views may
			// name, whether or not anything else in the bundle does
			local := map[string]bool{}
			for _, p := range b.Properties {
				local[p.Property] = true
			}
			for _, v := range b.Views {
				at := v.Id
				if at == "" {
					at = "(unnamed view)"
				}
				judge := func(prop, what string) {
					if prop == "" || local[prop] {
						return
					}
					key := resolvePropertyTerm(doc.PropertyKeys, prop)
					if declared[key] || declared[prop] || bundle.HasRelation(domain.RelationKey(key)) {
						return
					}
					out = append(out, BadViewProperty{
						File: f, View: at, Slot: what, Property: prop,
					})
				}
				for _, raw := range v.Filters {
					for _, prop := range filterLeafProperties(raw) {
						judge(prop, "filter")
					}
				}
				for _, s := range v.Sorts {
					judge(s.Property, "sort")
				}
				for _, c := range v.Columns {
					judge(c.Property, "column")
				}
			}
		}
	}
	return out, nil
}

type viewPropSlot struct {
	Property string `json:"property"`
}

// BadViewProperty is a view slot naming a property nothing in the bundle
// declares.
type BadViewProperty struct {
	File     string
	View     string // the view's id
	Slot     string // "filter", "sort" or "column"
	Property string
}

// ReportViewProperties renders the findings, one line each, saying what the
// slot does instead of what it was meant to do.
func ReportViewProperties(bs []BadViewProperty) string {
	effect := map[string]string{
		"filter": "narrows nothing, so the view shows everything",
		"sort":   "leaves the order untouched",
		"column": "stays empty",
	}
	var b strings.Builder
	for _, x := range bs {
		what := effect[x.Slot]
		if what == "" {
			what = "does nothing"
		}
		fmt.Fprintf(&b, "  %s\n    view %q: the %s on %q %s — no document, no type and no property "+
			"dictionary declares that property, and it is not a bundled one. Nothing reports it at import.\n",
			x.File, x.View, x.Slot, x.Property, what)
	}
	return b.String()
}

// filterLeafProperties returns the properties a filter node names, descending
// through group nodes, which name none by design.
func filterLeafProperties(raw json.RawMessage) []string {
	var node struct {
		Property string            `json:"property"`
		Filters  []json.RawMessage `json:"filters"`
	}
	if json.Unmarshal(raw, &node) != nil {
		return nil
	}
	if len(node.Filters) > 0 {
		var out []string
		for _, sub := range node.Filters {
			out = append(out, filterLeafProperties(sub)...)
		}
		return out
	}
	if node.Property == "" {
		return nil
	}
	return []string{node.Property}
}
