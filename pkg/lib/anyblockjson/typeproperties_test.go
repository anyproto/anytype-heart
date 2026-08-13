package anyblockjson

import (
	"strings"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type testPropertyResolver struct {
	byId  map[string]PropertyDefinition
	byKey map[domain.RelationKey]string
}

func (r *testPropertyResolver) PropertyById(id string) (PropertyDefinition, bool) {
	def, ok := r.byId[id]
	return def, ok
}

func (r *testPropertyResolver) PropertyId(def PropertyDefinition) (string, bool) {
	id, ok := r.byKey[def.Key]
	return id, ok
}

func newTestPropertyResolver() *testPropertyResolver {
	r := &testPropertyResolver{
		byId: map[string]PropertyDefinition{
			"relid-dueDate":  {Key: "dueDate", Name: "Due date", Format: model.RelationFormat_date},
			"relid-assignee": {Key: "assignee", Name: "Assignee", Format: model.RelationFormat_object},
			"relid-status":   {Key: "status", Name: "Status", Format: model.RelationFormat_status},
			"relid-origin":   {Key: "origin", Name: "Origin", Format: model.RelationFormat_longtext},
			"relid-fileExt":  {Key: "fileExt", Name: "File extension", Format: model.RelationFormat_shorttext},
		},
		byKey: map[domain.RelationKey]string{},
	}
	for id, def := range r.byId {
		r.byKey[def.Key] = id
	}
	return r
}

func typeSnapshot() *model.SmartBlockSnapshotBase {
	return &model.SmartBlockSnapshotBase{
		Key: "task",
		Details: fields(map[string]*types.Value{
			"id":                           str("typeObjectId"),
			"name":                         str("Task"),
			"recommendedFeaturedRelations": strList("relid-dueDate", "relid-assignee"),
			"recommendedRelations":         strList("relid-status"),
			"recommendedFileRelations":     strList("relid-fileExt"),
			"recommendedHiddenRelations":   strList("relid-origin"),
		}),
		Blocks: []*model.Block{
			{Id: "typeObjectId", ChildrenIds: nil, Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
		},
	}
}

func TestTypePropertiesExport(t *testing.T) {
	t.Run("recommended lists become typeProperties in section order", func(t *testing.T) {
		// given
		opts := Options{ResolveProperties: newTestPropertyResolver()}
		want := `"typeProperties": [
    {
      "key": "due_date",
      "name": "Due date",
      "format": "date",
      "section": "featured"
    },
    {
      "key": "assignee",
      "name": "Assignee",
      "format": "objects",
      "section": "featured"
    },
    {
      "key": "status",
      "name": "Status",
      "format": "select"
    },
    {
      "key": "file_ext",
      "name": "File extension",
      "format": "text",
      "section": "file"
    },
    {
      "key": "origin",
      "name": "Origin",
      "format": "text",
      "section": "hidden"
    }
  ]`

		// when
		data, err := Marshal(model.SmartBlockType_STType, typeSnapshot(), opts)

		// then
		require.NoError(t, err)
		assert.Contains(t, string(data), want)
		assert.NotContains(t, string(data), "recommendedRelations")
		assert.NotContains(t, string(data), "relid-")
	})

	t.Run("unresolvable ids are dropped", func(t *testing.T) {
		// given
		snapshot := typeSnapshot()
		snapshot.Details.Fields["recommendedRelations"] = strList("relid-status", "relid-gone")

		// when
		data, err := Marshal(model.SmartBlockType_STType, snapshot, Options{ResolveProperties: newTestPropertyResolver()})

		// then
		require.NoError(t, err)
		assert.Contains(t, string(data), `"key": "status"`)
		assert.NotContains(t, string(data), "relid-gone")
	})

	t.Run("no resolver keeps raw id lists in properties", func(t *testing.T) {
		// when
		data, err := Marshal(model.SmartBlockType_STType, typeSnapshot(), Options{})

		// then
		require.NoError(t, err)
		assert.NotContains(t, string(data), "typeProperties")
		assert.Contains(t, string(data), "recommended_featured_relations")
		assert.Contains(t, string(data), "relid-dueDate")
	})

	t.Run("non-type documents never emit typeProperties", func(t *testing.T) {
		// given
		snapshot := &model.SmartBlockSnapshotBase{
			Details: fields(map[string]*types.Value{
				"id":                   str("pageId"),
				"recommendedRelations": strList("relid-status"),
			}),
		}

		// when
		data, err := Marshal(model.SmartBlockType_Page, snapshot, Options{ResolveProperties: newTestPropertyResolver()})

		// then
		require.NoError(t, err)
		assert.NotContains(t, string(data), "typeProperties")
	})

	t.Run("all-empty lists emit an explicit empty array", func(t *testing.T) {
		// given: a type whose four recommended lists are all empty
		snapshot := typeSnapshot()
		for _, key := range []string{"recommendedFeaturedRelations", "recommendedRelations",
			"recommendedFileRelations", "recommendedHiddenRelations"} {
			snapshot.Details.Fields[key] = strList()
		}

		// when
		data, err := Marshal(model.SmartBlockType_STType, snapshot, Options{ResolveProperties: newTestPropertyResolver()})

		// then: presence of the (empty) array is what lets import rebuild
		// the four lists as explicit empty lists
		require.NoError(t, err)
		assert.Contains(t, string(data), `"typeProperties": []`)
	})

	t.Run("bare bundle key entries resolve via the bundle fallback", func(t *testing.T) {
		// given: legacy type objects store property KEYS in the lists
		snapshot := typeSnapshot()
		snapshot.Details.Fields["recommendedHiddenRelations"] = strList("creator", "createdDate")

		// when
		data, err := Marshal(model.SmartBlockType_STType, snapshot, Options{ResolveProperties: newTestPropertyResolver()})

		// then
		require.NoError(t, err)
		assert.Contains(t, string(data), `"key": "creator"`)
		assert.Contains(t, string(data), `"key": "created_date"`)
	})

	t.Run("compact ids emit no legend entries for lifted lists", func(t *testing.T) {
		// given: recommended ids are long enough to be compacted if collected
		snapshot := typeSnapshot()
		snapshot.Details.Fields["recommendedRelations"] = strList("relid-status")

		// when
		data, err := Marshal(model.SmartBlockType_STType, snapshot, Options{
			ResolveProperties: newTestPropertyResolver(),
			CompactIds:        true,
		})

		// then
		require.NoError(t, err)
		assert.NotContains(t, string(data), `"refs"`)
	})
}

func TestTypePropertiesImport(t *testing.T) {
	docJSON := `{
  "version": 1,
  "kind": "objectType",
  "key": "task",
  "properties": { "name": "Task" },
  "typeProperties": [
    { "key": "due_date", "name": "Due date", "format": "date", "section": "featured" },
    { "key": "status", "name": "Status", "format": "select" },
    { "key": "origin", "section": "hidden" }
  ]
}`

	t.Run("lists rebuilt through the resolver", func(t *testing.T) {
		// given
		opts := Options{ResolveProperties: newTestPropertyResolver(), GenerateId: seqIds("id")}

		// when
		sbType, snapshot, err := Unmarshal([]byte(docJSON), opts)

		// then
		require.NoError(t, err)
		assert.Equal(t, model.SmartBlockType_STType, sbType)
		assert.Equal(t, strList("relid-dueDate"), snapshot.Details.Fields["recommendedFeaturedRelations"])
		assert.Equal(t, strList("relid-status"), snapshot.Details.Fields["recommendedRelations"])
		assert.Equal(t, strList("relid-origin"), snapshot.Details.Fields["recommendedHiddenRelations"])
		// empty sections come back as explicit empty lists, not absent keys
		assert.Equal(t, strList(), snapshot.Details.Fields["recommendedFileRelations"])
	})

	t.Run("unresolved keys pass through for the wiring", func(t *testing.T) {
		// given
		resolver := newTestPropertyResolver()
		delete(resolver.byKey, "status")
		opts := Options{ResolveProperties: resolver, GenerateId: seqIds("id")}

		// when
		_, snapshot, err := Unmarshal([]byte(docJSON), opts)

		// then
		require.NoError(t, err)
		assert.Equal(t, strList("status"), snapshot.Details.Fields["recommendedRelations"])
	})

	t.Run("empty array rebuilds all four lists as empty", func(t *testing.T) {
		// given
		doc := `{"version": 1, "kind": "objectType", "key": "task", "typeProperties": []}`

		// when
		_, snapshot, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("id")})

		// then
		require.NoError(t, err)
		for _, key := range []string{"recommendedFeaturedRelations", "recommendedRelations",
			"recommendedFileRelations", "recommendedHiddenRelations"} {
			assert.Equal(t, strList(), snapshot.Details.Fields[key], key)
		}
	})

	t.Run("absent typeProperties leaves the lists absent", func(t *testing.T) {
		// given
		doc := `{"version": 1, "kind": "objectType", "key": "task"}`

		// when
		_, snapshot, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("id")})

		// then
		require.NoError(t, err)
		assert.Nil(t, snapshot.Details.Fields["recommendedRelations"])
	})

	t.Run("no resolver passes every key through", func(t *testing.T) {
		// when
		_, snapshot, err := Unmarshal([]byte(docJSON), Options{GenerateId: seqIds("id")})

		// then
		require.NoError(t, err)
		assert.Equal(t, strList("dueDate"), snapshot.Details.Fields["recommendedFeaturedRelations"])
		assert.Equal(t, strList("status"), snapshot.Details.Fields["recommendedRelations"])
	})
}

func TestTypePropertiesRoundTrip(t *testing.T) {
	t.Run("export import export is byte-stable", func(t *testing.T) {
		// given
		opts := Options{ResolveProperties: newTestPropertyResolver(), GenerateId: seqIds("id")}
		first, err := Marshal(model.SmartBlockType_STType, typeSnapshot(), opts)
		require.NoError(t, err)

		// when
		sbType, snapshot, err := Unmarshal(first, opts)
		require.NoError(t, err)
		second, err := Marshal(sbType, snapshot, opts)

		// then
		require.NoError(t, err)
		assert.Equal(t, string(first), string(second))
	})

	t.Run("all-empty lists are byte-stable", func(t *testing.T) {
		// given: the regression case — a type with zero recommended entries
		opts := Options{ResolveProperties: newTestPropertyResolver(), GenerateId: seqIds("id")}
		snapshot := typeSnapshot()
		for _, key := range []string{"recommendedFeaturedRelations", "recommendedRelations",
			"recommendedFileRelations", "recommendedHiddenRelations"} {
			snapshot.Details.Fields[key] = strList()
		}
		first, err := Marshal(model.SmartBlockType_STType, snapshot, opts)
		require.NoError(t, err)

		// when
		sbType, reimported, err := Unmarshal(first, opts)
		require.NoError(t, err)
		second, err := Marshal(sbType, reimported, opts)

		// then
		require.NoError(t, err)
		assert.Equal(t, string(first), string(second))
		assert.Equal(t, strList(), reimported.Details.Fields["recommendedRelations"])
	})
}

func TestTypePropertiesValidation(t *testing.T) {
	t.Run("rejected outside type documents", func(t *testing.T) {
		// given
		doc := `{"version": 1, "typeProperties": [{"key": "due_date"}]}`

		// when
		err := Validate([]byte(doc))

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), `kind "objectType"`)
	})

	t.Run("rejected alongside raw recommended lists", func(t *testing.T) {
		// given
		doc := `{
  "version": 1,
  "kind": "objectType",
  "properties": { "recommendedRelations": ["relid-status"] },
  "typeProperties": [{"key": "due_date"}]
}`

		// when
		err := Validate([]byte(doc))

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "conflicts with typeProperties")
	})

	t.Run("unknown section and missing key rejected by schema", func(t *testing.T) {
		for _, doc := range []string{
			`{"version": 1, "kind": "objectType", "typeProperties": [{"key": "a", "section": "sidebar"}]}`,
			`{"version": 1, "kind": "objectType", "typeProperties": [{"name": "No key"}]}`,
			`{"version": 1, "kind": "objectType", "typeProperties": [{"key": "a", "format": "status"}]}`,
		} {
			assert.Error(t, Validate([]byte(doc)), strings.ReplaceAll(doc, "\n", " "))
		}
	})

	t.Run("valid type document passes", func(t *testing.T) {
		doc := `{
  "version": 1,
  "kind": "objectType",
  "key": "task",
  "typeProperties": [
    { "key": "due_date", "name": "Due date", "format": "date", "section": "featured" }
  ]
}`
		assert.NoError(t, Validate([]byte(doc)))
	})
}
