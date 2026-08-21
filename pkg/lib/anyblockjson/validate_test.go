package anyblockjson

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// The whole reason this format exists is the generate → validate → feed-back
// loop (§12), so a confident wrong issue is worse than a verbose one: an
// agent told `/blocks/0/type: property "type" is not allowed` deletes `type`.
// Two schema mechanics produce those: `unevaluatedProperties: false` reports
// every property of an object whose type-specific subschema failed (its
// annotations are discarded), and an `anyOf` reports every branch it tried.
func TestValidate_ErrorsDoNotCascade(t *testing.T) {
	issues := func(t *testing.T, doc string) []Issue {
		err := Validate([]byte(doc))
		require.Error(t, err)
		var ve *ValidationError
		require.True(t, errors.As(err, &ve))
		return ve.Issues
	}

	t.Run("a bad type is one issue, not three", func(t *testing.T) {
		// the camelCase spelling is now the plausible mistake: it is what the
		// pre-snake_case draft used, and what a model trained on it emits
		got := issues(t, `{"version": 1, "blocks": [
			{"type": "bulletedListItem", "text": "x"}]}`)
		require.Len(t, got, 1, "got: %v", got)
		assert.Equal(t, "/blocks/0/type", got[0].Path)
		assert.Contains(t, got[0].Message, "value must be one of")
	})

	t.Run("a bad field type is one issue, not four", func(t *testing.T) {
		got := issues(t, `{"version": 1, "blocks": [
			{"type": "checkbox", "checked": "yes", "text": "x"}]}`)
		require.Len(t, got, 1, "got: %v", got)
		assert.Equal(t, "/blocks/0/checked", got[0].Path)
		assert.Contains(t, got[0].Message, "got string, want boolean")
	})

	t.Run("the anyOf branch the author meant is the one reported", func(t *testing.T) {
		got := issues(t, `{"version": 1, "blocks": [
			{"type": "table", "columns": [{"id": "c1"}],
			 "rows": [{"id": "r1", "cells": [{"type": "paragraph", "id": "x1", "text": "a"}]}]}]}`)
		require.Len(t, got, 1, "got: %v", got)
		assert.Equal(t, "/blocks/0/rows/0/cells/0/id", got[0].Path)
	})

	t.Run("a cell of no admissible shape names every shape once", func(t *testing.T) {
		got := issues(t, `{"version": 1, "blocks": [
			{"type": "table", "columns": [{"id": "c1"}],
			 "rows": [{"id": "r1", "cells": [7]}]}]}`)
		require.Len(t, got, 1, "got: %v", got)
		assert.Equal(t, "/blocks/0/rows/0/cells/0", got[0].Path)
		for _, want := range []string{"number", "string", "null", "object", "array"} {
			assert.Contains(t, got[0].Message, want)
		}
	})

	t.Run("an unknown key is still reported when it is the only fault", func(t *testing.T) {
		got := issues(t, `{"version": 1, "blocks": [
			{"type": "paragraph", "text": "x", "bogus": 1}]}`)
		require.Len(t, got, 1, "got: %v", got)
		assert.Equal(t, "/blocks/0/bogus", got[0].Path)
		assert.Contains(t, got[0].Message, `property "bogus" is not allowed`)
	})

	t.Run("an unknown key survives a sibling error", func(t *testing.T) {
		// suppression is aimed at names the schema knows and could not
		// evaluate; a hallucinated key is never admissible, so the verdict
		// on it stands and the agent gets both facts in one round
		got := issues(t, `{"version": 1, "blocks": [
			{"type": "checkbox", "checked": "yes", "bogus": 1}]}`)
		require.Len(t, got, 2, "got: %v", got)
		paths := []string{got[0].Path, got[1].Path}
		assert.Contains(t, paths, "/blocks/0/checked")
		assert.Contains(t, paths, "/blocks/0/bogus")
	})

	t.Run("the children migration hint survives", func(t *testing.T) {
		got := issues(t, `{"version": 1, "blocks": [
			{"type": "paragraph", "text": "x", "children": []}]}`)
		require.Len(t, got, 1, "got: %v", got)
		assert.Contains(t, got[0].Message, "nest with indent instead")
	})

	t.Run("a wrong field on the right type is still reported", func(t *testing.T) {
		// `checked` belongs to checkbox, and nothing else in this block
		// failed, so the closed-set verdict is trustworthy
		got := issues(t, `{"version": 1, "blocks": [
			{"type": "paragraph", "checked": true}]}`)
		require.Len(t, got, 1, "got: %v", got)
		assert.Equal(t, "/blocks/0/checked", got[0].Path)
	})
}

// A tag-shaped sequence the grammar does not recognize is literal text and
// never an error (§10) — that leniency is what keeps a stored document
// readable across a version that adds a tag. But canonical export escapes
// those bytes (§8.2), so finding them unescaped means the text was
// hand-written or produced by a version that knows the tag, which is worth
// one warning and no more.
func TestValidate_UnknownTagStaysLiteralAndWarns(t *testing.T) {
	warningsFor := func(t *testing.T, doc string) []Issue {
		var got []Issue
		require.NoError(t, ValidateWarn([]byte(doc), func(i Issue) { got = append(got, i) }),
			"an unknown tag is not a validation error")
		return got
	}

	t.Run("unrecognized tag warns once and known tags do not", func(t *testing.T) {
		got := warningsFor(t, `{"version": 1, "blocks": [
			{"type": "paragraph", "text": "<sub>x</sub> and <u>y</u>"}]}`)
		require.Len(t, got, 1, "one warning per unrecognized name, not per occurrence")
		assert.Equal(t, "/blocks/0/text", got[0].Path)
		assert.Contains(t, got[0].Message, `"<sub"`)
	})

	t.Run("escaped tag is unambiguous, so no warning", func(t *testing.T) {
		assert.Empty(t, warningsFor(t, `{"version": 1, "blocks": [
			{"type": "paragraph", "text": "\\<sub>x\\</sub>"}]}`))
	})

	t.Run("a table cell string is warned about too", func(t *testing.T) {
		got := warningsFor(t, `{"version": 1, "blocks": [
			{"type": "table", "columns": [{"id": "c1"}],
			 "rows": [{"id": "r1", "cells": ["<mark>hi</mark>"]}]}]}`)
		require.Len(t, got, 1)
		assert.Equal(t, "/blocks/0/rows/0/cells/0", got[0].Path)
	})
}

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
				{"id": "b1", "type": "heading_2", "text": "Goals"},
				{"id": "b2", "type": "paragraph", "text": "Ship the **new export**"},
				{"type": "bulleted_list_item", "text": "item"},
				{"indent": 1, "type": "bulleted_list_item", "text": "nested"},
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
				{"id": "r1", "is_header": true, "cells": ["Name", "Status"]},
				{"id": "r2", "cells": ["Export", {"type": "checkbox", "checked": true, "text": "done"}]},
				{"id": "r3", "cells": [null]}
			 ]}
		]}`},
		{"dataview", `{"version": 1, "blocks": [
			{"type": "dataview", "object_id": "bafyset",
			 "properties": [{"key": "name", "format": "text"}, {"key": "status", "format": "select"}],
			 "views": [
				{"id": "v1", "type": "kanban", "name": "By status", "group_by": "status",
				 "sorts": [{"property": "dueDate", "direction": "asc", "empty_placement": "end"}],
				 "filters": [
					{"property": "dueDate", "condition": "less", "date_preset": "current_week"},
					{"operator": "or", "filters": [
						{"property": "done", "condition": "equal", "value": false},
						{"property": "done", "condition": "empty"}
					]}
				 ],
				 "columns": [{"property": "name"}, {"property": "status", "width": 30, "aggregation": "count_distinct", "align": "right"}]}
			 ]}
		]}`},
		{"template", `{"version": 1, "type": "template", "template_for": "task"}`},
		{"collection items", `{"version": 1, "type": "collection", "items": ["obj1", "obj2"]}`},
		{"widget", `{"version": 1, "kind": "widget", "blocks": [
			{"type": "widget", "layout": "tree", "limit": 6},
			{"indent": 1, "type": "link", "object_id": "obj1"}
		]}`},
		{"explicit indent 0", `{"version": 1, "blocks": [{"indent": 0, "type": "paragraph", "text": "x"}]}`},
		{"cell array with descendants", `{"version": 1, "blocks": [
			{"type": "table", "columns": [{"id": "c1"}], "rows": [{"id": "r1", "cells": [[
				{"type": "toggle", "text": "cell"},
				{"indent": 1, "type": "paragraph", "text": "nested"}
			]]}]}
		]}`},
		{"heading_4 alias", `{"version": 1, "blocks": [{"type": "heading_4", "text": "deep"}]}`},
		{"equation alias", `{"version": 1, "blocks": [{"type": "equation", "text": "E=mc^2"}]}`},
		{"option_ids", `{"version": 1, "properties": {"tag": ["High"], "c#_lang": ["C#"]},
			"option_ids": {"tag": {"import issue": "bafyreiabc", "High": "bafyreidef"},
				"c#_lang": {"C#": "bafyreighi"}}}`},
		// view-id uniqueness is scoped to the dataview BLOCK (§6.2): the app
		// mints every set/collection/type default view as "default", and
		// creating an inline set from one copies its views verbatim, so a
		// page with two inline collections legitimately holds two "default"s
		{"one view id in two dataviews", `{"version": 1, "blocks": [
			{"type": "dataview", "object_id": "bafyone", "views": [{"id": "default", "name": "A"}]},
			{"type": "dataview", "object_id": "bafytwo", "views": [{"id": "default", "name": "B"}]}
		]}`},
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
		{"template_for without template type", `{"version": 1, "type": "page", "template_for": "task"}`, "template_for"},
		{"language and fields.lang conflict", `{"version": 1, "blocks": [
			{"type": "code", "language": "go", "fields": {"lang": "go"}}
		]}`, "fields.lang"},
		{"inline markup error", `{"version": 1, "blocks": [{"type": "paragraph", "text": "<u>unclosed"}]}`, "/blocks/0/text"},
		{"inline markup error in cell", `{"version": 1, "blocks": [
			{"type": "table", "columns": [{"id": "c1"}], "rows": [{"id": "r1", "cells": ["<mention>x</mention>"]}]}
		]}`, "/blocks/0/rows/0/cells/0"},
		{"an option_ids spelling with a control character",
			`{"version": 1, "option_ids": {"a\nb": {"High": "bafy1"}}}`,
			`/option_ids/a` + "\n" + `b: option_ids property spelling "a\nb" carries a control character`},
		{"an empty option name",
			`{"version": 1, "properties": {"tag": ["High"]}, "option_ids": {"tag": {"": "bafy1"}}}`,
			`/option_ids/tag/: option name is empty`},
		{"filter mixing group and leaf", `{"version": 1, "blocks": [
			{"type": "dataview", "views": [{"id": "v", "filters": [{"operator": "and", "property": "x", "filters": []}]}]}
		]}`, "/blocks/0/views/0/filters/0"},
		{"reserved compact filter field", `{"version": 1, "blocks": [
			{"type": "dataview", "views": [{"id": "v", "filter": "done = false"}]}
		]}`, "filter"},
		// §6.2: view ids are unique WITHIN a dataview block. Until this,
		// views[].id was the one id slot in the document with no uniqueness
		// check at all — invalid but unvalidated on every channel, create and
		// import included.
		{"duplicate view id in one dataview", `{"version": 1, "blocks": [
			{"type": "dataview", "views": [{"id": "v1", "name": "A"}, {"id": "v1", "name": "B"}]}
		]}`, `duplicate view id "v1" in this dataview`},
		{"duplicate view id path", `{"version": 1, "blocks": [
			{"type": "dataview", "views": [{"id": "v1", "name": "A"}, {"id": "v1", "name": "B"}]}
		]}`, "/blocks/0/views/1/id"},
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
	// the version integer is the sole authority on format identity (§10): a
	// document declaring a newer one is rejected outright, named in the error,
	// and never reaches schema validation
	t.Run("newer version is rejected and named", func(t *testing.T) {
		// given
		doc := `{"version": 2, "blocks": [{"type": "paragraph", "sparkles": true}]}`

		// when
		err := Validate([]byte(doc))

		// then
		require.Error(t, err)
		var ve *ValidationError
		require.True(t, errors.As(err, &ve))
		assert.True(t, ve.NewerFormat)
		assert.True(t, strings.Contains(err.Error(), "newer version"))
		assert.True(t, strings.Contains(err.Error(), "2"))
		// the unknown field never got a chance to produce a constraint failure
		assert.False(t, strings.Contains(err.Error(), "sparkles"))
	})

	t.Run("$schema does not affect format identity", func(t *testing.T) {
		// a stale or invented $schema is decorative; only "version" gates
		// given
		doc := `{
			"$schema": "https://schemas.anytype.io/anyblock/9/object.schema.json",
			"version": 1,
			"blocks": [{"type": "paragraph", "text": "fine"}]
		}`

		// when
		err := Validate([]byte(doc))

		// then
		require.NoError(t, err)
	})
}

// TestVersionIdentity pins the one copy of the format version the compiler
// cannot keep honest: the $id and the version const inside each embedded
// schema file. The Go URLs are derived from FormatVersion, so a bump moves
// them automatically — this catches the JSON that a bump must move by hand.
func TestVersionIdentity(t *testing.T) {
	// given
	want := map[string]struct {
		raw []byte
		url string
	}{
		"object": {raw: schemaJSON, url: SchemaURL},
		"index":  {raw: indexSchemaJSON, url: IndexSchemaURL},
	}

	for name, tc := range want {
		t.Run(name, func(t *testing.T) {
			// when
			var got struct {
				Id      string `json:"$id"`
				Version struct {
					Const *int `json:"const"`
				} `json:"-"`
			}
			require.NoError(t, json.Unmarshal(tc.raw, &got))

			var props struct {
				Properties struct {
					Version struct {
						Const *int `json:"const"`
					} `json:"version"`
				} `json:"properties"`
			}
			require.NoError(t, json.Unmarshal(tc.raw, &props))

			// then
			assert.Equal(t, tc.url, got.Id, "schema $id must equal the derived URL")
			require.NotNil(t, props.Properties.Version.Const, "schema must pin the version")
			assert.Equal(t, FormatVersion, *props.Properties.Version.Const)
			assert.True(t, strings.HasPrefix(tc.url, schemaBaseURL+strconv.Itoa(FormatVersion)+"/"),
				"URL must carry FormatVersion and no minor axis")
		})
	}
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

// A key slot the schema constrains through `propertyNames` — the `properties`
// map, the `property_keys` legend, both levels of `option_ids` (§3, §9a) — has
// to name the member that broke the rule, like every other issue §12 promises. The
// schema cannot: `propertyNames` validates each name as a standalone string
// instance, so the library's verdict carries neither the enclosing object's
// location nor, for a length bound, the name itself. A 200-character property
// key came back as `maxLength: got 200, want 128` at the document ROOT, which
// tells an agent running the generate → validate → feed-back loop (§13)
// nothing it can act on. The rule stays in the published schema — an external
// validator runs that and nothing else — and is restated where the key is in
// hand, which is the verdict this package reports.
func TestValidate_KeySlotIssuesNameTheOffendingMember(t *testing.T) {
	long := strings.Repeat("a", maxPropertyKeyLen+1)
	tests := []struct {
		name     string
		doc      string
		wantPath string
		wantIn   []string
	}{
		{
			name:     "an over-long property key",
			doc:      `{"version": 1, "properties": {"` + long + `": "x"}}`,
			wantPath: "/properties/" + long,
			wantIn:   []string{long, "129", "128"},
		},
		{
			name:     "a property key carrying a control character",
			doc:      `{"version": 1, "properties": {"a\nb": "x"}}`,
			wantPath: "/properties/a\nb",
			wantIn:   []string{`"a\nb"`, "control character"},
		},
		{
			name:     "the empty property key",
			doc:      `{"version": 1, "properties": {"": "x"}}`,
			wantPath: "/properties/",
			wantIn:   []string{"empty"},
		},
		{
			name:     "an unwritable legend spelling",
			doc:      `{"version": 1, "property_keys": {"a\nb": "due_date"}}`,
			wantPath: "/property_keys/a\nb",
			wantIn:   []string{`"a\nb"`, "control character"},
		},
		{
			name:     "an unwritable legend stored key",
			doc:      `{"version": 1, "property_keys": {"prio": "` + long + `"}}`,
			wantPath: "/property_keys/prio",
			wantIn:   []string{long, "129", "128"},
		},
		{
			name:     "an empty legend stored key",
			doc:      `{"version": 1, "property_keys": {"prio": ""}}`,
			wantPath: "/property_keys/prio",
			wantIn:   []string{"empty"},
		},
		{
			name:     "an option_ids spelling past the bound",
			doc:      `{"version": 1, "option_ids": {"` + long + `": {"High": "bafyreiabc"}}}`,
			wantPath: "/option_ids/" + long,
			wantIn:   []string{long, "129", "128"},
		},
		{
			// the INNER propertyNames, whose only rule is non-empty. Its own
			// site in the schema is reported at the document root without
			// this case (§12), and the pointer has to reach the level too —
			// `/option_ids/tag/` is the empty member of `tag`'s map.
			name:     "an empty option name",
			doc:      `{"version": 1, "option_ids": {"tag": {"": "bafyreiabc"}}}`,
			wantPath: "/option_ids/tag/",
			wantIn:   []string{"empty"},
		},
		// A spelling carrying a pointer metacharacter is escaped (RFC 6901),
		// and the escape is what keeps the count at one: the schema's own
		// verdict on the same value is suppressed through a ledger keyed by
		// pointer, so an unescaped location missed it and the one empty value
		// was reported three times, twice at a location the document has no
		// member at. Both metacharacters are legal in a stored key and in a
		// spelling — the writable-key rule bounds length and control
		// characters, nothing else (§3).
		{
			name:     "a legend spelling holding a slash",
			doc:      `{"version": 1, "property_keys": {"a/b": ""}}`,
			wantPath: "/property_keys/a~1b",
			wantIn:   []string{"empty"},
		},
		{
			name:     "a type legend spelling holding a tilde",
			doc:      `{"version": 1, "type_keys": {"a~b": ""}}`,
			wantPath: "/type_keys/a~0b",
			wantIn:   []string{"empty"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate([]byte(tc.doc))
			require.Error(t, err)
			var ve *ValidationError
			require.True(t, errors.As(err, &ve))
			require.Len(t, ve.Issues, 1, "one member, one issue: %v", ve.Issues)
			assert.Equal(t, tc.wantPath, ve.Issues[0].Path)
			for _, want := range tc.wantIn {
				assert.Contains(t, ve.Issues[0].Message, want)
			}
		})
	}
}

// propertyNamesSites lists every place in a schema document that constrains
// property names, as JSON pointers. It descends through ARRAYS as well as
// objects, because half of this schema's subschemas hang off array-valued
// keywords — `allOf`, `anyOf`, `oneOf` (the block dispatch, the table cell,
// the filter node) — and a walk that only follows map values would report a
// clean sweep of a schema it had not finished reading.
func propertyNamesSites(node any, at string) []string {
	var sites []string
	switch n := node.(type) {
	case map[string]any:
		if _, has := n["propertyNames"]; has {
			sites = append(sites, at)
		}
		for _, k := range sortedMapKeys(n) {
			sites = append(sites, propertyNamesSites(n[k], at+"/"+escapeJSONPointer(k))...)
		}
	case []any:
		for i, e := range n {
			sites = append(sites, propertyNamesSites(e, fmt.Sprintf("%s/%d", at, i))...)
		}
	}
	return sites
}

// The restated rule has to cover every `propertyNames` the schema carries, or
// a key slot loses its addressable message the moment one is added — the
// schema's own verdict is still reported for anything this pass does not
// speak for, so the failure would be silent noise rather than a crash.
func TestValidate_EveryPropertyNamesSiteHasAnAddressableMessage(t *testing.T) {
	var doc any
	require.NoError(t, json.Unmarshal(SchemaJSON(), &doc))

	sites := propertyNamesSites(doc, "")
	sort.Strings(sites)

	assert.Equal(t, []string{
		"/$defs/propertyMap", // the properties map, via $ref from /properties
		"/properties/option_ids",
		// the option-name level: `option_ids` carries a propertyNames at BOTH
		// levels and each owes its own case, which is the easy one to
		// under-count
		"/properties/option_ids/additionalProperties",
		"/properties/property_keys",
		"/properties/type_keys",
	}, sites, "a new propertyNames site needs a case in propertyNameIssues")
}

// …and the sweep above is only a guarantee if the walk reaches everywhere a
// site can be. Every site in the schema today is a plain map value, so the
// array descent is unfalsifiable against the schema itself: this fixture is
// what makes it fail when it stops working. The shapes are the ones the
// schema already uses for its subschemas — a block arm under `allOf`, a table
// cell arm under `anyOf`, a filter arm under `oneOf` — plus `prefixItems`,
// which a tuple-shaped slot would use.
func TestPropertyNamesSites_DescendsIntoArrayKeywords(t *testing.T) {
	var doc any
	require.NoError(t, json.Unmarshal([]byte(`{
		"allOf": [{"then": {"properties": {"legend": {"propertyNames": {"maxLength": 8}}}}}],
		"$defs": {
			"cell": {"anyOf": [{"type": "string"}, {"propertyNames": {"maxLength": 8}}]},
			"node": {"oneOf": [{"prefixItems": [{"propertyNames": {"maxLength": 8}}]}]}
		}
	}`), &doc))

	sites := propertyNamesSites(doc, "")
	sort.Strings(sites)

	assert.Equal(t, []string{
		"/$defs/cell/anyOf/1",
		"/$defs/node/oneOf/0/prefixItems/0",
		"/allOf/0/then/properties/legend",
	}, sites)
}

// TestValidate_IndentErrorMessage: the V1 message is the agent-facing repair
// loop — it must name both indents (§12).
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
// equals the equivalent valid document's (§4).
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
// guarantee, made testable (§4).
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

// TestValidate_UnknownEnvelopeMembersAreAddressedOneByOne pins the general
// rule the `refs` diagnostic is one case of: the envelope is closed with
// `additionalProperties: false`, which the library reports as ONE verdict per
// OBJECT — every unknown member named inside its text, at the object's own
// location. Inside a block the same fault comes back per member (blocks close
// with `unevaluatedProperties`), so before this the format's one promise about
// issues — "an issue names the member it is about" (§12) — held everywhere but
// the envelope, and exactly at the envelope is where a document written
// against an older grammar fails.
//
// The fixture carries TWO unknown members whose sorted order is the reverse of
// nothing in particular: the library builds its list by ranging over the
// instance's map, so an unsorted reader answers in a different order run to
// run, and this asserts the whole ordered slice rather than a set.
func TestValidate_UnknownEnvelopeMembersAreAddressedOneByOne(t *testing.T) {
	// given — one legend the format used to carry and one name it never had
	doc := `{"version": 1, "refs": {"idxxx": "bafyreitarget"}, "zzz_unknown": 1,
		"blocks": [{"type": "paragraph", "text": "x"}]}`
	want := []string{"/refs", "/zzz_unknown"}

	// when
	err := Validate([]byte(doc))

	// then
	require.Error(t, err)
	var ve *ValidationError
	require.True(t, errors.As(err, &ve), "got %v", err)
	got := make([]string, 0, len(ve.Issues))
	for _, i := range ve.Issues {
		got = append(got, i.Path)
	}
	assert.Equal(t, want, got,
		"each unknown envelope member gets its own pointer, in a stable order")
	for _, i := range ve.Issues {
		assert.Contains(t, i.Message, "is not allowed")
		assert.NotContains(t, i.Message, "zzz_unknown\", \"refs",
			"no issue may still carry the merged list the split replaced")
	}
}
