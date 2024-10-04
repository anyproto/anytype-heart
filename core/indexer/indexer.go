package indexer

import (
	"context"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/syncstatus/syncsubscriptions"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/spacecore/storage"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	CName = "indexer"
)

var log = logging.Logger("anytype-doc-indexer")

func New() Indexer {
	return &indexer{}
}

type Indexer interface {
	ForceFTIndex()
	StartFullTextIndex() error
	ReindexMarketplaceSpace(space clientspace.Space) error
	ReindexSpace(space clientspace.Space) error
	RemoveIndexes(spaceId string) (err error)
	Index(ctx context.Context, info smartblock.DocInfo, options ...smartblock.IndexOption) error
	app.ComponentRunnable
}

type Hasher interface {
	Hash() string
}

type techSpaceIdGetter interface {
	TechSpaceId() string
}

type indexer struct {
	store               objectstore.ObjectStore
	fileStore           filestore.FileStore
	source              source.Service
	picker              cache.ObjectGetter
	ftsearch            ftsearch.FTSearch
	storageService      storage.ClientStorage
	subscriptionService subscription.Service

	componentCtx       context.Context
	componentCtxCancel context.CancelFunc
	ftQueueFinished    chan struct{}
	config             *config.Config

	btHash  Hasher
	forceFt chan struct{}

	spacesPrioritySubscription *syncsubscriptions.ObjectSubscription[*types.Struct]
	lock                       sync.Mutex
	reindexLogFields           []zap.Field
	spacesPriority             []string
	spaceIndexers              map[string]*spaceIndexer
}

func (i *indexer) Init(a *app.App) (err error) {
	i.store = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	i.storageService = a.MustComponent(spacestorage.CName).(storage.ClientStorage)
	i.source = a.MustComponent(source.CName).(source.Service)
	i.btHash = a.MustComponent("builtintemplate").(Hasher)
	i.fileStore = app.MustComponent[filestore.FileStore](a)
	i.ftsearch = app.MustComponent[ftsearch.FTSearch](a)
	i.picker = app.MustComponent[cache.ObjectGetter](a)
	i.ftQueueFinished = make(chan struct{})
	i.forceFt = make(chan struct{})
	i.config = app.MustComponent[*config.Config](a)
	i.subscriptionService = app.MustComponent[subscription.Service](a)
	i.componentCtx, i.componentCtxCancel = context.WithCancel(context.Background())
	i.spaceIndexers = map[string]*spaceIndexer{}
	return
}

func (i *indexer) Name() (name string) {
	return CName
}

func (i *indexer) Run(context.Context) (err error) {
	err = i.subscribeToSpaces()
	if err != nil {
		return
	}
	return i.StartFullTextIndex()
}

func (i *indexer) StartFullTextIndex() (err error) {
	if ftErr := i.ftInit(); ftErr != nil {
		log.Errorf("can't init ft: %v", ftErr)
	}
	i.ftQueueFinished = make(chan struct{})
	go i.ftLoopRoutine()
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
	if i.componentCtxCancel != nil {
		i.componentCtxCancel()
		// we need to wait for the ftQueue processing to be finished gracefully. Because we may be in the middle of badger transaction
		<-i.ftQueueFinished
	}
	return nil
}

func (i *indexer) RemoveAclIndexes(spaceId string) (err error) {
	ids, _, err := i.store.SpaceIndex(spaceId).QueryObjectIds(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_participant)),
			},
		},
	})
	if err != nil {
		return
	}
	return i.store.SpaceIndex(spaceId).DeleteDetails(i.componentCtx, ids)
}

func (i *indexer) Index(ctx context.Context, info smartblock.DocInfo, options ...smartblock.IndexOption) error {
	i.lock.Lock()
	spaceInd, ok := i.spaceIndexers[info.Space.Id()]
	if !ok {
		spaceInd = newSpaceIndexer(i.componentCtx, i.store.SpaceIndex(info.Space.Id()), i.store, i.storageService)
		i.spaceIndexers[info.Space.Id()] = spaceInd
	}
	i.lock.Unlock()

	return spaceInd.Index(ctx, info, options...)
}

// subscribeToSpaces subscribes to the lastOpenedSpaces subscription
// it used by fulltext and reindexing to prioritize most recent spaces
func (i *indexer) subscribeToSpaces() error {
	objectReq := subscription.SubscribeRequest{
		SubId:             "lastOpenedSpaces",
		Internal:          true,
		NoDepSubscription: true,
		Keys:              []string{bundle.RelationKeyTargetSpaceId.String(), bundle.RelationKeyLastOpenedDate.String(), bundle.RelationKeyLastModifiedDate.String()},
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_spaceView)),
			},
		},
		Sorts: []*model.BlockContentDataviewSort{
			{
				RelationKey:    bundle.RelationKeyLastOpenedDate.String(),
				Type:           model.BlockContentDataviewSort_Desc,
				IncludeTime:    true,
				Format:         model.RelationFormat_date,
				EmptyPlacement: model.BlockContentDataviewSort_End,
			},
		},
	}
	spacePriorityUpdateChan := make(chan []*types.Struct)
	go i.spacesPrioritySubscriptionWatcher(spacePriorityUpdateChan)
	i.spacesPrioritySubscription = syncsubscriptions.NewSubscription(i.subscriptionService, objectReq)
	return i.spacesPrioritySubscription.Run(spacePriorityUpdateChan)
}

func (i *indexer) spacesPriorityUpdate(priority []string) {
	i.lock.Lock()
	defer i.lock.Unlock()
	i.spacesPriority = priority
}

func (i *indexer) spacesPriorityGet() []string {
	i.lock.Lock()
	defer i.lock.Unlock()
	return i.spacesPriority
}

func (i *indexer) spacesPrioritySubscriptionWatcher(ch chan []*types.Struct) {
	for {
		select {
		// subscription and chan will be closed on indexer close
		case records := <-ch:
			if records == nil {
				return
			}
			i.spacesPriorityUpdate(pbtypes.ExtractString(records, bundle.RelationKeyTargetSpaceId.String(), true))
		}
	}
}
