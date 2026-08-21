package block

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator/mock_objectcreator"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// pageTypeUniqueKey is the raw unique key form ("ot-page") accepted by
// CreateLinkToTheNewObject.
var pageTypeUniqueKey = domain.MustUniqueKey(coresb.SmartBlockTypeObjectType, "page").Marshal()

func TestService_CreateLinkToTheNewObject_BakesCreatedInContextIntoInitialDetails(t *testing.T) {
	// given
	const (
		spaceId  = "space1"
		parentId = "parentPage"
	)
	creator := mock_objectcreator.NewMockService(t)
	svc := &Service{objectCreator: creator}
	sctx := session.NewContext()

	stop := errors.New("stop before link block creation")

	var captured objectcreator.CreateObjectRequest
	creator.EXPECT().
		CreateObject(mock.Anything, spaceId, mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, req objectcreator.CreateObjectRequest) (string, *domain.Details, error) {
			captured = req
			return "", nil, stop
		})

	// when
	linkID, _, _, err := svc.CreateLinkToTheNewObject(context.Background(), sctx, &pb.RpcBlockLinkCreateWithObjectRequest{
		SpaceId:             spaceId,
		ContextId:           parentId,
		ObjectTypeUniqueKey: pageTypeUniqueKey,
	})

	// then
	require.ErrorIs(t, err, stop)
	require.NotEmpty(t, linkID, "linkID must be pre-generated before CreateObject is called")
	require.NotNil(t, captured.Details)
	assert.Equal(t, parentId, captured.Details.GetString(bundle.RelationKeyCreatedInContext))
	assert.Equal(t, linkID, captured.Details.GetString(bundle.RelationKeyCreatedInContextRef),
		"createdInContextRef must equal the pre-generated link block id")
}

func TestService_CreateLinkToTheNewObject_NoContextSkipsCreatedInContext(t *testing.T) {
	// given
	const spaceId = "space1"
	creator := mock_objectcreator.NewMockService(t)
	svc := &Service{objectCreator: creator}

	var captured objectcreator.CreateObjectRequest
	creator.EXPECT().
		CreateObject(mock.Anything, spaceId, mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, req objectcreator.CreateObjectRequest) (string, *domain.Details, error) {
			captured = req
			return "newObj", domain.NewDetails(), nil
		})

	// when
	linkID, objectId, _, err := svc.CreateLinkToTheNewObject(context.Background(), nil, &pb.RpcBlockLinkCreateWithObjectRequest{
		SpaceId:             spaceId,
		ContextId:           "",
		ObjectTypeUniqueKey: pageTypeUniqueKey,
	})

	// then
	require.NoError(t, err)
	assert.Empty(t, linkID, "no link block should be created when ContextId is empty")
	assert.Equal(t, "newObj", objectId)
	require.NotNil(t, captured.Details)
	assert.Empty(t, captured.Details.GetString(bundle.RelationKeyCreatedInContext))
	assert.Empty(t, captured.Details.GetString(bundle.RelationKeyCreatedInContextRef))
}

func TestService_CreateLinkToTheNewObject_RejectsNonLinkBlockBeforeObjectCreation(t *testing.T) {
	// Regression: with ContextId set, validation must happen before objectCreator.CreateObject.
	// Otherwise a malformed Block leaves an orphan object in the space (the new object is
	// created, then the link-block insert fails, then the RPC returns an error — but the
	// object stays).
	// given
	creator := mock_objectcreator.NewMockService(t)
	svc := &Service{objectCreator: creator}
	// no EXPECT().CreateObject(...) — testify mock auto-fails on any call

	nonLinkBlock := &model.Block{
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{Text: "not a link"},
		},
	}

	// when
	linkID, objectId, details, err := svc.CreateLinkToTheNewObject(
		context.Background(),
		session.NewContext(),
		&pb.RpcBlockLinkCreateWithObjectRequest{
			SpaceId:             "space1",
			ContextId:           "parent",
			ObjectTypeUniqueKey: pageTypeUniqueKey,
			Block:               nonLinkBlock,
		},
	)

	// then
	require.Error(t, err, "non-link block must be rejected")
	assert.Empty(t, linkID)
	assert.Empty(t, objectId, "no objectId returned — proxy for: no orphan object in the space")
	assert.Nil(t, details)
	creator.AssertNotCalled(t, "CreateObject", mock.Anything, mock.Anything, mock.Anything)
}
