package pushnotification

import (
	"context"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
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
	"github.com/anyproto/anytype-heart/space"
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
	s.wakeUpCh = make(chan struct{}, 1)
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
	/*
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

	*/
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
	/*keys, err := s.getSpaceKeys(message.SpaceId)
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
	})*/
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
	// s.loadRemoteSubscriptions()
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

	return
}

func (s *service) collectSpaceViewsInfo() (needCallApi bool) {
	return
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
