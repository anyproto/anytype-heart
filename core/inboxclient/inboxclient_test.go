package inboxclient

import (
	"context"
	"errors"
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/anyproto/any-sync/coordinator/inboxclient/mock_inboxclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/techspace"
	"github.com/anyproto/anytype-heart/space/techspace/mock_techspace"
)

type fixture struct {
	ctrl            *gomock.Controller
	inboxClient     *inboxclient
	mockInboxClient *mock_inboxclient.MockInboxClient
	mockWallet      *mock_wallet.MockWallet
	mockTechSpace   *mock_techspace.MockTechSpace
	mockAccountObj  *mock_techspace.MockAccountObject
	senderAccount   *accountdata.AccountKeys
	receiverAccount *accountdata.AccountKeys
}

func newFixture(t *testing.T) *fixture {
	// Create real account keys for testing crypto operations
	senderAccount, err := accountdata.NewRandom()
	require.NoError(t, err)
	receiverAccount, err := accountdata.NewRandom()
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	mockInboxClient := mock_inboxclient.NewMockInboxClient(ctrl)
	mockWallet := mock_wallet.NewMockWallet(t)
	mockTechSpace := mock_techspace.NewMockTechSpace(t)
	mockAccountObj := mock_techspace.NewMockAccountObject(t)

	// Create a simple space service implementation
	spaceService := &spaceServiceStub{techSpace: &clientspace.TechSpace{TechSpace: mockTechSpace}}

	ic := &inboxclient{
		inboxClient:  mockInboxClient,
		spaceService: spaceService,
		wallet:       mockWallet,
		techSpace:    mockTechSpace,
		receivers:    make(map[coordinatorproto.InboxPayloadType]func(*coordinatorproto.InboxPacket) error),
	}

	return &fixture{
		ctrl:            ctrl,
		inboxClient:     ic,
		mockInboxClient: mockInboxClient,
		mockWallet:      mockWallet,
		mockTechSpace:   mockTechSpace,
		mockAccountObj:  mockAccountObj,
		senderAccount:   senderAccount,
		receiverAccount: receiverAccount,
	}
}

// spaceServiceStub is a simple stub for SpaceService
type spaceServiceStub struct {
	techSpace *clientspace.TechSpace
}

func (s *spaceServiceStub) TechSpace() *clientspace.TechSpace {
	return s.techSpace
}

// createValidMessage creates a properly signed and encrypted message
func (fx *fixture) createValidMessage(t *testing.T, id string, payloadBody []byte) *coordinatorproto.InboxMessage {
	// Encrypt the payload body with receiver's public key
	encrypted, err := fx.receiverAccount.SignKey.GetPublic().Encrypt(payloadBody)
	require.NoError(t, err)

	// Sign the encrypted body with sender's private key
	signature, err := fx.senderAccount.SignKey.Sign(encrypted)
	require.NoError(t, err)

	return &coordinatorproto.InboxMessage{
		Id:         id,
		PacketType: coordinatorproto.InboxPacketType_Default,
		Packet: &coordinatorproto.InboxPacket{
			SenderIdentity:   fx.senderAccount.SignKey.GetPublic().Account(),
			ReceiverIdentity: fx.receiverAccount.SignKey.GetPublic().Account(),
			SenderSignature:  signature,
			Payload: &coordinatorproto.InboxPayload{
				PayloadType: coordinatorproto.InboxPayloadType_InboxPayloadOneToOneInvite,
				Timestamp:   12345,
				Body:        encrypted,
			},
		},
	}
}

func TestInboxClient_VerifyPacketSignature(t *testing.T) {
	t.Run("valid signature", func(t *testing.T) {
		fx := newFixture(t)

		msg := fx.createValidMessage(t, "msg1", []byte("test body"))

		err := fx.inboxClient.verifyPacketSignature(msg.Packet)
		require.NoError(t, err)
	})

	t.Run("invalid signature", func(t *testing.T) {
		fx := newFixture(t)

		msg := fx.createValidMessage(t, "msg1", []byte("test body"))
		// Corrupt the signature
		msg.Packet.SenderSignature = []byte("invalid signature")

		err := fx.inboxClient.verifyPacketSignature(msg.Packet)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "signature is invalid")
	})

	t.Run("invalid sender identity", func(t *testing.T) {
		fx := newFixture(t)

		msg := fx.createValidMessage(t, "msg1", []byte("test body"))
		// Corrupt the sender identity
		msg.Packet.SenderIdentity = "invalid identity"

		err := fx.inboxClient.verifyPacketSignature(msg.Packet)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "decode sender identity")
	})

	t.Run("signature from different sender", func(t *testing.T) {
		fx := newFixture(t)

		// Create another account
		otherAccount, err := accountdata.NewRandom()
		require.NoError(t, err)

		msg := fx.createValidMessage(t, "msg1", []byte("test body"))
		// Replace sender identity but keep the original signature
		msg.Packet.SenderIdentity = otherAccount.SignKey.GetPublic().Account()

		err = fx.inboxClient.verifyPacketSignature(msg.Packet)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "signature is invalid")
	})
}

func TestInboxClient_FetchMessages(t *testing.T) {
	t.Run("fetch messages with valid signature and decryption", func(t *testing.T) {
		fx := newFixture(t)

		msg1 := fx.createValidMessage(t, "msg1", []byte("test body 1"))
		msg2 := fx.createValidMessage(t, "msg2", []byte("test body 2"))

		// Setup expectations for GetInboxOffset
		fx.mockAccountObj.EXPECT().GetInboxOffset().Return("", nil)
		fx.mockTechSpace.EXPECT().DoAccountObject(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, fn func(techspace.AccountObject) error) error {
				return fn(fx.mockAccountObj)
			})

		fx.mockInboxClient.EXPECT().InboxFetch(gomock.Any(), "").
			Return([]*coordinatorproto.InboxMessage{msg1, msg2}, false, nil)

		fx.mockWallet.EXPECT().Account().Return(fx.receiverAccount).Times(2)

		// Setup expectations for SetInboxOffset
		fx.mockAccountObj.EXPECT().SetInboxOffset("msg2").Return(nil)
		fx.mockTechSpace.EXPECT().DoAccountObject(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, fn func(techspace.AccountObject) error) error {
				return fn(fx.mockAccountObj)
			})

		// Execute
		messages, err := fx.inboxClient.fetchMessages()
		require.NoError(t, err)
		require.Len(t, messages, 2)

		// Verify messages were decrypted
		assert.Equal(t, []byte("test body 1"), messages[0].Packet.Payload.Body)
		assert.Equal(t, []byte("test body 2"), messages[1].Packet.Payload.Body)
	})

	t.Run("fetch messages with hasMore", func(t *testing.T) {
		fx := newFixture(t)

		msg1 := fx.createValidMessage(t, "msg1", []byte("test body 1"))
		msg2 := fx.createValidMessage(t, "msg2", []byte("test body 2"))

		// Setup expectations for GetInboxOffset
		fx.mockAccountObj.EXPECT().GetInboxOffset().Return("", nil)
		fx.mockTechSpace.EXPECT().DoAccountObject(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, fn func(techspace.AccountObject) error) error {
				return fn(fx.mockAccountObj)
			})

		// First batch
		fx.mockInboxClient.EXPECT().InboxFetch(gomock.Any(), "").
			Return([]*coordinatorproto.InboxMessage{msg1}, true, nil)
		fx.mockWallet.EXPECT().Account().Return(fx.receiverAccount)

		// Second batch
		fx.mockInboxClient.EXPECT().InboxFetch(gomock.Any(), "msg1").
			Return([]*coordinatorproto.InboxMessage{msg2}, false, nil)
		fx.mockWallet.EXPECT().Account().Return(fx.receiverAccount)

		// Setup expectations for SetInboxOffset (should be called with final offset)
		fx.mockAccountObj.EXPECT().SetInboxOffset("msg2").Return(nil)
		fx.mockTechSpace.EXPECT().DoAccountObject(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, fn func(techspace.AccountObject) error) error {
				return fn(fx.mockAccountObj)
			})

		// Execute
		messages, err := fx.inboxClient.fetchMessages()
		require.NoError(t, err)
		require.Len(t, messages, 2)
	})

	t.Run("skip message if signature verification fails", func(t *testing.T) {
		fx := newFixture(t)

		msg1 := fx.createValidMessage(t, "msg1", []byte("test body 1"))
		msg2 := fx.createValidMessage(t, "msg2", []byte("test body 2"))
		// Corrupt signature of msg1
		msg1.Packet.SenderSignature = []byte("invalid")

		// Setup expectations for GetInboxOffset
		fx.mockAccountObj.EXPECT().GetInboxOffset().Return("", nil)
		fx.mockTechSpace.EXPECT().DoAccountObject(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, fn func(techspace.AccountObject) error) error {
				return fn(fx.mockAccountObj)
			})

		fx.mockInboxClient.EXPECT().InboxFetch(gomock.Any(), "").
			Return([]*coordinatorproto.InboxMessage{msg1, msg2}, false, nil)

		// Only msg2 should be decrypted (msg1 fails signature check)
		fx.mockWallet.EXPECT().Account().Return(fx.receiverAccount)

		// Setup expectations for SetInboxOffset (should still be called with msg2 offset)
		fx.mockAccountObj.EXPECT().SetInboxOffset("msg2").Return(nil)
		fx.mockTechSpace.EXPECT().DoAccountObject(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, fn func(techspace.AccountObject) error) error {
				return fn(fx.mockAccountObj)
			})

		// Execute
		messages, err := fx.inboxClient.fetchMessages()
		require.NoError(t, err)
		// Only msg2 should be in the result (msg1 was skipped)
		require.Len(t, messages, 1)
		assert.Equal(t, []byte("test body 2"), messages[0].Packet.Payload.Body)
	})

	t.Run("skip message if decryption fails", func(t *testing.T) {
		fx := newFixture(t)

		msg1 := fx.createValidMessage(t, "msg1", []byte("test body 1"))
		msg2 := fx.createValidMessage(t, "msg2", []byte("test body 2"))

		// Setup expectations for GetInboxOffset
		fx.mockAccountObj.EXPECT().GetInboxOffset().Return("", nil)
		fx.mockTechSpace.EXPECT().DoAccountObject(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, fn func(techspace.AccountObject) error) error {
				return fn(fx.mockAccountObj)
			})

		fx.mockInboxClient.EXPECT().InboxFetch(gomock.Any(), "").
			Return([]*coordinatorproto.InboxMessage{msg1, msg2}, false, nil)

		// Create a different account that cannot decrypt the message
		wrongAccount, err := accountdata.NewRandom()
		require.NoError(t, err)

		// First call returns wrong account (decryption will fail)
		fx.mockWallet.EXPECT().Account().Return(wrongAccount).Once()
		// Second call returns correct account
		fx.mockWallet.EXPECT().Account().Return(fx.receiverAccount).Once()

		// Setup expectations for SetInboxOffset
		fx.mockAccountObj.EXPECT().SetInboxOffset("msg2").Return(nil)
		fx.mockTechSpace.EXPECT().DoAccountObject(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, fn func(techspace.AccountObject) error) error {
				return fn(fx.mockAccountObj)
			})

		// Execute
		messages, err := fx.inboxClient.fetchMessages()
		require.NoError(t, err)
		// Only msg2 should be in the result (msg1 decryption failed)
		require.Len(t, messages, 1)
		assert.Equal(t, []byte("test body 2"), messages[0].Packet.Payload.Body)
	})

	t.Run("handle fetch error", func(t *testing.T) {
		fx := newFixture(t)

		// Setup expectations for GetInboxOffset
		fx.mockAccountObj.EXPECT().GetInboxOffset().Return("", nil)
		fx.mockTechSpace.EXPECT().DoAccountObject(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, fn func(techspace.AccountObject) error) error {
				return fn(fx.mockAccountObj)
			})

		fx.mockInboxClient.EXPECT().InboxFetch(gomock.Any(), "").
			Return(nil, false, errors.New("network error"))

		// Execute - offset should NOT be set when there's an error
		messages, err := fx.inboxClient.fetchMessages()
		require.NoError(t, err)
		require.Len(t, messages, 0)
	})
}

func TestInboxClient_ReceiveNotify(t *testing.T) {
	t.Run("process messages with registered handler", func(t *testing.T) {
		fx := newFixture(t)

		msg := fx.createValidMessage(t, "msg1", []byte("test body"))

		// Register a handler
		var receivedPacket *coordinatorproto.InboxPacket
		err := fx.inboxClient.SetReceiverByType(
			coordinatorproto.InboxPayloadType_InboxPayloadOneToOneInvite,
			func(packet *coordinatorproto.InboxPacket) error {
				receivedPacket = packet
				return nil
			},
		)
		require.NoError(t, err)

		// Setup expectations for GetInboxOffset
		fx.mockAccountObj.EXPECT().GetInboxOffset().Return("", nil)
		fx.mockTechSpace.EXPECT().DoAccountObject(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, fn func(techspace.AccountObject) error) error {
				return fn(fx.mockAccountObj)
			})

		fx.mockInboxClient.EXPECT().InboxFetch(gomock.Any(), "").
			Return([]*coordinatorproto.InboxMessage{msg}, false, nil)

		fx.mockWallet.EXPECT().Account().Return(fx.receiverAccount)

		// Setup expectations for SetInboxOffset
		fx.mockAccountObj.EXPECT().SetInboxOffset("msg1").Return(nil)
		fx.mockTechSpace.EXPECT().DoAccountObject(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, fn func(techspace.AccountObject) error) error {
				return fn(fx.mockAccountObj)
			})

		// Execute
		fx.inboxClient.ReceiveNotify(&coordinatorproto.NotifySubscribeEvent{})

		// Verify handler was called
		require.NotNil(t, receivedPacket)
		assert.Equal(t, []byte("test body"), receivedPacket.Payload.Body)
	})

	t.Run("no handler registered for payload type", func(t *testing.T) {
		fx := newFixture(t)

		msg := fx.createValidMessage(t, "msg1", []byte("test body"))

		// Don't register any handler

		// Setup expectations for GetInboxOffset
		fx.mockAccountObj.EXPECT().GetInboxOffset().Return("", nil)
		fx.mockTechSpace.EXPECT().DoAccountObject(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, fn func(techspace.AccountObject) error) error {
				return fn(fx.mockAccountObj)
			})

		fx.mockInboxClient.EXPECT().InboxFetch(gomock.Any(), "").
			Return([]*coordinatorproto.InboxMessage{msg}, false, nil)

		fx.mockWallet.EXPECT().Account().Return(fx.receiverAccount)

		// Setup expectations for SetInboxOffset
		fx.mockAccountObj.EXPECT().SetInboxOffset("msg1").Return(nil)
		fx.mockTechSpace.EXPECT().DoAccountObject(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, fn func(techspace.AccountObject) error) error {
				return fn(fx.mockAccountObj)
			})

		// Execute - should not panic even without handler
		fx.inboxClient.ReceiveNotify(&coordinatorproto.NotifySubscribeEvent{})
	})
}

func TestInboxClient_SetReceiverByType(t *testing.T) {
	t.Run("register handler successfully", func(t *testing.T) {
		fx := newFixture(t)

		handler := func(packet *coordinatorproto.InboxPacket) error {
			return nil
		}

		err := fx.inboxClient.SetReceiverByType(coordinatorproto.InboxPayloadType_InboxPayloadOneToOneInvite, handler)
		require.NoError(t, err)

		// Verify handler was registered
		_, ok := fx.inboxClient.receivers[coordinatorproto.InboxPayloadType_InboxPayloadOneToOneInvite]
		assert.True(t, ok)
	})

	t.Run("reject nil handler", func(t *testing.T) {
		fx := newFixture(t)

		err := fx.inboxClient.SetReceiverByType(coordinatorproto.InboxPayloadType_InboxPayloadOneToOneInvite, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "handler must be a function but got nil")
	})
}
