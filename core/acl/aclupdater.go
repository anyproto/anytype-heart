package acl

import (
	"context"
	"errors"
	"time"

	"github.com/anyproto/any-sync/util/crypto"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/acl/minwaitqueue"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription/crossspacesub"
)

type participantGetter interface {
	Run(ctx context.Context) error
	Close() error
}

type participantRemover interface {
	ApproveLeave(ctx context.Context, spaceId string, identities []crypto.PubKey) error
}

type Message struct {
	SpaceId  string
	Identity crypto.PubKey
}

type aclUpdater struct {
	queue             *minwaitqueue.MinWaitQueue[Message]
	participantGetter participantGetter
}

func newAclUpdater(
	id string,
	ownIdentity string,
	crossSpaceSubService crossspacesub.Service,
	remover participantRemover,
	defaultTimeout time.Duration,
	maxTimeout time.Duration,
) *aclUpdater {
	queue := minwaitqueue.NewMinWaitQueue[Message](
		func(ctx context.Context, msg Message) error {
			return remover.ApproveLeave(ctx, msg.SpaceId, []crypto.PubKey{msg.Identity})
		},
		func(err error) bool {
			return !errors.Is(err, ErrRequestNotExists)
		},
		minwaitqueue.Config{
			DefaultTimeout: defaultTimeout,
			MaxTimeout:     maxTimeout,
		},
	)

	participantGetter := newParticipantGetter(
		id,
		ownIdentity,
		crossSpaceSubService,
		func(identity crypto.PubKey, spaceId string) error {
			id := domain.NewParticipantId(spaceId, identity.Account())
			queue.RemoveUpdate(id)
			return nil
		},
		func(identity crypto.PubKey, spaceId string) error {
			id := domain.NewParticipantId(spaceId, identity.Account())
			return queue.AddUpdate(id, Message{
				SpaceId:  spaceId,
				Identity: identity,
			}, 0)
		},
	)

	return &aclUpdater{
		queue:             queue,
		participantGetter: participantGetter,
	}
}

func (aw *aclUpdater) Run(ctx context.Context) error {
	aw.queue.Run()
	return aw.participantGetter.Run(ctx)
}

func (aw *aclUpdater) Close() error {
	if err := aw.participantGetter.Close(); err != nil {
		log.Debug("failed to close participant getter", zap.Error(err))
	}
	return aw.queue.Close()
}
