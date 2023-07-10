package space

import (
	"context"
	"time"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app/ocache"
	"github.com/anyproto/any-sync/commonspace"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
)

type Space interface {
	commonspace.Space
	GetObject(ctx context.Context, id string) (sb smartblock.SmartBlock, err error)
	GetObjectWithTimeout(ctx context.Context, id string) (sb smartblock.SmartBlock, err error)
	RemoveObjectFromCache(ctx context.Context, id string) error
	CreateTreeObjectWithPayload(ctx context.Context, payload treestorage.TreeStorageCreatePayload, initFunc InitFunc) (sb smartblock.SmartBlock, err error)
	CreateTreePayload(ctx context.Context, tp coresb.SmartBlockType, createdTime time.Time) (treestorage.TreeStorageCreatePayload, error)
	DerivePredefinedObjects(ctx session.Context, createTrees bool) (predefinedObjectIDs threads.DerivedSmartblockIds, err error)
	CreateTreeObject(ctx session.Context, tp coresb.SmartBlockType, initFunc InitFunc) (sb smartblock.SmartBlock, err error)
	DoLockedIfNotExists(objectID string, proc func() error) error
}

func newClientSpace(
	cc commonspace.Space,
	objectFactory ObjectFactory,
	sbtProvider SmartBlockTypeProvider,
	core core.Service,
	commonAccount accountservice.Service,
) (Space, error) {
	s := &clientSpace{
		Space:         cc,
		objectFactory: objectFactory,
		sbtProvider:   sbtProvider,
		core:          core,
		commonAccount: commonAccount,
		closing:       make(chan struct{}),
	}
	s.cache = ocache.New(
		s.cacheLoad,
		// ocache.WithLogger(log.Desugar()),
		ocache.WithGCPeriod(time.Minute),
		// TODO: [MR] Get ttl from config
		ocache.WithTTL(time.Duration(60)*time.Second),
	)
	return s, nil
}

type clientSpace struct {
	commonspace.Space
	cache         ocache.OCache
	objectFactory ObjectFactory
	sbtProvider   SmartBlockTypeProvider
	core          core.Service
	commonAccount accountservice.Service

	predefinedObjectWasMissing bool
	closing                    chan struct{}
}

func (s *clientSpace) Init(ctx context.Context) (err error) {
	return s.Space.Init(ctx)
}

func (s *clientSpace) Close() (err error) {
	close(s.closing)
	err = s.cache.Close()
	if err != nil {
		log.Error("failed to close cache", zap.String("spaceID", s.Id()), zap.Error(err))
	}
	return s.Space.Close()
}
