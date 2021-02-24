package indexer

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/threads"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/ftsearch"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/cheggaaa/mb"
	"github.com/gogo/protobuf/types"
)

var log = logging.Logger("anytype-doc-indexer")

var (
	ftIndexInterval = time.Minute / 3
	cleanupInterval = time.Minute
	docTTL          = time.Minute * 2
)

func NewIndexer(a anytype.Service, searchInfo GetSearchInfo) (Indexer, error) {
	ch, err := a.SubscribeForNewRecords()
	if err != nil {
		return nil, err
	}

	i := &indexer{
		store:      a.ObjectStore(),
		anytype:    a,
		searchInfo: searchInfo,
		cache:      make(map[string]*doc),
		quitWG:     &sync.WaitGroup{},
		quit:       make(chan struct{}),
	}
	i.quitWG.Add(2)
	if err := i.ftInit(); err != nil {
		log.Errorf("can't init ft: %v", err)
	}
	go i.reindexBundled()
	go i.detailsLoop(ch)
	go i.ftLoop()
	return i, nil
}

type Indexer interface {
	SetDetail(id string, key string, val *types.Value) error
	Close()
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
}

type indexer struct {
	store      localstore.ObjectStore
	anytype    anytype.Service
	searchInfo GetSearchInfo
	cache      map[string]*doc
	quitWG     *sync.WaitGroup
	quit       chan struct{}

	threadIdsBuf []string
	recBuf       []core.SmartblockRecordEnvelope
	mu           sync.Mutex
}


func (i *indexer) openDoc(id string) (state.Doc, error){
	s, err := source.NewSource(i.anytype, nil, id)
	if err != nil {
		err = fmt.Errorf("anytype.GetBlock error: %v", err)
		return nil, err
	}
	return s.ReadDoc(nil, false)
}

func (i *indexer) reindexBundled() {
	var (
		d state.Doc
		err error
	)

	for _, rk := range bundle.ListRelationsKeys() {
		id := "_br"+rk.String()
		if d, err = i.openDoc(id); err != nil {
			log.Errorf("reindexBundled failed to open %s: %s", id, err.Error())
			return
		}

		if err := i.store.UpdateObject(id, d.Details(), &pbrelation.Relations{d.ExtraRelations()}, nil, pbtypes.GetString(d.Details(), bundle.RelationKeyDescription.String())); err != nil {
			log.With("thread", id).Errorf("can't update object store: %v", err)
		}
	}

	for _, rk := range bundle.ListTypesKeys() {
		id := "_ot"+rk.String()
		if d, err = i.openDoc(id); err != nil {
			log.Errorf("reindexBundled failed to open %s: %s", id, err.Error())
			return
		}

		if err := i.store.UpdateObject(id, d.Details(), &pbrelation.Relations{d.ExtraRelations()}, nil, pbtypes.GetString(d.Details(), bundle.RelationKeyDescription.String())); err != nil {
			log.With("thread", id).Errorf("can't update object store: %v", err)
		}
	}

}

func (i *indexer) detailsLoop(ch chan core.SmartblockRecordWithThreadID) {
	batch := mb.New(0)
	defer batch.Close()
	go func() {
		defer i.quitWG.Done()
		var records []core.SmartblockRecordWithThreadID
		for {
			msgs := batch.Wait()
			if len(msgs) == 0 {
				return
			}
			records = records[:0]
			for _, msg := range msgs {
				records = append(records, msg.(core.SmartblockRecordWithThreadID))
			}
			i.applyRecords(records)
			// wait 100 millisecond for better batching
			time.Sleep(100 * time.Millisecond)
		}
	}()
	ticker := time.NewTicker(cleanupInterval)
	i.mu.Lock()
	quit := i.quit
	i.mu.Unlock()
	for {
		select {
		case rec, ok := <-ch:
			if !ok {
				return
			}
			batch.Add(rec)
		case <-ticker.C:
			i.cleanup()
		case <-quit:
			batch.Close()
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
		dataviewRelationsBefore []*pbrelation.Relation
		dataviewSourceBefore    string
	)
	d.mu.Lock()
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
		if err := i.store.UpdateObject(id, meta.Details, nil, nil, ""); err != nil {
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
			if err := i.store.UpdateRelationsInSet(id, dataviewSourceBefore, dv.Source, &pbrelation.Relations{dataviewRelationsBefore}, &pbrelation.Relations{dv.Relations}); err != nil {
				log.With("thread", id).Errorf("failed to index dataview relations")
			}
		}
	}

	if err := i.store.UpdateObject(id, meta.Details, &pbrelation.Relations{Relations: meta.Relations}, nil, ""); err != nil {
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

func (i *indexer) ftIndexDoc(id string, tm time.Time) (err error) {
	st := time.Now()
	info, err := i.searchInfo.GetSearchInfo(id)
	if err != nil {
		return
	}
	if err = i.store.UpdateObject(id, nil, nil, info.Links, info.Snippet); err != nil {
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

func (i *indexer) Close() {
	i.mu.Lock()
	quit := i.quit
	i.mu.Unlock()

	if quit != nil {
		close(quit)
		i.quitWG.Wait()
		i.mu.Lock()
		i.quit = nil
		i.mu.Unlock()
	}
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
