package pushnotifcation

import (
	"context"
	"fmt"
	"sync"

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
	Topics  []string
}

type Service interface {
	app.ComponentRunnable
	RegisterToken(ctx context.Context, req *pb.RpcPushNotificationRegisterTokenRequest) (err error)
	CreateSpace(ctx context.Context, spaceId string) (err error)
	Notify(ctx context.Context, spaceId string, topic []string, payload []byte) (err error)
	SubscribeToTopics(ctx context.Context, spaceId string, topic []string)
}

func New() Service {
	return &service{activeSubscriptions: make(map[string]TopicSet)}
}

type service struct {
	pushClient              client.Client
	wallet                  wallet.Wallet
	spaceKeyStore           spacekeystore.Store
	cfg                     *config.Config
	started                 bool
	activeSubscriptions     map[string]TopicSet
	activeSubscriptionsLock sync.Mutex
	ctx                     context.Context
	cancel                  context.CancelFunc
	batcher                 *mb.MB[newSubscription]
}

func (s *service) SubscribeToTopics(ctx context.Context, spaceId string, topics []string) {
	err := s.batcher.Add(ctx, newSubscription{spaceId, topics})
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
	s.ctx, s.cancel = context.WithCancel(context.Background())
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

func (s *service) subscribeAll(ctx context.Context) error {
	if !s.started {
		return nil
	}
	topics := s.buildPushTopics()
	_, err := s.pushClient.SubscribeAll(ctx, &pushapi.SubscribeAllRequest{
		Topics: &pushapi.Topics{Topics: topics},
	})
	return err
}

func (s *service) buildPushTopics() []*pushapi.Topic {
	s.activeSubscriptionsLock.Lock()
	defer s.activeSubscriptionsLock.Unlock()
	var allTopics []*pushapi.Topic
	for spaceKey, topicsSet := range s.activeSubscriptions {
		spaceTopics, err := s.createTopicsForSpace(spaceKey, topicsSet.Slice())
		if err != nil {
			continue
		}
		allTopics = append(allTopics, spaceTopics...)
	}
	return allTopics
}

func (s *service) createTopicsForSpace(spaceKey string, topicNames []string) ([]*pushapi.Topic, error) {
	privKey, err := s.spaceKeyStore.EncryptionKeyByKeyId(spaceKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get encryption key for space %s: %w", spaceKey, err)
	}
	topics, err := s.makeTopics(privKey, topicNames)
	if err != nil {
		return nil, fmt.Errorf("failed to make topics for space %s: %w", spaceKey, err)
	}
	return topics, nil
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
	topics, err := s.makeTopicsBySpaceId(spaceId, topic)
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
	if subscriptions == nil || subscriptions.Topics == nil {
		return nil
	}
	for _, topic := range subscriptions.Topics.Topics {
		spaceKey := string(topic.SpaceKey)
		s.activeSubscriptionsLock.Lock()
		if _, ok := s.activeSubscriptions[spaceKey]; !ok {
			s.activeSubscriptions[spaceKey] = NewTopicSet()
		}
		topicSet := s.activeSubscriptions[spaceKey]
		topicSet.Add(topic.Topic)
		s.activeSubscriptionsLock.Unlock()
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

func (s *service) makeTopicsBySpaceId(spaceId string, topics []string) (*pushapi.Topics, error) {
	spaceKey, err := s.spaceKeyStore.EncryptionKeyBySpaceId(spaceId)
	if err != nil {
		return nil, err
	}
	pushApiTopics, err := s.makeTopics(spaceKey, topics)
	if err != nil {
		return nil, err
	}
	return &pushapi.Topics{Topics: pushApiTopics}, nil
}

func (s *service) makeTopics(spaceKey crypto.PrivKey, topics []string) ([]*pushapi.Topic, error) {
	pushApiTopics := make([]*pushapi.Topic, 0, len(topics))
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
	return pushApiTopics, nil
}

func (s *service) run() {
	select {
	case <-s.ctx.Done():
		return
	default:
	}
	err := s.fillSubscriptions(s.ctx)
	if err != nil {
		log.Error("failed to fill subscriptions: ", err)
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
		var shouldUpdate bool
		for _, sub := range msgs {
			shouldUpdate, err = s.addNewSubscription(sub)
			if err != nil {
				log.Errorf("failed to get space key from keystore for space %s: %v", sub.SpaceId, err)
				continue
			}
		}
		if !shouldUpdate {
			continue
		}
		err = s.subscribeAll(s.ctx)
		if err != nil {
			log.Errorf("failed to subscribe to topic: %v", err)
		}
	}
}

func (s *service) addNewSubscription(sub newSubscription) (shouldUpdate bool, err error) {
	keyId, err := s.spaceKeyStore.KeyBySpaceId(sub.SpaceId)
	if err != nil {
		return false, err
	}
	s.activeSubscriptionsLock.Lock()
	defer s.activeSubscriptionsLock.Unlock()
	activeTopics, ok := s.activeSubscriptions[keyId]
	if !ok {
		activeTopics = NewTopicSet()
	}
	for _, topic := range sub.Topics {
		if activeTopics.Add(topic) {
			shouldUpdate = true
		}
	}
	s.activeSubscriptions[keyId] = activeTopics
	return shouldUpdate, nil
}
