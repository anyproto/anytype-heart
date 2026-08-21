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
// property key each entry's `key` term resolves to (propertyterm.go), which
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
	Key    string `json:"key"`
	Name   string `json:"name"`
	Format string `json:"format"`
	// OptionDefinition decodes both §2a forms (a bare name, or an object with
	// a color), so the prescan shares one decoder with anyblockjson rather
	// than restating the union.
	Options     []anyblockjson.OptionDefinition `json:"options"`
	ObjectTypes []string                        `json:"object_types"`
}

type prescanDoc struct {
	PropertyKeys   propertyLegend `json:"property_keys"`
	TypeProperties *[]typePropRaw `json:"type_properties"`
}

// ScanFormats reads every document's typeProperties (§2a) once, up front, to
// build a single batch-wide property-key -> format table. typeProperties is
// the only place a custom property's format is declared in the AnyBlock JSON
// format: plain §3 property values don't self-describe their format, so a
// "person" object referencing "team" only resolves correctly if some type
// document's typeProperties already declared "team"'s format — regardless of
// which file the directory walk visits first.
//
// The table is keyed by the STORED key each entry's `key` term resolves to,
// because that is what the converter reads it by: anyblockjson hands
// Options.ResolveFormat the output of importer.propertyKey, never the
// spelling. `type_properties[].key` is a translated slot (§3), so it runs the
// chain — this document's own property_keys legend, the bundled table,
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
		if doc.TypeProperties == nil {
			continue
		}
		for _, tp := range *doc.TypeProperties {
			if tp.Key == "" {
				continue
			}
			key := resolvePropertyTerm(doc.PropertyKeys, tp.Key)
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
				name = tp.Key
			}
			if existing, seen := out[key]; seen && len(tp.Options) == 0 && len(existing.Options) > 0 {
				// a second type referencing the same property need not
				// repeat its vocabulary
				continue
			}
			if existing, seen := out[key]; seen && existing.Format != format {
				fmt.Fprintf(os.Stderr, "warning: %s: property %q declared with conflicting formats (%s vs %s) — keeping the first seen\n",
					f, tp.Key+resolvedPropertyNote(tp.Key, key), existing.FormatName, tp.Format)
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
	// converter will actually look up, and what a type_properties entry has
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
// binds to a stored key through this document's own property_keys legend,
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
			PropertyKeys propertyLegend             `json:"property_keys"`
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
// sorted so a batch is deterministic regardless of directory order. The
// bundle index (§2c) is excluded: it describes the bundle rather than an
// object, has its own schema, and would fail every object-level check.
func DiscoverJSONFiles(root string) ([]string, error) {
	var files []string
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(p, ".json") &&
			filepath.Base(p) != anyblockjson.IndexFileName {
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
		fmt.Fprintf(&b, "  %s: property %q has no declared format%s — add it to some type's type_properties\n",
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
			Key  string `json:"key"`
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
// and two documents naming it two ways — one through its property_keys
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
			Kind           string         `json:"kind"`
			Key            string         `json:"key"`
			PropertyKeys   propertyLegend `json:"property_keys"`
			TypeProperties []typePropRaw  `json:"type_properties"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("parse %s: %w", f, err)
		}
		if doc.Kind != "object_type" {
			continue
		}
		// the envelope `key` is the raw stored key and is never translated
		// (§2), so it is the right label for the type as it stands
		typeName := doc.Key
		if typeName == "" {
			typeName = filepath.Base(f)
		}
		for _, tp := range doc.TypeProperties {
			if tp.Format != "select" && tp.Format != "multi_select" {
				continue
			}
			key := resolvePropertyTerm(doc.PropertyKeys, tp.Key)
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
// chain — this document's own type_keys legend, the bundled table, verbatim —
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
			TypeKeys       typeLegend    `json:"type_keys"`
			TypeProperties []typePropRaw `json:"type_properties"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("parse %s: %w", f, err)
		}
		for _, tp := range doc.TypeProperties {
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
					out = append(out, BadTarget{File: f, Property: tp.Key, Target: target, Reason: "that type is defined here but its document carries no \"id\", so nothing can reference it" + shadows + note})
				case defined:
					// a document in this bundle, with an id to point at
				case bundle.HasObjectTypeByKey(domain.TypeKey(key)):
					// bundled, and this bundle does not shadow it
				default:
					out = append(out, BadTarget{File: f, Property: tp.Key, Target: target, Reason: "no such type: not bundled, and not defined by this bundle" + note})
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
			TypeKeys     typeLegend                 `json:"type_keys"`
			PropertyKeys propertyLegend             `json:"property_keys"`
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
// property_keys legend, then the bundled table, then verbatim (§3). Probing
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
			out = append(out, BadTarget{
				File: anyblockjson.IndexFileName, Property: fmt.Sprintf("widgets[%d]", i), Target: w.Target,
				Reason: "a reserved listing the importer does not recognise — " +
					"widget.IsPredefinedWidgetTargetId knows only the four spelled " +
					"_favorite, _recent, _set and _collection here, " +
					"so this link is rewritten to _missing_object and the widget is dropped without an error",
			})
		case !ids[w.Target]:
			out = append(out, BadTarget{
				File: anyblockjson.IndexFileName, Property: fmt.Sprintf("widgets[%d]", i), Target: w.Target,
				Reason: "no object with that id in the bundle (and it is not a reserved widget target)",
			})
		}
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
