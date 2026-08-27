// Package anyblockbatch holds the cross-document concerns of an AnyBlock JSON
// bundle. pkg/lib/anyblockjson is deliberately one-document-at-a-time and
// leaves these to "the import wiring"; both anyblockconvert and
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

// FormatByName is the format vocabulary anyblockjson accepts in
// typeProperties entries and property values. It's small and spec-fixed but
// unexported inside pkg/lib/anyblockjson, so it's replicated here.
// "text" is the only text format: shorttext has no name of its own, so
// a property declared here as text is minted as longtext. Properties whose
// stored format is shorttext are the bundled ones, which this tool never
// mints — anyblockjson resolves those by key.
var FormatByName = map[string]model.RelationFormat{
	"text":        model.RelationFormat_longtext,
	"number":      model.RelationFormat_number,
	"select":      model.RelationFormat_status,
	"multiSelect": model.RelationFormat_tag,
	"date":        model.RelationFormat_date,
	"files":       model.RelationFormat_file,
	"checkbox":    model.RelationFormat_checkbox,
	"url":         model.RelationFormat_url,
	"email":       model.RelationFormat_email,
	"phone":       model.RelationFormat_phone,
	"objects":     model.RelationFormat_object,
}

// FormatInfo is what the batch knows about a custom property key, gathered
// from every objectType document's typeProperties before any document
// is actually converted.
type FormatInfo struct {
	Format     model.RelationFormat
	FormatName string
	Name       string
	// Options is the declared select vocabulary, in display order,
	// each entry carrying the color it declares (empty = the batch picks).
	Options []anyblockjson.OptionDefinition
	// ObjectTypes are the type keys an objects/files property may point at.
	ObjectTypes []string
}

type typePropRaw struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Format string `json:"format"`
	// OptionDefinition decodes both declared-vocabulary forms (a bare name, or an object with
	// a color), so the prescan shares one decoder with anyblockjson rather
	// than restating the union.
	Options     []anyblockjson.OptionDefinition `json:"options"`
	ObjectTypes []string                        `json:"objectTypes"`
}

type prescanDoc struct {
	TypeProperties *[]typePropRaw `json:"typeProperties"`
}

// ScanFormats reads every document's typeProperties once, up front, to
// build a single batch-wide property-key -> format table. typeProperties is
// the only place a custom property's format is declared in the AnyBlock JSON
// format: plain property values don't self-describe their format, so a
// "person" object referencing "team" only resolves correctly if some type
// document's typeProperties already declared "team"'s format — regardless of
// which file the directory walk visits first.
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
			format, ok := FormatByName[tp.Format]
			if !ok {
				// unrecognized or absent format: leave unresolved so the
				// property value passes through raw (degrades the same way
				// an unresolved format does everywhere else in this format,
				// SPEC.md).
				continue
			}
			name := tp.Name
			if name == "" {
				name = tp.Key
			}
			if existing, seen := out[tp.Key]; seen && len(tp.Options) == 0 && len(existing.Options) > 0 {
				// a second type referencing the same property need not
				// repeat its vocabulary
				continue
			}
			if existing, seen := out[tp.Key]; seen && existing.Format != format {
				fmt.Fprintf(os.Stderr, "warning: %s: property %q declared with conflicting formats (%s vs %s) — keeping the first seen\n",
					f, tp.Key, existing.FormatName, tp.Format)
				continue
			}
			out[tp.Key] = FormatInfo{Format: format, FormatName: tp.Format, Name: name, Options: tp.Options, ObjectTypes: tp.ObjectTypes}
		}
	}
	return out, nil
}

// Undeclared is a property value whose format nothing in the batch
// declares.
type Undeclared struct {
	File string
	Key  string
}

// CheckPropertyFormats finds property values whose format cannot be resolved.
// Formats do not travel with values in this format: a value is decoded
// against the format declared for its key, and the only declaration site is
// some type document's typeProperties — a dataview's properties[] is a
// per-view cache the converter never reads. When nothing declares a key, the
// value passes through as raw JSON and every format-driven conversion is
// silently skipped: a date stays an RFC-3339 string instead of unix seconds,
// a select mints no RelationOption, an objects value keeps an unresolved id,
// and no Relation object is created for the property at all.
//
// Nothing downstream reports this — the document validates and converts — so
// the batch has to catch it.
func CheckPropertyFormats(files []string, formats map[string]FormatInfo) ([]Undeclared, error) {
	var out []Undeclared
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f, err)
		}
		var doc struct {
			Properties map[string]json.RawMessage `json:"properties"`
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
			// id and type are lifted into the envelope, not property values
			if k == "id" || k == "type" {
				continue
			}
			if _, err := bundle.GetRelationFormat(domain.RelationKey(k)); err == nil {
				continue
			}
			if _, declared := formats[k]; declared {
				continue
			}
			out = append(out, Undeclared{File: f, Key: k})
		}
	}
	return out, nil
}

// DiscoverJSONFiles walks root and returns every .json object document,
// sorted so a batch is deterministic regardless of directory order. The
// bundle index is excluded: it describes the bundle rather than an
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
func Report(us []Undeclared) string {
	var b strings.Builder
	for _, u := range us {
		fmt.Fprintf(&b, "  %s: property %q has no declared format — add it to some type's typeProperties\n", u.File, u.Key)
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
		if probe.Kind == "objectType" && probe.Key != "" {
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
		if probe.Kind == "objectType" {
			types = append(types, f)
		} else {
			rest = append(rest, f)
		}
	}
	return append(types, rest...), nil
}

// SharedSelect is a select property declared by more than one type.
type SharedSelect struct {
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
			Kind           string        `json:"kind"`
			Key            string        `json:"key"`
			TypeProperties []typePropRaw `json:"typeProperties"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("parse %s: %w", f, err)
		}
		if doc.Kind != "objectType" {
			continue
		}
		typeName := doc.Key
		if typeName == "" {
			typeName = filepath.Base(f)
		}
		for _, tp := range doc.TypeProperties {
			if tp.Format != "select" && tp.Format != "multiSelect" {
				continue
			}
			d, ok := byKey[tp.Key]
			if !ok {
				d = &decl{seen: map[string]bool{}}
				byKey[tp.Key] = d
				order = append(order, tp.Key)
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
func CheckTargetTypes(files []string, typeIds map[string]string) ([]BadTarget, error) {
	var out []BadTarget
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f, err)
		}
		var doc struct {
			TypeProperties []typePropRaw `json:"typeProperties"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("parse %s: %w", f, err)
		}
		for _, tp := range doc.TypeProperties {
			for _, target := range tp.ObjectTypes {
				if bundle.HasObjectTypeByKey(domain.TypeKey(target)) {
					continue
				}
				id, defined := typeIds[target]
				switch {
				case !defined:
					out = append(out, BadTarget{File: f, Property: tp.Key, Target: target, Reason: "no such type: not bundled, and not defined by this bundle"})
				case id == "":
					out = append(out, BadTarget{File: f, Property: tp.Key, Target: target, Reason: "that type is defined here but its document carries no \"id\", so nothing can reference it"})
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
func CheckTemplateTargets(files []string, typeIds map[string]string) ([]BadTemplateTarget, error) {
	var out []BadTemplateTarget
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f, err)
		}
		var doc struct {
			Type        string                     `json:"type"`
			TemplateFor string                     `json:"templateFor"`
			Properties  map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("parse %s: %w", f, err)
		}
		if doc.Type != "template" {
			continue
		}
		if _, authored := doc.Properties[string(bundle.RelationKeyTargetObjectType)]; authored {
			// an explicit id (what a round-tripped export carries) is what the
			// converter keeps, whatever templateFor says
			continue
		}
		if doc.TemplateFor == "" {
			out = append(out, BadTemplateTarget{File: f,
				Reason: `no "templateFor": the template would belong to no type, and no type would list it`})
			continue
		}
		id, defined := typeIds[doc.TemplateFor]
		switch {
		case !defined && bundle.HasObjectTypeByKey(domain.TypeKey(doc.TemplateFor)):
			out = append(out, BadTemplateTarget{File: f, Target: doc.TemplateFor,
				Reason: `that type is bundled, but a template's target must be a document in this bundle: a bundled url is never relinked on import, so it would match no type — add an objectType document with this key and an "id"`})
		case !defined:
			out = append(out, BadTemplateTarget{File: f, Target: doc.TemplateFor,
				Reason: "no such type: not bundled, and not defined by this bundle"})
		case id == "":
			out = append(out, BadTemplateTarget{File: f, Target: doc.TemplateFor,
				Reason: `that type is defined here but its document carries no "id", so nothing can reference it`})
		}
	}
	return out, nil
}

// ReportTemplateTargets renders unwirable template targets, one per line.
func ReportTemplateTargets(bs []BadTemplateTarget) string {
	var b strings.Builder
	for _, t := range bs {
		if t.Target == "" {
			fmt.Fprintf(&b, "  %s: %s\n", t.File, t.Reason)
			continue
		}
		fmt.Fprintf(&b, "  %s: templateFor %q — %s\n", t.File, t.Target, t.Reason)
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

// CheckIndexTargets finds index.json references that name nothing the bundle
// defines. Reserved homepages and reserved widget targets name built-in
// screens and listings, so they are not expected to resolve.
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
		if anyblockjson.IsReservedWidgetTarget(w.Target) || ids[w.Target] {
			continue
		}
		out = append(out, BadTarget{
			File: anyblockjson.IndexFileName, Property: fmt.Sprintf("widgets[%d]", i), Target: w.Target,
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
