package space

import (
	"context"
	"time"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app/ocache"
	"github.com/anyproto/any-sync/commonspace"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/space/typeprovider"
)

type Space interface {
	commonspace.Space
	GetObject(ctx context.Context, id string) (sb smartblock.SmartBlock, err error)
	GetObjectWithTimeout(ctx context.Context, id string) (sb smartblock.SmartBlock, err error)
	RemoveObjectFromCache(ctx context.Context, id string) error
}

func newClientSpace(
	cc commonspace.Space,
	objectFactory ObjectFactory,
	sbtProvider typeprovider.SmartBlockTypeProvider,
	core core.Service,
	commonAccount accountservice.Service,
) (Space, error) {
	s := &clientSpace{
		Space:         cc,
		objectFactory: objectFactory,
		sbtProvider:   sbtProvider,
		core:          core,
		commonAccount: commonAccount,
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
	sbtProvider   typeprovider.SmartBlockTypeProvider
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
