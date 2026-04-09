package personalfavorites

import (
	"context"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/techspace"
)

const CName = "core.block.personalfavorites"

type Service interface {
	app.Component

	Subscribe(params SubscribeParams) (unsubscribe func())
	CreateWidget(ctx context.Context, entry WidgetEntry) error
	DeleteWidget(ctx context.Context, id string) error
	UpdateWidget(ctx context.Context, id string, updates WidgetUpdate) error
	GetWidgets(ctx context.Context, spaceId string) ([]WidgetEntry, error)

	// OnStoreUpdate is wired into the tech-space store object at
	// construction time (see factory.New). It runs on the store's dispatcher
	// goroutine, so observer calls may re-enter the store via GetWidgets
	// without deadlocking.
	OnStoreUpdate(spaceId string, changes []WidgetChange)
}

type subscription struct {
	spaceId  string
	observer Observer
	done     chan struct{} // closed when the subscription is removed or replaced
}

type service struct {
	spaceService space.Service

	mu            sync.RWMutex
	subscriptions map[string]*subscription // keyed by spaceId
}

func New() Service {
	return &service{
		subscriptions: make(map[string]*subscription),
	}
}

func (s *service) Init(a *app.App) error {
	s.spaceService = app.MustComponent[space.Service](a)
	return nil
}

func (s *service) Name() string {
	return CName
}

func (s *service) techSpace() techspace.TechSpace {
	ts := s.spaceService.TechSpace()
	if ts == nil {
		return nil
	}
	return ts.TechSpace
}

// Subscribe adds a subscription for the given space. A second Subscribe for
// the same space replaces the previous one — the old subscription's done
// channel is closed so any in-flight dispatch skips it, and the old
// unsubscribe closure becomes a no-op (identity check against the current
// map entry).
func (s *service) Subscribe(params SubscribeParams) (unsubscribe func()) {
	sub := &subscription{
		spaceId:  params.SpaceId,
		observer: params.Observer,
		done:     make(chan struct{}),
	}
	s.mu.Lock()
	if prev, ok := s.subscriptions[params.SpaceId]; ok {
		close(prev.done)
	}
	s.subscriptions[params.SpaceId] = sub
	s.mu.Unlock()

	return func() {
		s.mu.Lock()
		if current, ok := s.subscriptions[params.SpaceId]; ok && current == sub {
			delete(s.subscriptions, params.SpaceId)
			close(sub.done)
		}
		s.mu.Unlock()
	}
}

func (s *service) CreateWidget(ctx context.Context, entry WidgetEntry) error {
	return s.doStore(ctx, func(store StoreObject) error {
		return store.CreateWidget(ctx, entry)
	})
}

func (s *service) DeleteWidget(ctx context.Context, id string) error {
	return s.doStore(ctx, func(store StoreObject) error {
		return store.DeleteWidget(ctx, id)
	})
}

func (s *service) UpdateWidget(ctx context.Context, id string, updates WidgetUpdate) error {
	return s.doStore(ctx, func(store StoreObject) error {
		return store.UpdateWidget(ctx, id, updates)
	})
}

func (s *service) GetWidgets(ctx context.Context, spaceId string) ([]WidgetEntry, error) {
	var result []WidgetEntry
	err := s.doStore(ctx, func(store StoreObject) error {
		var err error
		result, err = store.GetWidgets(ctx, spaceId)
		return err
	})
	return result, err
}

// doStore opens the tech-space personal favorites store and calls apply
// under its smartblock lock. techspace exposes PersonalFavoritesStore as an
// opaque smartblock to avoid an import cycle, so the cast to StoreObject
// lives here.
func (s *service) doStore(ctx context.Context, apply func(store StoreObject) error) error {
	ts := s.techSpace()
	if ts == nil {
		return fmt.Errorf("tech space not initialized")
	}
	return ts.DoPersonalFavoritesStore(ctx, func(pfStore techspace.PersonalFavoritesStore) error {
		storeObj, ok := pfStore.(StoreObject)
		if !ok {
			return fmt.Errorf("personal favorites smartblock %T is not a StoreObject — check factory wiring for SmartBlockTypeTechSpaceObject", pfStore)
		}
		return apply(storeObj)
	})
}

func (s *service) OnStoreUpdate(spaceId string, changes []WidgetChange) {
	s.mu.RLock()
	sub, ok := s.subscriptions[spaceId]
	s.mu.RUnlock()
	if !ok {
		return
	}
	select {
	case <-sub.done:
		return
	default:
	}
	for _, ch := range changes {
		switch ch.Type {
		case ChangeCreate:
			sub.observer.OnWidgetCreate(ch.Entry)
		case ChangeModify:
			sub.observer.OnWidgetUpdate(ch.Entry)
		case ChangeDelete:
			sub.observer.OnWidgetDelete(ch.Entry.Id)
		}
	}
}
