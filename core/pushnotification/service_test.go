package pushnotification

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/anytype-push-server/pushclient/pushapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/space/spacecore/spacekeystore"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

const (
	spaceId     = "spaceId"
	topic       = "test"
	aclRecordId = "aclRecordId"
)

type testPayload struct {
	Text string `json:"text"`
}

type mockPushClient struct {
	t                *testing.T
	expectedNotify   *pushapi.NotifyRequest
	expectedTokenReq *pushapi.SetTokenRequest
}

func (m *mockPushClient) Subscriptions(ctx context.Context, req *pushapi.SubscriptionsRequest) (resp *pushapi.SubscriptionsResponse, err error) {
	return &pushapi.SubscriptionsResponse{}, nil
}

func (m *mockPushClient) Init(a *app.App) (err error) {
	return nil
}

func (m *mockPushClient) Name() (name string) {
	return "mockPushClient"
}

func (m *mockPushClient) SetToken(ctx context.Context, req *pushapi.SetTokenRequest) (resp *pushapi.Ok, err error) {
	m.expectedTokenReq = req
	return &pushapi.Ok{}, nil
}

func (m *mockPushClient) SubscribeAll(ctx context.Context, req *pushapi.SubscribeAllRequest) (resp *pushapi.Ok, err error) {
	return &pushapi.Ok{}, nil
}

func (m *mockPushClient) CreateSpace(ctx context.Context, req *pushapi.CreateSpaceRequest) (resp *pushapi.Ok, err error) {
	return &pushapi.Ok{}, nil
}

func (m *mockPushClient) Notify(ctx context.Context, req *pushapi.NotifyRequest) (resp *pushapi.Ok, err error) {
	m.expectedNotify = req
	return &pushapi.Ok{}, nil
}

func TestNotify(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// given
		wallet := mock_wallet.NewMockWallet(t)
		accKey, _, err := crypto.GenerateEd25519Key(rand.Reader)
		wallet.EXPECT().GetAccountPrivkey().Return(accKey)

		sender := mock_event.NewMockSender(t)
		a := &app.App{}
		a.Register(testutil.PrepareMock(context.TODO(), a, sender))
		store := spacekeystore.New()
		err = store.Init(a)
		assert.NoError(t, err)
		readKey := crypto.NewAES()
		privKey, _, err := crypto.GenerateEd25519Key(rand.Reader)
		assert.NoError(t, err)
		sender.EXPECT().Broadcast(mock.Anything).Return()
		store.SyncKeysFromAclState(spaceId, aclRecordId, privKey, readKey)

		pushClient := &mockPushClient{t: t}
		s := &service{
			started:       true,
			pushClient:    pushClient,
			wallet:        wallet,
			spaceKeyStore: store,
		}
		tp := &testPayload{Text: "test"}
		payload, err := json.Marshal(tp)
		assert.NoError(t, err)

		// when
		err = s.Notify(context.TODO(), spaceId, []string{topic}, payload)

		// then
		assert.NoError(t, err)
		assert.Len(t, pushClient.expectedNotify.Topics.Topics, 1)
		assert.Equal(t, topic, pushClient.expectedNotify.Topics.Topics[0].Topic)

		spaceKey, err := store.SignKeyBySpaceId(spaceId)
		raw, err := spaceKey.GetPublic().Raw()
		assert.NoError(t, err)
		assert.Equal(t, raw, pushClient.expectedNotify.Topics.Topics[0].SpaceKey)
		verify, err := spaceKey.GetPublic().Verify([]byte(topic), pushClient.expectedNotify.Topics.Topics[0].Signature)
		assert.NoError(t, err)
		assert.True(t, verify)

		keyBySpaceId, err := store.KeyBySpaceId(spaceId)
		assert.NoError(t, err)
		assert.Equal(t, keyBySpaceId, pushClient.expectedNotify.Message.KeyId)

		symKey, err := store.EncryptionKeyBySpaceId(keyBySpaceId)
		assert.NoError(t, err)
		decryptedJson, err := symKey.Decrypt(pushClient.expectedNotify.Message.Payload)
		assert.NoError(t, err)

		expectedMsg := &testPayload{}
		err = json.Unmarshal(decryptedJson, expectedMsg)
		assert.NoError(t, err)
		assert.Equal(t, expectedMsg.Text, tp.Text)
	})
	t.Run("error not found", func(t *testing.T) {
		// given
		wallet := mock_wallet.NewMockWallet(t)
		store := spacekeystore.New()
		pushClient := &mockPushClient{t: t}
		s := &service{
			pushClient:    pushClient,
			wallet:        wallet,
			spaceKeyStore: store,
		}
		tp := &testPayload{Text: "test"}
		payload, err := json.Marshal(tp)
		assert.NoError(t, err)

		// when
		err = s.Notify(context.TODO(), spaceId, []string{topic}, payload)

		// then
		assert.ErrorIs(t, err, spacekeystore.ErrNotFound)
	})
}

func TestRegisterToken(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// given
		wallet := mock_wallet.NewMockWallet(t)
		pushClient := &mockPushClient{t: t}
		s := &service{
			pushClient:    pushClient,
			wallet:        wallet,
			spaceKeyStore: spacekeystore.New(),
		}

		// when
		err := s.RegisterToken(context.TODO(), &pb.RpcPushNotificationRegisterTokenRequest{
			Token:    "token",
			Platform: pb.RpcPushNotificationRegisterToken_IOS,
		})

		// then
		assert.NoError(t, err)
		assert.Equal(t, "token", pushClient.expectedTokenReq.Token)
		assert.Equal(t, pushapi.Platform_IOS, pushClient.expectedTokenReq.Platform)
	})
	t.Run("token empty", func(t *testing.T) {
		// given
		s := &service{}

		// when
		err := s.RegisterToken(context.TODO(), &pb.RpcPushNotificationRegisterTokenRequest{
			Token:    "",
			Platform: pb.RpcPushNotificationRegisterToken_IOS,
		})

		// then
		assert.Error(t, err)
	})
}
