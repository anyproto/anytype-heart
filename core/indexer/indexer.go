package indexer

import (
	"context"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/app"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/chats/chatrepository"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
)

const (
	CName = "indexer"
)

var log = logging.Logger("anytype-doc-indexer")

func New() Indexer {
	return new(indexer)
}

type Indexer interface {
	ForceFTIndex()
	StartFullTextIndex() error
	ReindexMarketplaceSpace(space clientspace.Space) error
	ReindexSpace(space clientspace.Space) error
	RemoveIndexes(spaceId string) (err error)
	Index(info smartblock.DocInfo, options ...smartblock.IndexOption) error
	app.ComponentRunnable
}

type Hasher interface {
	Hash() string
}

type indexer struct {
	dbProvider           anystoreprovider.Provider
	store                objectstore.ObjectStore
	source               source.Service
	picker               cache.CachedObjectGetter
	formatFetcher        relationutils.RelationFormatFetcher
	chatRepository       chatrepository.Service
	ftsearch             ftsearch.FTSearch
	ftsearchLastIndexSeq uint64

	runCtx          context.Context
	runCtxCancel    context.CancelFunc
	ftQueueStop     context.CancelFunc
	ftQueueFinished chan struct{}
	config          *config.Config

	btHash  Hasher
	forceFt chan struct{}

	// state
	lock                sync.Mutex
	reindexLogFields    []zap.Field
	spaceIndexers       map[string]*spaceIndexer
	techSpaceIdProvider objectstore.TechSpaceIdProvider
	spaces              map[string]struct{}
	spacesLock          sync.RWMutex
}

func (i *indexer) Init(a *app.App) (err error) {
	i.store = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	i.source = app.MustComponent[source.Service](a)
	i.btHash = a.MustComponent("builtintemplate").(Hasher)
	i.ftsearch = app.MustComponent[ftsearch.FTSearch](a)
	i.picker = app.MustComponent[cache.CachedObjectGetter](a)
	i.runCtx, i.runCtxCancel = context.WithCancel(context.Background())
	i.forceFt = make(chan struct{})
	i.config = app.MustComponent[*config.Config](a)
	i.spaceIndexers = map[string]*spaceIndexer{}
	i.techSpaceIdProvider = app.MustComponent[objectstore.TechSpaceIdProvider](a)
	i.dbProvider = app.MustComponent[anystoreprovider.Provider](a)
	i.formatFetcher = app.MustComponent[relationutils.RelationFormatFetcher](a)
	i.chatRepository = app.MustComponent[chatrepository.Service](a)
	return
}

func (i *indexer) Name() (name string) {
	return CName
}

func (i *indexer) Run(context.Context) (err error) {
	return i.StartFullTextIndex()
}

func (i *indexer) StateChange(state int) {
	if state == int(domain.CompStateAppClosingInitiated) && i.ftQueueStop != nil {
		i.ftQueueStop()
	}
}

func (i *indexer) StartFullTextIndex() (err error) {
	if ftErr := i.ftInit(); ftErr != nil {
		log.Errorf("can't init ft: %v", ftErr)
	}
	i.ftQueueFinished = make(chan struct{})
	var ftCtx context.Context
	ftCtx, i.ftQueueStop = context.WithCancel(i.runCtx)
	go i.ftLoopRoutine(ftCtx)
	return
}

func (i *indexer) Close(ctx context.Context) (err error) {
	i.lock.Lock()
	for spaceId, si := range i.spaceIndexers {
		err = si.close()
		if err != nil {
			log.With("spaceId", spaceId, "error", err).Errorf("close spaceIndexer")
		}
		delete(i.spaceIndexers, spaceId)
	}
	i.lock.Unlock()
	if i.runCtxCancel != nil {
		i.runCtxCancel()
		// we need to wait for the ftQueue processing to be finished gracefully. Because we may be in the middle of badger transaction
		<-i.ftQueueFinished
	}
	return nil
}

func (i *indexer) RemoveAclIndexes(spaceId string) (err error) {
	// TODO: It seems we should also filter objects by Layout, because participants should be re-indexed to receive resolvedLayout
	store := i.store.SpaceIndex(spaceId)
	ids, _, err := store.QueryObjectIds(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(model.ObjectType_participant),
			},
		},
	})
	err = i.store.ClearFullTextQueue([]string{spaceId})
	if err != nil {
		return fmt.Errorf("remove fts: %w", err)
	}

	// todo: should we use the queue here as well?
	err = i.ftsearch.BatchDeleteObjects(ids)
	if err != nil {
		return fmt.Errorf("remove acl: %w", err)
	}

	return store.DeleteDetails(i.runCtx, ids)
}

func (i *indexer) isFulltextEnabled(space smartblock.Space) bool {
	return i.techSpaceIdProvider.TechSpaceId() != space.Id() &&
		space.Id() != addr.AnytypeMarketplaceWorkspace
}

func (i *indexer) Index(info smartblock.DocInfo, options ...smartblock.IndexOption) error {
	i.lock.Lock()
	spaceInd, ok := i.spaceIndexers[info.Space.Id()]
	if !ok {
		spaceInd = newSpaceIndexer(
			i.runCtx,
			i.store.SpaceIndex(info.Space.Id()),
			i.store,
			i.isFulltextEnabled(info.Space),
		)
		i.spaceIndexers[info.Space.Id()] = spaceInd
	}
	i.lock.Unlock()

	return spaceInd.Index(info, options...)
}
