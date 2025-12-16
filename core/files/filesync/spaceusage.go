package filesync

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/filestorage/rpcstore"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/objectsubscription"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type updateMessage struct {
	spaceId string
	limit   int
	usage   int
}

func (m updateMessage) freeSpace() int {
	free := m.limit - m.usage
	if free < 0 {
		free = 0
	}
	return free
}

type spaceUsageManager struct {
	ctx       context.Context
	ctxCancel context.CancelFunc

	techSpaceId         string
	subscriptionService subscription.Service
	rpcStore            rpcstore.RpcStore

	spaceViews *objectsubscription.ObjectSubscription[*spaceUsage]
	updateCh   chan updateMessage
}

func newSpaceUsageManager(subscriptionService subscription.Service, rpcStore rpcstore.RpcStore, techSpaceId string) *spaceUsageManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &spaceUsageManager{
		ctx:                 ctx,
		ctxCancel:           cancel,
		techSpaceId:         techSpaceId,
		subscriptionService: subscriptionService,
		rpcStore:            rpcStore,

		// Use buffered channel of size 1 to always receive at least one update signal
		updateCh: make(chan updateMessage, 1),
	}
}

func (m *spaceUsageManager) init() error {
	sub := objectsubscription.New[*spaceUsage](m.subscriptionService, subscription.SubscribeRequest{
		SpaceId: m.techSpaceId,
		Keys: []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeyTargetSpaceId.String(),
		},
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_spaceView)),
			},
			{
				RelationKey: bundle.RelationKeySpaceAccountStatus,
				Condition:   model.BlockContentDataviewFilter_NotIn,
				Value:       domain.Int64List([]model.SpaceStatus{model.SpaceStatus_SpaceDeleted, model.SpaceStatus_SpaceRemoving}),
			},
		},
	}, objectsubscription.SubscriptionParams[*spaceUsage]{
		SetDetails: func(details *domain.Details) (id string, entry *spaceUsage) {
			spaceId := details.GetString(bundle.RelationKeyTargetSpaceId)

			// Fan-in updates from per-space channels. It guarantees receiving an update for each space.
			// Remember, that updates for one space is throttled, so if we use single channel for updates
			// we will lose some updates.
			updateCh := make(chan updateMessage, 1)
			go func() {
				for {
					select {
					case <-m.ctx.Done():
						return
					case update := <-updateCh:
						select {
						case <-m.ctx.Done():
							return
						case m.updateCh <- update:
						}
					}
				}
			}()
			usage := newSpaceUsage(m.ctx, spaceId, m.rpcStore, updateCh)
			return spaceId, usage
		},
		UpdateKeys: func(keyValues []objectsubscription.RelationKeyValue, curEntry *spaceUsage) (updatedEntry *spaceUsage) {
			return curEntry
		},
		RemoveKeys: func(keys []string, curEntry *spaceUsage) (updatedEntry *spaceUsage) {
			return curEntry
		},
	})

	err := sub.Run()
	if err != nil {
		return fmt.Errorf("run: %w", err)
	}

	go func() {
		var errGroup errgroup.Group
		sub.Iterate(func(id string, usage *spaceUsage) bool {
			errGroup.Go(func() error {
				err := usage.Update(m.ctx)
				if err != nil {
					return err
				}
				return nil
			})
			return true
		})

		err = errGroup.Wait()
		if err != nil {
			log.Error("init space usage manager", zap.Error(err))
		}
	}()

	m.spaceViews = sub

	return nil
}

func (m *spaceUsageManager) getSpace(ctx context.Context, spaceId string) (*spaceUsage, error) {
	spc, ok := m.spaceViews.GetByKey(spaceId)
	if !ok {
		return nil, fmt.Errorf("spaceView not found")
	}
	return spc, nil
}

func (m *spaceUsageManager) close() {
	if m.ctxCancel != nil {
		m.ctxCancel()
	}
	close(m.updateCh)
}
