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
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/pushnotification/pushclient"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/objectsubscription"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

const CName = "core.pushnotification.service"

var log = logging.Logger(CName).Desugar()

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
	cfg                  *config.Config
	spaceService         space.Service
	eventSender          event.Sender
	subscriptionsService subscription.Service

	token             string
	platform          pushapi.Platform
	isTokenRegistered bool

	subscriptions map[spaceKeyType]*spaceInfo

	notifyQueue        *mb.MB[PushNotification]
	mu                 sync.Mutex
	runCtx             context.Context
	runCtxCancel       context.CancelFunc
	objectSubscription *objectsubscription.ObjectSubscription[spaceViewStatus]
	wakeUpCh           chan struct{}
}

type spaceKeyType string

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Init(a *app.App) (err error) {
	s.cfg = app.MustComponent[*config.Config](a)
	s.pushClient = app.MustComponent[pushclient.Client](a)
	s.wallet = app.MustComponent[wallet.Wallet](a)
	s.spaceService = app.MustComponent[space.Service](a)
	s.eventSender = app.MustComponent[event.Sender](a)
	s.subscriptionsService = app.MustComponent[subscription.Service](a)
	s.subscriptions = make(map[spaceKeyType]*spaceInfo)
	s.wakeUpCh = make(chan struct{}, 1)
	return
}

type spaceViewStatus struct {
	spaceId        string
	spaceViewId    string
	topics         []string
	status         spaceinfo.LocalStatus
	isTopicsExists bool
	shareable      bool
}

func (s *service) Run(ctx context.Context) (err error) {
	if s.cfg.IsLocalOnlyMode() {
		return nil
	}
	s.runCtx, s.runCtxCancel = context.WithCancel(context.Background())

	objectReq := subscription.SubscribeRequest{
		SpaceId:           s.spaceService.TechSpaceId(),
		SubId:             CName,
		Internal:          true,
		NoDepSubscription: true,
		Keys: []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeySpaceId.String(),
			bundle.RelationKeySpaceLocalStatus.String(),
			bundle.RelationKeySpaceShareableStatus.String(),
		},
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyType,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(bundle.TypeKeySpaceView.String()),
			},
		},
	}
	s.objectSubscription = objectsubscription.New[spaceViewStatus](s.subscriptionsService, objectsubscription.SubscriptionParams[spaceViewStatus]{
		Request: objectReq,
		Extract: func(details *domain.Details) (string, spaceViewStatus) {
			shareable := details.GetInt64(bundle.RelationKeySpaceShareableStatus) == int64(model.SpaceShareableStatus_StatusShareable)
			defer s.wakeUp()
			return details.GetString(bundle.RelationKeyId), spaceViewStatus{
				spaceId:        details.GetString(bundle.RelationKeySpaceId),
				spaceViewId:    details.GetString(bundle.RelationKeyId),
				shareable:      shareable,
				topics:         details.GetStringList(bundle.RelationKeyPushNotificationTopics),
				isTopicsExists: details.Has(bundle.RelationKeyPushNotificationTopics),
				status:         spaceinfo.LocalStatus(details.GetInt64(bundle.RelationKeySpaceLocalStatus)),
			}
		},
		Update: func(key string, value domain.Value, status spaceViewStatus) spaceViewStatus {
			defer s.wakeUp()
			switch domain.RelationKey(key) {
			case bundle.RelationKeySpaceLocalStatus:
				status.status = spaceinfo.LocalStatus(value.Int64())
				return status
			case bundle.RelationKeySpaceShareableStatus:
				status.shareable = value.Int64() == int64(model.SpaceShareableStatus_StatusShareable)
				return status
			case bundle.RelationKeyPushNotificationTopics:
				status.topics = value.StringList()
				status.isTopicsExists = true
				return status
			}
			return status
		},
		Unset: func(strings []string, status spaceViewStatus) spaceViewStatus {
			return status
		},
	})
	if err = s.objectSubscription.Run(); err != nil {
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
	needCreate      bool
}

func (s *service) Notify(ctx context.Context, spaceId string, topic []string, payload []byte) (err error) {

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
	for {
		select {
		case <-s.runCtx.Done():
			return
		case <-s.wakeUpCh:
		}
		if err = s.registerToken(); err != nil {
			log.Error("register token", zap.Error(err))
		}
		if err = s.sync(); err != nil {
			log.Error("sync", zap.Error(err))
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
				si *spaceInfo
				ok bool
			)
			if si, ok = s.subscriptions[spaceKeyType(remoteTopic.SpaceKey)]; !ok {
				si = &spaceInfo{
					spaceKey: remoteTopic.SpaceKey,
					status:   spaceViewStatus{},
					topics:   NewTopicSet(),
					created:  true,
					synced:   true,
				}
				s.subscriptions[spaceKeyType(remoteTopic.SpaceKey)] = si
			}
			si.topics.Add(remoteTopic.Topic)
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
	if err = s.pushClient.SetToken(s.runCtx, &pushapi.SetTokenRequest{
		Platform: platform,
		Token:    token,
	}); err != nil {
		return
	}
	s.mu.Lock()
	if token == s.token { // token didn't change while api call
		s.isTokenRegistered = true
	}
	s.mu.Unlock()
	return
}

type spaceInfo struct {
	spaceKey []byte
	status   spaceViewStatus
	topics   TopicSet
	created  bool
	synced   bool
}

func (s *service) sync() (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objectSubscription.Iterate(func(id string, data spaceViewStatus) bool {
		sk, err := s.getSpaceKeys(data.spaceId)
		if err != nil {
			log.Error("get space keys", zap.Error(err))
			return true
		}
		var (
			si *spaceInfo
			ok bool
		)
		if si, ok = s.subscriptions[sk.spaceKey]; !ok {
			si = &spaceInfo{
				spaceKey: []byte(sk.spaceKey),
				status:   data,
				topics:   NewTopicSet(),
			}
			s.subscriptions[sk.spaceKey] = si
		}
		if si.topics.Set(data.topics...) {
			si.synced = false
		}
		if sk.needCreate && !si.created {
			si.created = true
		}
		if data.status == spaceinfo.LocalStatusMissing &{
			si.topics.Set()
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
		spaceKey:        spaceKeyType(spaceKey),
		encryptionKey:   encryptionKey,
		encryptionKeyId: encryptionKeyId,
		signKey:         signKey,
		needCreate:      isOwner && len(state.Invites()) > 0,
	}, nil
}

func (s *service) Close(ctx context.Context) (err error) {
	if s.runCtxCancel != nil {
		s.runCtxCancel()
	}
	return s.notifyQueue.Close()
}
