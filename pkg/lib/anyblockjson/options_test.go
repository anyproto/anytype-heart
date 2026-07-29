package anyblockjson

// A select's vocabulary is otherwise discovered only from values that happen
// to be used, so a schema value no sample record carries never exists (its
// kanban column is simply missing), and minted options get no orderId and
// sort alphabetically. typeProperties[].options declares both (§2a).

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
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
	assert.Equal(t, []string{"Backlog", "In progress", "Done"}, r.defs[0].Options,
		"declaration order is the display order")
	assert.Empty(t, r.defs[1].Options)
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
			Options: []string{"Backlog", "In progress", "Done"}}}})
	require.NoError(t, err)
	assert.Contains(t, string(data), `"options"`)
	assert.Contains(t, string(data), `"Backlog"`)
	assert.NoError(t, Validate(data))
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
