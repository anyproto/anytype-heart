package acl

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/anyproto/any-sync/util/crypto"
	"github.com/cheggaaa/mb/v3"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	subscriptionservice "github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/crossspacesub"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type identityUpdateFunc = func(identity crypto.PubKey, spaceId string) error

func newParticipantGetter(
	id string,
	ownIdentity string,
	crossSpaceSubService crossspacesub.Service,
	onRemove identityUpdateFunc,
	onAdd identityUpdateFunc,
) participantGetter {
	ctx, cancel := context.WithCancel(context.Background())
	return &participantSub{
		ownIdentity:          ownIdentity,
		started:              make(chan struct{}),
		waiter:               make(chan struct{}),
		ctx:                  ctx,
		cancel:               cancel,
		id:                   id,
		crossSpaceSubService: crossSpaceSubService,
		onRemove:             onRemove,
		onAdd:                onAdd,
	}
}

type participantSub struct {
	id                   string
	ownIdentity          string
	crossSpaceSubService crossspacesub.Service
	started              chan struct{}
	waiter               chan struct{}
	internalQueue        *mb.MB[*pb.EventMessage]
	ctx                  context.Context
	cancel               context.CancelFunc
	onRemove             identityUpdateFunc
	onAdd                identityUpdateFunc
}

func (s *participantSub) Run(ctx context.Context) error {
	close(s.started)
	s.internalQueue = mb.New[*pb.EventMessage](0)
	resp, err := s.crossSpaceSubService.Subscribe(subscriptionservice.SubscribeRequest{
		SubId:             s.id,
		InternalQueue:     s.internalQueue,
		Keys:              []string{bundle.RelationKeyId.String(), bundle.RelationKeySpaceId.String(), bundle.RelationKeyIdentity.String()},
		NoDepSubscription: true,
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(model.ObjectType_participant),
			},
			{
				RelationKey: bundle.RelationKeyParticipantStatus,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(model.ParticipantStatus_Removing),
			},
		},
	}, newSubPredicate(s.ownIdentity))
	if err != nil {
		close(s.waiter)
		return fmt.Errorf("cross-space sub: %w", err)
	}
	for _, record := range resp.Records {
		identity := record.GetString(bundle.RelationKeyIdentity)
		spaceId := record.GetString(bundle.RelationKeySpaceId)
		if identity == "" || spaceId == "" {
			log.Debug("participant sub: empty identity or spaceId", zap.String("identity", identity), zap.String("spaceId", spaceId))
			continue
		}
		pubKey, err := crypto.DecodeAccountAddress(identity)
		if err != nil {
			log.Debug("participant sub: invalid identity", zap.String("identity", identity), zap.Error(err))
			continue
		}
		if err := s.onAdd(pubKey, spaceId); err != nil {
			log.Debug("participant sub: onAdd error", zap.Error(err))
		}
	}
	go s.monitorParticipants()
	return nil
}

// Close cancels the subscription and, if Run was started, waits for the
// monitor goroutine to exit before closing the internal queue. Safe to call
// when Run was never invoked (e.g. when Init of an enclosing component failed
// and the framework calls Close on partially-initialized components).
func (s *participantSub) Close() error {
	s.cancel()
	select {
	case <-s.started:
		<-s.waiter
	default:
		return nil
	}
	return s.internalQueue.Close()
}

func newSubPredicate(creatorId string) crossspacesub.Predicate {
	return func(details *domain.Details) bool {
		if details == nil {
			return false
		}
		return strings.Contains(details.GetString(bundle.RelationKeyCreator), creatorId)
	}
}

func (s *participantSub) getIdentity(participantId string) (pubKey crypto.PubKey, err error) {
	_, identity, err := domain.ParseParticipantId(participantId)
	if err != nil {
		log.Error("parse participant id", zap.String("id", participantId), zap.Error(err))
		return
	}
	return crypto.DecodeAccountAddress(identity)
}

func (s *participantSub) monitorParticipants() {
	defer close(s.waiter)
	matcher := subscriptionservice.EventMatcher{
		OnAdd: func(spaceId string, add *pb.EventObjectSubscriptionAdd) {
			pubKey, err := s.getIdentity(add.Id)
			if err != nil {
				log.Debug("participant sub: invalid id", zap.String("id", add.Id), zap.Error(err))
				return
			}
			if err := s.onAdd(pubKey, spaceId); err != nil {
				log.Debug("participant sub: onAdd error", zap.Error(err))
			}
		},
		OnRemove: func(spaceId string, remove *pb.EventObjectSubscriptionRemove) {
			pubKey, err := s.getIdentity(remove.Id)
			if err != nil {
				log.Debug("participant sub: invalid id", zap.String("id", remove.Id), zap.Error(err))
				return
			}
			if err := s.onRemove(pubKey, spaceId); err != nil {
				log.Debug("participant sub: onRemove error", zap.Error(err))
			}
		},
	}
	for {
		msg, err := s.internalQueue.WaitOne(s.ctx)
		if errors.Is(err, mb.ErrClosed) {
			return
		}
		if err != nil {
			log.Error("wait message", zap.Error(err))
			return
		}
		matcher.Match(msg)
	}
}
