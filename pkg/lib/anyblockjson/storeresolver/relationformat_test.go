package storeresolver

// relationformat_test.go — the anyblockjson.TypeResolver capability (SPEC
// §2d): the id↔key translation behind a relation document's `object_types`
// envelope field. The codec discovers it by type assertion on
// Options.ResolveProperties, so these tests drive both the methods and the
// real codec through fx.Options().

import (
	"encoding/json"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// TypeKeyById answers for a space row and for a legacy bundled url, and
// misses honestly for an id nothing serves; TypeIdByKey inverts the space
// half. Both must miss rather than invent, because the codec's degradation
// for an unanswered entry is verbatim pass-through — an invented answer
// would translate one direction with nothing to invert it on the other.
//
// How this can fail: fill idByKey from a different listing than keyById and
// the two directions stop being inverses; drop the bundled-url arm and a
// legacy `_ot…` entry stops resolving.
func TestTypeResolver_TranslatesIdsAndKeys(t *testing.T) {
	// given
	fx := newTargetsFixture(t)

	// then: the space row, both directions
	key, ok := fx.TypeKeyById("type-person")
	require.True(t, ok)
	assert.Equal(t, customTypeKey, key)
	id, ok := fx.TypeIdByKey(customTypeKey)
	require.True(t, ok)
	assert.Equal(t, "type-person", id)

	// the legacy bundled-url form, no store row needed
	key, ok = fx.TypeKeyById(bundle.TypeKeyTask.BundledURL())
	require.True(t, ok)
	assert.Equal(t, "task", key)

	// and honest misses
	_, ok = fx.TypeKeyById("type-vanished")
	assert.False(t, ok, "an id nothing serves must miss, not invent")
	_, ok = fx.TypeIdByKey("vanishedKey")
	assert.False(t, ok)
}

// The end-to-end guard, the objecttypes_test shape: a relation object whose
// stored targets are ids exports `object_types` as type keys, and the import
// through the same resolver stores the same ids back — the §2d round trip is
// id-exact under the capability, which is what lets snapshotdiff run with no
// new rule.
//
// How this can fail: remove the TypeResolver methods and the assertion on
// the document shows raw ids; break TypeIdByKey and the re-imported detail
// holds keys where the store wants ids.
func TestRelationDocumentTranslatesTargetTypes(t *testing.T) {
	// given a relation snapshot in the store's own shape: targets by id,
	// plus one id the space no longer serves
	fx := newTargetsFixture(t)
	snapshot := &model.SmartBlockSnapshotBase{
		Key: "assignee",
		Details: &types.Struct{Fields: map[string]*types.Value{
			"id": {Kind: &types.Value_StringValue{StringValue: "rel-assignee"}},
			"relationFormat": {Kind: &types.Value_NumberValue{
				NumberValue: float64(model.RelationFormat_object)}},
			"relationFormatObjectTypes": {Kind: &types.Value_ListValue{ListValue: &types.ListValue{
				Values: []*types.Value{
					{Kind: &types.Value_StringValue{StringValue: "type-person"}},
					{Kind: &types.Value_StringValue{StringValue: "type-vanished"}},
				},
			}}},
		}},
	}

	// when
	data, err := anyblockjson.Marshal(model.SmartBlockType_STRelation, snapshot, fx.Options())
	require.NoError(t, err)
	_, got, err := anyblockjson.Unmarshal(data, fx.Options())
	require.NoError(t, err)

	// then: the resolved key on the wire in its §3 SLUG spelling, with the
	// legend entry that inverts it — and the unresolvable id verbatim, its
	// own address, never dropped
	var doc struct {
		RelationSettings struct {
			Format      string   `json:"format"`
			ObjectTypes []string `json:"object_types"`
		} `json:"relation_settings"`
		TypeKeys map[string]string `json:"type_keys"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	assert.Equal(t, "objects", doc.RelationSettings.Format)
	assert.Equal(t, []string{"person", "type-vanished"}, doc.RelationSettings.ObjectTypes)
	assert.Equal(t, customTypeKey, doc.TypeKeys["person"],
		"the slug owes the legend entry that inverts it (§3)")

	// and ids back in the snapshot
	gotTargets := got.Details.Fields["relationFormatObjectTypes"].GetListValue()
	require.NotNil(t, gotTargets)
	values := make([]string, 0, len(gotTargets.Values))
	for _, v := range gotTargets.Values {
		values = append(values, v.GetStringValue())
	}
	assert.Equal(t, []string{"type-person", "type-vanished"}, values)
}

// A relation document from a space whose type listing is empty still
// round-trips: the capability answers nothing, entries pass through
// verbatim, and nothing errors.
//
// How this can fail: make TypeIdByKey (or TypeKeyById) panic or invent on an
// empty vocabulary, or make the codec require an answer.
func TestRelationDocumentPassThroughOnEmptySpace(t *testing.T) {
	// given a space with no type rows at all
	index := spaceindex.NewStoreFixture(t)
	index.AddObjects(t, []spaceindex.TestObject{{
		bundle.RelationKeyId:             domain.String("rel-assignee"),
		bundle.RelationKeyRelationKey:    domain.String("assignee"),
		bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_object)),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
	}})
	fx := &fixture{Resolvers: New(index), index: index}
	snapshot := &model.SmartBlockSnapshotBase{
		Key: "assignee",
		Details: &types.Struct{Fields: map[string]*types.Value{
			"id": {Kind: &types.Value_StringValue{StringValue: "rel-assignee"}},
			"relationFormat": {Kind: &types.Value_NumberValue{
				NumberValue: float64(model.RelationFormat_object)}},
			"relationFormatObjectTypes": {Kind: &types.Value_ListValue{ListValue: &types.ListValue{
				Values: []*types.Value{{Kind: &types.Value_StringValue{StringValue: "bafyreisometype"}}},
			}}},
		}},
	}

	// when
	data, err := anyblockjson.Marshal(model.SmartBlockType_STRelation, snapshot, fx.Options())
	require.NoError(t, err)
	_, got, err := anyblockjson.Unmarshal(data, fx.Options())

	// then
	require.NoError(t, err)
	assert.Equal(t, "bafyreisometype",
		got.Details.Fields["relationFormatObjectTypes"].GetListValue().Values[0].GetStringValue())
}
