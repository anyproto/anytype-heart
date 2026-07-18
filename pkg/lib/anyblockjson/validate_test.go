package anyblockjson

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
				{"type": "bulletedListItem", "text": "item", "children": [
					{"type": "bulletedListItem", "text": "nested"}
				]},
				{"type": "checkbox", "checked": true, "text": "Draft"},
				{"type": "code", "language": "go", "text": "func main() {}"},
				{"type": "divider", "style": "dots"},
				{"type": "row", "children": [
					{"type": "column", "children": [{"type": "paragraph", "text": "left"}]},
					{"type": "column", "children": [{"type": "paragraph", "text": "right"}]}
				]}
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
			 "properties": [{"key": "name", "format": "shortText"}, {"key": "status", "format": "select"}],
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
			{"type": "widget", "layout": "tree", "limit": 6, "children": [{"type": "link", "objectId": "obj1"}]}
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
		{"children on leaf block", `{"version": 1, "blocks": [{"type": "divider", "children": [{"type": "paragraph"}]}]}`, "children"},
		{"row child not column", `{"version": 1, "blocks": [{"type": "row", "children": [{"type": "paragraph"}]}]}`, "/blocks/0/children/0"},
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
		{"bad property value shape", `{"version": 1, "properties": {"x": {"nested": 1}}}`, "/properties/x"},
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
	// produced by a newer version (§10)
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
		{"type": "toggle", "children": [
			{"type": "paragraph", "text": "bad </font> here"}
		]}
	]}`
	err := Validate([]byte(doc))
	require.Error(t, err)
	var ve *ValidationError
	require.True(t, errors.As(err, &ve))
	require.Len(t, ve.Issues, 1)
	assert.Equal(t, "/blocks/1/children/0/text", ve.Issues[0].Path)
}
