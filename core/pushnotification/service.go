package pushnotification

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/anytype-push-server/pushclient/pushapi"
	"github.com/cheggaaa/mb/v3"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/pushnotification/pushclient"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/objectsubscription"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/space"
)

const CName = "core.pushnotification.service"

var log = logger.NewNamed(CName)

type Service interface {
	app.ComponentRunnable
	RegisterToken(req *pb.RpcPushNotificationRegisterTokenRequest)
	RevokeToken(background context.Context) (err error)
	Notify(ctx context.Context, spaceId, groupId string, topic []string, payload []byte) (err error)
	NotifyRead(ctx context.Context, spaceId, groupId string) (err error)
}

func New() Service {
	return &service{}
}

type PushNotification struct {
	SpaceId  string
	GroupId  string
	Topics   []string
	Payload  []byte
	Deadline time.Time
	Silent   bool
}

type service struct {
	pushClient           pushclient.Client
	wallet               wallet.Wallet
	config               *config.Config
	spaceService         space.Service
	subscriptionsService subscription.Service

	spaceViewSubscription *objectsubscription.ObjectSubscription[spaceViewStatus]

	token    string
	platform pushapi.Platform

	isTokenRegistered bool

	topics *spaceTopicsCollection

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
	s.subscriptionsService = app.MustComponent[subscription.Service](a)
	s.notifyQueue = mb.New[PushNotification](0)
	s.topics = newSpaceTopicsCollection(s.wallet.Account().SignKey.GetPublic().Account())
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
	go s.sendNotificationsLoop()
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
		if err = s.syncSubscriptions(); err != nil {
			log.Error("syncSubscriptions error", zap.Error(err), zap.Duration("dur", time.Since(st)))
		} else {
			log.Info("syncSubscriptions success", zap.Duration("dur", time.Since(st)))
		}
	}
}

func (s *service) loadRemoteSubscriptions() {
	resp, err := s.pushClient.Subscriptions(s.runCtx, &pushapi.SubscriptionsRequest{})
	if err != nil {
		log.Error("get subscriptions", zap.Error(err))
		return
	}
	s.topics.SetRemoteList(resp.Topics)
}

func (s *service) createSpace(ctx context.Context, spaceKey crypto.PrivKey) (err error) {
	signature, err := spaceKey.Sign([]byte(s.wallet.GetAccountPrivkey().GetPublic().Account()))
	if err != nil {
		return err
	}
	pubKeyRaw, err := spaceKey.GetPublic().Raw()
	if err != nil {
		return
	}
	err = s.pushClient.CreateSpace(ctx, &pushapi.CreateSpaceRequest{
		SpaceKey:         pubKeyRaw,
		AccountSignature: signature,
	})
	return err
}

func (s *service) Notify(ctx context.Context, spaceId, groupId string, topic []string, payload []byte) (err error) {
	return s.notifyQueue.Add(ctx, PushNotification{
		SpaceId: spaceId,
		GroupId: groupId,
		Topics:  topic,
		Payload: payload,
	})
}

func (s *service) NotifyRead(ctx context.Context, spaceId, groupId string) (err error) {
	return s.notifyQueue.Add(ctx, PushNotification{
		SpaceId: spaceId,
		GroupId: groupId,
		Topics:  []string{s.wallet.Account().SignKey.GetPublic().Account()},
		Silent:  true,
	})
}

func (s *service) notify(ctx context.Context, message PushNotification) (err error) {
	topics, err := s.topics.MakeTopics(message.SpaceId, message.Topics)
	if err != nil {
		return fmt.Errorf("make topics: %w", err)
	}
	encryptionKeyId, encryptedJson, err := s.topics.EncryptPayload(message.SpaceId, message.Payload)
	if err != nil {
		return err
	}
	signature, err := s.wallet.GetAccountPrivkey().Sign(encryptedJson)
	if err != nil {
		return err
	}
	p := &pushapi.Message{
		KeyId:     encryptionKeyId,
		Payload:   encryptedJson,
		Signature: signature,
	}
	err = s.pushClient.Notify(ctx, &pushapi.NotifyRequest{
		Topics:  topics,
		Message: p,
		GroupId: message.GroupId,
	})
	return err
}

func (s *service) notifySilent(ctx context.Context, message PushNotification) (err error) {
	topics, err := s.topics.MakeTopics(message.SpaceId, message.Topics)
	if err != nil {
		return fmt.Errorf("make topics: %w", err)
	}
	err = s.pushClient.NotifySilent(ctx, &pushapi.NotifyRequest{
		Topics:  topics,
		GroupId: message.GroupId,
	})
	return err
}

func (s *service) sendNotificationsLoop() {
	for {
		message, err := s.notifyQueue.WaitOne(s.runCtx)
		if err != nil {
			return
		}
		var f func(ctx context.Context, message PushNotification) error
		if message.Silent {
			f = s.notifySilent
		} else {
			f = s.notify
		}
		for range 6 {
			if err := f(s.runCtx, message); err != nil {
				if errors.Is(err, pushapi.ErrNoValidTopics) {
					break
				}
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

func (s *service) syncSubscriptions() (err error) {
	s.topics.ResetLocal()
	s.spaceViewSubscription.Iterate(func(id string, data spaceViewStatus) bool {
		s.topics.SetSpaceViewStatus(&data)
		return true
	})

	// create spaces
	spacesToCreate := s.topics.SpaceKeysToCreate()
	for _, spaceToCreate := range spacesToCreate {
		if err = s.createSpace(s.runCtx, spaceToCreate); err != nil {
			if !errors.Is(err, pushapi.ErrSpaceExists) {
				return fmt.Errorf("create space: %w", err)
			} else {
				err = nil
			}
		}
	}

	// subscribe
	topicsReq := s.topics.MakeApiRequest()
	if topicsReq != nil {
		if err = s.pushClient.SubscribeAll(s.runCtx, &pushapi.SubscribeAllRequest{
			Topics: topicsReq,
		}); err != nil {
			return fmt.Errorf("subscribe: %w", err)
		} else {
			s.topics.Flush()
		}
	}
	return
}

func (s *service) RevokeToken(ctx context.Context) (err error) {
	return s.pushClient.RevokeToken(ctx)
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
