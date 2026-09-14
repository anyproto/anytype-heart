package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/core/mock_apicore"
	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestSetBlockObjectLink_Idempotent(t *testing.T) {
	mw := mock_apicore.NewMockClientCommands(t)
	s := &Service{mw: mw}

	spaceId := "space1"
	srcId := "srcObj"
	tgtId := "tgtObj"
	blockId := "blk1"

	mw.EXPECT().ObjectShow(context.Background(), &pb.RpcObjectShowRequest{SpaceId: spaceId, ObjectId: srcId}).Return(&pb.RpcObjectShowResponse{
		ObjectView: &model.ObjectView{
			Blocks: []*model.Block{{
				Id: blockId,
				Content: &model.BlockContentOfLink{
					Link: &model.BlockContentLink{
						TargetBlockId: tgtId,
						Style:         model.BlockContentLink_Page,
						CardStyle:     model.BlockContentLink_Card,
					},
				},
			}},
		},
	}).Once()

	mw.EXPECT().ObjectShow(context.Background(), &pb.RpcObjectShowRequest{SpaceId: spaceId, ObjectId: tgtId}).Return(&pb.RpcObjectShowResponse{
		ObjectView: &model.ObjectView{Blocks: []*model.Block{{Id: tgtId}}},
	}).Once()

	out, err := s.SetBlockObjectLink(context.Background(), spaceId, srcId, blockId, apimodel.SetBlockObjectLinkRequest{TargetObjectId: tgtId})
	require.NoError(t, err)
	assert.False(t, out.Replaced)
	assert.Equal(t, blockId, out.BlockId)
}

func TestSetBlockObjectLink_TextToLink(t *testing.T) {
	mw := mock_apicore.NewMockClientCommands(t)
	s := &Service{mw: mw}

	spaceId := "space1"
	srcId := "srcObj"
	tgtId := "tgtObj"
	blockId := "blk1"
	newId := "blkNew"

	mw.EXPECT().ObjectShow(context.Background(), &pb.RpcObjectShowRequest{SpaceId: spaceId, ObjectId: srcId}).Return(&pb.RpcObjectShowResponse{
		ObjectView: &model.ObjectView{
			Blocks: []*model.Block{{
				Id:      blockId,
				Content: &model.BlockContentOfText{Text: &model.BlockContentText{}},
			}},
		},
	}).Once()

	mw.EXPECT().ObjectShow(context.Background(), &pb.RpcObjectShowRequest{SpaceId: spaceId, ObjectId: tgtId}).Return(&pb.RpcObjectShowResponse{
		ObjectView: &model.ObjectView{Blocks: []*model.Block{{Id: tgtId}}},
	}).Once()

	mw.EXPECT().BlockReplace(context.Background(), mock.MatchedBy(func(req *pb.RpcBlockReplaceRequest) bool {
		l := req.Block.GetLink()
		return req.ContextId == srcId && req.BlockId == blockId &&
			l != nil && l.TargetBlockId == tgtId && l.CardStyle == model.BlockContentLink_Card
	})).Return(&pb.RpcBlockReplaceResponse{
		BlockId: newId,
		Error:   &pb.RpcBlockReplaceResponseError{Code: pb.RpcBlockReplaceResponseError_NULL},
	}).Once()

	out, err := s.SetBlockObjectLink(context.Background(), spaceId, srcId, blockId, apimodel.SetBlockObjectLinkRequest{TargetObjectId: tgtId})
	require.NoError(t, err)
	assert.True(t, out.Replaced)
	assert.Equal(t, newId, out.BlockId)
}

func TestDeleteBlockObjectLink_TargetMismatch(t *testing.T) {
	mw := mock_apicore.NewMockClientCommands(t)
	s := &Service{mw: mw}

	mw.EXPECT().ObjectShow(context.Background(), mock.Anything).Return(&pb.RpcObjectShowResponse{
		ObjectView: &model.ObjectView{
			Blocks: []*model.Block{{
				Id:      "b1",
				Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: "other"}},
			}},
		},
	}).Once()

	err := s.DeleteBlockObjectLink(context.Background(), "s", "o", "b1", "wanted")
	require.ErrorIs(t, err, ErrTargetMismatch)
}

func TestSetBlockObjectLink_SameTargetUpgradesCardStyle(t *testing.T) {
	mw := mock_apicore.NewMockClientCommands(t)
	s := &Service{mw: mw}

	spaceId := "space1"
	srcId := "srcObj"
	tgtId := "tgtObj"
	blockId := "blk1"

	mw.EXPECT().ObjectShow(context.Background(), &pb.RpcObjectShowRequest{SpaceId: spaceId, ObjectId: srcId}).Return(&pb.RpcObjectShowResponse{
		ObjectView: &model.ObjectView{
			Blocks: []*model.Block{{
				Id: blockId,
				Content: &model.BlockContentOfLink{
					Link: &model.BlockContentLink{
						TargetBlockId: tgtId,
						Style:         model.BlockContentLink_Page,
						CardStyle:     model.BlockContentLink_Text,
					},
				},
			}},
		},
	}).Once()

	mw.EXPECT().ObjectShow(context.Background(), &pb.RpcObjectShowRequest{SpaceId: spaceId, ObjectId: tgtId}).Return(&pb.RpcObjectShowResponse{
		ObjectView: &model.ObjectView{Blocks: []*model.Block{{Id: tgtId}}},
	}).Once()

	mw.EXPECT().BlockReplace(context.Background(), mock.MatchedBy(func(req *pb.RpcBlockReplaceRequest) bool {
		l := req.Block.GetLink()
		return req.ContextId == srcId && req.BlockId == blockId &&
			l != nil && l.TargetBlockId == tgtId && l.CardStyle == model.BlockContentLink_Card
	})).Return(&pb.RpcBlockReplaceResponse{
		BlockId: blockId,
		Error:   &pb.RpcBlockReplaceResponseError{Code: pb.RpcBlockReplaceResponseError_NULL},
	}).Once()

	out, err := s.SetBlockObjectLink(context.Background(), spaceId, srcId, blockId, apimodel.SetBlockObjectLinkRequest{TargetObjectId: tgtId})
	require.NoError(t, err)
	assert.True(t, out.Replaced)
	assert.Equal(t, blockId, out.BlockId)
}

func TestSetBlockObjectLink_TextToLinkDashboardStyle(t *testing.T) {
	mw := mock_apicore.NewMockClientCommands(t)
	s := &Service{mw: mw}

	spaceId := "space1"
	srcId := "srcObj"
	tgtId := "tgtObj"
	blockId := "blk1"
	newId := "blkNew"

	mw.EXPECT().ObjectShow(context.Background(), &pb.RpcObjectShowRequest{SpaceId: spaceId, ObjectId: srcId}).Return(&pb.RpcObjectShowResponse{
		ObjectView: &model.ObjectView{
			Blocks: []*model.Block{{
				Id:      blockId,
				Content: &model.BlockContentOfText{Text: &model.BlockContentText{}},
			}},
		},
	}).Once()

	mw.EXPECT().ObjectShow(context.Background(), &pb.RpcObjectShowRequest{SpaceId: spaceId, ObjectId: tgtId}).Return(&pb.RpcObjectShowResponse{
		ObjectView: &model.ObjectView{Blocks: []*model.Block{{Id: tgtId}}},
	}).Once()

	mw.EXPECT().BlockReplace(context.Background(), mock.MatchedBy(func(req *pb.RpcBlockReplaceRequest) bool {
		l := req.Block.GetLink()
		return req.ContextId == srcId && req.BlockId == blockId &&
			l != nil && l.TargetBlockId == tgtId &&
			l.Style == model.BlockContentLink_Dashboard &&
			l.CardStyle == model.BlockContentLink_Card
	})).Return(&pb.RpcBlockReplaceResponse{
		BlockId: newId,
		Error:   &pb.RpcBlockReplaceResponseError{Code: pb.RpcBlockReplaceResponseError_NULL},
	}).Once()

	out, err := s.SetBlockObjectLink(context.Background(), spaceId, srcId, blockId, apimodel.SetBlockObjectLinkRequest{
		TargetObjectId: tgtId,
		LinkStyle:      "dashboard",
		CardStyle:      "card",
	})
	require.NoError(t, err)
	assert.True(t, out.Replaced)
	assert.Equal(t, newId, out.BlockId)
}

func TestSetBlockObjectLink_SyncPresentationFromReferenceBlock(t *testing.T) {
	mw := mock_apicore.NewMockClientCommands(t)
	s := &Service{mw: mw}

	spaceId := "space1"
	pageId := "pageObj"
	tgtDemucs := "tgtDemucs"
	blockDemucs := "blkDemucs"
	blockRefCard := "blkRefCard"
	newId := "blkNew"

	refLink := &model.BlockContentLink{
		TargetBlockId: "otherRepo",
		Style:         model.BlockContentLink_Page,
		CardStyle:     model.BlockContentLink_Card,
		IconSize:      model.BlockContentLink_SizeMedium,
		Description:   model.BlockContentLink_Content,
		Relations:     []string{"rel1", "rel2"},
	}

	mw.EXPECT().ObjectShow(context.Background(), &pb.RpcObjectShowRequest{SpaceId: spaceId, ObjectId: pageId}).Return(&pb.RpcObjectShowResponse{
		ObjectView: &model.ObjectView{
			Blocks: []*model.Block{
				{
					Id: blockDemucs,
					Content: &model.BlockContentOfLink{
						Link: &model.BlockContentLink{
							TargetBlockId: tgtDemucs,
							Style:         model.BlockContentLink_Page,
							CardStyle:     model.BlockContentLink_Text,
							Description:   model.BlockContentLink_None,
						},
					},
				},
				{
					Id:                blockRefCard,
					BackgroundColor:   "yellow",
					Content:           &model.BlockContentOfLink{Link: refLink},
				},
			},
		},
	}).Once()

	mw.EXPECT().ObjectShow(context.Background(), &pb.RpcObjectShowRequest{SpaceId: spaceId, ObjectId: tgtDemucs}).Return(&pb.RpcObjectShowResponse{
		ObjectView: &model.ObjectView{Blocks: []*model.Block{{Id: tgtDemucs}}},
	}).Once()

	mw.EXPECT().BlockReplace(context.Background(), mock.MatchedBy(func(req *pb.RpcBlockReplaceRequest) bool {
		l := req.Block.GetLink()
		if req.ContextId != pageId || req.BlockId != blockDemucs || l == nil {
			return false
		}
		return req.Block.BackgroundColor == "yellow" &&
			l.TargetBlockId == tgtDemucs &&
			l.Style == refLink.Style &&
			l.CardStyle == refLink.CardStyle &&
			l.IconSize == refLink.IconSize &&
			l.Description == refLink.Description &&
			len(l.Relations) == 2 && l.Relations[0] == "rel1" && l.Relations[1] == "rel2"
	})).Return(&pb.RpcBlockReplaceResponse{
		BlockId: newId,
		Error:   &pb.RpcBlockReplaceResponseError{Code: pb.RpcBlockReplaceResponseError_NULL},
	}).Once()

	out, err := s.SetBlockObjectLink(context.Background(), spaceId, pageId, blockDemucs, apimodel.SetBlockObjectLinkRequest{
		TargetObjectId:                  tgtDemucs,
		SyncLinkPresentationFromBlockId: blockRefCard,
	})
	require.NoError(t, err)
	assert.True(t, out.Replaced)
	assert.Equal(t, newId, out.BlockId)
}

func strPtr(s string) *string { return &s }

func TestSetBlockObjectLink_BackgroundColorOverride(t *testing.T) {
	mw := mock_apicore.NewMockClientCommands(t)
	s := &Service{mw: mw}

	spaceId := "space1"
	pageId := "pageObj"
	tgt := "tgtObj"
	blockId := "blk1"

	mw.EXPECT().ObjectShow(context.Background(), &pb.RpcObjectShowRequest{SpaceId: spaceId, ObjectId: pageId}).Return(&pb.RpcObjectShowResponse{
		ObjectView: &model.ObjectView{
			Blocks: []*model.Block{{
				Id: blockId,
				Content: &model.BlockContentOfLink{
					Link: &model.BlockContentLink{
						TargetBlockId: tgt,
						Style:         model.BlockContentLink_Page,
						CardStyle:     model.BlockContentLink_Card,
					},
				},
			}},
		},
	}).Once()

	mw.EXPECT().ObjectShow(context.Background(), &pb.RpcObjectShowRequest{SpaceId: spaceId, ObjectId: tgt}).Return(&pb.RpcObjectShowResponse{
		ObjectView: &model.ObjectView{Blocks: []*model.Block{{Id: tgt}}},
	}).Once()

	mw.EXPECT().BlockReplace(context.Background(), mock.MatchedBy(func(req *pb.RpcBlockReplaceRequest) bool {
		return req.Block.BackgroundColor == "red" &&
			req.Block.GetLink() != nil && req.Block.GetLink().TargetBlockId == tgt
	})).Return(&pb.RpcBlockReplaceResponse{
		BlockId: blockId,
		Error:   &pb.RpcBlockReplaceResponseError{Code: pb.RpcBlockReplaceResponseError_NULL},
	}).Once()

	out, err := s.SetBlockObjectLink(context.Background(), spaceId, pageId, blockId, apimodel.SetBlockObjectLinkRequest{
		TargetObjectId:  tgt,
		BackgroundColor: strPtr("red"),
	})
	require.NoError(t, err)
	assert.True(t, out.Replaced)
}

func TestSetBlockObjectLink_InvalidBackgroundColorRejected(t *testing.T) {
	mw := mock_apicore.NewMockClientCommands(t)
	s := &Service{mw: mw}

	mw.EXPECT().ObjectShow(context.Background(), &pb.RpcObjectShowRequest{SpaceId: "s", ObjectId: "page"}).Return(&pb.RpcObjectShowResponse{
		ObjectView: &model.ObjectView{
			Blocks: []*model.Block{{
				Id: "b1",
				Content: &model.BlockContentOfLink{
					Link: &model.BlockContentLink{TargetBlockId: "t1", Style: model.BlockContentLink_Page, CardStyle: model.BlockContentLink_Card},
				},
			}},
		},
	}).Once()
	mw.EXPECT().ObjectShow(context.Background(), &pb.RpcObjectShowRequest{SpaceId: "s", ObjectId: "t1"}).Return(&pb.RpcObjectShowResponse{
		ObjectView: &model.ObjectView{Blocks: []*model.Block{{Id: "t1"}}},
	}).Once()

	_, err := s.SetBlockObjectLink(context.Background(), "s", "page", "b1", apimodel.SetBlockObjectLinkRequest{
		TargetObjectId:  "t1",
		BackgroundColor: strPtr("green"),
	})
	require.Error(t, err)
}
