package personalfavorites

import (
	"context"
	"fmt"
	"sync"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-sync/app/logger"

	"github.com/anyproto/anytype-heart/core/block/editor/anystoredebug"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var log = logger.NewNamedSugared("common.editor.personalfavorites")

const (
	collectionName    = "personalFavorites"
	dispatchQueueSize = 64
)

// StoreObject is the per-account CRDT store of personal favorites entries
// living in the tech space. Ordering is encoded in each entry's AfterId, so
// reorder is just an UpdateWidget on every entry whose AfterId changed —
// there is no separate move/repair helper.
type StoreObject interface {
	smartblock.SmartBlock
	anystoredebug.AnystoreDebug

	CreateWidget(ctx context.Context, entry WidgetEntry) error
	DeleteWidget(ctx context.Context, id string) error
	UpdateWidget(ctx context.Context, id string, updates WidgetUpdate) error
	GetWidgets(ctx context.Context, spaceId string) ([]WidgetEntry, error)
	GetWidget(ctx context.Context, id string) (WidgetEntry, error)
}

type storeObject struct {
	anystoredebug.AnystoreDebug
	smartblock.SmartBlock
	storeState  *storestate.StoreState
	storeSource source.Store
	crdtDb      anystore.DB
	handler     *favoritesHandler
	ctx         context.Context
	cancel      context.CancelFunc
	arenaPool   *anyenc.ArenaPool

	// onUpdate is set once at construction time and read by runDispatcher.
	onUpdate func(changes []pendingChange)

	dispatchQueue chan []pendingChange
	dispatcherWg  sync.WaitGroup
}

func NewStore(sb smartblock.SmartBlock, crdtDb anystore.DB, onUpdate func(changes []pendingChange)) StoreObject {
	return &storeObject{
		SmartBlock: sb,
		crdtDb:     crdtDb,
		arenaPool:  &anyenc.ArenaPool{},
		onUpdate:   onUpdate,
	}
}

func (s *storeObject) Init(ctx *smartblock.InitContext) error {
	st := ctx.Doc.(*state.State)
	st.SetObjectTypeKey(bundle.TypeKeyDashboard)
	st.SetDetailAndBundledRelation(bundle.RelationKeyLayout, domain.Int64(int64(model.ObjectType_dashboard)))
	st.SetDetailAndBundledRelation(bundle.RelationKeyIsHidden, domain.Bool(true))

	if err := s.SmartBlock.Init(ctx); err != nil {
		return fmt.Errorf("init smartblock: %w", err)
	}

	s.handler = &favoritesHandler{}
	stateStore, err := storestate.New(ctx.Ctx, s.Id(), s.crdtDb, s.handler)
	if err != nil {
		return fmt.Errorf("create state store: %w", err)
	}
	s.storeState = stateStore

	s.AnystoreDebug = anystoredebug.New(s.SmartBlock)
	storeSource, ok := ctx.Source.(source.Store)
	if !ok {
		return fmt.Errorf("source is not a store")
	}
	s.storeSource = storeSource

	err = storeSource.ReadStoreDoc(ctx.Ctx, stateStore, source.ReadStoreDocParams{
		OnUpdateHook: s.dispatchUpdate,
	})
	if err != nil {
		return fmt.Errorf("read store doc: %w", err)
	}

	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.dispatchQueue = make(chan []pendingChange, dispatchQueueSize)
	s.dispatcherWg.Add(1)
	go s.runDispatcher()

	template.InitTemplate(ctx.State,
		template.WithEmpty,
		template.WithObjectTypes([]domain.TypeKey{bundle.TypeKeyDashboard}),
		template.WithLayout(model.ObjectType_dashboard),
		template.WithDetail(bundle.RelationKeyIsHidden, domain.Bool(true)),
	)

	return s.SmartBlock.Apply(ctx.State, smartblock.NotPushChanges, smartblock.NoHistory)
}

func (s *storeObject) Close() error {
	if s.cancel != nil {
		s.cancel()
	}
	s.dispatcherWg.Wait()
	return s.SmartBlock.Close()
}

// dispatchUpdate is the OnUpdateHook passed to source.ReadStoreDoc. It hands
// off accumulated changes to runDispatcher; the indirection is required
// because this hook runs inside PushStoreChange under the caller's
// SmartBlock lock and observers must not re-enter that lock.
func (s *storeObject) dispatchUpdate() {
	changes := s.handler.FlushPendingChanges()
	if len(changes) == 0 {
		return
	}
	if s.onUpdate == nil || s.dispatchQueue == nil {
		return
	}
	select {
	case <-s.ctx.Done():
		return
	case s.dispatchQueue <- changes:
	default:
		// Observers rebuild full state on every callback, so dropping a
		// delta only costs an intermediate redraw.
		log.With("objectId", s.Id()).Warnf("dispatch queue full, dropping %d changes", len(changes))
	}
}

func (s *storeObject) runDispatcher() {
	defer s.dispatcherWg.Done()
	for {
		select {
		case <-s.ctx.Done():
			return
		case changes, ok := <-s.dispatchQueue:
			if !ok {
				return
			}
			s.onUpdate(changes)
		}
	}
}

func (s *storeObject) CreateWidget(ctx context.Context, entry WidgetEntry) error {
	arena := s.arenaPool.Get()
	defer func() {
		arena.Reset()
		s.arenaPool.Put(arena)
	}()

	obj := arena.NewObject()
	obj.Set("id", arena.NewString(entry.Id))
	obj.Set("spaceId", arena.NewString(entry.SpaceId))
	obj.Set("targetId", arena.NewString(entry.TargetId))
	obj.Set("layout", arena.NewNumberInt(int(entry.Layout)))
	obj.Set("limit", arena.NewNumberInt(int(entry.Limit)))
	obj.Set("viewId", arena.NewString(entry.ViewId))
	obj.Set("afterId", arena.NewString(entry.AfterId))

	builder := &storestate.Builder{}
	if err := builder.Create(collectionName, entry.Id, obj); err != nil {
		return fmt.Errorf("build create change: %w", err)
	}
	if _, err := s.pushChanges(ctx, builder); err != nil {
		return fmt.Errorf("push create change: %w", err)
	}
	return nil
}

func (s *storeObject) DeleteWidget(ctx context.Context, id string) error {
	builder := &storestate.Builder{}
	builder.Delete(collectionName, id)
	if _, err := s.pushChanges(ctx, builder); err != nil {
		return fmt.Errorf("push delete change: %w", err)
	}
	return nil
}

func (s *storeObject) UpdateWidget(ctx context.Context, id string, updates WidgetUpdate) error {
	arena := s.arenaPool.Get()
	defer func() {
		arena.Reset()
		s.arenaPool.Put(arena)
	}()

	builder := &storestate.Builder{}
	if updates.Layout != nil {
		if err := builder.Modify(collectionName, id, []string{"layout"}, pb.ModifyOp_Set, arena.NewNumberInt(int(*updates.Layout))); err != nil {
			return fmt.Errorf("build layout modify: %w", err)
		}
	}
	if updates.Limit != nil {
		if err := builder.Modify(collectionName, id, []string{"limit"}, pb.ModifyOp_Set, arena.NewNumberInt(int(*updates.Limit))); err != nil {
			return fmt.Errorf("build limit modify: %w", err)
		}
	}
	if updates.ViewId != nil {
		if err := builder.Modify(collectionName, id, []string{"viewId"}, pb.ModifyOp_Set, arena.NewString(*updates.ViewId)); err != nil {
			return fmt.Errorf("build viewId modify: %w", err)
		}
	}
	if updates.AfterId != nil {
		if err := builder.Modify(collectionName, id, []string{"afterId"}, pb.ModifyOp_Set, arena.NewString(*updates.AfterId)); err != nil {
			return fmt.Errorf("build afterId modify: %w", err)
		}
	}
	if builder.StoreChange == nil {
		return nil
	}
	if _, err := s.pushChanges(ctx, builder); err != nil {
		return fmt.Errorf("push update change: %w", err)
	}
	return nil
}

func (s *storeObject) pushChanges(ctx context.Context, builder *storestate.Builder) (string, error) {
	return s.storeSource.PushStoreChange(ctx, source.PushStoreChangeParams{
		Changes: builder.ChangeSet,
		State:   s.storeState,
		Time:    time.Now(),
	})
}

func (s *storeObject) GetWidgets(ctx context.Context, spaceId string) ([]WidgetEntry, error) {
	entries, err := s.getAllWidgets(ctx)
	if err != nil {
		return nil, fmt.Errorf("get all widgets: %w", err)
	}
	var filtered []WidgetEntry
	for _, e := range entries {
		if e.SpaceId == spaceId {
			filtered = append(filtered, e)
		}
	}
	return resolveOrder(filtered), nil
}

func (s *storeObject) GetWidget(ctx context.Context, id string) (WidgetEntry, error) {
	coll, err := s.storeState.Collection(ctx, collectionName)
	if err != nil {
		return WidgetEntry{}, fmt.Errorf("get collection: %w", err)
	}
	doc, err := coll.FindId(ctx, id)
	if err != nil {
		return WidgetEntry{}, fmt.Errorf("find document: %w", err)
	}
	return entryFromDoc(doc), nil
}

func (s *storeObject) getAllWidgets(ctx context.Context) ([]WidgetEntry, error) {
	coll, err := s.storeState.Collection(ctx, collectionName)
	if err != nil {
		return nil, fmt.Errorf("get collection: %w", err)
	}
	iter, err := coll.Find(nil).Iter(ctx)
	if err != nil {
		return nil, fmt.Errorf("find documents: %w", err)
	}
	defer iter.Close()

	var entries []WidgetEntry
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, fmt.Errorf("read document: %w", err)
		}
		entries = append(entries, entryFromDoc(doc))
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("iterate documents: %w", err)
	}
	return entries, nil
}

func entryFromDoc(doc anystore.Doc) WidgetEntry {
	return WidgetEntry{
		Id:       doc.Value().GetString("id"),
		SpaceId:  doc.Value().GetString("spaceId"),
		TargetId: doc.Value().GetString("targetId"),
		Layout:   model.BlockContentWidgetLayout(doc.Value().GetInt("layout")),
		Limit:    int32(doc.Value().GetInt("limit")),
		ViewId:   doc.Value().GetString("viewId"),
		AfterId:  doc.Value().GetString("afterId"),
	}
}

// resolveOrder reconstructs the linked list order from afterId references.
// Returns entries sorted from first (afterId=="") to last.
// Handles corrupted state (duplicate afterId, missing links) by appending orphans at the end.
func resolveOrder(entries []WidgetEntry) []WidgetEntry {
	if len(entries) == 0 {
		return nil
	}

	byAfterId := make(map[string][]WidgetEntry, len(entries))
	for _, e := range entries {
		byAfterId[e.AfterId] = append(byAfterId[e.AfterId], e)
	}

	heads := byAfterId[""]
	if len(heads) == 0 {
		return entries
	}

	result := make([]WidgetEntry, 0, len(entries))
	seen := make(map[string]bool, len(entries))

	current := heads[0]
	for {
		if seen[current.Id] {
			break
		}
		seen[current.Id] = true
		result = append(result, current)
		next := byAfterId[current.Id]
		if len(next) == 0 {
			break
		}
		current = next[0]
	}

	for _, e := range entries {
		if !seen[e.Id] {
			result = append(result, e)
		}
	}
	return result
}
