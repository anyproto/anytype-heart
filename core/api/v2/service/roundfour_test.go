package v2service

import (
	"context"
	"encoding/json"
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
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// A deleted type, and a delete receipt, spell a type the way every read does.

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
		entry, ok, _, _ := fx.resolveTypeInput(testSpaceId, "machine", entries)
		assert.True(t, ok)
		assert.Equal(t, "6aad7fbf61fab205fe53c2f2", entry.Key)
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

// Round-four group G: the carried-forward round-three findings.
func TestV2RoundFourCarriedForward(t *testing.T) {
	ctx := context.Background()
	setup := func(t *testing.T) *v2Fixture {
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		fx.addTaskType(t)
		return fx
	}

	t.Run("R3-a: the views ambiguity carries the query schema", func(t *testing.T) {
		fx := setup(t)

		_, err := fx.CreateQuery(ctx, testSpaceId, v2model.CreateQueryRequest{Name: "Q", Type: "chore",
			Views: json.RawMessage(`[{"name":"V"}]`), Sorts: json.RawMessage(`[{"property":"severity"}]`)}, true, true)

		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeAmbiguousInput, apiErr.Code)
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, []v2model.Ref{v2model.RefGetSchema("query")}, apiErr.Issues[0].SeeAlso)
	})

	t.Run("R3-b: a view the type channel minted is spelled as reads spell it", func(t *testing.T) {
		fx := newTypeOpsFixture(t)
		fx.captureTypeDetails()
		fx.expectTypeViewEdit(typeReadWithViews(viewWithColumns("viewAll1", "All", "name")))

		result, err := fx.UpdateType(ctx, testSpaceId, "plant", "", opsBody(`{"op":"insert_view","name":"Board"}`), false, false)

		require.NoError(t, err)
		require.Len(t, result.CreatedViews, 1, "%v", result.CreatedViews)
		for _, id := range result.CreatedViews {
			assert.NotRegexp(t, "^[0-9a-f]{24}$", id, "a compact label, not the 24-hex stored id: %s", id)
		}
	})

	t.Run("R3-c: a created option names its property as the surface serves it", func(t *testing.T) {
		fx := setup(t)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("rel-spice-level"),
			bundle.RelationKeyRelationKey:    domain.String("6a8f2c1d9e4b7a3f5c2d8e55"),
			bundle.RelationKeyApiObjectKey:   domain.String("spice_level"),
			bundle.RelationKeyName:           domain.String("Spice level"),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
		})

		result, err := fx.CreateObject(ctx, testSpaceId,
			[]byte(`{"formatVersion":"2.0","type":"chore","properties":{"name":"X","spice_level":"Hot"}}`), true, true)

		require.NoError(t, err)
		require.NotNil(t, result.Created)
		require.Len(t, result.Created.Options, 1)
		assert.Equal(t, "spice_level", result.Created.Options[0].Property)
		assert.Equal(t, "Hot", result.Created.Options[0].Name)
	})

	t.Run("R3-d: a name-only create under an existing display name is refused; an explicit key creates another with a warning", func(t *testing.T) {
		fx := setup(t)
		// a name whose derived slug ("cafe") collides with nothing: the slug
		// check cannot catch it, the name check must (an ASCII twin like
		// "Severity" is already refused by the slug collision)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:           domain.String("rel-cafe"),
			bundle.RelationKeyRelationKey:  domain.String("6a8f2c1d9e4b7a3f5c2d8e66"),
			bundle.RelationKeyApiObjectKey: domain.String("coffee_rating"),
			bundle.RelationKeyName:         domain.String("Café"),
		})

		_, err := fx.CreateProperty(ctx, testSpaceId, v2model.CreatePropertyRequest{Name: "Café", Format: "text"}, true)
		apiErr := v2Err(t, err)
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "/name", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, `a property named "Café" already exists (key "coffee_rating")`)
		assert.Contains(t, apiErr.Issues[0].Hint, `use the existing property "coffee_rating", or pass an explicit different key`)

		result, err := fx.CreateProperty(ctx, testSpaceId, v2model.CreatePropertyRequest{Key: "severity2", Name: "Severity", Format: "text"}, true)
		require.NoError(t, err)
		require.Len(t, result.Warnings, 1)
		assert.Equal(t, "/name", result.Warnings[0].Path)
		assert.Contains(t, result.Warnings[0].Message, "this creates another property under the same name")
		assert.Equal(t, []v2model.Ref{v2model.RefListProperties(testSpaceId)}, result.Warnings[0].SeeAlso)
	})

	t.Run("R3-c: an option on a property minted by the same request, and one added by a type op, are spelled by slug", func(t *testing.T) {
		fx := setup(t)

		// a type document minting the property and its option together
		result, err := fx.CreateType(ctx, testSpaceId,
			[]byte(`{"name":"Plant","property_definitions":[{"name":"Harvest Season","format":"select","options":[{"name":"Summer"}]}]}`), true, true)
		require.NoError(t, err)
		require.NotNil(t, result.Created)
		require.Len(t, result.Created.Options, 1, "%v", result.Created)
		assert.Equal(t, "harvest_season", result.Created.Options[0].Property)

		// a type op adding an option to an existing bson-keyed select, with
		// no document import to build the vocabulary
		fx2 := newTypeOpsFixture(t)
		fx2.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("rel-spice-level"),
			bundle.RelationKeyRelationKey:    domain.String("6a8f2c1d9e4b7a3f5c2d8e55"),
			bundle.RelationKeyApiObjectKey:   domain.String("spice_level"),
			bundle.RelationKeyName:           domain.String("Spice level"),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
		})
		fx2.captureTypeDetails()
		fx2.expectTypeViewEdit(typeReadWithViews(viewWithColumns("viewAll1", "All", "name")))
		result, err = fx2.UpdateType(ctx, testSpaceId, "plant", "",
			opsBody(`{"op":"add_property","property":"spice_level","options":[{"name":"Hot"}]}`), true, true)
		require.NoError(t, err)
		require.NotNil(t, result.Created)
		require.Len(t, result.Created.Options, 1, "%v", result.Created)
		assert.Equal(t, "spice_level", result.Created.Options[0].Property)
	})

	t.Run("R3-c: a real mint that suffixed its slug reports the stored slug on the property and its option", func(t *testing.T) {
		// a HIDDEN holder of harvest_season: the request namespace does not
		// see it, the mint does, and stores harvest_season_2
		fx := setup(t)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:           domain.String("rel-hidden-hs"),
			bundle.RelationKeyRelationKey:  domain.String("6a7663db61fab21cd4b9e301"),
			bundle.RelationKeyApiObjectKey: domain.String("harvest_season"),
			bundle.RelationKeyName:         domain.String("Hidden holder"),
			bundle.RelationKeyIsHidden:     domain.Bool(true),
		})
		// the row the mint creates, as the store will index it (hidden so
		// the name step cannot resolve to it before the mint)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("rel-hs"),
			bundle.RelationKeyRelationKey:    domain.String("6a7663db61fab21cd4b9e302"),
			bundle.RelationKeyApiObjectKey:   domain.String("harvest_season_2"),
			bundle.RelationKeyName:           domain.String("Harvest Season (minted)"),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
			bundle.RelationKeyIsHidden:       domain.Bool(true),
		})
		fx.mwMock.EXPECT().ObjectCreateRelation(mock.Anything, mock.Anything).Return(&pb.RpcObjectCreateRelationResponse{
			ObjectId: "rel-hs", Key: "6a7663db61fab21cd4b9e302",
			Details: &types.Struct{Fields: map[string]*types.Value{bundle.RelationKeyApiObjectKey.String(): pbtypes.String("harvest_season_2")}},
			Error:   &pb.RpcObjectCreateRelationResponseError{Code: pb.RpcObjectCreateRelationResponseError_NULL},
		}).Once()
		fx.mwMock.EXPECT().ObjectCreateRelationOption(mock.Anything, mock.Anything).Return(&pb.RpcObjectCreateRelationOptionResponse{
			ObjectId: "opt-summer", Error: &pb.RpcObjectCreateRelationOptionResponseError{Code: pb.RpcObjectCreateRelationOptionResponseError_NULL},
		}).Once()
		fx.mwMock.EXPECT().ObjectCreateObjectType(mock.Anything, mock.Anything).Return(&pb.RpcObjectCreateObjectTypeResponse{
			ObjectId: "type-plant", Error: &pb.RpcObjectCreateObjectTypeResponseError{Code: pb.RpcObjectCreateObjectTypeResponseError_NULL},
		}).Once()
		fx.expectEtagRead("type-plant")

		result, err := fx.CreateType(ctx, testSpaceId,
			[]byte(`{"name":"Plant","property_definitions":[{"name":"Harvest Season","format":"select","options":[{"name":"Summer"}]}]}`), false, true)

		require.NoError(t, err)
		require.NotNil(t, result.Created)
		require.Len(t, result.Created.Properties, 1)
		assert.Equal(t, "harvest_season_2", result.Created.Properties[0].Key, "the stored slug, never the proposal")
		require.Len(t, result.Created.Options, 1)
		assert.Equal(t, "harvest_season_2", result.Created.Options[0].Property)
	})

	t.Run("R3-c: a mint whose suffix walk ran out reports the minted key, the property's only address", func(t *testing.T) {
		fx := setup(t)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("rel-hs-bare"),
			bundle.RelationKeyRelationKey:    domain.String("6a7663db61fab21cd4b9e303"),
			bundle.RelationKeyName:           domain.String("Harvest Season (minted)"),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
			bundle.RelationKeyIsHidden:       domain.Bool(true),
		})
		// the mint answered with an EMPTY apiObjectKey: authoritative, not
		// absent — the proposal must not survive it
		fx.mwMock.EXPECT().ObjectCreateRelation(mock.Anything, mock.Anything).Return(&pb.RpcObjectCreateRelationResponse{
			ObjectId: "rel-hs-bare", Key: "6a7663db61fab21cd4b9e303",
			Details: &types.Struct{Fields: map[string]*types.Value{bundle.RelationKeyApiObjectKey.String(): pbtypes.String("")}},
			Error:   &pb.RpcObjectCreateRelationResponseError{Code: pb.RpcObjectCreateRelationResponseError_NULL},
		}).Once()
		// the option is created against the minted relation key — the
		// property's only address
		fx.mwMock.EXPECT().ObjectCreateRelationOption(mock.Anything, mock.MatchedBy(func(req *pb.RpcObjectCreateRelationOptionRequest) bool {
			return pbtypes.GetString(req.Details, bundle.RelationKeyRelationKey.String()) == "6a7663db61fab21cd4b9e303"
		})).Return(&pb.RpcObjectCreateRelationOptionResponse{
			ObjectId: "opt-summer", Error: &pb.RpcObjectCreateRelationOptionResponseError{Code: pb.RpcObjectCreateRelationOptionResponseError_NULL},
		}).Once()
		fx.mwMock.EXPECT().ObjectCreateObjectType(mock.Anything, mock.Anything).Return(&pb.RpcObjectCreateObjectTypeResponse{
			ObjectId: "type-plant", Error: &pb.RpcObjectCreateObjectTypeResponseError{Code: pb.RpcObjectCreateObjectTypeResponseError_NULL},
		}).Once()
		fx.expectEtagRead("type-plant")

		result, err := fx.CreateType(ctx, testSpaceId,
			[]byte(`{"name":"Plant","property_definitions":[{"name":"Harvest Season","format":"select","options":[{"name":"Summer"}]}]}`), false, true)

		require.NoError(t, err)
		require.NotNil(t, result.Created)
		require.Len(t, result.Created.Properties, 1)
		assert.Equal(t, "6a7663db61fab21cd4b9e303", result.Created.Properties[0].Key, "the minted key, not the proposal")
		require.Len(t, result.Created.Options, 1)
		assert.Equal(t, "6a7663db61fab21cd4b9e303", result.Created.Options[0].Property)
	})

	t.Run("R3-f: list_objects lists the system keys as served too", func(t *testing.T) {
		fx := setup(t)

		_, _, _, err := fx.ListObjects(ctx, testSpaceId, []string{"last_opened_dat"}, 0, 25)

		apiErr := v2Err(t, err)
		require.NotEmpty(t, apiErr.Issues)
		assert.Contains(t, apiErr.Issues[0].Message, "last_opened_date")
		assert.NotContains(t, apiErr.Issues[0].Message, "lastOpenedDate")
	})

	t.Run("R3-f: the system query keys are accepted and listed as list_properties serves them", func(t *testing.T) {
		fx := setup(t)

		_, _, _, _, err := fx.SearchObjects(ctx, testSpaceId, v2model.SearchRequest{Type: "chore", Fields: []string{"last_opened_date", "lastOpenedDate"}}, 0, 25)
		require.NoError(t, err, "both spellings are accepted")

		_, _, _, _, err = fx.SearchObjects(ctx, testSpaceId, v2model.SearchRequest{Type: "chore", Fields: []string{"last_opened_dat"}}, 0, 25)
		issue := issueAt(t, err, "/fields/0")
		assert.Contains(t, issue.Message, "last_opened_date")
		assert.NotContains(t, issue.Message, "lastOpenedDate")
	})
}
