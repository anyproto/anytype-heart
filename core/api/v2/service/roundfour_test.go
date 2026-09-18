package v2service

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/api/util"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/storeresolver"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
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
		assert.Empty(t, result.Warnings[0].SeeAlso, "list_objects has no type filter — no reference promises one")
		assert.Contains(t, result.Warnings[0].Hint, "a dry run reports this without deleting")

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

	t.Run("the warning spells the served slug however the type was addressed, and says when a twin will demote it", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addGadgetType(t, false)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{bundle.RelationKeyId: domain.String("g1"), bundle.RelationKeyType: domain.String("type-gadget"), bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic))},
		})

		result, err := fx.DeleteType(ctx, testSpaceId, "Gadget", true) // by display name

		require.NoError(t, err)
		require.Len(t, result.Warnings, 1)
		assert.Contains(t, result.Warnings[0].Message, `1 object is of type "gadget"; they keep it, and reads still spell it "gadget"`)
		assert.NotContains(t, result.Warnings[0].Message, gadgetTypeKey)

		// a removed type already answers to the slug: after this delete both
		// read under their stored keys, and the warning says so
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:           domain.String("type-gadget-old"),
			bundle.RelationKeyUniqueKey:    domain.String("ot-6aad7fbf61fab205fe53c2f1"),
			bundle.RelationKeyApiObjectKey: domain.String("gadget"),
			bundle.RelationKeyName:         domain.String("Gadget (old)"),
			bundle.RelationKeyIsArchived:   domain.Bool(true),
		})
		result, err = fx.DeleteType(ctx, testSpaceId, "gadget", true)
		require.NoError(t, err)
		require.Len(t, result.Warnings, 1)
		assert.Contains(t, result.Warnings[0].Message, `another removed type used "gadget" before, so after this delete the objects of both read under their stored keys ("`+gadgetTypeKey+`" here)`)
	})

	t.Run("a removed slug never folds onto a live type NAMED that way", func(t *testing.T) {
		// the blocker: removed "gadget", live "machine" named "gadget" — a
		// search, a create and a delete by "gadget" went to machine
		fx := newV2Fixture(t)
		fx.addGadgetType(t, true)
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:           domain.String("type-machine"),
			bundle.RelationKeyUniqueKey:    domain.String("ot-6aad7fbf61fab205fe53c2f2"),
			bundle.RelationKeyApiObjectKey: domain.String("machine"),
			bundle.RelationKeyName:         domain.String("gadget"),
		})

		_, _, _, _, err := fx.SearchObjects(ctx, testSpaceId, v2model.SearchRequest{Type: "gadget"}, 0, 25)
		assert.Contains(t, v2Err(t, err).Issues[0].Message, "was removed from this space")

		_, err = fx.CreateObject(ctx, testSpaceId, []byte(`{"formatVersion":"2.0","type":"gadget","properties":{"name":"n"}}`), true, true)
		assert.Contains(t, v2Err(t, err).Issues[0].Message, "was removed from this space")

		_, err = fx.DeleteType(ctx, testSpaceId, "gadget", true)
		requireNotFoundError(t, err)
		assert.Contains(t, err.Error(), "it was removed")

		// the live namesake stays reachable by its own slug and its name
		// spelled with a different case is still its own
		entries, _ := fx.liveTypes(testSpaceId)
		entry, ok, _ := fx.resolveTypeInput(testSpaceId, "machine", entries)
		assert.True(t, ok)
		assert.Equal(t, "6aad7fbf61fab205fe53c2f2", entry.Key)
	})

	t.Run("a tombstoned type is refused as removed by its key and by its slug", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addTypeTombstone(t, "drv-ot-"+gadgetTypeKey, gadgetTypeKey, "gadget")

		for _, spelling := range []string{gadgetTypeKey, "gadget"} {
			_, _, _, _, err := fx.SearchObjects(ctx, testSpaceId, v2model.SearchRequest{Type: spelling}, 0, 25)
			assert.Contains(t, v2Err(t, err).Issues[0].Message, "was removed from this space", spelling)
		}
	})

	t.Run("addressing the old type by its key when a live type took the slug names both", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addGadgetType(t, true)
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:           domain.String("type-gadget-2"),
			bundle.RelationKeyUniqueKey:    domain.String("ot-6aad7fbf61fab205fe53c2f9"),
			bundle.RelationKeyApiObjectKey: domain.String("gadget"),
			bundle.RelationKeyName:         domain.String("Gadget again"),
		})

		_, _, _, _, err := fx.SearchObjects(ctx, testSpaceId, v2model.SearchRequest{Type: gadgetTypeKey}, 0, 25)

		msg := v2Err(t, err).Issues[0].Message
		assert.Contains(t, msg, `(formerly "gadget") was removed`)
		assert.Contains(t, msg, `"gadget" now names a different type`)
	})

	t.Run("GET types by a removed type's slug is a 404 that says removed, with live candidates", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addGadgetType(t, true)
		fx.addTaskType(t)

		_, _, err := fx.GetType(ctx, testSpaceId, "gadget", ObjectQuery{})

		apiErr := requireNotFoundError(t, err)
		assert.Contains(t, apiErr.Message, "it was removed")
		assert.Contains(t, apiErr.Message, "chore")
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, []v2model.Ref{v2model.RefListTypes(testSpaceId)}, apiErr.Issues[0].SeeAlso)
	})

	t.Run("a read of an object whose type was removed says so, and the markdown envelope spells the slug", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addGadgetType(t, true)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, deleteObjId).Return(gadgetRead(), nil)

		body, _, err := fx.GetObject(ctx, testSpaceId, deleteObjId, ObjectQuery{})

		require.NoError(t, err)
		doc := decodeBody(t, body)
		warnings, _ := doc["warnings"].([]any)
		require.Len(t, warnings, 1, "%v", doc)
		warning, _ := warnings[0].(map[string]any)
		assert.Equal(t, "/type", warning["path"])
		assert.Contains(t, warning["message"], `the type "gadget" of this object was removed from the space`)

		fx2 := newV2Fixture(t)
		fx2.addGadgetType(t, true)
		fx2.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, deleteObjId).Return(gadgetRead(), nil)
		fx2.mwMock.EXPECT().ObjectExport(mock.Anything, mock.Anything).Return(&pb.RpcObjectExportResponse{Result: "# Thing\n"})
		body, _, err = fx2.GetObject(ctx, testSpaceId, deleteObjId, ObjectQuery{Format: "md"})
		require.NoError(t, err)
		assert.Equal(t, "gadget", decodeBody(t, body)["type"])
	})

	t.Run("a tombstone whose slug a query-visible corpse already spells reads under its key in rows", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addGadgetType(t, true) // query-visible corpse, slug gadget
		fx.addTypeTombstone(t, "type-gadget-ts", "6aad7fbf61fab205fe53c2f3", "gadget")
		builder, err := fx.newObjectRowBuilder(testSpaceId, nil)
		require.NoError(t, err)

		corpse := domain.NewDetails()
		corpse.SetString(bundle.RelationKeyId, "o1")
		corpse.SetString(bundle.RelationKeyType, "type-gadget")
		tomb := domain.NewDetails()
		tomb.SetString(bundle.RelationKeyId, "o2")
		tomb.SetString(bundle.RelationKeyType, "type-gadget-ts")

		assert.Equal(t, "gadget", builder.row(database.Record{Details: corpse}).Type)
		assert.Equal(t, "6aad7fbf61fab205fe53c2f3", builder.row(database.Record{Details: tomb}).Type, "one address, one holder")
	})

	t.Run("a tombstone demoted behind a visible corpse still reads as removed", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addGadgetType(t, true) // visible corpse, slug gadget
		fx.addTypeTombstone(t, "drv-ot-6aad7fbf61fab205fe53c2f3", "6aad7fbf61fab205fe53c2f3", "gadget")

		v := fx.apiKeys(testSpaceId, storeresolver.New(fx.store.SpaceIndex(testSpaceId)))

		assert.Equal(t, "6aad7fbf61fab205fe53c2f3", v.TypeSlug("6aad7fbf61fab205fe53c2f3"), "one address, one holder")
		assert.True(t, v.TypeRemoved("6aad7fbf61fab205fe53c2f3"), "the marker does not depend on the spelling")
	})

	t.Run("a tombstone is found by its slug through the indexed marker, however many other rows were deleted", func(t *testing.T) {
		fx := newV2Fixture(t)
		for i := 0; i < 20; i++ {
			fx.addTombstone(t, fmt.Sprintf("gone-%02d", i)) // ordinary deleted objects
		}
		fx.addTypeTombstone(t, "drv-ot-"+gadgetTypeKey, gadgetTypeKey, "gadget")

		entry, removed, err := fx.removedTypeBySpelling(testSpaceId, "gadget")

		require.NoError(t, err)
		require.True(t, removed)
		assert.Equal(t, gadgetTypeKey, entry.Key)

		// a bare tombstone (no marker, no snapshot) is not a type
		_, removed, err = fx.removedTypeBySpelling(testSpaceId, "gone-01")
		require.NoError(t, err)
		assert.False(t, removed)
	})

	t.Run("the markdown envelope carries the removed-type warning too", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addGadgetType(t, true)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, deleteObjId).Return(gadgetRead(), nil)
		fx.mwMock.EXPECT().ObjectExport(mock.Anything, mock.Anything).Return(&pb.RpcObjectExportResponse{Result: "# Thing\n"})

		body, _, err := fx.GetObject(ctx, testSpaceId, deleteObjId, ObjectQuery{Format: "md"})

		require.NoError(t, err)
		doc := decodeBody(t, body)
		warnings, _ := doc["warnings"].([]any)
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0].(map[string]any)["message"], "searches cannot filter by it")
	})

	t.Run("a non-bson custom key reaches the tombstone probe too", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addTypeTombstone(t, "drv-ot-customNote", "customNote", "custom_note")

		v := fx.apiKeys(testSpaceId, storeresolver.New(fx.store.SpaceIndex(testSpaceId)))

		assert.Equal(t, "custom_note", v.TypeSlug("customNote"))
		assert.True(t, v.TypeRemoved("customNote"))
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

// Round-four group F: R4-3 / R4-9 (message counts and order), R4-8 (the
// read receipt), R4-4 / R4-5 (whoami), R4-7 / R4-10 (update_space).
func TestV2RoundFourChatAndIdentity(t *testing.T) {
	ctx := context.Background()

	t.Run("messages are served ascending whichever way the RPC handed them over", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		newer, older := chatProtoMessage(), chatProtoMessage()
		newer.Id, newer.OrderId = "m2", "0002"
		older.Id, older.OrderId = "m1", "0001"
		fx.mwMock.EXPECT().ChatGetMessages(mock.Anything, mock.Anything).Return(&pb.RpcChatGetMessagesResponse{
			Messages: []*model.ChatMessage{newer, older}, MessageCount: 2, LifetimeMessageCount: 5,
		})

		got, err := fx.GetChatMessages(ctx, testSpaceId, testChatId, ChatMessagesQuery{Limit: 25})

		require.NoError(t, err)
		require.Len(t, got.Messages, 2)
		assert.Equal(t, "0001", got.Messages[0].Order)
		assert.Equal(t, "0002", got.Messages[1].Order)
		assert.Equal(t, 2, got.MessageCount)
		assert.Equal(t, 5, got.LifetimeMessageCount)
	})

	t.Run("read_chat's receipt carries the chat's state after the move", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		fx.mwMock.EXPECT().ChatReadMessages(mock.Anything, mock.Anything).Return(&pb.RpcChatReadMessagesResponse{})
		fx.mwMock.EXPECT().ChatGetMessages(mock.Anything, mock.MatchedBy(func(req *pb.RpcChatGetMessagesRequest) bool {
			return req.ChatObjectId == testChatId && req.Limit == 1
		})).Return(&pb.RpcChatGetMessagesResponse{ChatState: &model.ChatState{
			Messages: &model.ChatStateUnreadState{Counter: 0}, Mentions: &model.ChatStateUnreadState{Counter: 0}, LastStateId: "state43",
		}})

		got, err := fx.ReadChat(ctx, testSpaceId, testChatId, v2model.ChatReadRequest{UpTo: "00a5", LastStateId: "state42"}, false)

		require.NoError(t, err)
		require.NotNil(t, got.State, "a write nothing else made observable now says what it moved")
		assert.Equal(t, 0, got.State.UnreadMessages)
		assert.Equal(t, "state43", got.State.LastStateId)
	})

	t.Run("the reactions scope receipt carries the state too", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		fx.mwMock.EXPECT().ChatReadReactions(mock.Anything, mock.Anything).Return(&pb.RpcChatReadReactionsResponse{})
		fx.mwMock.EXPECT().ChatGetMessages(mock.Anything, mock.Anything).Return(&pb.RpcChatGetMessagesResponse{ChatState: &model.ChatState{LastStateId: "state44"}})

		got, err := fx.ReadChat(ctx, testSpaceId, testChatId, v2model.ChatReadRequest{Scope: v2model.ChatReadScopeReactions}, false)

		require.NoError(t, err)
		require.NotNil(t, got.State)
		assert.Equal(t, "state44", got.State.LastStateId)
	})

	t.Run("a restricted grant's list is its boundary: the spaces flag changes nothing", func(t *testing.T) {
		fx := newV2FixtureBare(t)
		fx.registerNamedSpace(t, "spaceA", "Work")
		fx.registerNamedSpace(t, "spaceB", "Personal")
		grant := &util.ApiGrant{Spaces: []string{"spaceA"}, Perms: util.GrantPermsRead}

		got, err := fx.Whoami(whoamiCtx(util.ApiKeyInfo{Id: "h", Name: "k"}, grant), true)

		require.NoError(t, err)
		assert.True(t, got.Grant.Restricted)
		assert.Nil(t, got.Grant.SpaceCount)
		require.Len(t, got.Grant.Spaces, 1)
		assert.Equal(t, "spaceA", got.Grant.Spaces[0].Id)
	})

	t.Run("an empty space update names what is not writable here", func(t *testing.T) {
		fx := newV2FixtureBare(t)
		fx.registerNamedSpace(t, "spaceS", "Work")

		_, err := fx.UpdateSpace(ctx, "spaceS", v2model.UpdateSpaceRequest{}, true)

		apiErr := v2Err(t, err)
		require.NotEmpty(t, apiErr.Issues)
		assert.Contains(t, apiErr.Issues[0].Message, "the space icon and the default object type are not writable through this API")
	})
}
