package indexer

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/core/anytype/config"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	ds "github.com/ipfs/go-datastore"
	"github.com/textileio/go-threads/core/thread"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/core/recordsbatcher"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/ftsearch"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/gogo/protobuf/types"
)

const (
	CName = "indexer"

	// increasing counters below will trigger existing account to reindex their data
	ForceThreadsObjectsReindexCounter int32 = 0 // reindex thread-based objects
	ForceFilesReindexCounter          int32 = 2 // reindex ipfs-file-based objects
	ForceBundledObjectsReindexCounter int32 = 1 // reindex objects like anytypeProfile
	ForceIdxRebuildCounter            int32 = 3 // erases localstore indexes and reindex all type of objects (no need to increase ForceThreadsObjectsReindexCounter & ForceFilesReindexCounter)
	ForceFulltextIndexCounter         int32 = 1 // performs fulltext indexing for all type of objects (useful when we change fulltext config)
)

var log = logging.Logger("anytype-doc-indexer")

var (
	ftIndexInterval = time.Minute / 3
	cleanupInterval = time.Minute
	docTTL          = time.Minute * 2
)

func New() Indexer {
	return &indexer{}
}

type Indexer interface {
	SetDetail(id string, key string, val *types.Value) error
	app.ComponentRunnable
}

type SearchInfo struct {
	Id      string
	Title   string
	Snippet string
	Text    string
	Links   []string
}

type GetSearchInfo interface {
	GetSearchInfo(id string) (info SearchInfo, err error)
	app.Component
}

type batchReader interface {
	Read(buffer []core.SmartblockRecordWithThreadID) int
}

type ThreadLister interface {
	Threads() (thread.IDSlice, error)
}

type Hasher interface {
	Hash() string
}

type indexer struct {
	store objectstore.ObjectStore
	// todo: move logstore to separate component?
	threadService     threads.Service
	anytype           core.Service
	source            source.Service
	searchInfo        GetSearchInfo
	cache             map[string]*doc
	quitWG            *sync.WaitGroup
	quit              chan struct{}
	newRecordsBatcher batchReader
	recBuf            []core.SmartblockRecordEnvelope
	threadIdsBuf      []string
	mu                sync.Mutex
	btHash            Hasher

	newAccount bool
}

func (i *indexer) Init(a *app.App) (err error) {
	i.newAccount = a.MustComponent(config.CName).(*config.Config).NewAccount
	i.anytype = a.MustComponent(core.CName).(core.Service)
	ts := a.Component(threads.CName)
	if ts != nil {
		i.threadService = ts.(threads.Service)
	}
	i.searchInfo = a.MustComponent("blockService").(GetSearchInfo)
	i.store = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	i.cache = make(map[string]*doc)
	i.newRecordsBatcher = a.MustComponent(recordsbatcher.CName).(recordsbatcher.RecordsBatcher)
	i.source = a.MustComponent(source.CName).(source.Service)
	i.btHash = a.MustComponent("builtintemplate").(Hasher)
	i.quitWG = new(sync.WaitGroup)
	i.quit = make(chan struct{})
	return
}

func (i *indexer) Name() (name string) {
	return CName
}

func (i *indexer) saveLatestChecksums() error {
	// todo: add layout indexing when needed
	checksums := model.ObjectStoreChecksums{
		BundledObjectTypes:         bundle.TypeChecksum,
		BundledRelations:           bundle.RelationChecksum,
		BundledTemplates:           i.btHash.Hash(),
		ObjectsForceReindexCounter: ForceThreadsObjectsReindexCounter,
		FilesForceReindexCounter:   ForceFilesReindexCounter,

		IdxRebuildCounter: ForceIdxRebuildCounter,
		FulltextRebuild:   ForceFulltextIndexCounter,
		BundledObjects:    ForceBundledObjectsReindexCounter,
	}
	return i.store.SaveChecksums(&checksums)
}

func (i *indexer) saveLatestCounters() error {
	// todo: add layout indexing when needed
	checksums := model.ObjectStoreChecksums{
		BundledObjectTypes:         bundle.TypeChecksum,
		BundledRelations:           bundle.RelationChecksum,
		BundledTemplates:           i.btHash.Hash(),
		ObjectsForceReindexCounter: ForceThreadsObjectsReindexCounter,
		FilesForceReindexCounter:   ForceFilesReindexCounter,
		IdxRebuildCounter:          ForceIdxRebuildCounter,
		FulltextRebuild:            ForceFulltextIndexCounter,
		BundledObjects: 			ForceBundledObjectsReindexCounter,
	}
	return i.store.SaveChecksums(&checksums)
}

func (i *indexer) Run() (err error) {
	if ftErr := i.ftInit(); ftErr != nil {
		log.Errorf("can't init ft: %v", ftErr)
	}
	i.quitWG.Add(2)
	err = i.reindexIfNeeded()
	if err != nil {
		return err
	}

	go i.detailsLoop()
	go i.ftLoop()
	return
}

func (i *indexer) reindexIfNeeded() error {
	var (
		err       error
		checksums *model.ObjectStoreChecksums
	)
	if i.newAccount {
		checksums = &model.ObjectStoreChecksums{
			// do no add bundled relations checksums, because we want to index them for new accounts
			ObjectsForceReindexCounter: ForceThreadsObjectsReindexCounter,
			FilesForceReindexCounter:   ForceFilesReindexCounter,
			IdxRebuildCounter:          ForceIdxRebuildCounter,
			FulltextRebuild:            ForceFulltextIndexCounter,
		}
	} else {
		checksums, err = i.store.GetChecksums()
		if err != nil && err != ds.ErrNotFound {
			return err
		}
	}

	if checksums == nil {
		// zero values are valid
		// means we didn't perform new indexer before
		checksums = &model.ObjectStoreChecksums{}
	}

	var (
		reindexBundledTypes     bool
		reindexBundledRelations bool
		eraseIndexes            bool
		reindexThreadObjects    bool
		reindexFileObjects      bool
		reindexFulltext         bool
		reindexBundledTemplates bool
		reindexBundledObjects   bool
	)

	if checksums.BundledRelations != bundle.RelationChecksum {
		reindexBundledRelations = true
	}
	if checksums.BundledObjectTypes != bundle.TypeChecksum {
		reindexBundledTypes = true
	}
	if checksums.ObjectsForceReindexCounter != ForceThreadsObjectsReindexCounter {
		reindexThreadObjects = true
	}
	if checksums.FilesForceReindexCounter != ForceFilesReindexCounter {
		reindexFileObjects = true
	}
	if checksums.FulltextRebuild != ForceFulltextIndexCounter {
		reindexFulltext = true
	}
	if checksums.BundledTemplates != i.btHash.Hash() {
		reindexBundledTemplates = true
	}
	if checksums.BundledObjects != ForceBundledObjectsReindexCounter {
		reindexBundledObjects = true
	}
	if checksums.IdxRebuildCounter != ForceIdxRebuildCounter {
		eraseIndexes = true
		reindexFileObjects = true
		reindexThreadObjects = true
		reindexBundledRelations = true
		reindexBundledTypes = true
		reindexBundledTemplates = true
		reindexBundledObjects = true
	}

	if eraseIndexes || reindexFileObjects || reindexThreadObjects || reindexBundledRelations || reindexBundledTypes || reindexFulltext || reindexBundledTemplates || reindexBundledObjects {
		log.Infof("start store reindex (eraseIndexes=%v, reindexFileObjects=%v, reindexThreadObjects=%v, reindexBundledRelations=%v, reindexBundledTypes=%v, reindexFulltext=%v, reindexBundledTemplates=%v, reindexBundledObjects=%v)", eraseIndexes, reindexFileObjects, reindexThreadObjects, reindexBundledRelations, reindexBundledTypes, reindexFulltext, reindexBundledTemplates, reindexBundledObjects)
	}

	getIdsForTypes := func(sbt ...smartblock.SmartBlockType) ([]string, error) {
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
	var indexesWereRemoved bool
	if eraseIndexes {
		err = i.store.EraseIndexes()
		if err != nil {
			log.Errorf("reindex failed to erase indexes: %v", err.Error())
		} else {
			log.Infof("all store indexes succesfully erased")
			// store this flag because underlying localstore needs to now if it needs to amend indexes based on the prev value
			indexesWereRemoved = true
		}
	}
	if reindexThreadObjects {
		ids, err := getIdsForTypes(
			smartblock.SmartBlockTypePage,
			smartblock.SmartBlockTypeSet,
			smartblock.SmartBlockTypeObjectType,
			smartblock.SmartBlockTypeProfilePage,
			smartblock.SmartBlockTypeArchive,
			smartblock.SmartBlockTypeHome,
			smartblock.SmartBlockTypeTemplate,
			smartblock.SmartblockTypeMarketplaceType,
			smartblock.SmartblockTypeMarketplaceTemplate,
			smartblock.SmartblockTypeMarketplaceRelation,
		)
		if err != nil {
			return err
		}
		successfullyReindexed := i.reindexIdsIgnoreErr(indexesWereRemoved, ids...)
		log.Infof("%d/%d objects have been successfully reindexed", successfullyReindexed, len(ids))
	}
	if reindexFileObjects {
		ids, err := getIdsForTypes(smartblock.SmartBlockTypeFile)
		if err != nil {
			return err
		}
		successfullyReindexed := i.reindexIdsIgnoreErr(indexesWereRemoved, ids...)
		msg := fmt.Sprintf("%d/%d files have been successfully reindexed", successfullyReindexed, len(ids))
		if len(ids)-successfullyReindexed != 0 {
			log.Error(msg)
		} else {
			log.Info(msg)
		}
	}
	if reindexBundledRelations {
		ids, err := getIdsForTypes(smartblock.SmartBlockTypeBundledRelation)
		if err != nil {
			return err
		}
		successfullyReindexed := i.reindexIdsIgnoreErr(indexesWereRemoved, ids...)
		msg := fmt.Sprintf("%d/%d bundled relations have been successfully reindexed", successfullyReindexed, len(ids))
		if len(ids)-successfullyReindexed != 0 {
			log.Error(msg)
		} else {
			log.Info(msg)
		}
	}
	if reindexBundledTypes {
		// lets add anytypeProfile here, because it's seems too much to create one more counter especially for it
		ids, err := getIdsForTypes(smartblock.SmartBlockTypeBundledObjectType, smartblock.SmartBlockTypeAnytypeProfile)
		if err != nil {
			return err
		}
		successfullyReindexed := i.reindexIdsIgnoreErr(indexesWereRemoved, ids...)
		msg := fmt.Sprintf("%d/%d bundled types have been successfully reindexed", successfullyReindexed, len(ids))
		if len(ids)-successfullyReindexed != 0 {
			log.Error(msg)
		} else {
			log.Info(msg)
		}
	}
	if reindexBundledObjects {
		// hardcoded for now
		ids := []string{addr.AnytypeProfileId}
		successfullyReindexed := i.reindexIdsIgnoreErr(indexesWereRemoved, ids...)
		msg := fmt.Sprintf("%d/%d bundled objects have been successfully reindexed", successfullyReindexed, len(ids))
		if len(ids)-successfullyReindexed != 0 {
			log.Error(msg)
		} else {
			log.Info(msg)
		}
	}

	if reindexBundledTemplates {
		existsRec, _, err := i.store.QueryObjectInfo(database.Query{}, []smartblock.SmartBlockType{smartblock.SmartBlockTypeBundledTemplate})
		if err != nil {
			return err
		}
		existsIds := make([]string, 0, len(existsRec))
		for _, rec := range existsRec {
			existsIds = append(existsIds, rec.Id)
		}
		ids, err := getIdsForTypes(smartblock.SmartBlockTypeBundledTemplate)
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
		successfullyReindexed := i.reindexIdsIgnoreErr(indexesWereRemoved, ids...)
		msg := fmt.Sprintf("%d/%d bundled templates have been successfully reindexed; removed: %d", successfullyReindexed, len(ids), removed)
		if len(ids)-successfullyReindexed != 0 {
			log.Error(msg)
		} else {
			log.Info(msg)
		}
	}
	if reindexFulltext {
		var ids []string
		ids, err := getIdsForTypes(smartblock.SmartBlockTypePage, smartblock.SmartBlockTypeFile, smartblock.SmartBlockTypeBundledRelation, smartblock.SmartBlockTypeBundledObjectType, smartblock.SmartBlockTypeAnytypeProfile)
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

func (i *indexer) openDoc(id string) (state.Doc, error) {
	// set listenToOwnChanges to false because it doesn't means. We do not use source's applyRecords
	s, err := i.source.NewSource(id, false)
	if err != nil {
		err = fmt.Errorf("anytype.GetBlock error: %v", err)
		return nil, err
	}
	d, err := s.ReadDoc(nil, false)
	if err != nil {
		return nil, err
	}

	st := d.(*state.State)
	if d.ObjectType() == "" {
		ot, exists := bundle.DefaultObjectTypePerSmartblockType[smartblock.SmartBlockType(s.Type())]
		if !exists {
			ot = bundle.TypeKeyPage
		}
		st.SetObjectType(ot.URL())
	}

	for _, relKey := range bundle.RequiredInternalRelations {
		if st.HasRelation(relKey.String()) {
			continue
		}
		rel := bundle.MustGetRelation(relKey)
		st.AddRelation(rel)
	}

	return st, nil
}

func (i *indexer) reindexDoc(id string, indexesWereRemoved bool) error {
	t, err := smartblock.SmartBlockTypeFromID(id)
	if err != nil {
		return fmt.Errorf("incorrect sb type: %v", err)
	}

	if t == smartblock.SmartBlockTypeArchive {
		if err := i.store.AddToIndexQueue(id); err != nil {
			log.With("thread", id).Errorf("can't add archive to index queue: %v", err)
		} else {
			log.With("thread", id).Debugf("archive added to index queue")
		}
		return nil
	}

	d, err := i.openDoc(id)
	if err != nil {
		log.Errorf("reindexDoc failed to open %s: %s", id, err.Error())
		return fmt.Errorf("failed to open doc: %s", err.Error())
	}

	details := d.Details()
	var curDetails *types.Struct
	curDetailsO, _ := i.store.GetDetails(id)
	if curDetailsO != nil {
		curDetails = curDetailsO.Details
	}
	// compare only real object scoped details
	detailsObjectScope := pbtypes.StructCutKeys(details, append(bundle.LocalRelationsKeys, bundle.DerivedRelationsKeys...))
	curDetailsObjectScope := pbtypes.StructCutKeys(curDetails, append(bundle.LocalRelationsKeys, bundle.DerivedRelationsKeys...))
	if indexesWereRemoved || curDetailsObjectScope == nil || !detailsObjectScope.Equal(curDetailsObjectScope) {
		if indexesWereRemoved || curDetails == nil {
			if err := i.store.CreateObject(id, details, &model.Relations{d.ExtraRelations()}, nil, pbtypes.GetString(details, bundle.RelationKeyDescription.String())); err != nil {
				return fmt.Errorf("can't update object store: %v", err)
			}
		} else {
			if err := i.store.UpdateObjectDetails(id, details, &model.Relations{d.ExtraRelations()}, false); err != nil {
				return fmt.Errorf("can't update object store: %v", err)
			}
		}
		if curDetails == nil || t == smartblock.SmartBlockTypeFile {
			// add to fulltext only in case
			if err = i.store.AddToIndexQueue(id); err != nil {
				log.With("thread", id).Errorf("can't add to index: %v", err)
			}
		}
	}
	return nil
}

func (i *indexer) reindexIdsIgnoreErr(indexRemoved bool, ids ...string) (successfullyReindexed int) {
	for _, id := range ids {
		err := i.reindexDoc(id, indexRemoved)
		if err != nil {
			log.With("thread", id).Errorf("failed to reindex: %v", err)
		} else {
			successfullyReindexed++
		}
	}
	return
}

func (i *indexer) detailsLoop() {
	go func() {
		defer i.quitWG.Done()
		var records = make([]core.SmartblockRecordWithThreadID, 100)
		for {
			records = records[0:cap(records)]
			n := i.newRecordsBatcher.Read(records)
			if n == 0 {
				// means no more data is available
				return
			}
			records = records[0:n]

			i.applyRecords(records)
		}
	}()
	ticker := time.NewTicker(cleanupInterval)
	i.mu.Lock()
	quit := i.quit
	i.mu.Unlock()
	for {
		select {
		case <-ticker.C:
			i.cleanup()
		case <-quit:
			// wait until we have batch closed on other side
			i.quitWG.Wait()
			return
		}
	}
}

func (i *indexer) applyRecords(records []core.SmartblockRecordWithThreadID) {
	threadIds := i.threadIdsBuf[:0]
	// find unique threads
	for _, rec := range records {
		if slice.FindPos(threadIds, rec.ThreadID) == -1 {
			threadIds = append(threadIds, rec.ThreadID)
		}
	}
	// group and apply records by thread
	for _, tid := range threadIds {
		threadRecords := i.recBuf[:0]
		for _, rec := range records {
			if rec.ThreadID == tid {
				threadRecords = append(threadRecords, rec.SmartblockRecordEnvelope)
			}
		}
		i.index(tid, threadRecords, false)
	}
}

func (i *indexer) getDoc(id string) (d *doc, err error) {
	var ok bool
	i.mu.Lock()
	defer i.mu.Unlock()
	if d, ok = i.cache[id]; !ok {
		if d, err = newDoc(id, i.anytype); err != nil {
			return
		}
		i.cache[id] = d
	}
	return
}

func (i *indexer) index(id string, records []core.SmartblockRecordEnvelope, onlyDetails bool) {
	d, err := i.getDoc(id)
	if err != nil {
		log.Warnf("can't get doc '%s': %v", id, err)
		return
	}
	var (
		dataviewRelationsBefore []*model.Relation
		dataviewSourceBefore    string
	)
	d.mu.Lock()
	if d.sb.Type() == smartblock.SmartBlockTypeArchive {
		if err := i.store.AddToIndexQueue(id); err != nil {
			log.With("thread", id).Errorf("can't add archive to index queue: %v", err)
		} else {
			log.With("thread", id).Debugf("archive added to index queue")
		}
		d.mu.Unlock()
		return
	}
	if len(d.st.ObjectTypes()) == 1 && d.st.ObjectTypes()[0] == bundle.TypeKeySet.URL() {
		b := d.st.Get("dataview")
		if b != nil && b.Model().GetDataview() != nil {
			b = b.Copy()
			dataviewRelationsBefore = b.Model().GetDataview().Relations
			dataviewSourceBefore = b.Model().GetDataview().Source
		}
	}

	d.mu.Unlock()
	lastChangeTS, lastChangeBy, _ := d.addRecords(records...)

	meta := d.meta()

	if meta.Details != nil && meta.Details.Fields != nil {
		prevModifiedDate := int64(pbtypes.GetFloat64(meta.Details, bundle.RelationKeyLastModifiedDate.String()))

		if lastChangeTS > prevModifiedDate {
			meta.Details.Fields[bundle.RelationKeyLastModifiedDate.String()] = pbtypes.Float64(float64(lastChangeTS))
			if profileId, err := threads.ProfileThreadIDFromAccountAddress(lastChangeBy); err == nil {
				meta.Details.Fields[bundle.RelationKeyLastModifiedBy.String()] = pbtypes.String(profileId.String())
			}
		}
	}

	if onlyDetails {
		if err := i.store.UpdateObjectDetails(id, meta.Details, nil, true); err != nil {
			log.With("thread", id).Errorf("can't update object store: %v", err)
		} else {
			log.With("thread", id).Infof("indexed %d records: det: %v", len(records), pbtypes.GetString(meta.Details, bundle.RelationKeyName.String()))
		}
		return
	}

	if len(meta.ObjectTypes) == 1 && meta.ObjectTypes[0] == bundle.TypeKeySet.URL() {
		b := d.st.Get("dataview")
		var dv *model.BlockContentDataview
		if b != nil {
			dv = b.Model().GetDataview()
		}
		if b != nil && dv != nil {
			if err := i.store.UpdateRelationsInSet(id, dataviewSourceBefore, dv.Source, &model.Relations{dataviewRelationsBefore}, &model.Relations{dv.Relations}); err != nil {
				log.With("thread", id).Errorf("failed to index dataview relations")
			}
		}
	}

	if len(meta.ObjectTypes) > 0 && meta.Details != nil {
		meta.Details.Fields[bundle.RelationKeyType.String()] = pbtypes.StringList(meta.ObjectTypes)
	}

	if err := i.store.UpdateObjectDetails(id, meta.Details, &model.Relations{Relations: meta.Relations}, true); err != nil {
		log.With("thread", id).Errorf("can't update object store: %v", err)
	} else {
		log.With("thread", id).Infof("indexed %d records: det: %v", len(records), pbtypes.GetString(meta.Details, bundle.RelationKeyName.String()))
	}

	if err := i.store.AddToIndexQueue(id); err != nil {
		log.With("thread", id).Errorf("can't add id to index queue: %v", err)
	} else {
		log.With("thread", id).Debugf("to index queue")
	}
}

func (i *indexer) ftLoop() {
	defer i.quitWG.Done()
	ticker := time.NewTicker(ftIndexInterval)
	i.ftIndex()
	for {
		select {
		case <-i.quit:
			return
		case <-ticker.C:
			i.ftIndex()
		}
	}
}

func (i *indexer) ftIndex() {
	if err := i.store.IndexForEach(i.ftIndexDoc); err != nil {
		log.Errorf("store.IndexForEach error: %v", err)
	}
}

func (i *indexer) ftIndexDoc(id string, _ time.Time) (err error) {
	st := time.Now()
	info, err := i.searchInfo.GetSearchInfo(id)
	if err != nil {
		return
	}
	if err = i.store.UpdateObjectLinksAndSnippet(id, info.Links, info.Snippet); err != nil {
		return
	}

	if fts := i.store.FTSearch(); fts != nil {
		if err := fts.Index(ftsearch.SearchDoc{
			Id:    id,
			Title: info.Title,
			Text:  info.Text,
		}); err != nil {
			log.Errorf("can't ft index doc: %v", err)
		}
	}
	log.With("thread", id).Infof("ft index updated for a %v", time.Since(st))
	return
}

func (i *indexer) ftInit() error {
	if ft := i.store.FTSearch(); ft != nil {
		docCount, err := ft.DocCount()
		if err != nil {
			return err
		}
		if docCount == 0 {
			all, err := i.store.List()
			if err != nil {
				return err
			}
			for _, d := range all {
				if err := i.store.AddToIndexQueue(d.Id); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (i *indexer) cleanup() {
	i.mu.Lock()
	defer i.mu.Unlock()
	toCleanup := time.Now().Add(-docTTL)
	removed := 0
	count := len(i.cache)
	for k, v := range i.cache {
		v.mu.Lock()
		if v.lastUsage.Before(toCleanup) {
			delete(i.cache, k)
			removed++
		}
		v.mu.Unlock()
	}
	log.Infof("indexer cleanup: removed %d from %d", removed, count)
}

func (i *indexer) Close() error {
	i.mu.Lock()
	quit := i.quit
	i.mu.Unlock()
	if i.threadService != nil {
		err := i.threadService.Close()
		log.Errorf("explicitly stop threadService first: %v", err)
		if err != nil {
			return err
		}
	}
	if quit != nil {
		close(quit)
		i.quitWG.Wait()
		i.mu.Lock()
		i.quit = nil
		i.mu.Unlock()
	}
	return nil
}

func (i *indexer) SetDetail(id string, key string, val *types.Value) error {
	d, err := i.getDoc(id)
	if err != nil {
		return err
	}

	d.SetDetail(key, val)
	i.index(id, nil, true)
	return nil
}
