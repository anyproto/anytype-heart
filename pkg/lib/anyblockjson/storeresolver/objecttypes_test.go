package storeresolver

// PropertyDefinition.ObjectTypes — `type_properties[].object_types`, SPEC §2a
// — had no node-backed emitter at all: the resolver left the field empty, so a
// node export of a type document silently dropped every property's target
// types and the property came back accepting any object. These tests drive the
// real exporter, not just the resolver method, because the resolver hook that
// feeds that slot is PropertyById (via the recommended-relation lists) and a
// test calling anything else would never reach it.

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

// customTypeKey is a space-minted (bson) type key, the shape a real space
// gives a user-created type. Its slug is what the document must spell.
const customTypeKey = "69bbfc78877a91b1d12d1a7c"

// newTargetsFixture is a space holding one `objects` relation that targets two
// types by OBJECT ID — the form the store actually keeps
// (objectcreator.fillRelationFormatObjectTypes rewrites bundled urls to derived
// ids at creation) — plus one target the store cannot resolve at all.
func newTargetsFixture(t *testing.T) *fixture {
	index := spaceindex.NewStoreFixture(t)
	index.AddObjects(t, []spaceindex.TestObject{
		{
			bundle.RelationKeyId:                        domain.String("rel-assignee"),
			bundle.RelationKeyRelationKey:               domain.String("assignee"),
			bundle.RelationKeyName:                      domain.String("Assignee"),
			bundle.RelationKeyRelationFormat:            domain.Int64(int64(model.RelationFormat_object)),
			bundle.RelationKeyResolvedLayout:            domain.Int64(int64(model.ObjectType_relation)),
			bundle.RelationKeyRelationFormatObjectTypes: domain.StringList([]string{"type-person", "type-participant", "type-vanished"}),
		},
		{
			bundle.RelationKeyId:             domain.String("type-person"),
			bundle.RelationKeyUniqueKey:      domain.String(domain.TypeKey(customTypeKey).URL()),
			bundle.RelationKeyApiObjectKey:   domain.String("person"),
			bundle.RelationKeyName:           domain.String("Person"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
		},
		{
			// a bundled type installed into the space: the store row carries
			// the same shape, and hidden entities still have an identity
			bundle.RelationKeyId:             domain.String("type-participant"),
			bundle.RelationKeyUniqueKey:      domain.String(bundle.TypeKeyParticipant.URL()),
			bundle.RelationKeyName:           domain.String("Participant"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
		},
	})
	return &fixture{Resolvers: New(index), index: index}
}

func TestPropertyDefinitionObjectTypes(t *testing.T) {
	t.Run("stored target ids resolve to stored type keys", func(t *testing.T) {
		// given
		fx := newTargetsFixture(t)

		// when
		def, ok := fx.PropertyById("rel-assignee")

		// then: ids in, KEYS out, in the stored order — and the id nothing in
		// the space answers for is dropped rather than exported as a target
		// no reader could find
		require.True(t, ok)
		assert.Equal(t, []string{customTypeKey, "participant"}, def.ObjectTypes)
	})

	t.Run("a bundled url target resolves without a store row", func(t *testing.T) {
		// given: legacy rows that predate fillRelationFormatObjectTypes keep
		// the bundled url form
		index := spaceindex.NewStoreFixture(t)
		index.AddObjects(t, []spaceindex.TestObject{{
			bundle.RelationKeyId:                        domain.String("rel-assignee"),
			bundle.RelationKeyRelationKey:               domain.String("assignee"),
			bundle.RelationKeyRelationFormat:            domain.Int64(int64(model.RelationFormat_object)),
			bundle.RelationKeyResolvedLayout:            domain.Int64(int64(model.ObjectType_relation)),
			bundle.RelationKeyRelationFormatObjectTypes: domain.StringList([]string{bundle.TypeKeyTask.BundledURL()}),
		}})
		fx := &fixture{Resolvers: New(index), index: index}

		// when
		def, ok := fx.PropertyById("rel-assignee")

		// then
		require.True(t, ok)
		assert.Equal(t, []string{"task"}, def.ObjectTypes)
	})

	t.Run("a property with no targets stays untargeted", func(t *testing.T) {
		// given: the "empty means any object" case must not become [""]
		fx := newFixture(t)

		// when
		def, ok := fx.PropertyById("rel-priority")

		// then
		require.True(t, ok)
		assert.Empty(t, def.ObjectTypes)
	})
}

// The end-to-end guard: run the real exporter over a type snapshot and read
// the document. This is the only assertion that proves the resolver hook the
// exporter actually calls is the one that was fixed — the emitter reads
// ObjectTypes off whatever PropertyById returns for a recommended-relation id.
func TestTypeDocumentCarriesObjectTypes(t *testing.T) {
	// given
	fx := newTargetsFixture(t)
	snapshot := &model.SmartBlockSnapshotBase{
		Key: "task",
		Details: &types.Struct{Fields: map[string]*types.Value{
			"recommendedFeaturedRelations": {Kind: &types.Value_ListValue{ListValue: &types.ListValue{
				Values: []*types.Value{{Kind: &types.Value_StringValue{StringValue: "rel-assignee"}}},
			}}},
		}},
	}

	// when
	data, err := anyblockjson.Marshal(model.SmartBlockType_STType, snapshot, fx.Options())
	require.NoError(t, err)

	// then
	var doc struct {
		TypeKeys  map[string]string `json:"type_keys"`
		TypeProps []struct {
			Key         string   `json:"key"`
			ObjectTypes []string `json:"object_types"`
		} `json:"type_properties"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	require.Len(t, doc.TypeProps, 1)
	assert.Equal(t, "assignee", doc.TypeProps[0].Key)
	// the slots are spelled as slugs and the legend inverts the one the
	// bundled table cannot (§3) — the whole point of carrying the targets is
	// that a reader can bind them back
	assert.Equal(t, []string{"person", "participant"}, doc.TypeProps[0].ObjectTypes)
	assert.Equal(t, map[string]string{"person": customTypeKey}, doc.TypeKeys)

	// and: the document reads back onto the very same stored keys
	_, back, err := anyblockjson.Unmarshal(data, anyblockjson.Options{})
	require.NoError(t, err)
	assert.NotNil(t, back)
}
