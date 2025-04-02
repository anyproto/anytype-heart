package pushnotifcation

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/anytype-push-server/pushclient/pushapi"

	"github.com/anyproto/anytype-heart/core/pushnotifcation/client"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/space/spacecore/spacekeystore"
)

const CName = "core.pushnotification.service"

type Service interface {
	app.Component
	RegisterToken(ctx context.Context, req *pb.RpcPushNotificationRegisterTokenRequest) (err error)
	SubscribeAll(ctx context.Context, spaceId string, topics []string) (err error)
	CreateSpace(ctx context.Context, spaceId string) (err error)
	Notify(ctx context.Context, spaceId string, topic []string, payload []byte) (err error)
}

func New() Service {
	return &service{}
}

type service struct {
	pushClient    client.Client
	wallet        wallet.Wallet
	spaceKeyStore spacekeystore.Store
}

func (s *service) Init(a *app.App) (err error) {
	s.pushClient = client.NewClient()
	s.wallet = app.MustComponent[wallet.Wallet](a)
	s.spaceKeyStore = app.MustComponent[spacekeystore.Store](a)
	return
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) RegisterToken(ctx context.Context, req *pb.RpcPushNotificationRegisterTokenRequest) (err error) {
	if req.Token == "" {
		return fmt.Errorf("token is empty")
	}
	_, err = s.pushClient.SetToken(ctx, &pushapi.SetTokenRequest{
		Platform: pushapi.Platform(req.Platform),
		Token:    req.Token,
	})
	return err
}

func (s *service) SubscribeAll(ctx context.Context, spaceId string, topics []string) (err error) {
	pushApiTopics, err := s.makeTopics(spaceId, topics)
	if err != nil {
		return err
	}
	_, err = s.pushClient.SubscribeAll(ctx, &pushapi.SubscribeAllRequest{
		Topics: pushApiTopics,
	})
	return err
}

func (s *service) CreateSpace(ctx context.Context, spaceId string) (err error) {
	spaceKey, err := s.spaceKeyStore.EncryptionKeyBySpaceId(spaceId)
	if err != nil {
		return err
	}
	signature, err := spaceKey.Sign([]byte(s.wallet.GetAccountPrivkey().GetPublic().Account()))
	if err != nil {
		return err
	}
	pubKey := spaceKey.GetPublic()
	rawKey, err := pubKey.Raw()
	if err != nil {
		return err
	}
	_, err = s.pushClient.CreateSpace(ctx, &pushapi.CreateSpaceRequest{
		SpaceKey:         rawKey,
		AccountSignature: signature,
	})
	return err
}

func (s *service) Notify(ctx context.Context, spaceId string, topic []string, payload []byte) (err error) {
	topics, err := s.makeTopics(spaceId, topic)
	if err != nil {
		return err
	}
	keyId, err := s.spaceKeyStore.KeyBySpaceId(spaceId)
	if err != nil {
		return err
	}
	key, err := s.spaceKeyStore.EncryptionKeyBySpaceId(spaceId)
	if err != nil {
		return err
	}
	encryptedJson, err := s.prepareEncryptedJson(key, payload)
	if err != nil {
		return err
	}
	signature, err := s.wallet.GetAccountPrivkey().Sign(encryptedJson)
	if err != nil {
		return err
	}
	p := &pushapi.Message{
		KeyId:     keyId,
		Payload:   encryptedJson,
		Signature: signature,
	}
	_, err = s.pushClient.Notify(ctx, &pushapi.NotifyRequest{
		Topics:  topics,
		Message: p,
	})
	return err
}

func (s *service) prepareEncryptedJson(key crypto.PrivKey, payload []byte) ([]byte, error) {
	encryptedJson, err := key.GetPublic().Encrypt(payload)
	if err != nil {
		return nil, err
	}
	return encryptedJson, nil
}

func (s *service) makeTopics(spaceId string, topics []string) (*pushapi.Topics, error) {
	pushApiTopics := make([]*pushapi.Topic, 0, len(topics))
	spaceKey, err := s.spaceKeyStore.EncryptionKeyBySpaceId(spaceId)
	if err != nil {
		return nil, err
	}
	pubKey := spaceKey.GetPublic()
	rawKey, err := pubKey.Raw()
	if err != nil {
		return nil, err
	}
	for _, topic := range topics {
		signature, err := spaceKey.Sign([]byte(topic))
		if err != nil {
			return nil, err
		}
		pushApiTopics = append(pushApiTopics, &pushapi.Topic{
			SpaceKey:  rawKey,
			Topic:     topic,
			Signature: signature,
		})
	}
	return &pushapi.Topics{Topics: pushApiTopics}, nil
}
