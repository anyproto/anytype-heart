package pushnotification

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/anytype-push-server/pushclient/pushapi"
	"github.com/cheggaaa/mb/v3"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/pushnotification/pushclient"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/objectsubscription"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

const CName = "core.pushnotification.service"

var log = logging.Logger(CName)

type Service interface {
	app.ComponentRunnable
	RegisterToken(req *pb.RpcPushNotificationRegisterTokenRequest)
	Notify(ctx context.Context, spaceId string, topic []string, payload []byte) (err error)
}

func New() Service {
	return &service{}
}

type PushNotification struct {
	SpaceId  string
	Topics   []string
	Payload  []byte
	Deadline time.Time
}

type service struct {
	pushClient           pushclient.Client
	wallet               wallet.Wallet
	config               *config.Config
	spaceService         space.Service
	eventSender          event.Sender
	subscriptionsService subscription.Service

	spaceViewSubscription *objectsubscription.ObjectSubscription[spaceViewStatus]

	token    string
	platform pushapi.Platform

	isTokenRegistered bool

	allSpaceTopics map[string]*SpaceTopics

	notifyQueue  *mb.MB[PushNotification]
	mu           sync.Mutex
	runCtx       context.Context
	runCtxCancel context.CancelFunc

	wakeUpCh chan struct{}
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Init(a *app.App) (err error) {
	s.config = app.MustComponent[*config.Config](a)
	s.pushClient = app.MustComponent[pushclient.Client](a)
	s.wallet = app.MustComponent[wallet.Wallet](a)
	s.spaceService = app.MustComponent[space.Service](a)
	s.eventSender = app.MustComponent[event.Sender](a)
	s.subscriptionsService = app.MustComponent[subscription.Service](a)
	s.allSpaceTopics = make(map[string]*SpaceTopics)
	s.notifyQueue = mb.New[PushNotification](0)
	return
}

func (s *service) Run(_ context.Context) (err error) {
	if s.config.IsLocalOnlyMode() {
		return nil
	}
	s.runCtx, s.runCtxCancel = context.WithCancel(context.Background())
	if s.spaceViewSubscription, err = newSpaceViewSubscription(s.subscriptionsService, s.spaceService.TechSpaceId(), s.wakeUp); err != nil {
		return err
	}
	go s.run()
	return nil
}

func (s *service) wakeUp() {
	select {
	case s.wakeUpCh <- struct{}{}:
	default:
	}
}

func (s *service) RegisterToken(req *pb.RpcPushNotificationRegisterTokenRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.token = req.Token
	s.platform = pushapi.Platform(req.Platform)
	s.isTokenRegistered = false
	s.wakeUp()
	return
}

func (s *service) createSpace(ctx context.Context, spaceId string) (err error) {
	keys, err := s.getSpaceKeys(spaceId)
	if err != nil {
		return fmt.Errorf("get space keys: %w", err)
	}

	signature, err := keys.spaceKeyPrivate.Sign([]byte(s.wallet.GetAccountPrivkey().GetPublic().Account()))
	if err != nil {
		return err
	}
	err = s.pushClient.CreateSpace(ctx, &pushapi.CreateSpaceRequest{
		SpaceKey:         keys.spaceKey,
		AccountSignature: signature,
	})
	s.mu.Lock()
	if spaceTopics, ok := s.allSpaceTopics[keys.spaceKeyString]; ok {
		spaceTopics.needCreateSpace = false
	}
	s.mu.Unlock()
	return err
}

func (s *service) Notify(ctx context.Context, spaceId string, topic []string, payload []byte) (err error) {
	return s.notifyQueue.Add(ctx, PushNotification{
		SpaceId: spaceId,
		Topics:  topic,
		Payload: payload,
	})
}

func (s *service) notify(ctx context.Context, message PushNotification) (err error) {
	keys, err := s.getSpaceKeys(message.SpaceId)
	if err != nil {
		return fmt.Errorf("get space keys: %w", err)
	}
	topics, err := s.makeTopics(keys.spaceKeyPrivate, message.Topics)
	if err != nil {
		return err
	}
	encryptedJson, err := keys.encryptionKey.Encrypt(message.Payload)
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
	var err error
	s.loadRemoteSubscriptions()
	s.wakeUp()
	for {
		select {
		case <-s.runCtx.Done():
			return
		case <-s.wakeUpCh:
		case <-time.After(time.Minute * 5):
		}
		st := time.Now()
		if err = s.registerToken(); err != nil {
			log.Error("register token", zap.Error(err))
		}
		if err = s.sync(); err != nil {
			log.Error("sync error", zap.Error(err), zap.Duration("dur", time.Since(st)))
		} else {
			log.Info("sync success", zap.Duration("dur", time.Since(st)))
		}
	}
}

func (s *service) sendNotificationsLoop() {
	for {
		message, err := s.notifyQueue.WaitOne(s.runCtx)
		if err != nil {
			return
		}
		for range 6 {
			if err := s.notify(s.runCtx, message); err != nil {
				log.Warn("notify error", zap.Error(err))
			} else {
				break
			}
			select {
			case <-s.runCtx.Done():
			case <-time.After(time.Second * 10):
			}
		}
	}
}

func (s *service) loadRemoteSubscriptions() {
	var timeout time.Duration
	for {
		resp, err := s.pushClient.Subscriptions(s.runCtx, &pushapi.SubscriptionsRequest{})
		if err != nil {
			log.Error("load remote subscriptions", zap.Error(err))
			if timeout < time.Minute {
				timeout += time.Second
			}
			select {
			case <-s.runCtx.Done():
				return
			case <-time.After(timeout):
				continue
			}
		}
		for _, remoteTopic := range resp.Topics.GetTopics() {
			var (
				spaceTopics *SpaceTopics
				ok          bool
			)
			if spaceTopics, ok = s.allSpaceTopics[string(remoteTopic.SpaceKey)]; !ok {
				spaceTopics = &SpaceTopics{
					topics: newTopicSet(),
				}
				s.allSpaceTopics[string(remoteTopic.SpaceKey)] = spaceTopics
			}
			spaceTopics.topics.Add(remoteTopic.Topic)
		}
	}
}

func (s *service) registerToken() (err error) {
	s.mu.Lock()
	if s.isTokenRegistered || s.token == "" {
		s.mu.Unlock()
		return
	}
	token := s.token
	platform := s.platform
	s.mu.Unlock()
	return s.pushClient.SetToken(s.runCtx, &pushapi.SetTokenRequest{
		Platform: platform,
		Token:    token,
	})
}

func (s *service) sync() (err error) {
	s.mu.Lock()
	needApiCall := s.collectSpaceViewsInfo()
	if !needApiCall {
		s.mu.Unlock()
		return
	}

	var (
		allTopics        []*pushapi.Topic
		createSpaceIds   []string
		needUpdateTopics bool
	)
	for _, spaceTopics := range s.allSpaceTopics {
		if spaceTopics.spaceKeys == nil {
			continue
		}
		topics, err := s.makeTopics(spaceTopics.spaceKeys.spaceKeyPrivate, spaceTopics.topics.Slice())
		if err != nil {
			log.Error("create topics", zap.Error(err))
			continue
		}
		allTopics = append(allTopics, topics...)
		if spaceTopics.needCreateSpace {
			createSpaceIds = append(createSpaceIds, spaceTopics.spaceId)
		}
		if spaceTopics.needUpdateTopics {
			needUpdateTopics = true
		}
	}
	s.mu.Unlock()
	if len(createSpaceIds) > 0 {
		for _, createSpaceId := range createSpaceIds {
			if err := s.createSpace(s.runCtx, createSpaceId); err != nil {
				return fmt.Errorf("create space: %w", err)
			}
		}
	}
	if needUpdateTopics {
		err := s.pushClient.SubscribeAll(s.runCtx, &pushapi.SubscribeAllRequest{
			Topics: &pushapi.Topics{Topics: allTopics},
		})
		if err != nil {
			return fmt.Errorf("subscribe topics: %w", err)
		}
	}
	return
}

func (s *service) collectSpaceViewsInfo() (needCallApi bool) {
	markNeedCallApi := func() {
		if !needCallApi {
			needCallApi = true
		}
	}
	s.spaceViewSubscription.Iterate(func(id string, data spaceViewStatus) bool {
		keys, err := s.getSpaceKeys(id)
		if err != nil {
			log.Error("get space keys", zap.Error(err))
		}
		spaceTopics, ok := s.allSpaceTopics[keys.spaceKeyString]
		if !ok {
			needCreateSpace := keys.isOwnerAndShared && data.status != spaceinfo.LocalStatusMissing
			spaceTopics = &SpaceTopics{
				spaceId:         data.spaceId,
				topics:          newTopicSet(),
				needCreateSpace: needCreateSpace,
			}
			if needCreateSpace {
				markNeedCallApi()
			}
			s.allSpaceTopics[keys.spaceKeyString] = spaceTopics
		}
		spaceTopics.spaceId = data.spaceId
		spaceTopics.spaceKeys = keys
		var topics []string
		switch data.status {
		case spaceinfo.LocalStatusOk, spaceinfo.LocalStatusLoading:
			switch data.topics {
			case model.PushNotificationTopics_All:
				topics = chatTopics
			case model.PushNotificationTopics_Mention:
				topics = []string{s.wallet.Account().SignKey.GetPublic().Account()}
			}
			if spaceTopics.topics.Set(topics...) {
				spaceTopics.needUpdateTopics = true
				markNeedCallApi()
			}
		case spaceinfo.LocalStatusMissing:
			if spaceTopics.topics.Set() {
				spaceTopics.needUpdateTopics = true
				markNeedCallApi()
			}
		}
		return true
	})
	return
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

	// send event to the client
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

	// update keys in allSpaceTopics
	s.mu.Lock()
	defer s.mu.Unlock()
	if spaceTopic, ok := s.allSpaceTopics[keys.spaceKeyString]; ok {
		spaceTopic.spaceKeys = keys
	}
	return nil
}

func (s *service) getSpaceKeys(spaceId string) (*spaceKeys, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, spaceTopics := range s.allSpaceTopics {
		if spaceTopics.spaceId == spaceId {
			if spaceTopics.spaceKeys != nil {
				return spaceTopics.spaceKeys, nil
			} else {
				return s.loadSpaceKeys(spaceId)
			}
		}
	}
	return s.loadSpaceKeys(spaceId)
}

func (s *service) loadSpaceKeys(spaceId string) (*spaceKeys, error) {
	space, err := s.spaceService.Get(s.runCtx, spaceId)
	if err != nil {
		return nil, fmt.Errorf("get space: %w", err)
	}
	space.CommonSpace().Acl().Lock()
	defer space.CommonSpace().Acl().Unlock()
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
	var isOwner bool
	for _, acc := range state.CurrentAccounts() {
		if acc.Permissions.IsOwner() {
			if s.wallet.Account().SignKey.GetPublic().Account() == acc.PubKey.Account() {
				isOwner = true
			}
		}
	}
	return &spaceKeys{
		spaceKey:        spaceKey,
		spaceKeyString:  string(spaceKey),
		spaceKeyPrivate: signKey,

		encryptionKey:    encryptionKey,
		encryptionKeyId:  encryptionKeyId,
		isOwnerAndShared: isOwner && len(state.Invites()) > 0,
	}, nil
}

func (s *service) Close(ctx context.Context) (err error) {
	if s.runCtxCancel != nil {
		s.runCtxCancel()
	}
	if s.spaceViewSubscription != nil {
		s.spaceViewSubscription.Close()
	}
	return s.notifyQueue.Close()
}
