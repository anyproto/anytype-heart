package backlinks

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/ocache"
	"github.com/cheggaaa/mb"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/slice"
)

const (
	CName = "backlinks-update-watcher"

	defaultAggregationInterval = time.Second * 5
)

var log = logging.Logger(CName)

type backlinksUpdater interface {
	SubscribeLinksUpdate(callback func(info spaceindex.LinksUpdateInfo))
}

type backLinksUpdate struct {
	added   []string
	removed []string
}

type UpdateWatcher interface {
	app.ComponentRunnable

	FlushUpdates()
}

type watcher struct {
	updater      backlinksUpdater
	store        objectstore.ObjectStore
	resolver     idresolver.Resolver
	spaceService space.Service

	infoBatch            *mb.MB
	lock                 sync.Mutex
	accumulatedBacklinks map[string]*backLinksUpdate
	aggregationInterval  time.Duration
}

func New() UpdateWatcher {
	return &watcher{}
}

func (w *watcher) Name() string {
	return CName
}

func (w *watcher) Init(a *app.App) error {
	w.updater = app.MustComponent[backlinksUpdater](a)
	w.store = app.MustComponent[objectstore.ObjectStore](a)
	w.resolver = app.MustComponent[idresolver.Resolver](a)
	w.spaceService = app.MustComponent[space.Service](a)
	w.infoBatch = mb.New(0)
	w.accumulatedBacklinks = make(map[string]*backLinksUpdate)
	w.aggregationInterval = defaultAggregationInterval
	return nil
}

func (w *watcher) Close(context.Context) error {
	if err := w.infoBatch.Close(); err != nil {
		log.Errorf("failed to close message batch: %v", err)
	}
	return nil
}

func (w *watcher) Run(context.Context) error {
	w.updater.SubscribeLinksUpdate(func(info spaceindex.LinksUpdateInfo) {
		if err := w.infoBatch.Add(info); err != nil {
			log.With("objectId", info.LinksFromId).Errorf("failed to add backlinks update info to message batch: %v", err)
		}
	})

	go w.backlinksUpdateHandler()
	return nil
}

func (w *watcher) FlushUpdates() {
	w.lock.Lock()
	defer w.lock.Unlock()

	w.updateAccumulatedBacklinks()
}

func applyUpdates(m map[string]*backLinksUpdate, update spaceindex.LinksUpdateInfo) {
	if update.LinksFromId == "" {
		return
	}

	for _, removed := range update.Removed {
		if _, ok := m[removed]; !ok {
			m[removed] = &backLinksUpdate{}
		}
		if i := lo.IndexOf(m[removed].added, update.LinksFromId); i >= 0 {
			m[removed].added = append(m[removed].added[:i], m[removed].added[i+1:]...)
		}
		if !lo.Contains(m[removed].removed, update.LinksFromId) {
			m[removed].removed = append(m[removed].removed, update.LinksFromId)
		}
	}

	for _, added := range update.Added {
		if _, ok := m[added]; !ok {
			m[added] = &backLinksUpdate{}
		}
		if i := lo.IndexOf(m[added].removed, update.LinksFromId); i >= 0 {
			m[added].removed = append(m[added].removed[:i], m[added].removed[i+1:]...)
		}
		if !lo.Contains(m[added].added, update.LinksFromId) {
			m[added].added = append(m[added].added, update.LinksFromId)
		}
	}
}

func (w *watcher) backlinksUpdateHandler() {
	var (
		lastReceivedUpdates time.Time
		closedCh            = make(chan struct{})
	)
	defer close(closedCh)

	go func() {
		for {
			select {
			case <-closedCh:
				w.lock.Lock()
				w.updateAccumulatedBacklinks()
				w.lock.Unlock()
				return
			case <-time.After(w.aggregationInterval):
				w.lock.Lock()
				if time.Since(lastReceivedUpdates) < w.aggregationInterval || len(w.accumulatedBacklinks) == 0 {
					w.lock.Unlock()
					continue
				}

				w.updateAccumulatedBacklinks()
				w.lock.Unlock()
			}
		}
	}()

	for {
		msgs := w.infoBatch.Wait()
		if len(msgs) == 0 {
			return
		}

		w.lock.Lock()
		for _, msg := range msgs {
			info, ok := msg.(spaceindex.LinksUpdateInfo)
			if !ok {
				continue
			}
			info = cleanSelfLinks(info)
			applyUpdates(w.accumulatedBacklinks, info)
		}
		lastReceivedUpdates = time.Now()
		w.lock.Unlock()
	}
}

func (w *watcher) updateAccumulatedBacklinks() {
	log.Debugf("updating backlinks for %d objects", len(w.accumulatedBacklinks))
	for id, updates := range w.accumulatedBacklinks {
		w.updateBackLinksInObject(id, updates)
	}
	w.accumulatedBacklinks = make(map[string]*backLinksUpdate)
}

func shouldIndexBacklinks(ids threads.DerivedSmartblockIds, id string) bool {
	switch id {
	case ids.Workspace, ids.Archive, ids.Home, ids.Widgets, ids.Profile:
		return false
	default:
		return true
	}
}

func (w *watcher) updateBackLinksInObject(id string, backlinksUpdate *backLinksUpdate) {
	spaceId, err := w.resolver.ResolveSpaceID(id)
	if err != nil {
		log.With("objectId", id).Errorf("failed to resolve space id for object: %v", err)
		return
	}
	spc, err := w.spaceService.Get(context.Background(), spaceId)
	if err != nil {
		log.With("objectId", id, "spaceId", spaceId).Errorf("failed to get space: %v", err)
		return
	}
	spaceDerivedIds := spc.DerivedIDs()

	updateBacklinks := func(current *domain.Details, backlinksChange *backLinksUpdate) (*domain.Details, bool, error) {
		if current == nil {
			return nil, false, nil
		}
		backlinks := current.GetStringList(bundle.RelationKeyBacklinks)

		for _, removed := range backlinksChange.removed {
			backlinks = slice.Remove(backlinks, removed)
		}

		for _, added := range backlinksChange.added {
			if !lo.Contains(backlinks, added) {
				backlinks = append(backlinks, added)
			}
		}

		backlinks = slice.Filter(backlinks, func(s string) bool {
			// filter-out backlinks to system objects
			return shouldIndexBacklinks(spaceDerivedIds, s)
		})

		current.SetStringList(bundle.RelationKeyBacklinks, backlinks)
		return current, true, nil
	}

	if shouldIndexBacklinks(spaceDerivedIds, id) {
		// filter-out backlinks in system objects
		err = spc.DoLockedIfNotExists(id, func() error {
			return w.store.SpaceIndex(spaceId).ModifyObjectDetails(id, func(details *domain.Details) (*domain.Details, bool, error) {
				return updateBacklinks(details, backlinksUpdate)
			})
		})
	}

	if err == nil {
		return
	}

	if !errors.Is(err, ocache.ErrExists) {
		log.With("objectId", id).Errorf("failed to update backlinks for not cached object: %v", err)
	}
	if err = spc.Do(id, func(b smartblock.SmartBlock) error {
		if cr, ok := b.(source.ChangeReceiver); ok {
			return cr.StateAppend(func(d state.Doc) (s *state.State, changes []*pb.ChangeContent, err error) {
				return d.NewState(), nil, nil
			})
		}
		// do no do apply, stateAppend send the event and run the index
		return nil
	}); err != nil {
		log.With("objectId", id).Errorf("failed to update backlinks: %v", err)
	}

}

func hasSelfLinks(info spaceindex.LinksUpdateInfo) bool {
	for _, link := range info.Added {
		if link == info.LinksFromId {
			return true
		}
	}
	for _, link := range info.Removed {
		if link == info.LinksFromId {
			return true
		}
	}
	return false
}

func cleanSelfLinks(info spaceindex.LinksUpdateInfo) spaceindex.LinksUpdateInfo {
	if !hasSelfLinks(info) {
		// optimisation to avoid additional allocations
		return info
	}
	infoFilter := spaceindex.LinksUpdateInfo{
		LinksFromId: info.LinksFromId,
		Added:       make([]string, 0, len(info.Added)),
		Removed:     make([]string, 0, len(info.Removed)),
	}
	for _, link := range info.Added {
		if link != info.LinksFromId {
			infoFilter.Added = append(infoFilter.Added, link)
		}
	}
	for _, link := range info.Removed {
		if link != info.LinksFromId {
			infoFilter.Removed = append(infoFilter.Removed, link)
		}
	}
	return infoFilter
}
