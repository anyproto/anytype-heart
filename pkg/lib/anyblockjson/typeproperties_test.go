package anyblockjson

import (
	"encoding/json"
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
	t.Run("recommended lists become property_definitions in section order", func(t *testing.T) {
		// given
		opts := Options{ResolveProperties: newTestPropertyResolver()}
		want := `"type_settings": {
    "property_definitions": [
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
    ]
  }`

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
		assert.NotContains(t, string(data), "type_settings")
		assert.Contains(t, string(data), "recommended_featured_relations")
		assert.Contains(t, string(data), "relid-dueDate")
	})

	t.Run("non-type documents never emit type_settings", func(t *testing.T) {
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
		assert.NotContains(t, string(data), "type_settings")
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
		assert.Contains(t, string(data), `"property_definitions": []`)
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

	t.Run("a lifted recommended id is spelled out, not labelled", func(t *testing.T) {
		// given: an id long enough that the old refs labeller would have
		// compacted it, had it collected the lifted lists at all
		snapshot := typeSnapshot()
		snapshot.Details.Fields["recommendedRelations"] = strList("relid-status")

		// when
		data, err := Marshal(model.SmartBlockType_STType, snapshot, Options{
			ResolveProperties: newTestPropertyResolver(),
			CompactIds:        true,
		})

		// then — a POSITIVE statement about what is there: an absence
		// assertion on a legend that no longer exists would hold no matter
		// what the export did
		require.NoError(t, err)
		assert.Contains(t, string(data), `"key": "status"`,
			"the lifted list resolves to a type_properties entry")
		assert.NotContains(t, string(data), `"relid-status"`,
			"the raw id is consumed by the lift, not carried as a label")
	})
}

func TestTypePropertiesImport(t *testing.T) {
	docJSON := `{
  "version": 1,
  "kind": "object_type",
  "key": "task",
  "properties": { "name": "Task" },
  "type_settings": {"property_definitions": [
    { "key": "due_date", "name": "Due date", "format": "date", "section": "featured" },
    { "key": "status", "name": "Status", "format": "select" },
    { "key": "origin", "section": "hidden" }
  ]}
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
		doc := `{"version": 1, "kind": "object_type", "key": "task", "type_settings": {"property_definitions": []}}`

		// when
		_, snapshot, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("id")})

		// then
		require.NoError(t, err)
		for _, key := range []string{"recommendedFeaturedRelations", "recommendedRelations",
			"recommendedFileRelations", "recommendedHiddenRelations"} {
			assert.Equal(t, strList(), snapshot.Details.Fields[key], key)
		}
	})

	t.Run("absent type_properties leaves the lists absent", func(t *testing.T) {
		// given
		doc := `{"version": 1, "kind": "object_type", "key": "task"}`

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
		doc := `{"version": 1, "type_settings": {"property_definitions": [{"key": "due_date"}]}}`

		// when
		err := Validate([]byte(doc))

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), `kind "object_type"`)
	})

	t.Run("rejected alongside raw recommended lists", func(t *testing.T) {
		// given
		doc := `{
  "version": 1,
  "kind": "object_type",
  "properties": { "recommendedRelations": ["relid-status"] },
  "type_settings": {"property_definitions": [{"key": "due_date"}]}
}`

		// when
		err := Validate([]byte(doc))

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "conflicts with type_settings.property_definitions")
	})

	t.Run("unknown section and missing key rejected by schema", func(t *testing.T) {
		for _, doc := range []string{
			`{"version": 1, "kind": "object_type", "type_settings": {"property_definitions": [{"key": "a", "section": "sidebar"}]}}`,
			`{"version": 1, "kind": "object_type", "type_settings": {"property_definitions": [{"name": "No key"}]}}`,
			`{"version": 1, "kind": "object_type", "type_settings": {"property_definitions": [{"key": "a", "format": "status"}]}}`,
		} {
			assert.Error(t, Validate([]byte(doc)), strings.ReplaceAll(doc, "\n", " "))
		}
	})

	t.Run("valid type document passes", func(t *testing.T) {
		doc := `{
  "version": 1,
  "kind": "object_type",
  "key": "task",
  "type_settings": {"property_definitions": [
    { "key": "due_date", "name": "Due date", "format": "date", "section": "featured" }
  ]}
}`
		assert.NoError(t, Validate([]byte(doc)))
	})
}

// BuildRecommendedLists is the PATCH-types channel for the same §2a array a
// type document carries, so it must refuse what the document path refuses —
// on the same resolved keys, with the same JSON pointers. It refused nothing:
// a vocabulary answering "" for a spelling (a stale slug index, a hand-rolled
// KeyVocabulary — Options.Keys is a public interface) wrote the empty key
// straight into a type's recommended lists, where it names nothing, is
// invisible in every UI and disappears on the next export. The document path
// has refused the type half of exactly this since the seam was written; the
// property half was unrefused on BOTH paths.
func TestBuildRecommendedListsRefusesUnwritableResolvedKeys(t *testing.T) {
	t.Run("a property key the vocabulary resolves onto nothing", func(t *testing.T) {
		// given — PropertyKey("blank") answers ("", true)
		props := []TypeProperty{{Key: "blank", Name: "Blank", Section: "featured"}}

		// when
		lists, err := BuildRecommendedLists(props, Options{Keys: blankKeyVocab{}})

		// then
		require.Error(t, err, `recommendedFeaturedRelations: [""] names nothing`)
		assert.Nil(t, lists)
		var ve *ValidationError
		require.ErrorAs(t, err, &ve, "the refusal is path-addressed, as on the document path")
		assert.Contains(t, err.Error(), "/type_settings/property_definitions/0/key")
	})

	t.Run("an object_types entry the vocabulary resolves onto nothing", func(t *testing.T) {
		// given — TypeKey("blanktype") answers ("", true); the good sibling
		// stands first so a fix that merely skipped the bad entry is caught
		props := []TypeProperty{{
			Key:         "assignee",
			ObjectTypes: []string{"page", "blanktype"},
			Section:     "featured",
		}}

		// when
		lists, err := BuildRecommendedLists(props, Options{
			Keys:              blankTypeVocab{},
			ResolveProperties: &recordingPropertyResolver{},
		})

		// then
		require.Error(t, err)
		assert.Nil(t, lists)
		assert.Contains(t, err.Error(), "/type_settings/property_definitions/0/object_types/1")
	})

	t.Run("the refusal does not depend on a resolver being wired", func(t *testing.T) {
		// object_types used to be resolved only inside the resolver branch, so
		// the same input got two different verdicts depending on the caller's
		// wiring — while applyTypeProperties refuses unconditionally
		props := []TypeProperty{{Key: "assignee", ObjectTypes: []string{"blanktype"}}}

		_, err := BuildRecommendedLists(props, Options{Keys: blankTypeVocab{}})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "/type_settings/property_definitions/0/object_types/0")
	})

	t.Run("a resolvable array still builds", func(t *testing.T) {
		props := []TypeProperty{{Key: "due_date", ObjectTypes: []string{"page"}, Section: "featured"}}

		lists, err := BuildRecommendedLists(props, Options{Keys: blankTypeVocab{}})

		require.NoError(t, err)
		require.NotEmpty(t, lists)
		assert.Equal(t, []string{"dueDate"}, lists[0].Ids)
	})
}

// The document path's own property half, which had the same gap: the schema
// bounds the SPELLING (type_properties[].key is minLength 1), but only the
// RESOLVED key can be judged, and a wider vocabulary resolves past the schema.
// The type half of this entry has been refused since the seam was written
// (TestImport_SeamRefusesAnEmptyResolvedTypeKey), three lines below.
func TestImport_TypePropertyKeyRefusesAnUnwritableResolvedKey(t *testing.T) {
	doc := `{"version": 1, "kind": "object_type", "id": "t1", "key": "k",
		"type_settings": {"property_definitions": [{"key": "blank", "format": "text"}]}}`
	require.NoError(t, Validate([]byte(doc)),
		"the document's own chain resolves blank verbatim — Validate cannot see the vocabulary")

	_, _, err := Unmarshal([]byte(doc),
		Options{GenerateId: seqIds("g"), Keys: blankKeyVocab{}, ResolveProperties: &recordingPropertyResolver{}})

	require.Error(t, err)
	var ve *ValidationError
	require.ErrorAs(t, err, &ve, "the refusal is path-addressed")
	assert.Contains(t, err.Error(), "/type_settings/property_definitions/0/key")
}

// The two doors into the §2a array must also agree on what a DECLARED FORMAT
// means. `text` is one name for two stored formats, and §3 makes it resolve
// per key: a key already known to be shorttext — every bundled `name`,
// `icon_emoji`, `cover_id`, and whatever the wiring's ResolveFormat
// recognizes — keeps that format, and only an unknown key becomes longtext.
// The document door ran that rule (declaredFormat); the PATCH door read the
// name literally, so the same array minted the bundled `name` property as
// longtext through one endpoint and left it shorttext through the other.
func TestTypePropertyFormatIsTheSameThroughBothDoors(t *testing.T) {
	viaDocument := func(t *testing.T, opts Options, tp TypeProperty) []PropertyDefinition {
		t.Helper()
		r := &recordingPropertyResolver{}
		opts.ResolveProperties = r
		opts.GenerateId = seqIds("g")
		entry := &omap{}
		entry.set("key", tp.Key)
		entry.setNonEmpty("name", tp.Name)
		entry.setNonEmpty("format", tp.Format)
		entry.setNonEmpty("section", tp.Section)
		raw, err := json.Marshal(entry)
		require.NoError(t, err)
		doc := `{"version": 1, "kind": "object_type", "id": "t1", "key": "k",
			"type_settings": {"property_definitions": [` + string(raw) + `]}}`
		_, _, err = Unmarshal([]byte(doc), opts)
		require.NoError(t, err)
		return r.defs
	}
	viaPatch := func(t *testing.T, opts Options, tp TypeProperty) []PropertyDefinition {
		t.Helper()
		r := &recordingPropertyResolver{}
		opts.ResolveProperties = r
		_, err := BuildRecommendedLists([]TypeProperty{tp}, opts)
		require.NoError(t, err)
		return r.defs
	}

	cases := map[string]struct {
		opts Options
		tp   TypeProperty
		want model.RelationFormat
	}{
		"a bundled short-text key keeps its stored format": {
			tp:   TypeProperty{Key: "name", Name: "Name", Format: "text", Section: "featured"},
			want: model.RelationFormat_shorttext,
		},
		"a key the wiring resolves as short text keeps it too": {
			opts: Options{ResolveFormat: func(key domain.RelationKey) (model.RelationFormat, bool) {
				return model.RelationFormat_shorttext, key == "headline"
			}},
			tp:   TypeProperty{Key: "headline", Name: "Headline", Format: "text"},
			want: model.RelationFormat_shorttext,
		},
		"a new key declared as text is long text": {
			tp:   TypeProperty{Key: "summary", Name: "Summary", Format: "text"},
			want: model.RelationFormat_longtext,
		},
		"any other name is taken literally": {
			tp:   TypeProperty{Key: "name", Name: "Name", Format: "number"},
			want: model.RelationFormat_number,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			// when
			doorA := viaDocument(t, tc.opts, tc.tp)
			doorB := viaPatch(t, tc.opts, tc.tp)

			// then
			require.Len(t, doorA, 1)
			require.Len(t, doorB, 1)
			assert.Equal(t, tc.want, doorA[0].Format, "the document door")
			assert.Equal(t, tc.want, doorB[0].Format, "the PATCH door")
			assert.Equal(t, doorA[0], doorB[0], "one array, one meaning")
		})
	}
}

// §13: a path addresses the fault. A dropped §2a entry has no index in the
// document — it is not there — and the warning used to carry
// `/type_settings/property_definitions/<len(out)>`, which is the index the next SURVIVING entry
// takes. So the diagnostic for the broken entry pointed at a healthy one, and
// a caller that trusted the pointer read the wrong property.
func TestExport_ADroppedTypePropertyIsReportedAtTheArray(t *testing.T) {
	// given: k1's definition has no key at all (a vocabulary bug's residue,
	// the shape real type objects hold), k2's is healthy
	snapshot := &model.SmartBlockSnapshotBase{
		Details: fields(map[string]*types.Value{
			"id":                           str("t1"),
			"recommendedFeaturedRelations": strList("k1", "k2"),
		}),
		ObjectTypes: []string{"ot-objectType"},
	}
	var warned []Issue

	// when
	data, err := Marshal(model.SmartBlockType_STType, snapshot, Options{
		ResolveProperties: censusPropResolver{},
		OnWarning:         func(i Issue) { warned = append(warned, i) },
	})
	require.NoError(t, err)

	// then
	doc := decodeEnvelope(t, data)
	require.Len(t, doc.TypeProps(), 1)
	assert.Equal(t, "owner", doc.TypeProps()[0].Key, "entry 0 of the document is the HEALTHY one")
	require.Len(t, warned, 1)
	assert.Equal(t, typePropertyDefinitionsPath, warned[0].Path,
		"the array is the fault's address; the dropped entry has no index in it")
	assert.Contains(t, warned[0].Message, "is dropped",
		"and the message names the key, which is what identifies it")
}
