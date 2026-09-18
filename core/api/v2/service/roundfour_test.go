package v2service

import (
	"context"
	"net/http"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/storeresolver"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// Round-four eval (APIV2_ROUND4_SCENARIOS.md) R4-1 and R4-2: a deleted type
// and a delete receipt spell a type the way every read does.

const gadgetTypeKey = "6aad7fbf61fab205fe53c2f0"

func (fx *v2Fixture) addGadgetType(t *testing.T, removed bool) {
	obj := objectstore.TestObject{
		bundle.RelationKeyId:           domain.String("type-gadget"),
		bundle.RelationKeyUniqueKey:    domain.String("ot-" + gadgetTypeKey),
		bundle.RelationKeyApiObjectKey: domain.String("gadget"),
		bundle.RelationKeyName:         domain.String("Gadget"),
	}
	if removed {
		obj[bundle.RelationKeyIsArchived] = domain.Bool(true)
	}
	fx.addType(t, testSpaceId, obj)
}

func gadgetRead() apicore.ObjectRead {
	return apicore.ObjectRead{
		SbType: model.SmartBlockType_Page,
		Snapshot: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				"id":   pbtypes.String(deleteObjId),
				"name": pbtypes.String("Thing one"),
			}},
			ObjectTypes: []string{"ot-" + gadgetTypeKey},
			Blocks:      []*model.Block{{Id: deleteObjId, Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}},
		},
		Heads: []string{"headA"},
	}
}

func TestV2DeletedTypeKeepsItsSpelling(t *testing.T) {
	ctx := context.Background()

	t.Run("delete_type says which objects survive it, on the dry run too", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		fx.addTaskType(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{bundle.RelationKeyId: domain.String("c1"), bundle.RelationKeyType: domain.String("type-chore"), bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic))},
			{bundle.RelationKeyId: domain.String("c2"), bundle.RelationKeyType: domain.String("type-chore"), bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic))},
		})

		result, err := fx.DeleteType(ctx, testSpaceId, "chore", true)

		require.NoError(t, err)
		require.Len(t, result.Warnings, 1)
		assert.Contains(t, result.Warnings[0].Message, `2 objects are of type "chore"`)
		assert.Contains(t, result.Warnings[0].Message, `reads still spell it "chore"`)
		assert.Equal(t, []v2model.Ref{v2model.RefListObjects(testSpaceId).With("type", "chore")}, result.Warnings[0].SeeAlso)

		fx.mwMock.EXPECT().ObjectSetIsArchived(mock.Anything, &pb.RpcObjectSetIsArchivedRequest{ContextId: "type-chore", IsArchived: true}).
			Return(&pb.RpcObjectSetIsArchivedResponse{Error: &pb.RpcObjectSetIsArchivedResponseError{Code: pb.RpcObjectSetIsArchivedResponseError_NULL}})
		real, err := fx.DeleteType(ctx, testSpaceId, "chore", false)
		require.NoError(t, err)
		require.Len(t, real.Warnings, 1)
	})

	t.Run("a type with no objects deletes without a warning", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		fx.addTaskType(t)

		result, err := fx.DeleteType(ctx, testSpaceId, "chore", true)

		require.NoError(t, err)
		assert.Empty(t, result.Warnings)
	})

	t.Run("a removed type's objects read and list under its slug, not a hex", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addGadgetType(t, true)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, deleteObjId).Return(gadgetRead(), nil)

		body, _, err := fx.GetObject(ctx, testSpaceId, deleteObjId, ObjectQuery{})

		require.NoError(t, err)
		assert.Equal(t, "gadget", decodeBody(t, body)["type"])
		v := fx.apiKeys(testSpaceId, storeresolver.New(fx.store.SpaceIndex(testSpaceId)))
		assert.Equal(t, "gadget", v.TypeSlug(gadgetTypeKey))
		_, understood := v.TypeKey("gadget")
		assert.False(t, understood, "emit only: the slug is not an address for a create")
	})

	t.Run("a live type that took the slug keeps it; the corpse reads under its key", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addGadgetType(t, true)
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:           domain.String("type-gadget-2"),
			bundle.RelationKeyUniqueKey:    domain.String("ot-6aad7fbf61fab205fe53c2f9"),
			bundle.RelationKeyApiObjectKey: domain.String("gadget"),
			bundle.RelationKeyName:         domain.String("Gadget again"),
		})

		v := fx.apiKeys(testSpaceId, storeresolver.New(fx.store.SpaceIndex(testSpaceId)))

		assert.Equal(t, "gadget", v.TypeSlug("6aad7fbf61fab205fe53c2f9"))
		assert.Equal(t, gadgetTypeKey, v.TypeSlug(gadgetTypeKey))
	})

	t.Run("filtering or creating by a removed type's slug is refused as removed, not unknown", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addGadgetType(t, true)

		_, _, _, _, err := fx.SearchObjects(ctx, testSpaceId, v2model.SearchRequest{Type: "gadget"}, 0, 25)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		require.NotEmpty(t, apiErr.Issues)
		assert.Contains(t, apiErr.Issues[0].Message, `type "gadget" was removed from this space`)
		assert.NotContains(t, apiErr.Issues[0].Message, gadgetTypeKey)
		assert.Equal(t, []v2model.Ref{v2model.RefListTypes(testSpaceId)}, apiErr.Issues[0].SeeAlso)

		_, err = fx.CreateObject(ctx, testSpaceId, []byte(`{"formatVersion":"2.0","type":"gadget","properties":{"name":"n"}}`), true, true)
		assert.Contains(t, v2Err(t, err).Issues[0].Message, "was removed from this space")
	})

	t.Run("delete_object's receipt spells a custom type as reads do", func(t *testing.T) {
		for _, removed := range []bool{false, true} {
			fx := newV2Fixture(t)
			fx.addGadgetType(t, removed)
			fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, deleteObjId).Return(gadgetRead(), nil).Maybe()
			fx.provenanceMock.EXPECT().CreatorProvenance(mock.Anything, testSpaceId, deleteObjId).Return(true, "Claude Desktop", nil).Maybe()
			fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
				bundle.RelationKeyId:             domain.String(deleteObjId),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
			}})

			result, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, true)

			require.NoError(t, err)
			assert.Equal(t, "gadget", result.Type, "removed=%v", removed)
		}
	})
}
