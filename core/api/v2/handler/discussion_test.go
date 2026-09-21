package v2handler

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/gogo/protobuf/types"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// discussionRead is a live page read, with or without a discussion id on
// the parent's details.
func discussionRead(discussionId string) apicore.ObjectRead {
	details := map[string]*types.Value{"id": pbtypes.String("obj1"), "name": pbtypes.String("Doc")}
	if discussionId != "" {
		details[bundle.RelationKeyDiscussionId.String()] = pbtypes.String(discussionId)
	}
	return apicore.ObjectRead{
		SbType: model.SmartBlockType_Page,
		Snapshot: &model.SmartBlockSnapshotBase{
			Details:     &types.Struct{Fields: details},
			ObjectTypes: []string{"ot-page"},
			Blocks: []*model.Block{{Id: "obj1",
				Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}},
		},
		Heads: []string{"h"},
	}
}

func discussionRouterFixture(t *testing.T) *v2HandlerFixture {
	fx := chatRouterFixture(t)
	fx.router.POST("/v2/spaces/:space_id/objects/:object_id/discussion", CreateDiscussionHandler(fx.svc))
	return fx
}

func TestCreateDiscussionHandler(t *testing.T) {
	t.Run("a minted discussion is a 201 carrying created", func(t *testing.T) {
		// given
		fx := discussionRouterFixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, "space1", "obj1").Return(discussionRead(""), nil)
		fx.mwMock.EXPECT().ObjectAddDiscussion(mock.Anything, &pb.RpcObjectDiscussionAddRequest{ObjectId: "obj1"}).
			Return(&pb.RpcObjectDiscussionAddResponse{DiscussionId: "disc1"})

		// when
		w := serveChat(fx, "POST", "/v2/spaces/space1/objects/obj1/discussion", "")

		// then
		require.Equal(t, http.StatusCreated, w.Code)
		assert.JSONEq(t, `{"id":"disc1","created":true}`, w.Body.String())
	})

	t.Run("an existing discussion is a 200 with the same id and no created", func(t *testing.T) {
		// given: no ObjectAddDiscussion expectation
		fx := discussionRouterFixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, "space1", "obj1").Return(discussionRead("disc1"), nil)

		// when
		w := serveChat(fx, "POST", "/v2/spaces/space1/objects/obj1/discussion", "")

		// then
		require.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `{"id":"disc1"}`, w.Body.String())
	})

	t.Run("dry run mints nothing and answers 200", func(t *testing.T) {
		// given: no ObjectAddDiscussion expectation — a call would fail the test
		fx := discussionRouterFixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, "space1", "obj1").Return(discussionRead(""), nil)

		// when
		w := serveChat(fx, "POST", "/v2/spaces/space1/objects/obj1/discussion?dry_run=true", "")

		// then
		require.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `{"created":true,"dry_run":true}`, w.Body.String())
	})

	t.Run("an unknown object is a 404", func(t *testing.T) {
		fx := discussionRouterFixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, "space1", "nope").Return(apicore.ObjectRead{}, treestorage.ErrUnknownTreeId)

		w := serveChat(fx, "POST", "/v2/spaces/space1/objects/nope/discussion", "")

		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("the minted id works as chat_id on the messages route", func(t *testing.T) {
		// given: the discussion object is indexed under its layout
		fx := discussionRouterFixture(t)
		fx.store.AddObjects(t, "space1", []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("disc1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_discussion)),
		}})
		fx.mwMock.EXPECT().ChatAddMessage(mock.Anything, mock.MatchedBy(func(req *pb.RpcChatAddMessageRequest) bool {
			return req.ChatObjectId == "disc1"
		})).Return(&pb.RpcChatAddMessageResponse{MessageId: "msgNew"})

		// when
		w := serveChat(fx, "POST", "/v2/spaces/space1/chats/disc1/messages", `{"text":"a comment"}`)

		// then
		require.Equal(t, http.StatusCreated, w.Code)
		assert.Contains(t, w.Body.String(), `"id":"msgNew"`)
	})
}
