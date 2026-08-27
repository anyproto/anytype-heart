package anyblockjson

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestValidate_Valid(t *testing.T) {
	tests := []struct {
		name string
		doc  string
	}{
		{"minimal", `{"version": 1}`},
		{"envelope", `{
			"$schema": "https://schemas.anytype.io/anyblock/1.0/object.schema.json",
			"version": 1,
			"id": "bafyrei123",
			"type": "page",
			"properties": {"name": "Test", "iconEmoji": "🔥", "status": ["In progress"], "priority": 3, "done": false},
			"blocks": [
				{"id": "b1", "type": "heading2", "text": "Goals"},
				{"id": "b2", "type": "paragraph", "text": "Ship the **new export**"},
				{"type": "bulletedListItem", "text": "item"},
				{"indent": 1, "type": "bulletedListItem", "text": "nested"},
				{"type": "checkbox", "checked": true, "text": "Draft"},
				{"type": "code", "language": "go", "text": "func main() {}"},
				{"type": "divider", "style": "dots"},
				{"type": "row"},
				{"indent": 1, "type": "column"},
				{"indent": 2, "type": "paragraph", "text": "left"},
				{"indent": 1, "type": "column"},
				{"indent": 2, "type": "paragraph", "text": "right"}
			]
		}`},
		{"table", `{"version": 1, "blocks": [
			{"type": "table",
			 "columns": [{"id": "c1"}, {"id": "c2", "width": 120}],
			 "rows": [
				{"id": "r1", "isHeader": true, "cells": ["Name", "Status"]},
				{"id": "r2", "cells": ["Export", {"type": "checkbox", "checked": true, "text": "done"}]},
				{"id": "r3", "cells": [null]}
			 ]}
		]}`},
		{"dataview", `{"version": 1, "blocks": [
			{"type": "dataview", "objectId": "bafyset",
			 "properties": [{"key": "name", "format": "text"}, {"key": "status", "format": "select"}],
			 "views": [
				{"id": "v1", "type": "kanban", "name": "By status", "groupBy": "status",
				 "sorts": [{"property": "dueDate", "direction": "asc", "emptyPlacement": "end"}],
				 "filters": [
					{"property": "dueDate", "condition": "less", "datePreset": "currentWeek"},
					{"operator": "or", "filters": [
						{"property": "done", "condition": "equal", "value": false},
						{"property": "done", "condition": "empty"}
					]}
				 ],
				 "columns": [{"property": "name"}, {"property": "status", "width": 30, "aggregation": "countDistinct", "align": "right"}]}
			 ]}
		]}`},
		{"template", `{"version": 1, "type": "template", "templateFor": "task"}`},
		{"collection items", `{"version": 1, "type": "collection", "items": ["obj1", "obj2"]}`},
		{"widget", `{"version": 1, "kind": "widget", "blocks": [
			{"type": "widget", "layout": "tree", "limit": 6},
			{"indent": 1, "type": "link", "objectId": "obj1"}
		]}`},
		{"explicit indent 0", `{"version": 1, "blocks": [{"indent": 0, "type": "paragraph", "text": "x"}]}`},
		{"cell array with descendants", `{"version": 1, "blocks": [
			{"type": "table", "columns": [{"id": "c1"}], "rows": [{"id": "r1", "cells": [[
				{"type": "toggle", "text": "cell"},
				{"indent": 1, "type": "paragraph", "text": "nested"}
			]]}]}
		]}`},
		{"heading4 alias", `{"version": 1, "blocks": [{"type": "heading4", "text": "deep"}]}`},
		{"equation alias", `{"version": 1, "blocks": [{"type": "equation", "text": "E=mc^2"}]}`},
		{"refs", `{"version": 1, "refs": {"roman": "bafyreiabc", "x_1-2": "bafyreidef"}}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, Validate([]byte(tc.doc)))
		})
	}
}

func TestValidate_Invalid(t *testing.T) {
	tests := []struct {
		name    string
		doc     string
		wantMsg string // substring expected in the error
	}{
		{"not json", `{`, "invalid JSON"},
		{"not object", `[1]`, "must be a JSON object"},
		{"version missing", `{"blocks": []}`, "version is required"},
		{"version newer", `{"version": 2}`, "newer than the supported version 1"},
		{"version zero", `{"version": 0}`, "unknown version"},
		{"unknown envelope field", `{"version": 1, "banana": true}`, "banana"},
		{"unknown kind", `{"version": 1, "kind": "banana"}`, "/kind"},
		{"unknown block type", `{"version": 1, "blocks": [{"type": "banana"}]}`, "/blocks/0"},
		{"block type missing", `{"version": 1, "blocks": [{"text": "x"}]}`, "/blocks/0"},
		{"unknown block prop", `{"version": 1, "blocks": [{"type": "paragraph", "banana": 1}]}`, "banana"},
		{"prop from wrong type", `{"version": 1, "blocks": [{"type": "paragraph", "checked": true}]}`, "checked"},
		{"bad align", `{"version": 1, "blocks": [{"type": "paragraph", "align": "top"}]}`, "align"},
		{"bad block id charset", `{"version": 1, "blocks": [{"type": "paragraph", "id": "a b"}]}`, "/blocks/0/id"},
		{"children removed from the format", `{"version": 1, "blocks": [{"type": "toggle", "children": [{"type": "paragraph"}]}]}`, "children"},
		{"first block indented", `{"version": 1, "blocks": [{"indent": 1, "type": "paragraph", "text": "x"}]}`, "first block must be at indent 0"},
		{"indent jump", `{"version": 1, "blocks": [
			{"type": "paragraph", "text": "a"},
			{"indent": 2, "type": "paragraph", "text": "b"}
		]}`, "indent 2 follows indent 0"},
		{"nested under leaf block", `{"version": 1, "blocks": [
			{"type": "divider"},
			{"indent": 1, "type": "paragraph", "text": "x"}
		]}`, "divider blocks cannot have children"},
		{"row child not column", `{"version": 1, "blocks": [
			{"type": "row"},
			{"indent": 1, "type": "paragraph", "text": "x"}
		]}`, "a row block can only contain column blocks"},
		{"indent above bound", `{"version": 1, "blocks": [{"indent": 33, "type": "paragraph", "text": "x"}]}`, "/blocks/0/indent"},
		{"negative indent", `{"version": 1, "blocks": [{"indent": -1, "type": "paragraph", "text": "x"}]}`, "/blocks/0/indent"},
		{"indent on bare cell block", `{"version": 1, "blocks": [
			{"type": "table", "columns": [{"id": "c1"}], "rows": [{"id": "r1", "cells": [{"indent": 1, "type": "paragraph", "text": "x"}]}]}
		]}`, "/blocks/0/rows/0/cells/0"},
		{"id on cell array first block", `{"version": 1, "blocks": [
			{"type": "table", "columns": [{"id": "c1"}], "rows": [{"id": "r1", "cells": [[
				{"id": "x", "type": "toggle", "text": "cell"},
				{"indent": 1, "type": "paragraph", "text": "nested"}
			]]}]}
		]}`, "cell blocks cannot carry an id"},
		{"duplicate ids", `{"version": 1, "blocks": [{"id": "b1", "type": "paragraph"}, {"id": "b1", "type": "quote"}]}`, "duplicate id"},
		{"derived cell id collision", `{"version": 1, "blocks": [
			{"id": "r1-c1", "type": "paragraph"},
			{"type": "table", "columns": [{"id": "c1"}], "rows": [{"id": "r1", "cells": ["x"]}]}
		]}`, "duplicate id"},
		{"row with too many cells", `{"version": 1, "blocks": [
			{"type": "table", "columns": [{"id": "c1"}], "rows": [{"id": "r1", "cells": ["a", "b"]}]}
		]}`, "1 columns"},
		{"cell with id", `{"version": 1, "blocks": [
			{"type": "table", "columns": [{"id": "c1"}], "rows": [{"id": "r1", "cells": [{"id": "x", "type": "paragraph", "text": "a"}]}]}
		]}`, "/blocks/0/rows/0/cells/0"},
		{"table inner id with dash", `{"version": 1, "blocks": [
			{"type": "table", "columns": [{"id": "c-1"}], "rows": []}
		]}`, "/blocks/0/columns/0/id"},
		{"templateFor without template type", `{"version": 1, "type": "page", "templateFor": "task"}`, "templateFor"},
		{"language and fields.lang conflict", `{"version": 1, "blocks": [
			{"type": "code", "language": "go", "fields": {"lang": "go"}}
		]}`, "fields.lang"},
		{"inline markup error", `{"version": 1, "blocks": [{"type": "paragraph", "text": "<u>unclosed"}]}`, "/blocks/0/text"},
		{"inline markup error in cell", `{"version": 1, "blocks": [
			{"type": "table", "columns": [{"id": "c1"}], "rows": [{"id": "r1", "cells": ["<mention>x</mention>"]}]}
		]}`, "/blocks/0/rows/0/cells/0"},
		{"bad refs key", `{"version": 1, "refs": {"a b": "bafy1"}}`, "'a b' does not match pattern"},
		{"filter mixing group and leaf", `{"version": 1, "blocks": [
			{"type": "dataview", "views": [{"id": "v", "filters": [{"operator": "and", "property": "x", "filters": []}]}]}
		]}`, "/blocks/0/views/0/filters/0"},
		{"reserved compact filter field", `{"version": 1, "blocks": [
			{"type": "dataview", "views": [{"id": "v", "filter": "done = false"}]}
		]}`, "filter"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate([]byte(tc.doc))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantMsg)
		})
	}
}

func TestValidate_NewerFormatHint(t *testing.T) {
	// a document citing schema 1.3 with an unknown field must be reported as
	// produced by a newer version
	doc := `{
		"$schema": "https://schemas.anytype.io/anyblock/1.3/object.schema.json",
		"version": 1,
		"blocks": [{"type": "paragraph", "sparkles": true}]
	}`
	err := Validate([]byte(doc))
	require.Error(t, err)
	var ve *ValidationError
	require.True(t, errors.As(err, &ve))
	assert.True(t, ve.NewerFormat)
	assert.True(t, strings.Contains(err.Error(), "newer version"))
}

func TestValidate_PathAddressing(t *testing.T) {
	doc := `{"version": 1, "blocks": [
		{"type": "paragraph", "text": "fine"},
		{"type": "toggle", "text": "parent"},
		{"indent": 1, "type": "paragraph", "text": "bad </font> here"}
	]}`
	err := Validate([]byte(doc))
	require.Error(t, err)
	var ve *ValidationError
	require.True(t, errors.As(err, &ve))
	require.Len(t, ve.Issues, 1)
	assert.Equal(t, "/blocks/2/text", ve.Issues[0].Path)
}

// TestValidate_IndentErrorMessage: the V1 message is the agent-facing repair
// loop — it must name both indents.
func TestValidate_IndentErrorMessage(t *testing.T) {
	doc := `{"version": 1, "blocks": [
		{"type": "paragraph", "text": "a"},
		{"indent": 1, "type": "paragraph", "text": "b"},
		{"indent": 3, "type": "paragraph", "text": "c"}
	]}`
	err := Validate([]byte(doc))
	require.Error(t, err)
	var ve *ValidationError
	require.True(t, errors.As(err, &ve))
	require.Len(t, ve.Issues, 1)
	assert.Equal(t, "/blocks/2", ve.Issues[0].Path)
	assert.Equal(t, "indent 3 follows indent 1 — a block can be at most one level deeper than its predecessor", ve.Issues[0].Message)
}

// TestNormalizeIndent: lenient mode clamps over-deep indents to the deepest
// establishable level with a path-addressed warning, and the imported state
// equals the equivalent valid document's.
func TestNormalizeIndent(t *testing.T) {
	invalid := `{"version": 1, "blocks": [
		{"id": "a", "type": "paragraph", "text": "a"},
		{"indent": 3, "id": "b", "type": "paragraph", "text": "b"}
	]}`
	valid := `{"version": 1, "blocks": [
		{"id": "a", "type": "paragraph", "text": "a"},
		{"indent": 1, "id": "b", "type": "paragraph", "text": "b"}
	]}`

	// strict rejects
	_, _, err := Unmarshal([]byte(invalid), Options{GenerateId: seqIds("g")})
	require.Error(t, err)

	var warnings []Issue
	opts := Options{GenerateId: seqIds("g"), NormalizeIndent: true, OnWarning: func(i Issue) { warnings = append(warnings, i) }}
	_, snap, err := Unmarshal([]byte(invalid), opts)
	require.NoError(t, err)
	require.Len(t, warnings, 1)
	assert.Equal(t, "/blocks/1", warnings[0].Path)
	assert.Contains(t, warnings[0].Message, "clamped to 1")

	_, want, err := Unmarshal([]byte(valid), Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	assert.Equal(t, want.Blocks, snap.Blocks)

	t.Run("first block clamps to 0", func(t *testing.T) {
		doc := `{"version": 1, "blocks": [{"indent": 2, "id": "a", "type": "paragraph", "text": "a"}]}`
		var w []Issue
		o := Options{GenerateId: seqIds("g"), NormalizeIndent: true, OnWarning: func(i Issue) { w = append(w, i) }}
		_, snap, err := Unmarshal([]byte(doc), o)
		require.NoError(t, err)
		require.Len(t, w, 1)
		assert.Equal(t, "/blocks/0", w[0].Path)
		assert.Contains(t, w[0].Message, "clamped to 0")
		root := snap.Blocks[0]
		assert.Equal(t, []string{"a"}, root.ChildrenIds)
	})

	t.Run("bounds stay errors in lenient mode", func(t *testing.T) {
		doc := `{"version": 1, "blocks": [{"indent": 33, "type": "paragraph", "text": "x"}]}`
		o := Options{GenerateId: seqIds("g"), NormalizeIndent: true}
		_, _, err := Unmarshal([]byte(doc), o)
		require.Error(t, err)
	})
}

// TestValidate_PrefixProperty: pre-order plus the monotonicity rule makes
// every prefix of an exported blocks array a valid document — the truncation
// guarantee, made testable.
func TestValidate_PrefixProperty(t *testing.T) {
	data, err := Marshal(model.SmartBlockType_Page, richSnapshot(), testOptions())
	require.NoError(t, err)
	var doc struct {
		Blocks []json.RawMessage `json:"blocks"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	require.NotEmpty(t, doc.Blocks)
	for n := 0; n <= len(doc.Blocks); n++ {
		parts := make([]string, 0, n)
		for _, b := range doc.Blocks[:n] {
			parts = append(parts, string(b))
		}
		prefix := `{"version": 1, "blocks": [` + strings.Join(parts, ",") + `]}`
		require.NoError(t, Validate([]byte(prefix)), "prefix of %d blocks", n)
	}
}
