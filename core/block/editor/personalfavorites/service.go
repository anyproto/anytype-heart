package personalfavorites

import (
	"context"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/techspace"
)

const CName = "core.block.editor.personalfavorites"

type WidgetEntry struct {
	Id       string
	SpaceId  string
	TargetId string
	Layout   model.BlockContentWidgetLayout
	Limit    int32
	ViewId   string
	AfterId  string
}

type WidgetUpdate struct {
	Layout  *model.BlockContentWidgetLayout
	Limit   *int32
	ViewId  *string
	AfterId *string
}

type Observer interface {
	OnWidgetCreate(entry WidgetEntry)
	OnWidgetUpdate(entry WidgetEntry)
	OnWidgetDelete(wrapperId string)
}

type RegisterParams struct {
	SpaceId  string
	Observer Observer
}

type Service interface {
	app.Component

	Register(params RegisterParams) (unregister func())
	CreateWidget(ctx context.Context, entry WidgetEntry) error
	DeleteWidget(ctx context.Context, id string) error
	UpdateWidget(ctx context.Context, id string, updates WidgetUpdate) error
	GetWidgets(ctx context.Context, spaceId string) ([]WidgetEntry, error)

	// OnStoreUpdate is the callback wired into the tech-space store object at
	// construction time (see factory.New). It runs on a dedicated goroutine
	// spawned by the store, so observer calls may freely re-enter the store
	// via GetWidgets without deadlocking.
	OnStoreUpdate(changes []pendingChange)
}

type registration struct {
	spaceId  string
	observer Observer
	done     chan struct{} // closed when the registration is removed
}

type service struct {
	spaceService space.Service

	// mu protects registrations. Register/unregister may be called without
	// the store's SmartBlock lock, while OnStoreUpdate reads registrations
	// from its dispatcher goroutine.
	mu            sync.RWMutex
	registrations map[*registration]struct{}
}

func New() Service {
	return &service{
		registrations: make(map[*registration]struct{}),
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

func (s *service) Register(params RegisterParams) (unregister func()) {
	reg := &registration{
		spaceId:  params.SpaceId,
		observer: params.Observer,
		done:     make(chan struct{}),
	}
	s.mu.Lock()
	s.registrations[reg] = struct{}{}
	s.mu.Unlock()

	return func() {
		s.mu.Lock()
		if _, ok := s.registrations[reg]; ok {
			delete(s.registrations, reg)
			close(reg.done)
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

// OnStoreUpdate is wired into the tech-space store object at construction time
// (see core/block/editor/factory.go) and runs on a goroutine spawned by the
// store. It fans out changes to observers while skipping registrations that
// have already unregistered.
func (s *service) OnStoreUpdate(changes []pendingChange) {
	s.mu.RLock()
	regs := make([]*registration, 0, len(s.registrations))
	for reg := range s.registrations {
		regs = append(regs, reg)
	}
	s.mu.RUnlock()

	for _, reg := range regs {
		// Skip dispatch for registrations that were removed between the
		// snapshot above and now — their observer may be closed.
		select {
		case <-reg.done:
			continue
		default:
		}
		for _, ch := range changes {
			if ch.entry.SpaceId != "" && ch.entry.SpaceId != reg.spaceId {
				continue
			}
			switch ch.typ {
			case changeCreate:
				reg.observer.OnWidgetCreate(ch.entry)
			case changeModify:
				reg.observer.OnWidgetUpdate(ch.entry)
			case changeDelete:
				reg.observer.OnWidgetDelete(ch.entry.Id)
			}
		}
	}
}
