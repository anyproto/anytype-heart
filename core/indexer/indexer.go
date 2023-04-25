package indexer

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/anytypeio/any-sync/app"
	"github.com/gogo/protobuf/types"
	ds "github.com/ipfs/go-datastore"
	"github.com/textileio/go-threads/core/thread"
	"golang.org/x/exp/slices"

	"github.com/anytypeio/go-anytype-middleware/core/anytype/config"
	"github.com/anytypeio/go-anytype-middleware/core/block"
	smartblock2 "github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/filestore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/space"
	"github.com/anytypeio/go-anytype-middleware/space/typeprovider"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

const (
	CName = "indexer"

	// ### Increasing counters below will trigger existing account to reindex their

	// ForceThreadsObjectsReindexCounter reindex thread-based objects
	ForceThreadsObjectsReindexCounter int32 = 8
	// ForceFilesReindexCounter reindex ipfs-file-based objects
	ForceFilesReindexCounter int32 = 9 //
	// ForceBundledObjectsReindexCounter reindex objects like anytypeProfile
	ForceBundledObjectsReindexCounter int32 = 5 // reindex objects like anytypeProfile
	// ForceIdxRebuildCounter erases localstore indexes and reindex all type of objects
	// (no need to increase ForceThreadsObjectsReindexCounter & ForceFilesReindexCounter)
	ForceIdxRebuildCounter int32 = 37
	// ForceFulltextIndexCounter  performs fulltext indexing for all type of objects (useful when we change fulltext config)
	ForceFulltextIndexCounter int32 = 4
	// ForceFilestoreKeysReindexCounter reindex filestore keys in all objects
	ForceFilestoreKeysReindexCounter int32 = 2
)

var log = logging.Logger("anytype-doc-indexer")

var (
	ftIndexInterval         = time.Minute
	ftIndexForceMinInterval = time.Second * 10
)

func New() Indexer {
	return &indexer{}
}

type Indexer interface {
	ForceFTIndex()
	app.ComponentRunnable
}

type ThreadLister interface {
	Threads() (thread.IDSlice, error)
}

type Hasher interface {
	Hash() string
}

type reindexFlags struct {
	bundledTypes            bool
	removeAllIndexedObjects bool
	bundledRelations        bool
	eraseIndexes            bool
	threadObjects           bool
	fileObjects             bool
	fulltext                bool
	bundledTemplates        bool
	bundledObjects          bool
	fileKeys                bool
}

func (f *reindexFlags) any() bool {
	return f.bundledTypes ||
		f.removeAllIndexedObjects ||
		f.bundledRelations ||
		f.eraseIndexes ||
		f.threadObjects ||
		f.fileObjects ||
		f.fulltext ||
		f.bundledTemplates ||
		f.bundledObjects ||
		f.fileKeys
}

func (f *reindexFlags) enableAll() {
	f.bundledTypes = true
	f.removeAllIndexedObjects = true
	f.bundledRelations = true
	f.eraseIndexes = true
	f.threadObjects = true
	f.fileObjects = true
	f.fulltext = true
	f.bundledTemplates = true
	f.bundledObjects = true
	f.fileKeys = true
}

func (f *reindexFlags) String() string {
	return fmt.Sprintf("%#v", f)
}

type indexer struct {
	store     objectstore.ObjectStore
	fileStore filestore.FileStore
	// todo: move logstore to separate component?
	anytype core.Service
	source  source.Service
	picker  block.Picker

	quit        chan struct{}
	mu          sync.Mutex
	btHash      Hasher
	archivedMap map[string]struct{}
	favoriteMap map[string]struct{}
	newAccount  bool
	forceFt     chan struct{}

	typeProvider typeprovider.ObjectTypeProvider
	spaceService space.Service
}

func (i *indexer) Init(a *app.App) (err error) {
	i.newAccount = a.MustComponent(config.CName).(*config.Config).NewAccount
	i.anytype = a.MustComponent(core.CName).(core.Service)
	i.store = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	i.typeProvider = a.MustComponent(typeprovider.CName).(typeprovider.ObjectTypeProvider)
	i.source = a.MustComponent(source.CName).(source.Service)
	i.btHash = a.MustComponent("builtintemplate").(Hasher)
	i.picker = app.MustComponent[block.Picker](a)
	i.fileStore = app.MustComponent[filestore.FileStore](a)
	i.spaceService = app.MustComponent[space.Service](a)
	i.quit = make(chan struct{})
	i.archivedMap = make(map[string]struct{}, 100)
	i.favoriteMap = make(map[string]struct{}, 100)
	i.forceFt = make(chan struct{})
	return
}

func (i *indexer) Name() (name string) {
	return CName
}

func (i *indexer) Run(context.Context) (err error) {
	if ftErr := i.ftInit(); ftErr != nil {
		log.Errorf("can't init ft: %v", ftErr)
	}
	err = i.reindexIfNeeded()
	if err != nil {
		return err
	}
	i.migrateRemoveNonindexableObjects()
	go i.ftLoop()
	return
}

func (i *indexer) migrateRemoveNonindexableObjects() {
	ids, err := i.getIdsForTypes(
		smartblock.SmartblockTypeMarketplaceType, smartblock.SmartblockTypeMarketplaceRelation,
		smartblock.SmartblockTypeMarketplaceTemplate, smartblock.SmartBlockTypeDate, smartblock.SmartBlockTypeBreadcrumbs,
	)
	if err != nil {
		log.Errorf("migrateRemoveNonindexableObjects: failed to get ids: %s", err.Error())
	}

	for _, id := range ids {
		err = i.store.DeleteDetails(id)
		if err != nil {
			log.Errorf("migrateRemoveNonindexableObjects: failed to get ids: %s", err.Error())
		}
	}
}

func (i *indexer) Close(ctx context.Context) (err error) {
	i.mu.Lock()
	quit := i.quit
	i.mu.Unlock()
	if quit != nil {
		close(quit)
		i.mu.Lock()
		i.quit = nil
		i.mu.Unlock()
	}
	return nil
}

func (i *indexer) Index(ctx context.Context, info smartblock2.DocInfo) error {
	startTime := time.Now()
	sbType, err := i.typeProvider.Type(info.Id)
	if err != nil {
		sbType = smartblock.SmartBlockTypePage
	}
	saveIndexedHash := func() {
		if headsHash := headsHash(info.Heads); headsHash != "" {
			err = i.store.SaveLastIndexedHeadsHash(info.Id, headsHash)
			if err != nil {
				log.With("thread", info.Id).Errorf("failed to save indexed heads hash: %v", err)
			}
		}
	}

	indexDetails, indexLinks := sbType.Indexable()
	if !indexDetails && !indexLinks {
		saveIndexedHash()
		return nil
	}

	details := info.State.CombinedDetails()
	details.Fields[bundle.RelationKeyLinks.String()] = pbtypes.StringList(info.Links)
	setCreator := pbtypes.GetString(info.State.LocalDetails(), bundle.RelationKeyCreator.String())
	if setCreator == "" {
		setCreator = i.anytype.ProfileID()
	}
	indexSetTime := time.Now()
	var hasError bool
	if indexLinks {
		if err = i.store.UpdateObjectLinks(info.Id, info.Links); err != nil {
			hasError = true
			log.With("thread", info.Id).Errorf("failed to save object links: %v", err)
		}
	}

	indexLinksTime := time.Now()
	if indexDetails {
		if err := i.store.UpdateObjectDetails(info.Id, details, false); err != nil {
			hasError = true
			log.With("thread", info.Id).Errorf("can't update object store: %v", err)
		}
		if err := i.store.AddToIndexQueue(info.Id); err != nil {
			log.With("thread", info.Id).Errorf("can't add id to index queue: %v", err)
		} else {
			log.With("thread", info.Id).Debugf("to index queue")
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

	metrics.SharedClient.RecordEvent(metrics.IndexEvent{
		ObjectId:                info.Id,
		IndexLinksTimeMs:        indexLinksTime.Sub(indexSetTime).Milliseconds(),
		IndexDetailsTimeMs:      indexDetailsTime.Sub(indexLinksTime).Milliseconds(),
		IndexSetRelationsTimeMs: indexSetTime.Sub(startTime).Milliseconds(),
		RelationsCount:          len(info.State.PickRelationLinks()),
		DetailsCount:            detailsCount,
	})

	return nil
}

func (i *indexer) reindexIfNeeded() error {
	checksums, err := i.store.GetChecksums()
	if err != nil && err != ds.ErrNotFound {
		return err
	}
	if checksums == nil {
		checksums = &model.ObjectStoreChecksums{
			// do no add bundled relations checksums, because we want to index them for new accounts
			ObjectsForceReindexCounter:       ForceThreadsObjectsReindexCounter,
			FilesForceReindexCounter:         ForceFilesReindexCounter,
			IdxRebuildCounter:                ForceIdxRebuildCounter,
			FilestoreKeysForceReindexCounter: ForceFilestoreKeysReindexCounter,
		}
	}

	var flags reindexFlags
	if checksums.BundledRelations != bundle.RelationChecksum {
		flags.bundledRelations = true
	}
	if checksums.BundledObjectTypes != bundle.TypeChecksum {
		flags.bundledTypes = true
	}
	if checksums.ObjectsForceReindexCounter != ForceThreadsObjectsReindexCounter {
		flags.threadObjects = true
	}
	if checksums.FilestoreKeysForceReindexCounter != ForceFilestoreKeysReindexCounter {
		flags.fileKeys = true
	}
	if checksums.FilesForceReindexCounter != ForceFilesReindexCounter {
		flags.fileObjects = true
	}
	if checksums.FulltextRebuild != ForceFulltextIndexCounter {
		flags.fulltext = true
	}
	if checksums.BundledTemplates != i.btHash.Hash() {
		flags.bundledTemplates = true
	}
	if checksums.BundledObjects != ForceBundledObjectsReindexCounter {
		flags.bundledObjects = true
	}
	if checksums.IdxRebuildCounter != ForceIdxRebuildCounter {
		flags.enableAll()
	}
	return i.reindex(context.WithValue(context.TODO(), metrics.CtxKeyRequest, "reindex_forced"), flags)
}

func (i *indexer) reindex(ctx context.Context, flags reindexFlags) (err error) {
	if flags.any() {
		log.Infof("start store reindex (%s)", flags.String())
	}

	if flags.fileKeys {
		err = i.fileStore.RemoveEmpty()
		if err != nil {
			log.Errorf("reindex failed to RemoveEmpty filekeys: %v", err.Error())
		} else {
			log.Infof("RemoveEmpty filekeys succeed")
		}
	}

	if flags.removeAllIndexedObjects {
		ids, err := i.store.ListIds()
		if err != nil {
			log.Errorf("reindex failed to get all ids(removeAllIndexedObjects): %v", err.Error())
		}
		for _, id := range ids {
			err = i.store.DeleteDetails(id)
			if err != nil {
				log.Errorf("reindex failed to delete details(removeAllIndexedObjects): %v", err.Error())
			}
		}
	}
	var indexesWereRemoved bool
	if flags.eraseIndexes {
		err = i.store.EraseIndexes()
		if err != nil {
			log.Errorf("reindex failed to erase indexes: %v", err.Error())
		} else {
			log.Infof("all store indexes succesfully erased")
			// store this flag because underlying localstore needs to now if it needs to amend indexes based on the prev value
			indexesWereRemoved = true
		}
	}

	// We derive or init predefined blocks here in order to ensure consistency of object store.
	// If we call this method before removing objects from store, we will end up with inconsistent state
	// because indexing of predefined objects will not run again
	err = i.anytype.EnsurePredefinedBlocks(ctx)
	if err != nil {
		return err
	}

	if flags.any() {
		d, err := i.getObjectInfo(ctx, i.anytype.PredefinedBlocks().Archive)
		if err != nil {
			log.Errorf("reindex failed to open archive: %s", err.Error())
		} else {
			for _, target := range d.Links {
				i.archivedMap[target] = struct{}{}
			}
		}

		d, err = i.getObjectInfo(ctx, i.anytype.PredefinedBlocks().Home)
		if err != nil {
			log.Errorf("reindex failed to open archive: %s", err.Error())
		} else {
			for _, b := range d.Links {
				i.favoriteMap[b] = struct{}{}
			}
		}
	}

	// for all ids except home and archive setting cache timeout for reindexing
	// ctx = context.WithValue(ctx, ocache.CacheTimeout, cacheTimeout)
	if flags.threadObjects {
		ids, err := i.getIdsForTypes(
			smartblock.SmartBlockTypePage,
			smartblock.SmartBlockTypeSet,
			smartblock.SmartBlockTypeObjectType,
			smartblock.SmartBlockTypeProfilePage,
			smartblock.SmartBlockTypeTemplate,
			smartblock.SmartblockTypeMarketplaceType,
			smartblock.SmartblockTypeMarketplaceTemplate,
			smartblock.SmartblockTypeMarketplaceRelation,
			smartblock.SmartBlockTypeArchive,
			smartblock.SmartBlockTypeHome,
			smartblock.SmartBlockTypeWorkspaceOld,
		)
		if err != nil {
			return err
		}
		start := time.Now()
		successfullyReindexed := i.reindexIdsIgnoreErr(ctx, indexesWereRemoved, ids...)
		if metrics.Enabled {
			metrics.SharedClient.RecordEvent(metrics.ReindexEvent{
				ReindexType:    metrics.ReindexTypeThreads,
				Total:          len(ids),
				Success:        successfullyReindexed,
				SpentMs:        int(time.Since(start).Milliseconds()),
				IndexesRemoved: indexesWereRemoved,
			})
		}
		log.Infof("%d/%d objects have been successfully reindexed", successfullyReindexed, len(ids))
	} else {
		go func() {
			start := time.Now()
			total, success, err := i.reindexOutdatedThreads()
			if err != nil {
				log.Infof("failed to reindex outdated objects: %s", err.Error())
			} else {
				log.Infof("%d/%d outdated objects have been successfully reindexed", success, total)
			}
			if metrics.Enabled && total > 0 {
				metrics.SharedClient.RecordEvent(metrics.ReindexEvent{
					ReindexType:    metrics.ReindexTypeOutdatedHeads,
					Total:          total,
					Success:        success,
					SpentMs:        int(time.Since(start).Milliseconds()),
					IndexesRemoved: indexesWereRemoved,
				})
			}
		}()
	}

	if flags.fileObjects {
		ids, err := i.getIdsForTypes(smartblock.SmartBlockTypeFile)
		if err != nil {
			return err
		}
		start := time.Now()
		successfullyReindexed := i.reindexIdsIgnoreErr(ctx, indexesWereRemoved, ids...)
		if metrics.Enabled && len(ids) > 0 {
			metrics.SharedClient.RecordEvent(metrics.ReindexEvent{
				ReindexType:    metrics.ReindexTypeFiles,
				Total:          len(ids),
				Success:        successfullyReindexed,
				SpentMs:        int(time.Since(start).Milliseconds()),
				IndexesRemoved: indexesWereRemoved,
			})
		}
		msg := fmt.Sprintf("%d/%d files have been successfully reindexed", successfullyReindexed, len(ids))
		if len(ids)-successfullyReindexed != 0 {
			log.Error(msg)
		} else {
			log.Info(msg)
		}
	}
	if flags.bundledRelations {
		ids, err := i.getIdsForTypes(smartblock.SmartBlockTypeBundledRelation)
		if err != nil {
			return err
		}
		start := time.Now()
		successfullyReindexed := i.reindexIdsIgnoreErr(ctx, indexesWereRemoved, ids...)
		if metrics.Enabled && len(ids) > 0 {
			metrics.SharedClient.RecordEvent(metrics.ReindexEvent{
				ReindexType:    metrics.ReindexTypeBundledRelations,
				Total:          len(ids),
				Success:        successfullyReindexed,
				SpentMs:        int(time.Since(start).Milliseconds()),
				IndexesRemoved: indexesWereRemoved,
			})
		}
		msg := fmt.Sprintf("%d/%d bundled relations have been successfully reindexed", successfullyReindexed, len(ids))
		if len(ids)-successfullyReindexed != 0 {
			log.Error(msg)
		} else {
			log.Info(msg)
		}
	}
	if flags.bundledTypes {
		// lets add anytypeProfile here, because it's seems too much to create one more counter especially for it
		ids, err := i.getIdsForTypes(smartblock.SmartBlockTypeBundledObjectType, smartblock.SmartBlockTypeAnytypeProfile)
		if err != nil {
			return err
		}
		start := time.Now()
		successfullyReindexed := i.reindexIdsIgnoreErr(ctx, indexesWereRemoved, ids...)
		if metrics.Enabled && len(ids) > 0 {
			metrics.SharedClient.RecordEvent(metrics.ReindexEvent{
				ReindexType:    metrics.ReindexTypeBundledTypes,
				Total:          len(ids),
				Success:        successfullyReindexed,
				SpentMs:        int(time.Since(start).Milliseconds()),
				IndexesRemoved: indexesWereRemoved,
			})
		}
		msg := fmt.Sprintf("%d/%d bundled types have been successfully reindexed", successfullyReindexed, len(ids))
		if len(ids)-successfullyReindexed != 0 {
			log.Error(msg)
		} else {
			log.Info(msg)
		}

		var ots = make([]string, 0, len(bundle.SystemTypes))
		for _, ot := range bundle.SystemTypes {
			ots = append(ots, ot.BundledURL())
		}

		for _, ot := range bundle.InternalTypes {
			ots = append(ots, ot.BundledURL())
		}

		var rels = make([]*model.Relation, 0, len(bundle.RequiredInternalRelations))
		for _, rel := range bundle.SystemRelations {
			rels = append(rels, bundle.MustGetRelation(rel))
		}
	}
	if flags.bundledObjects {
		// hardcoded for now
		ids := []string{addr.AnytypeProfileId}
		start := time.Now()
		successfullyReindexed := i.reindexIdsIgnoreErr(ctx, indexesWereRemoved, ids...)
		if metrics.Enabled && len(ids) > 0 {
			metrics.SharedClient.RecordEvent(metrics.ReindexEvent{
				ReindexType: metrics.ReindexTypeBundledTemplates,
				Total:       len(ids),
				Success:     successfullyReindexed,
				SpentMs:     int(time.Since(start).Milliseconds()),
			})
		}
		msg := fmt.Sprintf("%d/%d bundled objects have been successfully reindexed", successfullyReindexed, len(ids))
		if len(ids)-successfullyReindexed != 0 {
			log.Error(msg)
		} else {
			log.Info(msg)
		}
	}

	if flags.bundledTemplates {
		existsRec, _, err := i.store.QueryObjectInfo(database.Query{}, []smartblock.SmartBlockType{smartblock.SmartBlockTypeBundledTemplate})
		if err != nil {
			return err
		}
		existsIds := make([]string, 0, len(existsRec))
		for _, rec := range existsRec {
			existsIds = append(existsIds, rec.Id)
		}
		ids, err := i.getIdsForTypes(smartblock.SmartBlockTypeBundledTemplate)
		if err != nil {
			return err
		}
		var removed int
		for _, eId := range existsIds {
			if slice.FindPos(ids, eId) == -1 {
				removed++
				i.store.DeleteObject(eId)
			}
		}
		successfullyReindexed := i.reindexIdsIgnoreErr(ctx, indexesWereRemoved, ids...)
		msg := fmt.Sprintf("%d/%d bundled templates have been successfully reindexed; removed: %d", successfullyReindexed, len(ids), removed)
		if len(ids)-successfullyReindexed != 0 {
			log.Error(msg)
		} else {
			log.Info(msg)
		}
	}
	if flags.fulltext {
		var ids []string
		ids, err := i.getIdsForTypes(smartblock.SmartBlockTypePage, smartblock.SmartBlockTypeFile, smartblock.SmartBlockTypeBundledRelation, smartblock.SmartBlockTypeBundledObjectType, smartblock.SmartBlockTypeAnytypeProfile)
		if err != nil {
			return err
		}

		var addedToQueue int
		for _, id := range ids {
			if err := i.store.AddToIndexQueue(id); err != nil {
				log.Errorf("failed to add to index queue: %v", err)
			} else {
				addedToQueue++
			}
		}
		msg := fmt.Sprintf("%d/%d objects have been successfully added to the fulltext queue", addedToQueue, len(ids))
		if len(ids)-addedToQueue != 0 {
			log.Error(msg)
		} else {
			log.Info(msg)
		}
	}

	return i.saveLatestChecksums()
}

func (i *indexer) saveLatestChecksums() error {
	// todo: add layout indexing when needed
	checksums := model.ObjectStoreChecksums{
		BundledObjectTypes:         bundle.TypeChecksum,
		BundledRelations:           bundle.RelationChecksum,
		BundledTemplates:           i.btHash.Hash(),
		ObjectsForceReindexCounter: ForceThreadsObjectsReindexCounter,
		FilesForceReindexCounter:   ForceFilesReindexCounter,

		IdxRebuildCounter:                ForceIdxRebuildCounter,
		FulltextRebuild:                  ForceFulltextIndexCounter,
		BundledObjects:                   ForceBundledObjectsReindexCounter,
		FilestoreKeysForceReindexCounter: ForceFilestoreKeysReindexCounter,
	}
	return i.store.SaveChecksums(&checksums)
}

func (i *indexer) saveLatestCounters() error {
	// todo: add layout indexing when needed
	checksums := model.ObjectStoreChecksums{
		BundledObjectTypes:               bundle.TypeChecksum,
		BundledRelations:                 bundle.RelationChecksum,
		BundledTemplates:                 i.btHash.Hash(),
		ObjectsForceReindexCounter:       ForceThreadsObjectsReindexCounter,
		FilesForceReindexCounter:         ForceFilesReindexCounter,
		IdxRebuildCounter:                ForceIdxRebuildCounter,
		FulltextRebuild:                  ForceFulltextIndexCounter,
		BundledObjects:                   ForceBundledObjectsReindexCounter,
		FilestoreKeysForceReindexCounter: ForceFilestoreKeysReindexCounter,
	}
	return i.store.SaveChecksums(&checksums)
}

func (i *indexer) reindexOutdatedThreads() (toReindex, success int, err error) {
	spc, err := i.spaceService.AccountSpace(context.Background())
	if err != nil {
		return
	}

	tids := spc.StoredIds()
	var idsToReindex []string
	for _, tid := range tids {
		logErr := func(err error) {
			log.With("tree", tid).Errorf("reindexOutdatedThreads failed to get tree to reindex: %s", err.Error())
		}

		lastHash, err := i.store.GetLastIndexedHeadsHash(tid)
		if err != nil {
			logErr(err)
			continue
		}
		info, err := spc.Storage().TreeStorage(tid)
		if err != nil {
			logErr(err)
			continue
		}
		heads, err := info.Heads()
		if err != nil {
			logErr(err)
			continue
		}

		hh := headsHash(heads)
		if lastHash != hh {
			log.With("tree", tid).Warnf("not equal indexed heads hash: %s!=%s (%d logs)", lastHash, hh, len(heads))
			idsToReindex = append(idsToReindex, tid)
		}
	}
	if len(idsToReindex) > 0 {
		for _, id := range idsToReindex {
			// TODO: we should reindex it I guess at start
			// if i.anytype.PredefinedBlocks().IsAccount(id) {
			//	continue
			// }

			ctx := context.WithValue(context.Background(), metrics.CtxKeyRequest, "reindexOutdatedThreads")
			d, err := i.getObjectInfo(ctx, id)
			if err != nil {
				continue
			}

			err = i.Index(ctx, d)
			if err == nil {
				success++
			} else {
				log.With("thread", id).Errorf("reindexOutdatedThreads failed to index doc: %s", err.Error())
			}
		}
	}
	return len(idsToReindex), success, nil
}

func (i *indexer) reindexDoc(ctx context.Context, id string, indexesWereRemoved bool) error {
	t, err := i.typeProvider.Type(id)
	if err != nil {
		return fmt.Errorf("incorrect sb type: %v", err)
	}

	d, err := i.getObjectInfo(ctx, id)
	if err != nil {
		log.Errorf("reindexDoc failed to open %s: %s", id, err.Error())
		return fmt.Errorf("failed to open doc: %s", err.Error())
	}

	indexDetails, indexLinks := t.Indexable()
	if indexLinks {
		if err := i.store.UpdateObjectLinks(d.Id, d.Links); err != nil {
			log.With("thread", d.Id).Errorf("failed to save object links: %v", err)
		}
	}

	if !indexDetails {
		i.store.DeleteDetails(d.Id)
		return nil
	}

	details := d.State.CombinedDetails()
	_, isArchived := i.archivedMap[id]
	_, isFavorite := i.favoriteMap[id]

	details.Fields[bundle.RelationKeyIsArchived.String()] = pbtypes.Bool(isArchived)
	details.Fields[bundle.RelationKeyIsFavorite.String()] = pbtypes.Bool(isFavorite)
	details.Fields[bundle.RelationKeyLinks.String()] = pbtypes.StringList(d.Links)

	var curDetails *types.Struct
	curDetailsO, _ := i.store.GetDetails(id)
	if curDetailsO.GetDetails().GetFields() != nil {
		curDetails = curDetailsO.Details
	}
	// compare only real object scoped details
	detailsObjectScope := pbtypes.StructCutKeys(details, bundle.LocalRelationsKeys)
	curDetailsObjectScope := pbtypes.StructCutKeys(curDetails, bundle.LocalRelationsKeys)
	if indexesWereRemoved || curDetailsObjectScope == nil || !detailsObjectScope.Equal(curDetailsObjectScope) {
		if indexesWereRemoved || curDetails.GetFields() == nil {
			if err := i.store.CreateObject(id, details, d.Links, pbtypes.GetString(details, bundle.RelationKeyDescription.String())); err != nil {
				return fmt.Errorf("can't create object in the store: %v", err)
			}
		} else {
			if err := i.store.UpdateObjectDetails(id, details, true); err != nil {
				return fmt.Errorf("can't update object in the store: %v", err)
			}
		}
		if headsHash := headsHash(d.Heads); headsHash != "" {
			err = i.store.SaveLastIndexedHeadsHash(id, headsHash)
			if err != nil {
				log.With("thread", id).Errorf("failed to save indexed heads hash: %v", err)
			}
		}

		var skipFulltext bool
		if i.store.FTSearch() != nil {
			// skip fulltext if we already has the object indexed
			if exists, _ := i.store.FTSearch().Has(id); exists {
				skipFulltext = true
			}
		}

		if !skipFulltext {
			if err = i.store.AddToIndexQueue(id); err != nil {
				log.With("thread", id).Errorf("can't add to index: %v", err)
			}
		}
	}
	return nil
}

func (i *indexer) reindexIdsIgnoreErr(ctx context.Context, indexRemoved bool, ids ...string) (successfullyReindexed int) {
	for _, id := range ids {
		err := i.reindexDoc(ctx, id, indexRemoved)
		if err != nil {
			log.With("thread", id).Errorf("failed to reindex: %v", err)
		} else {
			successfullyReindexed++
		}
	}
	return
}

func (i *indexer) getObjectInfo(ctx context.Context, id string) (info smartblock2.DocInfo, err error) {
	err = block.DoWithContext(ctx, i.picker, id, func(sb smartblock2.SmartBlock) error {
		info = sb.GetDocInfo()
		return nil
	})
	return
}

func (i *indexer) getIdsForTypes(sbt ...smartblock.SmartBlockType) ([]string, error) {
	var ids []string
	for _, t := range sbt {
		st, err := i.source.SourceTypeBySbType(t)
		if err != nil {
			return nil, err
		}
		idsT, err := st.ListIds()
		if err != nil {
			return nil, err
		}
		ids = append(ids, idsT...)
	}
	return ids, nil
}

func extractOldRelationsFromState(s *state.State) []*model.Relation {
	var rels []*model.Relation
	if objRels := s.OldExtraRelations(); len(objRels) > 0 {
		rels = append(rels, s.OldExtraRelations()...)
	}

	if dvBlock := s.Pick(template.DataviewBlockId); dvBlock != nil {
		rels = append(rels, dvBlock.Model().GetDataview().GetRelations()...)
	}

	return rels
}

func headsHash(heads []string) string {
	if len(heads) == 0 {
		return ""
	}
	slices.Sort(heads)

	sum := sha256.Sum256([]byte(strings.Join(heads, ",")))
	return fmt.Sprintf("%x", sum)
}
