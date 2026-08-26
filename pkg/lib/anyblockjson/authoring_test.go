package anyblockjson

// authoring_test.go pins the authoring subset (§2g). The load-bearing claim
// is in the name: SUBSET. Every document the authoring schemas accept must be
// accepted by the full schemas and the full reader — that is what keeps the
// small surface honest, and it is a TEST here, not a claim: the worked
// example, a structural fixture battery, and a sweep that builds one document
// per enum value the authoring schemas state, each pushed through the full
// Validate and the real codec.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// authoringOnly runs ONLY the authoring schema — not the full validation the
// public ValidateAuthoring folds in — so the subset invariant can be stated
// as two independent verdicts: the subset accepts, and the full side must
// then accept too.
func authoringOnly(data []byte) error {
	return validateAuthoringSubset(data, compileAuthoringSchema, "outside the authoring subset")
}

func authoringIndexOnly(data []byte) error {
	return validateAuthoringSubset(data, compileAuthoringIndexSchema, "outside the authoring subset")
}

func authoringPropertiesOnly(data []byte) error {
	return validateAuthoringSubset(data, compileAuthoringPropertiesSchema, "outside the authoring subset")
}

// requireSubsetObject is the invariant, per document: in the subset, then
// full-valid, then importable by the real codec.
func requireSubsetObject(t *testing.T, doc string) {
	t.Helper()
	data := []byte(doc)
	require.NoError(t, authoringOnly(data), "the fixture must be inside the authoring subset:\n%s", doc)
	require.NoError(t, Validate(data), "subset invariant: an authoring-valid document must be full-valid:\n%s", doc)
	_, _, err := Unmarshal(data, Options{})
	require.NoError(t, err, "and the real codec must import it:\n%s", doc)
}

// --- the worked example -------------------------------------------------

// The habit_tracker bundle is the minimal worked example §2g points authors
// at: an index, one type, a property dictionary, a welcome page and two
// objects. It must stay valid against the authoring schemas AND the real
// codec, warning-free, and internally coherent — every id a file names is an
// id a file declares.
//
// How this can fail: a schema change that outgrows the example, or an edit
// to the example that reaches outside the subset. Either way the example is
// the first document an authoring agent imitates, so it rots loudest.
func TestAuthoringExample_HabitTracker(t *testing.T) {
	root := filepath.Join("testdata", "authoring", "habit_tracker")

	readFile := func(rel string) []byte {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(root, rel))
		require.NoError(t, err)
		return data
	}

	objectFiles := []string{
		filepath.Join("types", "habit.json"),
		filepath.Join("objects", "start.json"),
		filepath.Join("objects", "morning-run.json"),
		filepath.Join("objects", "weekly-review.json"),
	}

	declaredIds := map[string]bool{}
	declaredTypeKeys := map[string]bool{}
	referencedIds := map[string]string{} // id -> where it was referenced
	objectTypeTerms := map[string]string{}

	deepLink := regexp.MustCompile(`objectId=([A-Za-z0-9_-]+)`)

	for _, rel := range objectFiles {
		t.Run(rel, func(t *testing.T) {
			data := readFile(rel)

			// the subset, stated as its own verdict, then the invariant
			require.NoError(t, authoringOnly(data))
			var warnings []Issue
			require.NoError(t, ValidateWarn(data, func(i Issue) { warnings = append(warnings, i) }))
			assert.Empty(t, warnings, "the worked example must be warning-free")
			require.NoError(t, ValidateAuthoring(data))

			// the real codec
			_, snapshot, err := Unmarshal(data, Options{})
			require.NoError(t, err)
			require.NotNil(t, snapshot)

			var doc struct {
				Kind        string `json:"kind"`
				Id          string `json:"id"`
				Type        string `json:"type"`
				InternalKey string `json:"internal_key"`
				Blocks      []struct {
					Type     string `json:"type"`
					ObjectId string `json:"object_id"`
					Text     string `json:"text"`
				} `json:"blocks"`
			}
			require.NoError(t, json.Unmarshal(data, &doc))
			require.NotEmpty(t, doc.Id, "every example document declares its bundle-local id")
			declaredIds[doc.Id] = true
			if doc.Kind == "object_type" {
				declaredTypeKeys[doc.InternalKey] = true
			} else {
				objectTypeTerms[doc.Type] = rel
			}
			for _, b := range doc.Blocks {
				if b.Type == "link" {
					referencedIds[b.ObjectId] = rel + " (link block)"
				}
				for _, m := range deepLink.FindAllStringSubmatch(b.Text, -1) {
					referencedIds[m[1]] = rel + " (inline object link)"
				}
			}
		})
	}

	t.Run("index.json", func(t *testing.T) {
		data := readFile("index.json")
		require.NoError(t, authoringIndexOnly(data))
		require.NoError(t, ValidateAuthoringIndex(data))
		idx, err := UnmarshalIndex(data)
		require.NoError(t, err)
		require.NotEmpty(t, idx.EntryPoint())
		referencedIds[idx.EntryPoint()] = "index.json (entrypoint)"
		for _, w := range idx.Widgets {
			if !strings.HasPrefix(w.Target, "_") {
				referencedIds[w.Target] = "index.json (widget)"
			}
		}
	})

	t.Run("properties.json", func(t *testing.T) {
		data := readFile("properties.json")
		require.NoError(t, authoringPropertiesOnly(data))
		require.NoError(t, ValidateAuthoringPropertyDictionary(data))
		var warnings []Issue
		dict, err := UnmarshalPropertyDictionaryWarn(data, func(i Issue) { warnings = append(warnings, i) })
		require.NoError(t, err)
		assert.Empty(t, warnings, "the worked example must be warning-free")
		require.NotEmpty(t, dict.Properties)
	})

	t.Run("the bundle is coherent", func(t *testing.T) {
		for id, where := range referencedIds {
			assert.True(t, declaredIds[id],
				"%s names %q, but no document in the bundle declares that id", where, id)
		}
		for term, rel := range objectTypeTerms {
			if term == "page" { // the built-in type the welcome page uses
				continue
			}
			assert.True(t, declaredTypeKeys[term],
				"%s has type %q, which no type document in the bundle declares", rel, term)
		}
	})
}

// --- structural fixtures ------------------------------------------------

// One fixture per structure the authoring grammar can express, each held to
// the subset invariant. The battery is what catches a subset break that is
// not an enum value: a member combination the authoring schema admits and
// the full schema's per-type closing refuses.
func TestAuthoringSubset_StructuralFixtures(t *testing.T) {
	fixtures := map[string]string{
		"minimal document": `{"version": 1}`,
		"a full page envelope": `{"version": 1, "id": "page-a", "type": "page",
			"icon": {"format": "emoji", "emoji": "🌱"},
			"cover": {"format": "gradient", "gradient": "pinkOrange"},
			"properties": {"name": "A", "description": "a page", "is_favorite": true,
				"done": false, "custom_note": null, "tags_of_mine": ["x", "y"]}}`,
		"nested blocks": `{"version": 1, "blocks": [
			{"type": "heading_1", "text": "H"},
			{"type": "toggle", "text": "open me"},
			{"indent": 1, "type": "bulleted_list_item", "text": "one"},
			{"indent": 2, "type": "paragraph", "text": "deeper"},
			{"indent": 1, "type": "numbered_list_item", "text": "two"},
			{"type": "quote", "text": "said"},
			{"type": "code", "language": "go", "text": "fmt.Println(1)"}]}`,
		"columns": `{"version": 1, "blocks": [
			{"type": "row"},
			{"indent": 1, "type": "column"},
			{"indent": 2, "type": "paragraph", "text": "left"},
			{"indent": 1, "type": "column"},
			{"indent": 2, "type": "paragraph", "text": "right"}]}`,
		"a table with empty and padded cells": `{"version": 1, "blocks": [
			{"type": "table",
			 "columns": [{}, {}, {}],
			 "rows": [
				{"is_header": true, "cells": ["Name", "Status", "Note"]},
				{"cells": ["Export", null, "spec"]},
				{"cells": ["Short row"]}]}]}`,
		"an inline set on a page": `{"version": 1, "id": "page-b", "blocks": [
			{"type": "dataview", "object_id": "coll-shelf", "is_collection": true,
			 "properties": [{"property": "name", "format": "text"}],
			 "views": [{"name": "Shelf"}]}]}`,
		"a collection": `{"version": 1, "id": "coll-shelf", "type": "collection",
			"items": ["page-a", "page-b"],
			"blocks": [{"type": "dataview", "is_collection": true,
				"views": [{"type": "list", "name": "All"}]}]}`,
		"a template": `{"version": 1, "kind": "template", "id": "tpl-habit",
			"type": "template", "template_for": "habit",
			"properties": {"name": "New habit"},
			"blocks": [{"type": "paragraph", "text": "Why this habit matters:"}]}`,
		"a type with the whole settings surface": `{"version": 1, "kind": "object_type",
			"id": "type-r", "internal_key": "review",
			"icon": {"format": "icon", "name": "book", "color": "teal"},
			"properties": {"name": "Review", "description": "One review."},
			"type_settings": {
				"layout": "todo",
				"plural_name": "Reviews",
				"default_template": "tpl-habit",
				"default_view": "kanban",
				"property_definitions": [
					{"property": "verdict", "name": "Verdict", "format": "select",
					 "options": ["Ship", {"name": "Hold", "color": "red"}], "section": "featured"},
					{"name": "Reviewed on", "format": "date", "include_time": true},
					{"property": "owner", "name": "Owner", "format": "objects",
					 "object_types": ["participant"], "section": "hidden"}]}}`,
		"filters, groups and sorts": `{"version": 1, "blocks": [
			{"type": "dataview",
			 "properties": [{"property": "stage", "format": "select"}, {"property": "when", "format": "date"}],
			 "views": [{
				"name": "Overdue",
				"filters": [
					{"operator": "and", "filters": [
						{"property": "when", "condition": "not_empty"},
						{"property": "when", "condition": "less", "date_preset": "today"}]},
					{"operator": "or", "filters": [
						{"property": "stage", "condition": "in", "value": ["Open"]},
						{"property": "stage", "condition": "empty"}]}],
				"sorts": [{"property": "when", "direction": "asc", "empty_placement": "end"}],
				"columns": [
					{"property": "name"},
					{"property": "when"},
					{"property": "stage", "hidden": true},
					{"property": "name", "aggregation": "count"}]}]}]}`,
		"inline markup": `{"version": 1, "blocks": [
			{"type": "paragraph", "text": "Ship the **new export** by Q3 — see [the plan](https://example.com/plan), or <u>ask</u> in [the space](anytype://object?objectId=page-a). A literal \\*star\\*."},
			{"type": "callout", "icon": {"format": "emoji", "emoji": "💡"}, "text": "Escaping: \\<sub\\> stays prose."},
			{"type": "checkbox", "checked": true, "text": "done ~~and dusted~~"}]}`,
		"embeds and bookmarks": `{"version": 1, "blocks": [
			{"type": "embed", "processor": "mermaid", "text": "graph TD; A-->B"},
			{"type": "embed", "processor": "latex", "text": "e^{i\\pi}+1=0"},
			{"type": "bookmark", "url": "https://example.com"},
			{"type": "divider", "style": "dots"},
			{"type": "table_of_contents"}]}`,
	}

	for name, doc := range fixtures {
		t.Run(name, func(t *testing.T) {
			requireSubsetObject(t, doc)
		})
	}
}

// --- the enum sweep -----------------------------------------------------

// schemaAt walks a decoded schema by member names, failing loudly when the
// path is stale — a reshuffled authoring schema must break the sweep, not
// silently shrink it.
func schemaAt(t *testing.T, node any, path ...string) any {
	t.Helper()
	for _, step := range path {
		m, ok := node.(map[string]any)
		require.True(t, ok, "schema path %v: not an object at %q", path, step)
		node, ok = m[step]
		require.True(t, ok, "schema path %v: no member %q", path, step)
	}
	return node
}

func schemaEnum(t *testing.T, node any, path ...string) []string {
	t.Helper()
	raw, ok := schemaAt(t, node, path...).([]any)
	require.True(t, ok, "schema path %v: not an enum", path)
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		s, ok := v.(string)
		require.True(t, ok, "schema path %v: non-string enum value %v", path, v)
		out = append(out, s)
	}
	require.NotEmpty(t, out)
	return out
}

// blockConditional finds the authoring block's if/then branch that declares
// the given member, so the sweep does not depend on the allOf's order.
func blockConditional(t *testing.T, schema map[string]any, member string) map[string]any {
	t.Helper()
	conds, ok := schemaAt(t, schema, "$defs", "block", "allOf").([]any)
	require.True(t, ok)
	for _, c := range conds {
		then, ok := c.(map[string]any)["then"].(map[string]any)
		if !ok {
			continue
		}
		props, ok := then["properties"].(map[string]any)
		if !ok {
			continue
		}
		if _, ok := props[member]; ok {
			return props
		}
	}
	require.Failf(t, "no block conditional", "no authoring block branch declares %q", member)
	return nil
}

// filterBranch finds the filterNode branch that declares the given member —
// "condition" lands on the leaf, "operator" on the group.
func filterBranch(t *testing.T, schema map[string]any, member string) map[string]any {
	t.Helper()
	branches, ok := schemaAt(t, schema, "$defs", "filterNode", "oneOf").([]any)
	require.True(t, ok)
	for _, b := range branches {
		props, ok := b.(map[string]any)["properties"].(map[string]any)
		if !ok {
			continue
		}
		if _, ok := props[member]; ok {
			return props
		}
	}
	require.Failf(t, "no filter branch", "no filterNode branch declares %q", member)
	return nil
}

// Every enum value the authoring OBJECT schema states is exercised in a
// document and held to the subset invariant. This is the typo trap: an
// authoring enum value the full schema does not know produces documents the
// subset accepts and the format refuses, which is exactly the break the
// invariant forbids — and a hand-kept list of values would rot, so the sweep
// reads the schema itself.
func TestAuthoringSubset_EveryObjectEnumValueIsFullValid(t *testing.T) {
	var schema map[string]any
	require.NoError(t, json.Unmarshal(authoringSchemaJSON, &schema))

	typeDoc := func(settings string) string {
		return `{"version": 1, "kind": "object_type", "internal_key": "t1",
			"properties": {"name": "T"}, "type_settings": {` + settings + `}}`
	}
	dataviewDoc := func(view string) string {
		return `{"version": 1, "blocks": [{"type": "dataview", "views": [{` + view + `}]}]}`
	}

	sweep := func(name string, values []string, build func(v string) string) {
		t.Run(name, func(t *testing.T) {
			for _, v := range values {
				requireSubsetObject(t, build(v))
			}
		})
	}

	sweep("kind", schemaEnum(t, schema, "properties", "kind", "enum"), func(v string) string {
		switch v {
		case "object_type":
			return typeDoc(`"layout": "basic"`)
		case "template":
			return `{"version": 1, "kind": "template", "type": "template", "template_for": "t1"}`
		default:
			return `{"version": 1, "kind": "` + v + `"}`
		}
	})

	sweep("block type", schemaEnum(t, schema, "$defs", "block", "properties", "type", "enum"), func(v string) string {
		blocks := map[string]string{
			"checkbox":          `{"type": "checkbox", "checked": true, "text": "x"}`,
			"callout":           `{"type": "callout", "icon": {"format": "emoji", "emoji": "💡"}, "text": "x"}`,
			"code":              `{"type": "code", "language": "go", "text": "x"}`,
			"bookmark":          `{"type": "bookmark", "url": "https://example.com"}`,
			"link":              `{"type": "link", "object_id": "page-two", "card_style": "inline"}`,
			"divider":           `{"type": "divider"}`,
			"row":               `{"type": "row"}, {"indent": 1, "type": "column"}, {"indent": 2, "type": "paragraph", "text": "x"}`,
			"column":            `{"type": "row"}, {"indent": 1, "type": "column"}, {"indent": 2, "type": "paragraph", "text": "x"}`,
			"table":             `{"type": "table", "columns": [{}, {}], "rows": [{"cells": ["a", null]}]}`,
			"embed":             `{"type": "embed", "processor": "mermaid", "text": "graph TD; A-->B"}`,
			"table_of_contents": `{"type": "table_of_contents"}`,
			"dataview":          `{"type": "dataview", "views": [{"name": "v"}]}`,
		}
		b, ok := blocks[v]
		if !ok {
			b = `{"type": "` + v + `", "text": "x"}`
		}
		return `{"version": 1, "blocks": [` + b + `]}`
	})

	sweep("type_settings.layout", schemaEnum(t, schema, "properties", "type_settings", "properties", "layout", "enum"), func(v string) string {
		return typeDoc(`"layout": "` + v + `"`)
	})
	sweep("type_settings.default_view", schemaEnum(t, schema, "properties", "type_settings", "properties", "default_view", "enum"), func(v string) string {
		return typeDoc(`"default_view": "` + v + `"`)
	})
	sweep("property format", schemaEnum(t, schema, "$defs", "propertyFormat", "enum"), func(v string) string {
		return typeDoc(`"property_definitions": [{"property": "p1", "name": "P", "format": "` + v + `"}]`)
	})
	sweep("section", schemaEnum(t, schema, "$defs", "propertyDefinition", "properties", "section", "enum"), func(v string) string {
		return typeDoc(`"property_definitions": [{"property": "p1", "section": "` + v + `"}]`)
	})
	sweep("layout_align", schemaEnum(t, schema, "$defs", "blockAlign", "enum"), func(v string) string {
		return `{"version": 1, "properties": {"layout_align": "` + v + `"}}`
	})
	sweep("palette colour on icons", schemaEnum(t, schema, "$defs", "paletteColor", "enum"), func(v string) string {
		return `{"version": 1, "icon": {"format": "icon", "name": "book", "color": "` + v + `"}}`
	})
	sweep("palette colour on options", schemaEnum(t, schema, "$defs", "paletteColor", "enum"), func(v string) string {
		return typeDoc(`"property_definitions": [{"property": "p1", "format": "select",
			"options": [{"name": "O", "color": "` + v + `"}]}]`)
	})
	sweep("icon format", schemaEnum(t, schema, "$defs", "icon", "properties", "format", "enum"), func(v string) string {
		icons := map[string]string{
			"emoji": `{"format": "emoji", "emoji": "🌱"}`,
			"icon":  `{"format": "icon", "name": "book"}`,
			"color": `{"format": "color", "color": "teal"}`,
		}
		icon, ok := icons[v]
		require.True(t, ok, "no builder for icon format %q", v)
		return `{"version": 1, "icon": ` + icon + `}`
	})
	sweep("cover format", schemaEnum(t, schema, "$defs", "cover", "properties", "format", "enum"), func(v string) string {
		covers := map[string]string{
			"color":    `{"format": "color", "color": "black"}`,
			"gradient": `{"format": "gradient", "gradient": "sky"}`,
		}
		cover, ok := covers[v]
		require.True(t, ok, "no builder for cover format %q", v)
		return `{"version": 1, "cover": ` + cover + `}`
	})

	embed := blockConditional(t, schema, "processor")
	sweep("embed processor", schemaEnum(t, embed, "processor", "enum"), func(v string) string {
		return `{"version": 1, "blocks": [{"type": "embed", "processor": "` + v + `", "text": "x"}]}`
	})
	link := blockConditional(t, schema, "card_style")
	sweep("link card_style", schemaEnum(t, link, "card_style", "enum"), func(v string) string {
		return `{"version": 1, "blocks": [{"type": "link", "object_id": "page-two", "card_style": "` + v + `"}]}`
	})
	divider := blockConditional(t, schema, "style")
	sweep("divider style", schemaEnum(t, divider, "style", "enum"), func(v string) string {
		return `{"version": 1, "blocks": [{"type": "divider", "style": "` + v + `"}]}`
	})

	sweep("view type", schemaEnum(t, schema, "$defs", "view", "properties", "type", "enum"), func(v string) string {
		return dataviewDoc(`"type": "` + v + `", "name": "v"`)
	})
	sweep("view card_size", schemaEnum(t, schema, "$defs", "view", "properties", "card_size", "enum"), func(v string) string {
		return dataviewDoc(`"type": "gallery", "card_size": "` + v + `"`)
	})
	sweep("sort direction", schemaEnum(t, schema, "$defs", "sort", "properties", "direction", "enum"), func(v string) string {
		return dataviewDoc(`"sorts": [{"property": "p1", "direction": "` + v + `"}]`)
	})
	sweep("sort empty_placement", schemaEnum(t, schema, "$defs", "sort", "properties", "empty_placement", "enum"), func(v string) string {
		return dataviewDoc(`"sorts": [{"property": "p1", "empty_placement": "` + v + `"}]`)
	})
	sweep("column aggregation", schemaEnum(t, schema, "$defs", "viewColumn", "properties", "aggregation", "enum"), func(v string) string {
		return dataviewDoc(`"columns": [{"property": "p1", "aggregation": "` + v + `"}]`)
	})

	leaf := filterBranch(t, schema, "condition")
	sweep("filter condition", schemaEnum(t, leaf, "condition", "enum"), func(v string) string {
		value := `, "value": ["x"]`
		switch v {
		case "empty", "not_empty":
			value = ""
		case "greater", "less", "greater_or_equal", "less_or_equal":
			value = `, "value": 5`
		}
		return dataviewDoc(`"filters": [{"property": "p1", "condition": "` + v + `"` + value + `}]`)
	})
	sweep("filter date_preset", schemaEnum(t, leaf, "date_preset", "enum"), func(v string) string {
		return dataviewDoc(`"filters": [{"property": "p1", "condition": "greater_or_equal", "date_preset": "` + v + `"}]`)
	})
	group := filterBranch(t, schema, "operator")
	sweep("filter group operator", schemaEnum(t, group, "operator", "enum"), func(v string) string {
		return dataviewDoc(`"filters": [{"operator": "` + v + `", "filters": [{"property": "p1", "condition": "not_empty"}]}]`)
	})
}

// The index and dictionary sweeps, same invariant against their own full
// readers.
func TestAuthoringSubset_IndexAndDictionaryEnumValues(t *testing.T) {
	requireSubsetIndex := func(t *testing.T, doc string) {
		t.Helper()
		data := []byte(doc)
		require.NoError(t, authoringIndexOnly(data), "must be inside the subset:\n%s", doc)
		_, err := UnmarshalIndex(data)
		require.NoError(t, err, "subset invariant: the full index reader must accept:\n%s", doc)
	}
	requireSubsetDictionary := func(t *testing.T, doc string) {
		t.Helper()
		data := []byte(doc)
		require.NoError(t, authoringPropertiesOnly(data), "must be inside the subset:\n%s", doc)
		_, err := UnmarshalPropertyDictionary(data)
		require.NoError(t, err, "subset invariant: the full dictionary reader must accept:\n%s", doc)
	}

	var indexSchema map[string]any
	require.NoError(t, json.Unmarshal(authoringIndexSchemaJSON, &indexSchema))

	t.Run("widget layout", func(t *testing.T) {
		for _, v := range schemaEnum(t, indexSchema, "$defs", "widget", "properties", "layout", "enum") {
			requireSubsetIndex(t, `{"version": 1, "name": "X", "entrypoint": "page-a",
				"widgets": [{"target": "page-a", "layout": "`+v+`", "limit": 6}]}`)
		}
	})
	t.Run("reserved widget targets", func(t *testing.T) {
		branches, ok := schemaAt(t, indexSchema, "$defs", "widget", "properties", "target", "anyOf").([]any)
		require.True(t, ok)
		var reserved []string
		for _, b := range branches {
			if e, ok := b.(map[string]any)["enum"]; ok {
				for _, v := range e.([]any) {
					reserved = append(reserved, v.(string))
				}
			}
		}
		require.NotEmpty(t, reserved, "the target anyOf must state the reserved listings")
		for _, v := range reserved {
			requireSubsetIndex(t, `{"version": 1, "name": "X", "entrypoint": "page-a",
				"widgets": [{"target": "`+v+`"}]}`)
		}
	})

	var propsSchema map[string]any
	require.NoError(t, json.Unmarshal(authoringPropertiesSchemaJSON, &propsSchema))
	t.Run("dictionary format", func(t *testing.T) {
		for _, v := range schemaEnum(t, propsSchema, "$defs", "property", "properties", "format", "enum") {
			requireSubsetDictionary(t, `{"version": 1, "properties": [{"property": "p1", "format": "`+v+`"}]}`)
		}
	})
	t.Run("installed and a name-identified entry", func(t *testing.T) {
		requireSubsetDictionary(t, `{"version": 1, "installed": ["due_date", "tag"],
			"properties": [{"name": "Cooking Time", "format": "number"},
				{"property": "owner", "format": "objects", "object_types": ["participant"],
				 "description": "who runs it"},
				{"property": "when", "format": "date", "include_time": true}]}`)
	})
}

// --- what the subset refuses --------------------------------------------

// Each document here is VALID under the full schema and reader — that is
// asserted, so this list cannot drift into restating format errors — and
// refused by the authoring subset, because the member it carries is one only
// a live space can write honestly: ids, legends, provenance, output-only
// state, non-authorable kinds and variants.
func TestAuthoringSubset_RefusesBackupOnlySurfaces(t *testing.T) {
	cases := map[string]string{
		"a block id":                 `{"version": 1, "blocks": [{"id": "b1", "type": "paragraph", "text": "x"}]}`,
		"the store escape hatch":     `{"version": 1, "store": {"k": 1}}`,
		"the root escape hatch":      `{"version": 1, "root": {"background_color": "grey"}}`,
		"the property legend":        `{"version": 1, "property_internal_keys": {"prio": "6a32d4856761631534b22f85"}}`,
		"the type legend":            `{"version": 1, "type_internal_keys": {"task": "task"}}`,
		"the option legend":          `{"version": 1, "properties": {"prio": ["High"]}, "option_ids": {"prio": {"High": "bafyreiopt1"}}}`,
		"attribution in properties":  `{"version": 1, "properties": {"creator": "A6eK73Jm#roma"}}`,
		"a non-authorable kind":      `{"version": 1, "kind": "participant"}`,
		"an icon by file":            `{"version": 1, "icon": {"format": "file", "file": "bafyreicfd"}}`,
		"an image cover":             `{"version": 1, "cover": {"format": "image", "file": "bafyreigejp", "y": -0.25}}`,
		"internal_key on a page":     `{"version": 1, "internal_key": "x"}`,
		"a counting date preset":     `{"version": 1, "blocks": [{"type": "dataview", "views": [{"filters": [{"property": "p", "condition": "less", "date_preset": "number_of_days_ago", "value": 7}]}]}]}`,
		"a view id":                  `{"version": 1, "blocks": [{"type": "dataview", "views": [{"id": "v1", "name": "v"}]}]}`,
		"block alignment":            `{"version": 1, "blocks": [{"type": "paragraph", "text": "x", "align": "center"}]}`,
		"the heading_4 input alias":  `{"version": 1, "blocks": [{"type": "heading_4", "text": "x"}]}`,
		"the equation input alias":   `{"version": 1, "blocks": [{"type": "equation", "text": "E=mc^2"}]}`,
		"a widget block":             `{"version": 1, "blocks": [{"type": "widget", "layout": "tree"}]}`,
		"the legacy group container": `{"version": 1, "blocks": [{"type": "group"}]}`,
		"a file block":               `{"version": 1, "blocks": [{"type": "image", "object_id": "bafyimg"}]}`,
		"a custom sort order":        `{"version": 1, "blocks": [{"type": "dataview", "views": [{"sorts": [{"property": "p", "direction": "custom", "custom_order": ["b", "a"]}]}]}]}`,
		"dataview output-only state": `{"version": 1, "blocks": [{"type": "dataview", "source": ["bafysrc"], "views": [{"name": "v"}]}]}`,
		"include_time off a date property": `{"version": 1, "kind": "object_type", "internal_key": "t1",
			"properties": {"name": "T"},
			"type_settings": {"property_definitions": [{"property": "p1", "format": "text", "include_time": true}]}}`,
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			data := []byte(doc)
			require.NoError(t, Validate(data),
				"the case must be FULL-valid, or it is a format error rather than a subset boundary:\n%s", doc)
			err := ValidateAuthoring(data)
			require.Error(t, err, "the authoring subset must refuse:\n%s", doc)
			assert.Contains(t, err.Error(), "outside the authoring subset",
				"the refusal names itself a subset verdict, not a format one")
		})
	}

	t.Run("member-format coupling the format refuses at semantics", func(t *testing.T) {
		// The subset invariant's first failure, found by probing rather than
		// by a sweep: `options` off select/multi_select and `object_types`
		// off objects/files are §12 ERRORS the authoring schema originally
		// admitted — an authoring-valid document the format refused. The
		// coupling is schema-expressible, so the subset now refuses both at
		// generation time, and this pins that the two sides agree.
		for name, doc := range map[string]string{
			"options off select": `{"version": 1, "kind": "object_type", "internal_key": "t1",
				"properties": {"name": "T"},
				"type_settings": {"property_definitions": [{"property": "p1", "format": "date", "options": ["A"]}]}}`,
			"object_types off objects/files": `{"version": 1, "kind": "object_type", "internal_key": "t1",
				"properties": {"name": "T"},
				"type_settings": {"property_definitions": [{"property": "p1", "format": "number", "object_types": ["task"]}]}}`,
		} {
			t.Run(name, func(t *testing.T) {
				data := []byte(doc)
				require.Error(t, Validate(data), "§12 refuses the pairing")
				require.Error(t, authoringOnly(data), "and the subset must refuse it at the schema, or it admits what the format rejects")
			})
		}
	})

	t.Run("the pre-v0.22 template spelling is refused at the schema", func(t *testing.T) {
		// {"type": "template"} with no kind is the one shape both sides
		// refuse — the full reader by the §10 byte comparison, the authoring
		// schema by its own conditional — so an authoring-side generator
		// gets the verdict without ever reaching the format's error.
		assert.Error(t, authoringOnly([]byte(`{"version": 1, "type": "template"}`)))
	})

	t.Run("index surfaces", func(t *testing.T) {
		// `_all_objects` used to be the example here — a listing the importer
		// dropped, so the subset refused it. The importer knows the whole
		// inventory since GO-7383 and the listing is authorable now; what the
		// subset still refuses on the index is the machine-written state the
		// widget-object lift carries: the auto-widget ledger and the
		// auto-added flag, which only a live client can write honestly.
		for name, doc := range map[string]string{
			"the manifest": `{"version": 1, "name": "X", "entrypoint": "p1",
				"manifest": {"properties": "properties.json"}}`,
			"the auto-widget ledger": `{"version": 1, "name": "X", "entrypoint": "p1",
				"auto_widget_targets": ["_bin"]}`,
			"an auto-added widget": `{"version": 1, "name": "X", "entrypoint": "p1",
				"widgets": [{"target": "p1", "auto_added": true}]}`,
		} {
			t.Run(name, func(t *testing.T) {
				data := []byte(doc)
				_, err := UnmarshalIndex(data)
				require.NoError(t, err, "must be full-valid:\n%s", doc)
				require.Error(t, ValidateAuthoringIndex(data))
			})
		}
	})

	t.Run("a dictionary entry identified only by internal_key", func(t *testing.T) {
		doc := []byte(`{"version": 1, "properties": [{"internal_key": "6a32d4856761631534b22f85", "format": "number"}]}`)
		_, err := UnmarshalPropertyDictionary(doc)
		require.NoError(t, err, "must be full-valid")
		require.Error(t, ValidateAuthoringPropertyDictionary(doc),
			"an author states a spelling or a name; a stored id is the app's to mint")
	})

	t.Run("dictionary member-format coupling", func(t *testing.T) {
		// the full dictionary reader TOLERATES these pairings (unlike a type
		// entry, where §12 refuses them), so on this surface the coupling is
		// an ordinary subset narrowing: full-valid, subset-refused
		for name, doc := range map[string]string{
			"options off select":       `{"version": 1, "properties": [{"property": "p1", "format": "date", "options": ["A"]}]}`,
			"object_types off objects": `{"version": 1, "properties": [{"property": "p1", "format": "number", "object_types": ["task"]}]}`,
			"include_time off date":    `{"version": 1, "properties": [{"property": "p1", "format": "text", "include_time": true}]}`,
		} {
			t.Run(name, func(t *testing.T) {
				data := []byte(doc)
				_, err := UnmarshalPropertyDictionary(data)
				require.NoError(t, err, "must be full-valid:\n%s", doc)
				require.Error(t, ValidateAuthoringPropertyDictionary(data))
			})
		}
	})
}

// --- schema hygiene -----------------------------------------------------

// The three authoring schemas carry their published identity and none of the
// export machinery: no x-output-only member survives into a surface whose
// whole point is that nothing in it is output-only.
func TestAuthoringSchemas_IdentityAndHygiene(t *testing.T) {
	for name, tc := range map[string]struct {
		bytes []byte
		url   string
	}{
		"object":     {authoringSchemaJSON, AuthoringSchemaURL},
		"index":      {authoringIndexSchemaJSON, AuthoringIndexSchemaURL},
		"properties": {authoringPropertiesSchemaJSON, AuthoringPropertiesSchemaURL},
	} {
		t.Run(name, func(t *testing.T) {
			var doc struct {
				Id      string          `json:"$id"`
				Version json.RawMessage `json:"properties"`
			}
			require.NoError(t, json.Unmarshal(tc.bytes, &doc))
			assert.Equal(t, tc.url, doc.Id, "$id must be the published URL FormatVersion derives")
			assert.NotContains(t, string(tc.bytes), "x-output-only",
				"an authoring schema has no output-only members by construction")

			// and the URL still dispatches to the right grammar (§2g): the
			// trailing file name is what DocumentKind matches on
			kind, decided := documentKindOf([]byte(`{"$schema": "` + tc.url + `"}`))
			require.True(t, decided)
			switch name {
			case "object":
				assert.Equal(t, KindObject, kind)
			case "index":
				assert.Equal(t, KindIndex, kind)
			case "properties":
				assert.Equal(t, KindPropertyDictionary, kind)
			}
		})
	}
}
