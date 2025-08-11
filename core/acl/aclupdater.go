package acl

import (
	"context"
	"errors"
	"time"

	"github.com/anyproto/any-sync/util/crypto"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/acl/retryscheduler"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/crossspacesub"
)

type participantGetter interface {
	Run(ctx context.Context) error
	Close() error
}

type participantRemover interface {
	ApproveLeave(ctx context.Context, spaceId string, identities []crypto.PubKey) error
	Leave(ctx context.Context, spaceId string) error
}

type MsgType int

const (
	MsgTypeRemoveOther MsgType = iota
	MsgTypeRemoveSelf
)

type Message struct {
	SpaceId  string
	Identity crypto.PubKey
	MsgType  MsgType
}

type aclUpdater struct {
	scheduler         *retryscheduler.RetryScheduler[Message]
	participantGetter participantGetter
	spaceSub          *spaceSubscription
}

func newAclUpdater(
	id string,
	ownIdentity string,
	crossSpaceSubService crossspacesub.Service,
	subscriptionService subscription.Service,
	techSpaceId string,
	remover participantRemover,
	defaultTimeout time.Duration,
	maxTimeout time.Duration,
	requestTimeout time.Duration,
) (*aclUpdater, error) {
	scheduler := retryscheduler.NewRetryScheduler[Message](
		func(ctx context.Context, msg Message) error {
			ctx, cancel := context.WithTimeout(ctx, requestTimeout)
			defer cancel()
			switch msg.MsgType {
			case MsgTypeRemoveOther:
				return remover.ApproveLeave(ctx, msg.SpaceId, []crypto.PubKey{msg.Identity})
			case MsgTypeRemoveSelf:
				return remover.Leave(ctx, msg.SpaceId)
			default:
				return nil
			}
		},
		func(msg Message, err error) bool {
			switch msg.MsgType {
			case MsgTypeRemoveOther:
				return !errors.Is(err, ErrRequestNotExists)
			case MsgTypeRemoveSelf:
				// TODO: check relevant error
				return !errors.Is(err, ErrRequestNotExists)
			default:
				return false
			}
		},
		retryscheduler.Config{
			DefaultTimeout: defaultTimeout,
			MaxTimeout:     maxTimeout,
		},
	)
	pubKey, err := crypto.DecodeAccountAddress(ownIdentity)
	if err != nil {
		return nil, err
	}
	participantGetter := newParticipantGetter(
		id,
		ownIdentity,
		crossSpaceSubService,
		func(identity crypto.PubKey, spaceId string) error {
			id := domain.NewParticipantId(spaceId, identity.Account())
			scheduler.Remove(id)
			return nil
		},
		func(identity crypto.PubKey, spaceId string) error {
			id := domain.NewParticipantId(spaceId, identity.Account())
			return scheduler.Schedule(id, Message{
				SpaceId:  spaceId,
				Identity: identity,
				MsgType:  MsgTypeRemoveOther,
			}, 0)
		},
	)

	deleteSpaceSub := newSpaceSubscription(
		subscriptionService,
		ownIdentity,
		techSpaceId,
		func(sub *spaceViewObjectSubscription) {
			sub.Iterate(func(id string, status spaceViewStatus) bool {
				err := scheduler.Schedule(id, Message{
					SpaceId:  status.spaceId,
					Identity: pubKey,
					MsgType:  MsgTypeRemoveSelf,
				}, 0)
				if err != nil {
					log.Error("failed to schedule message", zap.Error(err))
				}
				return true
			})
		},
		func(status spaceViewStatus) {
			err := scheduler.Schedule(id, Message{
				SpaceId:  status.spaceId,
				Identity: pubKey,
				MsgType:  MsgTypeRemoveSelf,
			}, 0)
			if err != nil {
				log.Error("failed to schedule message", zap.Error(err))
			}
		},
		func(_ string, status spaceViewStatus) {
			id := domain.NewParticipantId(status.spaceId, ownIdentity)
			scheduler.Remove(id)
		})

	return &aclUpdater{
		scheduler:         scheduler,
		participantGetter: participantGetter,
		spaceSub:          deleteSpaceSub,
	}, nil
}

func (aw *aclUpdater) Run(ctx context.Context) error {
	aw.scheduler.Run()
	err := aw.spaceSub.Run()
	if err != nil {
		return err
	}
	return aw.participantGetter.Run(ctx)
}

func (aw *aclUpdater) Close() error {
	if err := aw.participantGetter.Close(); err != nil {
		log.Debug("failed to close participant getter", zap.Error(err))
	}
	aw.spaceSub.Close()
	return aw.scheduler.Close()
}
