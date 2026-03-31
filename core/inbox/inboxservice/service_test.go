package inboxservice

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/inbox/inboxclient/mock_inboxclient"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
	"github.com/anyproto/anytype-heart/space/techspace/mock_techspace"
)

type fixture struct {
	*inboxSender
	mockInboxClient  *mock_inboxclient.MockInboxClient
	mockTechSpace    *mock_techspace.MockTechSpace
	mockSpaceService *mock_space.MockService
}

func newFixture(t *testing.T) *fixture {
	mockInbox := mock_inboxclient.NewMockInboxClient(t)
	mockTS := mock_techspace.NewMockTechSpace(t)
	mockSS := mock_space.NewMockService(t)

	s := &inboxSender{
		inboxClient:  mockInbox,
		spaceService: mockSS,
		techSpace:    mockTS,
	}
	return &fixture{
		inboxSender:      s,
		mockInboxClient:  mockInbox,
		mockTechSpace:    mockTS,
		mockSpaceService: mockSS,
	}
}

func TestSendSpaceInvite(t *testing.T) {
	t.Run("sends invite with correct payload", func(t *testing.T) {
		// given
		fx := newFixture(t)
		ctx := context.Background()
		spaceId := "test-space-id"

		receiverKeys, err := accountdata.NewRandom()
		require.NoError(t, err)
		receiverIdentity := receiverKeys.SignKey.GetPublic().Account()

		want := spaceInvitePayload{
			SpaceId:    spaceId,
			SpaceName:  "Test Space",
			IconOption: 5,
		}

		mockSpaceView := mock_techspace.NewMockSpaceView(t)
		mockSpaceView.EXPECT().GetSpaceDescription().Return(spaceinfo.SpaceDescription{
			Name:       "Test Space",
			IconOption: 5,
		})
		fx.mockTechSpace.EXPECT().DoSpaceView(mock.Anything, spaceId, mock.Anything).
			RunAndReturn(func(ctx context.Context, id string, apply func(techspace.SpaceView) error) error {
				return apply(mockSpaceView)
			})
		fx.mockInboxClient.EXPECT().InboxAddMessage(mock.Anything, mock.Anything, mock.Anything).Return(nil)

		// when
		err = fx.SendRegularSpaceInvites(ctx, spaceId, receiverIdentity)

		// then
		require.NoError(t, err)
		fx.mockInboxClient.AssertCalled(t, "InboxAddMessage", mock.Anything, mock.Anything, mock.Anything)

		call := fx.mockInboxClient.Calls[0]
		msg := call.Arguments[2].(*coordinatorproto.InboxMessage)
		assert.Equal(t, inboxPayloadTypeRegularInvite, msg.Packet.Payload.PayloadType)
		assert.Equal(t, receiverIdentity, msg.Packet.ReceiverIdentity)

		var gotPayload spaceInvitePayload
		err = json.Unmarshal(msg.Packet.Payload.Body, &gotPayload)
		require.NoError(t, err)
		assert.Equal(t, want, gotPayload)
	})

	t.Run("returns error for invalid receiver identity", func(t *testing.T) {
		// given
		fx := newFixture(t)
		ctx := context.Background()
		spaceId := "test-space-id"

		mockSpaceView := mock_techspace.NewMockSpaceView(t)
		mockSpaceView.EXPECT().GetSpaceDescription().Return(spaceinfo.SpaceDescription{
			Name: "Test Space",
		})
		fx.mockTechSpace.EXPECT().DoSpaceView(mock.Anything, spaceId, mock.Anything).
			RunAndReturn(func(ctx context.Context, id string, apply func(techspace.SpaceView) error) error {
				return apply(mockSpaceView)
			})

		// when
		err := fx.SendRegularSpaceInvites(ctx, spaceId, "invalid-identity")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "decode receiver identity")
	})
}

func TestProcessSpaceInvite(t *testing.T) {
	t.Run("processes valid space invite", func(t *testing.T) {
		// given
		fx := newFixture(t)
		spaceId := "invited-space"
		payload := spaceInvitePayload{
			SpaceId:   spaceId,
			SpaceName: "My Space",
		}
		body, err := json.Marshal(payload)
		require.NoError(t, err)

		fx.mockSpaceService.On("InviteJoin", mock.Anything, spaceId, "").Return(nil)
		fx.mockTechSpace.EXPECT().SpaceViewSetData(mock.Anything, spaceId, mock.Anything).Return(nil)

		packet := &coordinatorproto.InboxPacket{
			SenderIdentity: "sender-identity",
			Payload: &coordinatorproto.InboxPayload{
				Body:        body,
				PayloadType: inboxPayloadTypeRegularInvite,
			},
		}

		// when
		err = fx.processRegularSpaceInvite(packet)

		// then
		require.NoError(t, err)
		fx.mockSpaceService.AssertCalled(t, "InviteJoin", mock.Anything, spaceId, "")
	})

	t.Run("returns error for nil payload body", func(t *testing.T) {
		// given
		fx := newFixture(t)
		packet := &coordinatorproto.InboxPacket{
			Payload: &coordinatorproto.InboxPayload{
				Body: nil,
			},
		}

		// when
		err := fx.processRegularSpaceInvite(packet)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "got nil payload body")
	})

	t.Run("returns error for nil payload", func(t *testing.T) {
		// given
		fx := newFixture(t)
		packet := &coordinatorproto.InboxPacket{
			Payload: nil,
		}

		// when
		err := fx.processRegularSpaceInvite(packet)

		// then
		require.Error(t, err)
	})

	t.Run("returns error for invalid json", func(t *testing.T) {
		// given
		fx := newFixture(t)
		packet := &coordinatorproto.InboxPacket{
			Payload: &coordinatorproto.InboxPayload{
				Body: []byte("not-json"),
			},
		}

		// when
		err := fx.processRegularSpaceInvite(packet)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unmarshal space invite payload")
	})
}
