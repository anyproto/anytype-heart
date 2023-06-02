package indexer

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/gogo/protobuf/types"
	ds "github.com/ipfs/go-datastore"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/editor"
	smartblock2 "github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/relation/relationutils"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/typeprovider"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

const (
	CName = "indexer"

	// ### Increasing counters below will trigger existing account to reindex their

	// ForceThreadsObjectsReindexCounter reindex thread-based objects
	ForceThreadsObjectsReindexCounter int32 = 8
	// ForceFilesReindexCounter reindex ipfs-file-based objects
	ForceFilesReindexCounter int32 = 11 //
	// ForceBundledObjectsReindexCounter reindex objects like anytypeProfile
	ForceBundledObjectsReindexCounter int32 = 5 // reindex objects like anytypeProfile
	// ForceIdxRebuildCounter erases localstore indexes and reindex all type of objects
	// (no need to increase ForceThreadsObjectsReindexCounter & ForceFilesReindexCounter)
	ForceIdxRebuildCounter int32 = 41
	// ForceFulltextIndexCounter  performs fulltext indexing for all type of objects (useful when we change fulltext config)
	ForceFulltextIndexCounter int32 = 5
	// ForceFilestoreKeysReindexCounter reindex filestore keys in all objects
	ForceFilestoreKeysReindexCounter int32 = 2
)

var log = logging.Logger("anytype-doc-indexer")

var (
	ftIndexInterval         = 10 * time.Second
	ftIndexForceMinInterval = time.Second * 10
)

func New(
	picker block.Picker,
	spaceService space.Service,
	fileService files.Service,
) Indexer {
	return &indexer{
		picker:       picker,
		spaceService: spaceService,
		fileService:  fileService,
		indexedFiles: &sync.Map{},
	}
}

type Indexer interface {
	ForceFTIndex()
	Index(ctx context.Context, info smartblock2.DocInfo, options ...smartblock2.IndexOption) error
	app.ComponentRunnable
}

type Hasher interface {
	Hash() string
}

type subObjectCreator interface {
	CreateSubObjectsInWorkspace(details []*types.Struct) (ids []string, objects []*types.Struct, err error)
}

type syncStarter interface {
	StartSync()
}

type indexer struct {
	store            objectstore.ObjectStore
	fileStore        filestore.FileStore
	anytype          core.Service
	source           source.Service
	picker           block.Picker
	ftsearch         ftsearch.FTSearch
	subObjectCreator subObjectCreator
	syncStarter      syncStarter
	fileService      files.Service

	quit       chan struct{}
	mu         sync.Mutex
	btHash     Hasher
	newAccount bool
	forceFt    chan struct{}

	typeProvider typeprovider.SmartBlockTypeProvider
	spaceService space.Service

	indexedFiles *sync.Map
}

func (i *indexer) Init(a *app.App) (err error) {
	i.newAccount = a.MustComponent(config.CName).(*config.Config).NewAccount
	i.anytype = a.MustComponent(core.CName).(core.Service)
	i.store = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	i.typeProvider = a.MustComponent(typeprovider.CName).(typeprovider.SmartBlockTypeProvider)
	i.source = a.MustComponent(source.CName).(source.Service)
	i.btHash = a.MustComponent("builtintemplate").(Hasher)
	i.fileStore = app.MustComponent[filestore.FileStore](a)
	i.ftsearch = app.MustComponent[ftsearch.FTSearch](a)
	i.subObjectCreator = app.MustComponent[subObjectCreator](a)
	i.syncStarter = app.MustComponent[syncStarter](a)
	i.quit = make(chan struct{})
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
		smartblock.SmartBlockTypeDate,
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

func (i *indexer) Index(ctx context.Context, info smartblock2.DocInfo, options ...smartblock2.IndexOption) error {
	// options are stored in smartblock pkg because of cyclic dependency :(
	startTime := time.Now()
	opts := &smartblock2.IndexOptions{}
	for _, o := range options {
		o(opts)
	}
	sbType, err := i.typeProvider.Type(info.Id)
	if err != nil {
		sbType = smartblock.SmartBlockTypePage
	}
	headHashToIndex := headsHash(info.Heads)
	saveIndexedHash := func() {
		if headHashToIndex == "" {
			return
		}

		err = i.store.SaveLastIndexedHeadsHash(info.Id, headHashToIndex)
		if err != nil {
			log.With("thread", info.Id).Errorf("failed to save indexed heads hash: %v", err)
		}
	}

	indexDetails, indexLinks := sbType.Indexable()
	if !indexDetails && !indexLinks {
		saveIndexedHash()
		return nil
	}

	lastIndexedHash, err := i.store.GetLastIndexedHeadsHash(info.Id)
	if err != nil {
		log.With("thread", info.Id).Errorf("failed to get last indexed heads hash: %v", err)
	}

	if opts.SkipIfHeadsNotChanged {
		if headHashToIndex == "" {
			log.With("thread", info.Id).Errorf("heads hash is empty")
		} else if lastIndexedHash == headHashToIndex {
			log.With("thread", info.Id).Debugf("heads not changed, skipping indexing")
			return nil
		}
	}

	details := info.State.CombinedDetails()

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

		if !(opts.SkipFullTextIfHeadsNotChanged && lastIndexedHash == headHashToIndex) {
			if err := i.store.AddToIndexQueue(info.Id); err != nil {
				log.With("thread", info.Id).Errorf("can't add id to index queue: %v", err)
			} else {
				log.With("thread", info.Id).Debugf("to index queue")
			}
		}

		i.indexLinkedFiles(ctx, info.FileHashes)
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

func (i *indexer) indexLinkedFiles(ctx context.Context, fileHashes []string) {
	if len(fileHashes) == 0 {
		return
	}
	existingIDs, err := i.store.HasIDs(fileHashes...)
	if err != nil {
		log.Errorf("failed to get existing file ids : %s", err.Error())
	}
	newIDs := slice.Difference(fileHashes, existingIDs)
	for _, id := range newIDs {
		go func(id string) {
			// Deduplicate
			_, ok := i.indexedFiles.LoadOrStore(id, struct{}{})
			if ok {
				return
			}
			// file's hash is id
			err = i.reindexDoc(ctx, id)
			if err != nil {
				log.With("id", id).Errorf("failed to reindex file: %s", err.Error())
			}
			err = i.store.AddToIndexQueue(id)
			if err != nil {
				log.With("id", id).Error(err.Error())
			}
		}(id)
	}
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
			log.Infof("all store indexes successfully erased")
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
	// starting sync of all other objects later, because we don't want to have problems with loading of derived objects
	// due to parallel load which can overload the stream
	i.syncStarter.StartSync()

	// for all ids except home and archive setting cache timeout for reindexing
	// ctx = context.WithValue(ctx, ocache.CacheTimeout, cacheTimeout)
	if flags.threadObjects {
		ids, err := i.getIdsForTypes(
			smartblock.SmartBlockTypePage,
			smartblock.SmartBlockTypeProfilePage,
			smartblock.SmartBlockTypeTemplate,
			smartblock.SmartBlockTypeArchive,
			smartblock.SmartBlockTypeHome,
			smartblock.SmartBlockTypeWorkspace,
		)
		if err != nil {
			return err
		}
		start := time.Now()
		successfullyReindexed := i.reindexIdsIgnoreErr(ctx, ids...)
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
		err = i.reindexIDsForSmartblockTypes(ctx, metrics.ReindexTypeFiles, indexesWereRemoved, smartblock.SmartBlockTypeFile)
		if err != nil {
			return err
		}
	}
	if flags.bundledRelations {
		err = i.reindexIDsForSmartblockTypes(ctx, metrics.ReindexTypeBundledRelations, indexesWereRemoved, smartblock.SmartBlockTypeBundledRelation)
		if err != nil {
			return err
		}
	}
	if flags.bundledTypes {
		err = i.reindexIDsForSmartblockTypes(ctx, metrics.ReindexTypeBundledTypes, indexesWereRemoved, smartblock.SmartBlockTypeBundledObjectType, smartblock.SmartBlockTypeAnytypeProfile)
		if err != nil {
			return err
		}
	}
	if flags.bundledObjects {
		// hardcoded for now
		ids := []string{addr.AnytypeProfileId, addr.MissingObject}
		err = i.reindexIDs(ctx, metrics.ReindexTypeBundledObjects, false, ids)
		if err != nil {
			return err
		}
	}

	if flags.bundledTemplates {
		existing, _, err := i.store.QueryObjectIds(database.Query{}, []smartblock.SmartBlockType{smartblock.SmartBlockTypeBundledTemplate})
		if err != nil {
			return err
		}
		for _, id := range existing {
			i.store.DeleteObject(id)
		}

		err = i.reindexIDsForSmartblockTypes(ctx, metrics.ReindexTypeBundledTemplates, indexesWereRemoved, smartblock.SmartBlockTypeBundledTemplate)
		if err != nil {
			return err
		}
	}

	err = i.ensurePreinstalledObjects()
	if err != nil {
		return fmt.Errorf("ensure preinstalled objects: %w", err)
	}

	if flags.fulltext {
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

func (i *indexer) reindexIDsForSmartblockTypes(ctx context.Context, reindexType metrics.ReindexType, indexesWereRemoved bool, sbTypes ...smartblock.SmartBlockType) error {
	ids, err := i.getIdsForTypes(sbTypes...)
	if err != nil {
		return err
	}
	return i.reindexIDs(ctx, reindexType, indexesWereRemoved, ids)
}

func (i *indexer) reindexIDs(ctx context.Context, reindexType metrics.ReindexType, indexesWereRemoved bool, ids []string) error {
	start := time.Now()
	successfullyReindexed := i.reindexIdsIgnoreErr(ctx, ids...)
	if metrics.Enabled && len(ids) > 0 {
		metrics.SharedClient.RecordEvent(metrics.ReindexEvent{
			ReindexType:    reindexType,
			Total:          len(ids),
			Success:        successfullyReindexed,
			SpentMs:        int(time.Since(start).Milliseconds()),
			IndexesRemoved: indexesWereRemoved,
		})
	}
	msg := fmt.Sprintf("%d/%d %s have been successfully reindexed", successfullyReindexed, len(ids), reindexType)
	if len(ids)-successfullyReindexed != 0 {
		log.Error(msg)
	} else {
		log.Info(msg)
	}
	return nil
}

func (i *indexer) ensurePreinstalledObjects() error {
	var objects []*types.Struct

	for _, ot := range bundle.SystemTypes {
		t, err := bundle.GetTypeByUrl(ot.BundledURL())
		if err != nil {
			continue
		}
		objects = append(objects, (&relationutils.ObjectType{ObjectType: t}).ToStruct())
	}

	for _, rk := range bundle.SystemRelations {
		rel := bundle.MustGetRelation(rk)
		for _, opt := range rel.SelectDict {
			opt.RelationKey = rel.Key
			objects = append(objects, (&relationutils.Option{RelationOption: opt}).ToStruct())
		}
		objects = append(objects, (&relationutils.Relation{Relation: rel}).ToStruct())
	}

	_, _, err := i.subObjectCreator.CreateSubObjectsInWorkspace(objects)
	if errors.Is(err, editor.ErrSubObjectAlreadyExists) {
		return nil
	}
	return err
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

func (i *indexer) reindexOutdatedThreads() (toReindex, success int, err error) {
	// reindex of subobject collection always leads to reindex of the all subobjects reindexing
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
			if lastHash != "" {
				log.With("tree", tid).Warnf("not equal indexed heads hash: %s!=%s (%d logs)", lastHash, hh, len(heads))
			}
			idsToReindex = append(idsToReindex, tid)
		}
	}

	ctx := context.WithValue(context.Background(), metrics.CtxKeyRequest, "reindexOutdatedThreads")
	success = i.reindexIdsIgnoreErr(ctx, idsToReindex...)
	return len(idsToReindex), success, nil
}

func (i *indexer) reindexDoc(ctx context.Context, id string) error {
	err := block.DoWithContext(ctx, i.picker, id, func(sb smartblock2.SmartBlock) error {
		d := sb.GetDocInfo()
		if v, ok := sb.(editor.SubObjectCollectionGetter); ok {
			// index all the subobjects
			v.GetAllDocInfoIterator(
				func(info smartblock2.DocInfo) (contin bool) {
					err := i.Index(ctx, info)
					if err != nil {
						log.Errorf("failed to index subobject %s: %s", info.Id, err)
					}
					return true
				},
			)
		}

		return i.Index(ctx, d)
	})
	return err
}

func (i *indexer) reindexIdsIgnoreErr(ctx context.Context, ids ...string) (successfullyReindexed int) {
	ctx = block.CacheOptsWithRemoteLoadDisabled(ctx)
	for _, id := range ids {
		err := i.reindexDoc(ctx, id)
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
		lister, err := i.source.IDsListerBySmartblockType(t)
		if err != nil {
			return nil, err
		}
		idsT, err := lister.ListIds()
		if err != nil {
			return nil, err
		}
		ids = append(ids, idsT...)
	}
	return ids, nil
}

func headsHash(heads []string) string {
	if len(heads) == 0 {
		return ""
	}
	slices.Sort(heads)

	sum := sha256.Sum256([]byte(strings.Join(heads, ",")))
	return fmt.Sprintf("%x", sum)
}
