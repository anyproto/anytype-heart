package pushnotification

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/anytype-push-server/pushclient/pushapi"
	"github.com/cheggaaa/mb/v3"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/pushnotification/pushclient"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/space"
)

const CName = "core.pushnotification.service"

var log = logging.Logger(CName).Desugar()

type requestSubscribe struct {
	spaceId string
	topics  []string
}

type Service interface {
	app.ComponentRunnable
	RegisterToken(ctx context.Context, req *pb.RpcPushNotificationRegisterTokenRequest) (err error)
	CreateSpace(ctx context.Context, spaceId string) (err error)
	Notify(ctx context.Context, spaceId string, topic []string, payload []byte) (err error)
	SubscribeToTopics(ctx context.Context, spaceId string, topic []string)
}

func New() Service {
	return &service{topicSubscriptions: make(map[spaceKeyType]map[string]*pushapi.Topic)}
}

type service struct {
	pushClient   pushclient.Client
	wallet       wallet.Wallet
	cfg          *config.Config
	spaceService space.Service
	eventSender  event.Sender

	started                 bool
	activeSubscriptionsLock sync.Mutex

	topicSubscriptions map[spaceKeyType]map[string]*pushapi.Topic
	ctx                context.Context
	cancel             context.CancelFunc
	requestsQueue      *mb.MB[requestSubscribe]
}

type spaceKeyType string

func (s *service) SubscribeToTopics(ctx context.Context, spaceId string, topics []string) {
	err := s.requestsQueue.Add(ctx, requestSubscribe{spaceId: spaceId, topics: topics})
	if err != nil {
		log.Error("add topic", zap.Error(err))
	}
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
	return s.requestsQueue.Close()
}

func (s *service) Init(a *app.App) (err error) {
	s.cfg = app.MustComponent[*config.Config](a)
	s.pushClient = app.MustComponent[pushclient.Client](a)
	s.wallet = app.MustComponent[wallet.Wallet](a)
	s.requestsQueue = mb.New[requestSubscribe](0)
	s.spaceService = app.MustComponent[space.Service](a)
	s.eventSender = app.MustComponent[event.Sender](a)
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
	err = s.pushClient.SetToken(ctx, &pushapi.SetTokenRequest{
		Platform: pushapi.Platform(req.Platform),
		Token:    req.Token,
	})
	return err
}

func (s *service) subscribeAll(ctx context.Context) error {
	if !s.started {
		return nil
	}

	s.activeSubscriptionsLock.Lock()
	var allTopics []*pushapi.Topic
	for _, topics := range s.topicSubscriptions {
		for _, topic := range topics {
			allTopics = append(allTopics, topic)
		}
	}
	s.activeSubscriptionsLock.Unlock()

	err := s.pushClient.SubscribeAll(ctx, &pushapi.SubscribeAllRequest{
		Topics: &pushapi.Topics{Topics: allTopics},
	})
	return err
}

func (s *service) CreateSpace(ctx context.Context, spaceId string) (err error) {
	if !s.started {
		return nil
	}
	keys, err := s.getSpaceKeys(spaceId)
	if err != nil {
		return fmt.Errorf("get space keys: %w", err)
	}

	signature, err := keys.signKey.Sign([]byte(s.wallet.GetAccountPrivkey().GetPublic().Account()))
	if err != nil {
		return err
	}
	err = s.pushClient.CreateSpace(ctx, &pushapi.CreateSpaceRequest{
		SpaceKey:         []byte(keys.spaceKey),
		AccountSignature: signature,
	})
	return err
}

type spaceKeys struct {
	// spaceKey is a public part of signKey
	spaceKey spaceKeyType
	signKey  crypto.PrivKey

	// id of current encryption key
	encryptionKeyId string
	encryptionKey   crypto.SymKey
}

func (s *service) Notify(ctx context.Context, spaceId string, topic []string, payload []byte) (err error) {
	if !s.started {
		return nil
	}

	keys, err := s.getSpaceKeys(spaceId)
	if err != nil {
		return fmt.Errorf("get space keys: %w", err)
	}
	topics, err := s.makeTopics(keys.signKey, topic)
	if err != nil {
		return err
	}
	encryptedJson, err := keys.encryptionKey.Encrypt(payload)
	if err != nil {
		return err
	}
	signature, err := s.wallet.GetAccountPrivkey().Sign(encryptedJson)
	if err != nil {
		return err
	}
	p := &pushapi.Message{
		KeyId:     keys.encryptionKeyId,
		Payload:   encryptedJson,
		Signature: signature,
	}
	err = s.pushClient.Notify(ctx, &pushapi.NotifyRequest{
		Topics:  &pushapi.Topics{Topics: topics},
		Message: p,
	})
	return err
}

func (s *service) loadSubscriptions(ctx context.Context) (err error) {
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
	s.activeSubscriptionsLock.Lock()
	for _, topic := range subscriptions.Topics.Topics {
		s.putTopicSubscription(spaceKeyType(topic.SpaceKey), topic)
	}
	s.activeSubscriptionsLock.Unlock()
	return nil
}

func (s *service) addPendingTopicSubscription(spaceKeys *spaceKeys, topic string) (bool, error) {
	s.activeSubscriptionsLock.Lock()
	defer s.activeSubscriptionsLock.Unlock()

	if s.hasTopicSubscription(spaceKeys.spaceKey, topic) {
		return false, nil
	}
	signature, err := spaceKeys.signKey.Sign([]byte(topic))
	if err != nil {
		return false, fmt.Errorf("sign topic: %w", err)
	}
	s.putTopicSubscription(spaceKeys.spaceKey, &pushapi.Topic{
		Topic:     topic,
		SpaceKey:  []byte(spaceKeys.spaceKey),
		Signature: signature,
	})
	return true, nil
}

func (s *service) hasTopicSubscription(spaceKey spaceKeyType, topic string) bool {
	topics, ok := s.topicSubscriptions[spaceKey]
	if !ok {
		return false
	}
	if _, ok := topics[topic]; ok {
		return false
	}
	return true
}

func (s *service) putTopicSubscription(spaceKey spaceKeyType, topic *pushapi.Topic) bool {
	topics, ok := s.topicSubscriptions[spaceKey]
	if !ok {
		topics = map[string]*pushapi.Topic{}
		s.topicSubscriptions[spaceKey] = topics
	}
	if _, ok := topics[topic.Topic]; ok {
		return false
	}
	topics[topic.Topic] = topic
	return true
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
	// comment out to avoid triggering old subs
	//
	// err := s.loadSubscriptions(s.ctx)
	// if err != nil {
	// 	log.Error("failed to load subscriptions", zap.Error(err))
	// }

	for {
		requests, err := s.requestsQueue.Wait(s.ctx)
		if err != nil {
			return
		}

		var shouldUpdateSubscriptions bool
		for _, req := range requests {
			keys, err := s.getSpaceKeys(req.spaceId)
			if err != nil {
				log.Error("failed to get space keys", zap.Error(err))
			}

			for _, topic := range req.topics {
				shouldUpdate, err := s.addPendingTopicSubscription(keys, topic)
				if err != nil {
					log.Error("failed to add pending topic subscription: ", zap.Error(err))
					continue
				}
				if shouldUpdate {
					shouldUpdateSubscriptions = true
				}
			}
		}
		if !shouldUpdateSubscriptions {
			continue
		}
		err = s.subscribeAll(s.ctx)
		if err != nil {
			log.Error("failed to subscribe to topic", zap.Error(err))
		}
	}
}

func (s *service) BroadcastKeyUpdate(spaceId string, aclState *list.AclState) error {
	keys, err := s.getSpaceKeysFromAcl(aclState)
	if err != nil {
		return fmt.Errorf("get space keys: %w", err)
	}

	raw, err := keys.encryptionKey.Raw()
	if err != nil {
		return err
	}
	encodedKey := base64.StdEncoding.EncodeToString(raw)
	s.eventSender.Broadcast(&pb.Event{
		Messages: []*pb.EventMessage{
			{
				SpaceId: spaceId,
				Value: &pb.EventMessageValueOfPushEncryptionKeyUpdate{
					PushEncryptionKeyUpdate: &pb.EventPushEncryptionKeyUpdate{
						EncryptionKeyId: keys.encryptionKeyId,
						EncryptionKey:   encodedKey,
					},
				},
			},
		},
	})
	return nil
}

func (s *service) getSpaceKeys(spaceId string) (*spaceKeys, error) {
	space, err := s.spaceService.Get(s.ctx, spaceId)
	if err != nil {
		return nil, fmt.Errorf("get space: %w", err)
	}
	state := space.CommonSpace().Acl().AclState()
	return s.getSpaceKeysFromAcl(state)
}

func (s *service) getSpaceKeysFromAcl(state *list.AclState) (*spaceKeys, error) {
	firstMetadataKey, err := state.FirstMetadataKey()
	if err != nil {
		return nil, fmt.Errorf("get first metadata key: %w", err)
	}
	signKey, err := deriveSpaceKey(firstMetadataKey)
	if err != nil {
		return nil, fmt.Errorf("derive space key: %w", err)
	}
	symKey, err := state.CurrentReadKey()
	if err != nil {
		return nil, fmt.Errorf("get current read key: %w", err)
	}

	spaceKey, err := signKey.GetPublic().Raw()
	if err != nil {
		return nil, fmt.Errorf("get raw space public key: %w", err)
	}

	readKeyId := state.CurrentReadKeyId()
	hasher := sha256.New()
	encryptionKeyId := hex.EncodeToString(hasher.Sum([]byte(readKeyId)))

	encryptionKey, err := deriveSymmetricKey(symKey)
	if err != nil {
		return nil, err
	}
	return &spaceKeys{
		spaceKey:        spaceKeyType(spaceKey),
		encryptionKey:   encryptionKey,
		encryptionKeyId: encryptionKeyId,
		signKey:         signKey,
	}, nil
}
