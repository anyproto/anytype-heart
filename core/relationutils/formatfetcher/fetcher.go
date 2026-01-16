package formatfetcher

import (
	"context"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/objectsubscription"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/futures"
)

const (
	CName                         = "relation-format-fetcher"
	relationFormatsSubscriptionId = "relationFormats"
)

var log = logger.NewNamed(CName)

func New() relationutils.RelationFormatFetcher {
	return &formatFetcher{}
}

type formatFetcher struct {
	subscription subscription.Service

	lock sync.Mutex
	subs map[string]*futures.Future[*objectsubscription.ObjectSubscription[model.RelationLink]]
}

func (f *formatFetcher) Name() string {
	return CName
}

func (f *formatFetcher) Init(a *app.App) error {
	f.subscription = app.MustComponent[subscription.Service](a)
	f.subs = make(map[string]*futures.Future[*objectsubscription.ObjectSubscription[model.RelationLink]])
	return nil
}

func (f *formatFetcher) Run(_ context.Context) error {
	return nil
}

func (f *formatFetcher) setupSub(spaceId string) (*objectsubscription.ObjectSubscription[model.RelationLink], error) {
	req := subscription.SubscribeRequest{
		SpaceId:           spaceId,
		SubId:             buildSubId(spaceId),
		Keys:              []string{bundle.RelationKeyId.String(), bundle.RelationKeyRelationKey.String(), bundle.RelationKeyRelationFormat.String()},
		NoDepSubscription: true,
		Internal:          true,
		Filters: []database.FilterRequest{{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Int64(int64(model.ObjectType_relation)),
		}},
	}

	sub := objectsubscription.New(f.subscription, req, f.buildSubscriptionParams())
	if err := sub.Run(); err != nil {
		return nil, fmt.Errorf("failed to run relation formats subscription: %w", err)
	}

	return sub, nil
}

func (f *formatFetcher) getSpaceSub(spaceId string) (*objectsubscription.ObjectSubscription[model.RelationLink], error) {
	return f.getSpaceSubFuture(spaceId).Wait()
}

func (f *formatFetcher) getSpaceSubFuture(spaceId string) *futures.Future[*objectsubscription.ObjectSubscription[model.RelationLink]] {
	f.lock.Lock()
	sub, ok := f.subs[spaceId]
	if ok {
		f.lock.Unlock()
		return sub
	}

	sub = futures.New[*objectsubscription.ObjectSubscription[model.RelationLink]]()
	f.subs[spaceId] = sub
	f.lock.Unlock()

	sub.Resolve(f.setupSub(spaceId))

	return sub
}

func (f *formatFetcher) buildSubscriptionParams() objectsubscription.SubscriptionParams[model.RelationLink] {
	return objectsubscription.SubscriptionParams[model.RelationLink]{
		SetDetails: func(details *domain.Details) (string, model.RelationLink) {
			key := domain.RelationKey(details.GetString(bundle.RelationKeyRelationKey))
			format := model.RelationFormat(details.GetInt64(bundle.RelationKeyRelationFormat)) // nolint:gosec
			return key.String(), model.RelationLink{
				Key:    key.String(),
				Format: format,
			}
		},
		UpdateKeys: func(keyValues []objectsubscription.RelationKeyValue, curEntry model.RelationLink) (updatedEntry model.RelationLink) {
			return curEntry
		},
		RemoveKeys: func(keys []string, curEntry model.RelationLink) (updatedEntry model.RelationLink) {
			return curEntry
		},
		CustomFilter: func(details *domain.Details) bool {
			key := domain.RelationKey(details.GetString(bundle.RelationKeyRelationKey))
			return !bundle.HasRelation(key)
		},
	}
}

func (f *formatFetcher) Close(_ context.Context) error {
	subIds := make([]string, 0, len(f.subs))
	for spaceId, future := range f.subs {
		sub, err := future.Wait()
		if err != nil {
			log.Warn("failed to get space subscription on Close", zap.Error(err))
			continue
		}
		sub.Close()
		subIds = append(subIds, buildSubId(spaceId))
	}

	if err := f.subscription.Unsubscribe(subIds...); err != nil {
		log.Warn("failed to close relation format subscriptions", zap.Error(err))
	}
	return nil
}

func (f *formatFetcher) GetRelationFormatByKey(spaceId string, key domain.RelationKey) (model.RelationFormat, error) {
	format, err := bundle.GetRelationFormat(key)
	if err == nil {
		return format, nil
	}

	sub, err := f.getSpaceSub(spaceId)
	if err != nil {
		return 0, err
	}

	relLink, found := sub.GetByKey(key.String())
	if found {
		return relLink.Format, nil
	}
	return 0, fmt.Errorf("relation format not found for key %s", key)
}

func buildSubId(spaceId string) string {
	return fmt.Sprintf("%s-%s", relationFormatsSubscriptionId, spaceId)
}
