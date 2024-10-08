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
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/slice"
)

const CName = "backlinks-update-watcher"

var log = logging.Logger(CName)

type backlinksUpdater interface {
	SubscribeLinksUpdate(callback func(info spaceindex.LinksUpdateInfo))
}

type backLinksUpdate struct {
	added   []string
	removed []string
}

type UpdateWatcher struct {
	app.ComponentRunnable

	updater      backlinksUpdater
	store        objectstore.ObjectStore
	resolver     idresolver.Resolver
	spaceService space.Service

	infoBatch *mb.MB
}

func New() app.Component {
	return &UpdateWatcher{}
}

func (uw *UpdateWatcher) Name() string {
	return CName
}

func (uw *UpdateWatcher) Init(a *app.App) error {
	uw.updater = app.MustComponent[backlinksUpdater](a)
	uw.store = app.MustComponent[objectstore.ObjectStore](a)
	uw.resolver = app.MustComponent[idresolver.Resolver](a)
	uw.spaceService = app.MustComponent[space.Service](a)
	uw.infoBatch = mb.New(0)

	return nil
}

func (uw *UpdateWatcher) Close(context.Context) error {
	if err := uw.infoBatch.Close(); err != nil {
		log.Errorf("failed to close message batch: %v", err)
	}
	return nil
}

func (uw *UpdateWatcher) Run(context.Context) error {
	uw.updater.SubscribeLinksUpdate(func(info spaceindex.LinksUpdateInfo) {
		if err := uw.infoBatch.Add(info); err != nil {
			log.With("objectId", info.LinksFromId).Errorf("failed to add backlinks update info to message batch: %v", err)
		}
	})

	go uw.backlinksUpdateHandler()
	return nil
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

func (uw *UpdateWatcher) backlinksUpdateHandler() {
	var (
		accumulatedBacklinks = make(map[string]*backLinksUpdate)
		l                    sync.Mutex
		lastReceivedUpdates  time.Time
		closedCh             = make(chan struct{})
		aggregationInterval  = time.Second * 5
	)
	defer close(closedCh)

	go func() {
		process := func() {
			log.Debugf("updating backlinks for %d objects", len(accumulatedBacklinks))
			for id, updates := range accumulatedBacklinks {
				uw.updateBackLinksInObject(id, updates)
			}
			accumulatedBacklinks = make(map[string]*backLinksUpdate)
		}
		for {
			select {
			case <-closedCh:
				l.Lock()
				process()
				l.Unlock()
				return
			case <-time.After(aggregationInterval):
				l.Lock()
				if time.Since(lastReceivedUpdates) < aggregationInterval || len(accumulatedBacklinks) == 0 {
					l.Unlock()
					continue
				}

				process()
				l.Unlock()
			}
		}
	}()

	for {
		msgs := uw.infoBatch.Wait()
		if len(msgs) == 0 {
			return
		}

		l.Lock()
		for _, msg := range msgs {
			info, ok := msg.(spaceindex.LinksUpdateInfo)
			if !ok || hasSelfLinks(info) {
				continue
			}

			applyUpdates(accumulatedBacklinks, info)
		}
		lastReceivedUpdates = time.Now()
		l.Unlock()
	}
}

func (uw *UpdateWatcher) updateBackLinksInObject(id string, backlinksUpdate *backLinksUpdate) {
	spaceId, err := uw.resolver.ResolveSpaceID(id)
	if err != nil {
		log.With("objectId", id).Errorf("failed to resolve space id for object: %v", err)
		return
	}
	spc, err := uw.spaceService.Get(context.Background(), spaceId)
	if err != nil {
		log.With("objectId", id, "spaceId", spaceId).Errorf("failed to get space: %v", err)
		return
	}

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

		current.SetStringList(bundle.RelationKeyBacklinks, backlinks)
		return current, true, nil
	}

	err = spc.DoLockedIfNotExists(id, func() error {
		return uw.store.SpaceIndex(spaceId).ModifyObjectDetails(id, func(details *domain.Details) (*domain.Details, bool, error) {
			return updateBacklinks(details, backlinksUpdate)
		})
	})

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
