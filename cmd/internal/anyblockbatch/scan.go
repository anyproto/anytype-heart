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
// from every objectType document's typeProperties (§2a) before any document
// is actually converted.
type FormatInfo struct {
	Format     model.RelationFormat
	FormatName string
	Name       string
	// Options is the declared select vocabulary, in display order (§2a).
	Options []string
}

type typePropRaw struct {
	Key     string   `json:"key"`
	Name    string   `json:"name"`
	Format  string   `json:"format"`
	Options []string `json:"options"`
}

type prescanDoc struct {
	TypeProperties *[]typePropRaw `json:"typeProperties"`
}

// ScanFormats reads every document's typeProperties (§2a) once, up front, to
// build a single batch-wide property-key -> format table. typeProperties is
// the only place a custom property's format is declared in the AnyBlock JSON
// format: plain §3 property values don't self-describe their format, so a
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
				// SPEC.md §3).
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
			out[tp.Key] = FormatInfo{Format: format, FormatName: tp.Format, Name: name, Options: tp.Options}
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

// DiscoverJSONFiles walks root and returns every .json file, sorted so a
// batch is deterministic regardless of directory order.
func DiscoverJSONFiles(root string) ([]string, error) {
	var files []string
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(p, ".json") {
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
