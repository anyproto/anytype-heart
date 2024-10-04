package indexer

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/syncstatus/syncsubscriptions"
	"github.com/anyproto/anytype-heart/metrics"
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

	componentCtx    context.Context
	componentCancel context.CancelFunc
	ftQueueFinished chan struct{}
	config          *config.Config

	btHash  Hasher
	forceFt chan struct{}

	spacesPrioritySubscription *syncsubscriptions.ObjectSubscription[*types.Struct]
	lock                       sync.Mutex
	reindexLogFields           []zap.Field
	spacesPriority             []string
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
	i.componentCtx, i.componentCancel = context.WithCancel(context.Background())
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
	go i.ftLoopRoutine()
	return
}

func (i *indexer) Close(ctx context.Context) (err error) {
	i.componentCancel()
	i.spacesPrioritySubscription.Close()
	// we need to wait for the ftQueue processing to be finished gracefully. Because we may be in the middle of badger transaction
	<-i.ftQueueFinished
	return nil
}

func (i *indexer) RemoveAclIndexes(spaceId string) (err error) {
	ids, _, err := i.store.QueryObjectIDs(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeySpaceId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(spaceId),
			},
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
	return i.store.DeleteDetails(ids...)
}

func (i *indexer) Index(ctx context.Context, info smartblock.DocInfo, options ...smartblock.IndexOption) error {
	// options are stored in smartblock pkg because of cyclic dependency :(
	startTime := time.Now()
	opts := &smartblock.IndexOptions{}
	for _, o := range options {
		o(opts)
	}

	err := i.storageService.BindSpaceID(info.Space.Id(), info.Id)
	if err != nil {
		log.Error("failed to bind space id", zap.Error(err), zap.String("id", info.Id))
		return err
	}
	headHashToIndex := headsHash(info.Heads)
	saveIndexedHash := func() {
		if headHashToIndex == "" {
			return
		}

		err = i.store.SaveLastIndexedHeadsHash(info.Id, headHashToIndex)
		if err != nil {
			log.With("objectID", info.Id).Errorf("failed to save indexed heads hash: %v", err)
		}
	}

	indexDetails, indexLinks := info.SmartblockType.Indexable()
	if !indexDetails && !indexLinks {
		return nil
	}

	lastIndexedHash, err := i.store.GetLastIndexedHeadsHash(info.Id)
	if err != nil {
		log.With("object", info.Id).Errorf("failed to get last indexed heads hash: %v", err)
	}

	if opts.SkipIfHeadsNotChanged {
		if headHashToIndex == "" {
			log.With("objectID", info.Id).Errorf("heads hash is empty")
		} else if lastIndexedHash == headHashToIndex {
			log.With("objectID", info.Id).Debugf("heads not changed, skipping indexing")
			return nil
		}
	}

	details := info.Details

	indexSetTime := time.Now()
	var hasError bool
	if indexLinks {
		if err = i.store.UpdateObjectLinks(info.Id, info.Links); err != nil {
			hasError = true
			log.With("objectID", info.Id).Errorf("failed to save object links: %v", err)
		}
	}

	indexLinksTime := time.Now()
	if indexDetails {
		if err := i.store.UpdateObjectDetails(ctx, info.Id, details); err != nil {
			hasError = true
			log.With("objectID", info.Id).Errorf("can't update object store: %v", err)
		} else {
			// todo: remove temp log
			if lastIndexedHash == headHashToIndex {
				l := log.With("objectID", info.Id).
					With("hashesAreEqual", lastIndexedHash == headHashToIndex).
					With("lastHashIsEmpty", lastIndexedHash == "").
					With("skipFlagSet", opts.SkipIfHeadsNotChanged)

				if opts.SkipIfHeadsNotChanged {
					l.Warnf("details have changed, but heads are equal")
				} else {
					l.Debugf("details have changed, but heads are equal")
				}
			}
		}

		if !(opts.SkipFullTextIfHeadsNotChanged && lastIndexedHash == headHashToIndex) {
			if err := i.store.AddToIndexQueue(domain.FullID{SpaceID: info.Space.Id(), ObjectID: info.Id}); err != nil {
				log.With("objectID", info.Id).Errorf("can't add id to index queue: %v", err)
			}
		}
	} else {
		_ = i.store.DeleteDetails(info.Id)
	}
	indexDetailsTime := time.Now()
	detailsCount := 0
	if details.GetFields() != nil {
		detailsCount = len(details.GetFields())
	}

	if !hasError {
		saveIndexedHash()
	}

	metrics.Service.Send(&metrics.IndexEvent{
		ObjectId:                info.Id,
		IndexLinksTimeMs:        indexLinksTime.Sub(indexSetTime).Milliseconds(),
		IndexDetailsTimeMs:      indexDetailsTime.Sub(indexLinksTime).Milliseconds(),
		IndexSetRelationsTimeMs: indexSetTime.Sub(startTime).Milliseconds(),
		DetailsCount:            detailsCount,
	})

	return nil
}

func headsHash(heads []string) string {
	if len(heads) == 0 {
		return ""
	}
	slices.Sort(heads)

	sum := sha256.Sum256([]byte(strings.Join(heads, ",")))
	return fmt.Sprintf("%x", sum)
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
