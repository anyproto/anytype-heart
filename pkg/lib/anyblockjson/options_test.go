package anyblockjson

// A select's vocabulary is otherwise discovered only from values that happen
// to be used, so a schema value no sample record carries never exists (its
// kanban column is simply missing), and minted options get no orderId and
// sort alphabetically. typeProperties[].options declares both.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/constant"
)

type recordingPropertyResolver struct{ defs []PropertyDefinition }

func (r *recordingPropertyResolver) PropertyById(string) (PropertyDefinition, bool) {
	return PropertyDefinition{}, false
}
func (r *recordingPropertyResolver) PropertyId(def PropertyDefinition) (string, bool) {
	r.defs = append(r.defs, def)
	return string(def.Key), true
}

func TestImport_OptionsReachTheWiring(t *testing.T) {
	doc := `{"version": 1, "kind": "objectType", "id": "t1", "key": "k",
		"typeProperties": [
			{"key": "stage", "name": "Stage", "format": "select",
			 "options": ["Backlog", "In progress", "Done"]},
			{"key": "note", "name": "Note", "format": "text"}]}`
	r := &recordingPropertyResolver{}
	_, _, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g"), ResolveProperties: r})
	require.NoError(t, err)

	require.Len(t, r.defs, 2)
	assert.Equal(t, domain.RelationKey("stage"), r.defs[0].Key)
	assert.Equal(t,
		[]OptionDefinition{{Name: "Backlog"}, {Name: "In progress"}, {Name: "Done"}},
		r.defs[0].Options, "declaration order is the display order")
	assert.Empty(t, r.defs[1].Options)
}

// A color is declared on the option it belongs to rather than in a parallel
// array, so inserting or reordering an option cannot shift it.
func TestImport_OptionColorsReachTheWiring(t *testing.T) {
	doc := `{"version": 1, "kind": "objectType", "id": "t1", "key": "k",
		"typeProperties": [{"key": "stage", "name": "Stage", "format": "select",
			"options": ["Backlog", {"name": "In progress", "color": "blue"},
				{"name": "Done", "color": "lime"}]}]}`
	want := []OptionDefinition{
		{Name: "Backlog"},
		{Name: "In progress", Color: "blue"},
		{Name: "Done", Color: "lime"},
	}
	r := &recordingPropertyResolver{}
	_, _, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g"), ResolveProperties: r})
	require.NoError(t, err)

	require.Len(t, r.defs, 1)
	assert.Equal(t, want, r.defs[0].Options,
		"a bare string is an option that declares no color")
}

func TestExport_OptionsRoundTrip(t *testing.T) {
	doc := `{"version": 1, "kind": "objectType", "id": "t1", "key": "k",
		"typeProperties": [{"key": "stage", "name": "Stage", "format": "select",
			"options": ["Backlog", "In progress", "Done"]}]}`
	_, snap, err := Unmarshal([]byte(doc), Options{
		GenerateId: seqIds("g"), ResolveProperties: &recordingPropertyResolver{}})
	require.NoError(t, err)

	data, err := Marshal(model.SmartBlockType_STType, snap, Options{ResolveProperties: &staticPropertyResolver{
		def: PropertyDefinition{Key: "stage", Name: "Stage", Format: 3,
			Options: []OptionDefinition{{Name: "Backlog"}, {Name: "In progress"}, {Name: "Done"}}}}})
	require.NoError(t, err)
	assert.Contains(t, string(data), `"options"`)
	assert.Contains(t, string(data), `"Backlog"`)
	assert.NoError(t, Validate(data))
}

// The bare string is canonical whenever the option carries no color, the
// object form otherwise — the rule table cells already follow.
func TestExport_ColorlessOptionStaysABareString(t *testing.T) {
	doc := `{"version": 1, "kind": "objectType", "id": "t1", "key": "k",
		"typeProperties": [{"key": "stage", "name": "Stage", "format": "select",
			"options": ["Backlog", {"name": "Done", "color": "lime"}]}]}`
	_, snap, err := Unmarshal([]byte(doc), Options{
		GenerateId: seqIds("g"), ResolveProperties: &recordingPropertyResolver{}})
	require.NoError(t, err)

	data, err := Marshal(model.SmartBlockType_STType, snap, Options{ResolveProperties: &staticPropertyResolver{
		def: PropertyDefinition{Key: "stage", Name: "Stage", Format: 3,
			Options: []OptionDefinition{{Name: "Backlog"}, {Name: "Done", Color: "lime"}}}}})
	require.NoError(t, err)

	var out struct {
		TypeProps []struct {
			Options []any `json:"options"`
		} `json:"typeProperties"`
	}
	require.NoError(t, json.Unmarshal(data, &out))
	require.Len(t, out.TypeProps, 1)
	assert.Equal(t,
		[]any{"Backlog", map[string]any{"name": "Done", "color": "lime"}},
		out.TypeProps[0].Options)
	assert.NoError(t, Validate(data))

	// canonical key order inside the object form is name then color
	rendered := string(data)[strings.Index(string(data), `"options"`):]
	assert.Less(t, strings.Index(rendered, `"name"`), strings.Index(rendered, `"color"`))
}

func TestValidate_OptionColorRules(t *testing.T) {
	t.Run("unknown color rejected", func(t *testing.T) {
		err := Validate([]byte(`{"version": 1, "kind": "objectType", "id": "t1", "key": "k",
			"typeProperties": [{"key": "stage", "format": "select",
				"options": [{"name": "a", "color": "chartreuse"}]}]}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "options/0/color")
	})
	t.Run("colored options are not duplicates of each other", func(t *testing.T) {
		assert.NoError(t, Validate([]byte(`{"version": 1, "kind": "objectType", "id": "t1", "key": "k",
			"typeProperties": [{"key": "stage", "format": "select",
				"options": ["a", {"name": "b", "color": "blue"}, {"name": "c", "color": "lime"}]}]}`)))
	})
	t.Run("duplicate across the two forms rejected", func(t *testing.T) {
		err := Validate([]byte(`{"version": 1, "kind": "objectType", "id": "t1", "key": "k",
			"typeProperties": [{"key": "stage", "format": "select",
				"options": ["a", {"name": "a", "color": "blue"}]}]}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate option")
	})
	t.Run("object form on a non-select rejected", func(t *testing.T) {
		err := Validate([]byte(`{"version": 1, "kind": "objectType", "id": "t1", "key": "k",
			"typeProperties": [{"key": "note", "format": "text", "options": [{"name": "a"}]}]}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only meaningful on select")
	})
}

// The palette lives in util/constant and is restated as an enum in the
// published schema, which is hand-maintained; this keeps the two in step.
func TestSchema_OptionColorEnumMatchesPalette(t *testing.T) {
	var schema struct {
		Defs struct {
			OptionColor struct {
				Enum []string `json:"enum"`
			} `json:"optionColor"`
		} `json:"$defs"`
	}
	require.NoError(t, json.Unmarshal(schemaJSON, &schema))

	want := make([]string, 0, len(constant.OptionColors()))
	for _, c := range constant.OptionColors() {
		want = append(want, c.String())
	}
	assert.Equal(t, want, schema.Defs.OptionColor.Enum)
}

type staticPropertyResolver struct{ def PropertyDefinition }

func (r *staticPropertyResolver) PropertyById(string) (PropertyDefinition, bool) {
	return r.def, true
}
func (r *staticPropertyResolver) PropertyId(def PropertyDefinition) (string, bool) {
	return string(def.Key), true
}

func TestValidate_OptionsRules(t *testing.T) {
	t.Run("rejected on a non-select format", func(t *testing.T) {
		err := Validate([]byte(`{"version": 1, "kind": "objectType", "id": "t1", "key": "k",
			"typeProperties": [{"key": "note", "format": "text", "options": ["a"]}]}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only meaningful on select")
	})
	t.Run("duplicates rejected", func(t *testing.T) {
		err := Validate([]byte(`{"version": 1, "kind": "objectType", "id": "t1", "key": "k",
			"typeProperties": [{"key": "stage", "format": "select", "options": ["a", "b", "a"]}]}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate option")
	})
	t.Run("accepted on select and multiSelect", func(t *testing.T) {
		for _, f := range []string{"select", "multiSelect"} {
			assert.NoError(t, Validate([]byte(`{"version": 1, "kind": "objectType", "id": "t1", "key": "k",
				"typeProperties": [{"key": "stage", "format": "`+f+`", "options": ["a", "b"]}]}`)), f)
		}
	})
}

func TestImport_ObjectTypesReachTheWiring(t *testing.T) {
	doc := `{"version": 1, "kind": "objectType", "id": "t1", "key": "k",
		"typeProperties": [
			{"key": "owner", "name": "Owner", "format": "objects",
			 "objectTypes": ["wikiPerson", "participant"]},
			{"key": "anything", "name": "Anything", "format": "objects"}]}`
	r := &recordingPropertyResolver{}
	_, _, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g"), ResolveProperties: r})
	require.NoError(t, err)

	require.Len(t, r.defs, 2)
	assert.Equal(t, []string{"wikiPerson", "participant"}, r.defs[0].ObjectTypes,
		"priority order is preserved")
	assert.Empty(t, r.defs[1].ObjectTypes, "untargeted accepts any object")
}

func TestValidate_ObjectTypesRules(t *testing.T) {
	t.Run("rejected on a non-object format", func(t *testing.T) {
		for _, f := range []string{"select", "date", "text"} {
			err := Validate([]byte(`{"version": 1, "kind": "objectType", "id": "t1", "key": "k",
				"typeProperties": [{"key": "p", "format": "` + f + `", "objectTypes": ["participant"]}]}`))
			require.Error(t, err, f)
			assert.Contains(t, err.Error(), "only meaningful on objects/files")
		}
	})
	t.Run("accepted on objects and files", func(t *testing.T) {
		for _, f := range []string{"objects", "files"} {
			assert.NoError(t, Validate([]byte(`{"version": 1, "kind": "objectType", "id": "t1", "key": "k",
				"typeProperties": [{"key": "p", "format": "`+f+`", "objectTypes": ["participant"]}]}`)), f)
		}
	})
}
