package backlinks

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/app/ocache"
	"github.com/cheggaaa/mb/v3"
	"github.com/samber/lo"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/dateutil"
	"github.com/anyproto/anytype-heart/util/slice"
)

const (
	CName = "backlinks-update-watcher"

	defaultAggregationInterval = time.Second * 5
)

var log = logger.NewNamed(CName)

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
	spaceService space.Service

	infoBatch            *mb.MB[spaceindex.LinksUpdateInfo]
	lock                 sync.Mutex
	accumulatedBacklinks map[domain.FullID]*backLinksUpdate
	aggregationInterval  time.Duration
	cancelCtx            context.CancelFunc
	ctx                  context.Context
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
	w.spaceService = app.MustComponent[space.Service](a)

	w.infoBatch = mb.New[spaceindex.LinksUpdateInfo](0)
	w.accumulatedBacklinks = make(map[domain.FullID]*backLinksUpdate)
	w.aggregationInterval = defaultAggregationInterval
	return nil
}

func (w *watcher) Close(context.Context) error {
	if w.cancelCtx != nil {
		w.cancelCtx()
	}
	if err := w.infoBatch.Close(); err != nil {
		log.Error("failed to close message batch", zap.Error(err))
	}
	return nil
}

func (w *watcher) Run(ctx context.Context) error {
	w.ctx, w.cancelCtx = context.WithCancel(context.Background())
	w.updater.SubscribeLinksUpdate(func(info spaceindex.LinksUpdateInfo) {
		if err := w.infoBatch.Add(w.ctx, info); err != nil {
			log.Error("failed to add backlinks update info to message batch", zap.String("objectId", info.LinksFromId.ObjectID), zap.Error(err))
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

func applyUpdate(m map[domain.FullID]*backLinksUpdate, update spaceindex.LinksUpdateInfo, parseId func(string) (domain.FullID, error)) {
	if update.LinksFromId.ObjectID == "" {
		return
	}

	for _, removed := range update.Removed {
		fullId, err := parseId(removed)
		if err != nil {
			log.Error("failed to parse id", zap.String("objectId", removed), zap.Error(err))
			continue
		}
		if fullId.SpaceID == "" {
			fullId.SpaceID = update.LinksFromId.SpaceID
		}
		if _, ok := m[fullId]; !ok {
			m[fullId] = &backLinksUpdate{}
		}
		if i := lo.IndexOf(m[fullId].added, update.LinksFromId.ObjectID); i >= 0 {
			m[fullId].added = append(m[fullId].added[:i], m[fullId].added[i+1:]...)
		}
		if !lo.Contains(m[fullId].removed, update.LinksFromId.ObjectID) {
			m[fullId].removed = append(m[fullId].removed, update.LinksFromId.ObjectID)
		}
	}

	for _, added := range update.Added {
		fullId, err := parseId(added)
		if err != nil {
			log.Error("failed to parse id", zap.String("objectId", added), zap.Error(err))
			continue
		}
		if fullId.SpaceID == "" {
			fullId.SpaceID = update.LinksFromId.SpaceID
		}
		if _, ok := m[fullId]; !ok {
			m[fullId] = &backLinksUpdate{}
		}
		if i := lo.IndexOf(m[fullId].removed, update.LinksFromId.ObjectID); i >= 0 {
			m[fullId].removed = append(m[fullId].removed[:i], m[fullId].removed[i+1:]...)
		}
		if !lo.Contains(m[fullId].added, update.LinksFromId.ObjectID) {
			m[fullId].added = append(m[fullId].added, update.LinksFromId.ObjectID)
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
		msgs, err := w.infoBatch.Wait(w.ctx)
		if err != nil {
			return
		}
		if len(msgs) == 0 {
			return
		}

		w.lock.Lock()
		for _, info := range msgs {
			info = cleanSelfLinks(info)
			applyUpdate(w.accumulatedBacklinks, info, domain.ParseLongId)
		}
		lastReceivedUpdates = time.Now()
		w.lock.Unlock()
	}
}

func (w *watcher) updateAccumulatedBacklinks() {
	log.Debug("updating backlinks", zap.Int64("objects number", int64(len(w.accumulatedBacklinks))))
	for id, updates := range w.accumulatedBacklinks {
		if err := w.updateBackLinksInObject(id, updates); err != nil {
			log.Error("failed to update backlinks", zap.String("objectId", id.ObjectID), zap.Error(err))
		}
	}
	w.accumulatedBacklinks = make(map[domain.FullID]*backLinksUpdate)
}

func shouldIndexBacklinks(ids threads.DerivedSmartblockIds, id string) bool {
	if _, parseDateErr := dateutil.BuildDateObjectFromId(id); parseDateErr == nil {
		return false
	}
	switch id {
	case ids.Workspace, ids.Archive, ids.Home, ids.Widgets, ids.Profile:
		return false
	default:
		return true
	}
}

func (w *watcher) updateBackLinksInObject(id domain.FullID, backlinksUpdate *backLinksUpdate) (err error) {
	spc, err := w.spaceService.Get(w.ctx, id.SpaceID)
	if err != nil {
		return fmt.Errorf("failed to get space: %w", err)
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

	if shouldIndexBacklinks(spaceDerivedIds, id.ObjectID) {
		// filter-out backlinks in system objects
		err = spc.DoLockedIfNotExists(id.ObjectID, func() error {
			return w.store.SpaceIndex(id.SpaceID).ModifyObjectDetails(id.ObjectID, func(details *domain.Details) (*domain.Details, bool, error) {
				return updateBacklinks(details, backlinksUpdate)
			})
		})
	}

	if err == nil {
		return
	}

	if !errors.Is(err, ocache.ErrExists) {
		log.Warn("failed to update backlinks for not cached object", zap.String("objectId", id.ObjectID), zap.Error(err))
	}
	if err = spc.Do(id.ObjectID, func(b smartblock.SmartBlock) error {
		if cr, ok := b.(source.ChangeReceiver); ok {
			return cr.StateAppend(func(d state.Doc) (s *state.State, changes []*pb.ChangeContent, err error) {
				return d.NewState(), nil, nil
			})
		}
		// do no do apply, stateAppend send the event and run the index
		return nil
	}); err != nil {
		return fmt.Errorf("failed to update backlinks: %w", err)
	}
	return
}

func hasSelfLinks(info spaceindex.LinksUpdateInfo) bool {
	for _, link := range info.Added {
		if link == info.LinksFromId.ObjectID {
			return true
		}
	}
	for _, link := range info.Removed {
		if link == info.LinksFromId.ObjectID {
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
		if link != info.LinksFromId.ObjectID {
			infoFilter.Added = append(infoFilter.Added, link)
		}
	}
	for _, link := range info.Removed {
		if link != info.LinksFromId.ObjectID {
			infoFilter.Removed = append(infoFilter.Removed, link)
		}
	}
	return infoFilter
}
