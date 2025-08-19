package formatfetcher

import (
	"context"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/cheggaaa/mb/v3"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/objectsubscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	CName                         = "relation-format-fetcher"
	relationFormatsSubscriptionId = "relationFormats"
)

var log = logger.NewNamed(CName)

type spaceSubscription struct {
	sub   *objectsubscription.ObjectSubscription[model.RelationLink]
	queue *mb.MB[*pb.EventMessage]
	cache map[domain.RelationKey]model.RelationFormat
}

func New() relationutils.RelationFormatFetcher {
	return &formatFetcher{}
}

type formatFetcher struct {
	subscription subscription.Service

	mu   sync.RWMutex
	subs map[string]*spaceSubscription
}

func (f *formatFetcher) Name() string {
	return CName
}

func (f *formatFetcher) Init(a *app.App) error {
	f.subscription = app.MustComponent[subscription.Service](a)
	f.subs = map[string]*spaceSubscription{}
	return nil
}

func (f *formatFetcher) Run(_ context.Context) error {
	return nil
}

func (f *formatFetcher) setupSub(spaceId string) (*spaceSubscription, error) {
	queue := mb.New[*pb.EventMessage](0)

	response, err := f.subscription.Search(subscription.SubscribeRequest{
		SpaceId:           spaceId,
		SubId:             buildSubId(spaceId),
		Keys:              []string{bundle.RelationKeyRelationKey.String(), bundle.RelationKeyRelationFormat.String()},
		NoDepSubscription: true,
		Internal:          true,
		InternalQueue:     queue,
		Filters: []database.FilterRequest{{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Int64(int64(model.ObjectType_relation)),
		}},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to setup relation formats subscription: %w", err)
	}

	formats := map[domain.RelationKey]model.RelationFormat{}
	for _, record := range response.Records {
		key := domain.RelationKey(record.GetString(bundle.RelationKeyRelationKey))
		if bundle.HasRelation(key) {
			continue
		}

		// nolint:gosec
		format := model.RelationFormat(record.GetInt64(bundle.RelationKeyRelationFormat))
		formats[key] = format
	}

	sub := objectsubscription.NewFromQueue(queue, f.buildSubscriptionParams(spaceId))
	if err = sub.Run(); err != nil {
		return nil, fmt.Errorf("failed to run relation formats subscription: %w", err)
	}
	spaceSub := &spaceSubscription{
		sub:   sub,
		queue: queue,
		cache: formats,
	}
	f.subs[spaceId] = spaceSub
	return spaceSub, nil
}

func (f *formatFetcher) buildSubscriptionParams(spaceId string) objectsubscription.SubscriptionParams[model.RelationLink] {
	return objectsubscription.SubscriptionParams[model.RelationLink]{
		SetDetails: func(details *domain.Details) (id string, entry model.RelationLink) {
			id = details.GetString(bundle.RelationKeyId)
			key := domain.RelationKey(details.GetString(bundle.RelationKeyRelationKey))
			format := model.RelationFormat(details.GetInt64(bundle.RelationKeyRelationFormat)) // nolint:gosec
			if !bundle.HasRelation(key) {
				f.mu.Lock()
				f.subs[spaceId].cache[key] = format
				f.mu.Unlock()
			}
			return id, model.RelationLink{
				Key:    key.String(),
				Format: format,
			}
		},
		UpdateKey: func(relationKey string, relationValue domain.Value, curEntry model.RelationLink) (updatedEntry model.RelationLink) {
			return curEntry
		},
		RemoveKeys: func(keys []string, curEntry model.RelationLink) (updatedEntry model.RelationLink) {
			return curEntry
		},
		OnAdded: func(id string, entry model.RelationLink) {
			key := domain.RelationKey(entry.Key)
			if !bundle.HasRelation(key) {
				f.mu.Lock()
				f.subs[spaceId].cache[key] = entry.Format
				f.mu.Unlock()
			}
		},
	}
}

func (f *formatFetcher) Close(_ context.Context) error {
	var subIds []string
	for spaceId, sub := range f.subs {
		sub.sub.Close()
		if err := sub.queue.Close(); err != nil {
			log.Warn("failed to close queue", zap.Error(err))
		}
		subIds = append(subIds, buildSubId(spaceId))
	}

	if err := f.subscription.Unsubscribe(subIds...); err != nil {
		log.Warn("failed to close relation format subscriptions", zap.Error(err))
	}
	return nil
}

func (f *formatFetcher) GetRelationFormatByKey(spaceId string, key domain.RelationKey) (model.RelationFormat, error) {
	rel, err := bundle.GetRelation(key)
	if err == nil {
		return rel.Format, nil
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	sub, ok := f.subs[spaceId]
	if ok {
		format, found := sub.cache[key]
		if found {
			return format, nil
		}
		return 0, fmt.Errorf("relation format not found for key %s", key)
	}

	sub, err = f.setupSub(spaceId)
	if err != nil {
		return 0, err
	}

	format, found := sub.cache[key]
	if found {
		return format, nil
	}
	return 0, fmt.Errorf("relation format not found for key %s", key)
}

func buildSubId(spaceId string) string {
	return fmt.Sprintf("%s-%s", relationFormatsSubscriptionId, spaceId)
}
