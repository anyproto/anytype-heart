package pushnotifcation

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/anytype-push-server/pushclient/pushapi"

	"github.com/anyproto/anytype-heart/core/pushnotifcation/client"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pb"

	"github.com/anyproto/anytype-heart/space/spacecore/spacekeystore"
)

const CName = "core.pushnotification.service"

const topicName = "chat"

type Service interface {
	app.Component
	RegisterToken(ctx context.Context, req *pb.RpcPushNotificationRegisterTokenRequest) (err error)
	SubscribeAll(ctx context.Context, spaceId string) (err error)
	CreateSpace(ctx context.Context, spaceId string) (err error)
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
	signature, err := s.wallet.GetAccountPrivkey().Sign([]byte(req.Token))
	if err != nil {
		return err
	}
	_, err = s.pushClient.SetToken(ctx, &pushapi.SetTokenRequest{
		Platform:  pushapi.Platform(req.Platform),
		Token:     req.Token,
		Signature: signature,
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *service) SubscribeAll(ctx context.Context, spaceId string) (err error) {
	topics, err := s.makeTopics(spaceId)
	if err != nil {
		return err
	}
	_, err = s.pushClient.SubscribeAll(ctx, &pushapi.SubscribeAllRequest{
		Topics: topics,
	})
	if err != nil {
		return err
	}
	return nil
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
	if err != nil {
		return err
	}
	return nil
}

func (s *service) makeTopics(spaceId string) (*pushapi.Topics, error) {
	signature, err := s.wallet.GetAccountPrivkey().Sign([]byte(topicName))
	if err != nil {
		return nil, err
	}
	spaceKey, err := s.spaceKeyStore.EncryptionKeyBySpaceId(spaceId)
	if err != nil {
		return nil, err
	}
	pubKey := spaceKey.GetPublic()
	rawKey, err := pubKey.Raw()
	if err != nil {
		return nil, err
	}
	topics := &pushapi.Topics{Topics: []*pushapi.Topic{
		{
			SpaceKey:  rawKey,
			Topic:     topicName,
			Signature: signature,
		},
	}}
	return topics, nil
}
