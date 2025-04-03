package pushnotifcation

import (
	"context"
	"fmt"
	"slices"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/anytype-push-server/pushclient/pushapi"
	"github.com/cheggaaa/mb/v3"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/pushnotifcation/client"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/space/spacecore/spacekeystore"
)

const CName = "core.pushnotification.service"

var log = logging.Logger(CName)

type newSubscription struct {
	SpaceId string
	Topic   string
}

type Service interface {
	app.ComponentRunnable
	RegisterToken(ctx context.Context, req *pb.RpcPushNotificationRegisterTokenRequest) (err error)
	SubscribeAll(ctx context.Context, spaceId string, topics []string) (err error)
	CreateSpace(ctx context.Context, spaceId string) (err error)
	Notify(ctx context.Context, spaceId string, topic []string, payload []byte) (err error)
	SubscribeToTopic(ctx context.Context, spaceId string, topic string)
}

func New() Service {
	return &service{activeSubscriptions: make(map[string][]string)}
}

type service struct {
	pushClient          client.Client
	wallet              wallet.Wallet
	spaceKeyStore       spacekeystore.Store
	cfg                 *config.Config
	started             bool
	activeSubscriptions map[string][]string
	ctx                 context.Context
	cancel              context.CancelFunc
	batcher             *mb.MB[newSubscription]
}

func (s *service) SubscribeToTopic(ctx context.Context, spaceId string, topic string) {
	err := s.batcher.Add(ctx, newSubscription{spaceId, topic})
	if err != nil {
		log.Errorf("add topic err: %v", err)
	}
	return
}

func (s *service) Run(ctx context.Context) (err error) {
	if s.cfg.IsLocalOnlyMode() {
		return nil
	}
	s.started = true
	s.ctx, s.cancel = context.WithCancel(ctx)
	go s.run()
	return nil
}

func (s *service) Close(ctx context.Context) (err error) {
	if !s.started {
		return nil
	}
	if s.cancel != nil {
		s.cancel()
	}
	return s.batcher.Close()
}

func (s *service) Init(a *app.App) (err error) {
	s.cfg = app.MustComponent[*config.Config](a)
	s.pushClient = app.MustComponent[client.Client](a)
	s.wallet = app.MustComponent[wallet.Wallet](a)
	s.spaceKeyStore = app.MustComponent[spacekeystore.Store](a)
	s.batcher = mb.New[newSubscription](0)
	return
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) RegisterToken(ctx context.Context, req *pb.RpcPushNotificationRegisterTokenRequest) (err error) {
	if !s.started {
		return nil
	}
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
	if !s.started {
		return nil
	}
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
	if !s.started {
		return nil
	}
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
	if !s.started {
		return nil
	}
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

func (s *service) fillSubscriptions(ctx context.Context) (err error) {
	if !s.started {
		return nil
	}
	subscriptions, err := s.pushClient.Subscriptions(ctx, &pushapi.SubscriptionsRequest{})
	if err != nil {
		return err
	}
	for _, topic := range subscriptions.Topics.Topics {
		s.activeSubscriptions[string(topic.SpaceKey)] = append(s.activeSubscriptions[string(topic.SpaceKey)], topic.Topic)
	}
	return nil
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

func (s *service) run() {
	select {
	case <-s.ctx.Done():
		return
	default:
	}
	err := s.fillSubscriptions(s.ctx)
	if err != nil {
		return
	}

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}
		msgs, err := s.batcher.Wait(s.ctx)
		if err != nil {
			return
		}
		if len(msgs) == 0 {
			return
		}
		for _, sub := range msgs {
			activeTopics, ok := s.activeSubscriptions[sub.SpaceId]
			if ok && slices.Contains(activeTopics, sub.Topic) {
				continue
			}
			activeTopics = append(activeTopics, sub.Topic)
			s.activeSubscriptions[sub.SpaceId] = activeTopics
			err := s.SubscribeAll(s.ctx, sub.SpaceId, activeTopics)
			if err != nil {
				log.Errorf("failed to subscribe to topic %s: %v", sub.Topic, err)
			}
		}
	}
}
