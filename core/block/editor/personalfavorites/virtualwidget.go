package personalfavorites

import (
	"context"
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/editor/widget"
	"github.com/anyproto/anytype-heart/core/block/personalfavorites"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const widgetWrapperSuffix = "-wrapper"

type VirtualWidgetObject struct {
	smartblock.SmartBlock
	basic.IHistory
	basic.Movable
	basic.Unlinkable
	basic.Updatable
	widget.Widget
	basic.DetailsSettable

	service     personalfavorites.Service
	unsubscribe func()
}

func NewVirtualWidget(
	sb smartblock.SmartBlock,
	objectStore spaceindex.Store,
	svc personalfavorites.Service,
	layoutConverter converter.LayoutConverter,
) *VirtualWidgetObject {
	bs := basic.NewBasic(sb, objectStore, layoutConverter, nil)
	return &VirtualWidgetObject{
		SmartBlock:      sb,
		Movable:         bs,
		Updatable:       bs,
		DetailsSettable: bs,
		IHistory:        basic.NewHistory(sb),
		Widget:          widget.NewWidget(sb),
		service:         svc,
	}
}

func (v *VirtualWidgetObject) Init(ctx *smartblock.InitContext) error {
	if err := v.SmartBlock.Init(ctx); err != nil {
		return fmt.Errorf("init smartblock: %w", err)
	}

	template.InitTemplate(ctx.State,
		template.WithEmpty,
		template.WithObjectTypes([]domain.TypeKey{bundle.TypeKeyDashboard}),
		template.WithLayout(model.ObjectType_dashboard),
		template.WithDetail(bundle.RelationKeyIsHidden, domain.Bool(true)),
	)

	v.unsubscribe = v.service.Subscribe(personalfavorites.SubscribeParams{
		SpaceId:  v.SpaceID(),
		Observer: v,
	})

	// Init must fail on load error: an empty initial state would be
	// snapshot-synced back on the next Apply and delete the store contents.
	entries, err := v.service.GetWidgets(ctx.Ctx, v.SpaceID())
	if err != nil {
		return fmt.Errorf("load initial widgets for space %s: %w", v.SpaceID(), err)
	}
	v.rebuildStateFromEntries(ctx.State, entries)

	return v.SmartBlock.Apply(ctx.State, smartblock.NotPushChanges, smartblock.NoHistory)
}

func (v *VirtualWidgetObject) Close() error {
	if v.unsubscribe != nil {
		v.unsubscribe()
	}
	return v.SmartBlock.Close()
}

// Apply overrides SmartBlock.Apply to intercept state changes and sync them to
// the CRDT store. We disable tree push since the store is the source of truth.
//
// The sync model is a deterministic snapshot-sync rather than an event-diff.
// After SmartBlock.Apply commits the caller's mutations locally, we walk the
// resulting block tree once to compute the *desired* list of store entries
// (with correct AfterId derived from sibling order under root), load the
// *current* store entries for this space, and emit one Create/Delete/Update
// per divergence. Each operation is independent: Update can change any subset
// of {Layout, Limit, ViewId, AfterId} atomically, so reorders don't need a
// linked-list repair dance — the store just stores whatever AfterId we tell
// it to.
func (v *VirtualWidgetObject) Apply(s *state.State, flags ...smartblock.ApplyFlag) error {
	if err := v.SmartBlock.Apply(s, append(flags, smartblock.NotPushChanges)...); err != nil {
		return fmt.Errorf("apply smartblock: %w", err)
	}
	v.syncToStore()
	return nil
}

func (v *VirtualWidgetObject) syncToStore() {
	ctx := context.Background()
	spaceId := v.SpaceID()
	desired := v.extractEntries(v.NewState())

	current, err := v.service.GetWidgets(ctx, spaceId)
	if err != nil {
		log.Errorf("sync widgets: load current state: %s", err)
		return
	}

	currentMap := make(map[string]personalfavorites.WidgetEntry, len(current))
	for _, e := range current {
		currentMap[e.Id] = e
	}
	desiredMap := make(map[string]personalfavorites.WidgetEntry, len(desired))
	for _, e := range desired {
		desiredMap[e.Id] = e
	}

	// Deletes: entries in the store that the desired state no longer contains.
	for id := range currentMap {
		if _, ok := desiredMap[id]; ok {
			continue
		}
		if err := v.service.DeleteWidget(ctx, id); err != nil {
			log.Warnf("sync widgets: delete %s: %s", id, err)
		}
	}

	// Creates + updates: walk desired in order and reconcile each entry with
	// the current store view.
	for _, d := range desired {
		d.SpaceId = spaceId
		cur, exists := currentMap[d.Id]
		if !exists {
			if err := v.service.CreateWidget(ctx, d); err != nil {
				log.Warnf("sync widgets: create %s: %s", d.Id, err)
			}
			continue
		}
		update, changed := diffEntry(cur, d)
		if changed {
			if err := v.service.UpdateWidget(ctx, d.Id, update); err != nil {
				log.Warnf("sync widgets: update %s: %s", d.Id, err)
			}
		}
	}
}

// diffEntry compares two entries (same Id, same SpaceId assumed) and returns
// the set of fields that changed. TargetId is immutable for a given entry —
// changing the target of a widget is modeled as delete + create in the block
// layer, so we don't need to handle it here.
func diffEntry(cur, desired personalfavorites.WidgetEntry) (personalfavorites.WidgetUpdate, bool) {
	var update personalfavorites.WidgetUpdate
	changed := false
	if cur.Layout != desired.Layout {
		v := desired.Layout
		update.Layout = &v
		changed = true
	}
	if cur.Limit != desired.Limit {
		v := desired.Limit
		update.Limit = &v
		changed = true
	}
	if cur.ViewId != desired.ViewId {
		v := desired.ViewId
		update.ViewId = &v
		changed = true
	}
	if cur.AfterId != desired.AfterId {
		v := desired.AfterId
		update.AfterId = &v
		changed = true
	}
	return update, changed
}

func (v *VirtualWidgetObject) extractEntries(s *state.State) []personalfavorites.WidgetEntry {
	return extractEntriesFromState(s, v.SpaceID())
}

// extractEntriesFromState walks the root's children expecting each to be a
// widget wrapper holding a single link child, and chains the resulting
// entries via AfterId. Blocks not matching the wrapper→link shape are
// dropped and logged — silent data loss from a corrupt state would be worse.
func extractEntriesFromState(s *state.State, spaceId string) []personalfavorites.WidgetEntry {
	root := s.Pick(s.RootId())
	if root == nil {
		return nil
	}

	var prevId string
	var entries []personalfavorites.WidgetEntry
	for _, wrapperId := range root.Model().ChildrenIds {
		wrapper := s.Pick(wrapperId)
		if wrapper == nil {
			log.Warnf("extract entries: missing wrapper %s under root", wrapperId)
			continue
		}
		wc, ok := wrapper.Model().Content.(*model.BlockContentOfWidget)
		if !ok {
			log.Warnf("extract entries: block %s under root is not a widget wrapper (%T)", wrapperId, wrapper.Model().Content)
			continue
		}
		if len(wrapper.Model().ChildrenIds) == 0 {
			log.Warnf("extract entries: widget wrapper %s has no child link", wrapperId)
			continue
		}
		linkId := wrapper.Model().ChildrenIds[0]
		link := s.Pick(linkId)
		if link == nil {
			log.Warnf("extract entries: missing link %s in wrapper %s", linkId, wrapperId)
			continue
		}
		lc, ok := link.Model().Content.(*model.BlockContentOfLink)
		if !ok {
			log.Warnf("extract entries: child %s of wrapper %s is not a link (%T)", linkId, wrapperId, link.Model().Content)
			continue
		}

		entries = append(entries, personalfavorites.WidgetEntry{
			Id:       linkId,
			SpaceId:  spaceId,
			TargetId: lc.Link.TargetBlockId,
			Layout:   wc.Widget.Layout,
			Limit:    wc.Widget.Limit,
			ViewId:   wc.Widget.ViewId,
			AfterId:  prevId,
		})
		prevId = linkId
	}
	return entries
}

// rebuildStateFromEntries rebuilds the block state from CRDT entries.
func (v *VirtualWidgetObject) rebuildStateFromEntries(st *state.State, entries []personalfavorites.WidgetEntry) {
	// Remove all existing widget blocks
	root := st.Pick(st.RootId())
	if root != nil {
		for _, childId := range root.Model().ChildrenIds {
			st.Unlink(childId)
		}
	}

	// Add widget blocks from entries (already in order from resolveOrder)
	for _, entry := range entries {
		linkBlock := simple.New(&model.Block{
			Id: entry.Id,
			Content: &model.BlockContentOfLink{
				Link: &model.BlockContentLink{
					TargetBlockId: entry.TargetId,
				},
			},
		})
		wrapperId := entry.Id + widgetWrapperSuffix
		wrapperBlock := simple.New(&model.Block{
			Id:          wrapperId,
			ChildrenIds: []string{entry.Id},
			Content: &model.BlockContentOfWidget{
				Widget: &model.BlockContentWidget{
					Layout: entry.Layout,
					Limit:  entry.Limit,
					ViewId: entry.ViewId,
				},
			},
		})
		// Set (not Add) so remote updates override stale wrapper/link blocks
		// still held by the parent state: wrapperId and linkId are stable
		// across rebuilds, so Add would be a no-op via Pick's parent walk and
		// Layout/Limit/ViewId changes would be silently dropped.
		st.Set(linkBlock)
		st.Set(wrapperBlock)
		st.InsertTo(st.RootId(), model.Block_Inner, wrapperId)
	}
}

// Observer interface implementation — called by the Service when CRDT changes arrive
// from remote devices. These run on the store's goroutine, so we must lock the virtual widget.

func (v *VirtualWidgetObject) OnWidgetCreate(entry personalfavorites.WidgetEntry) {
	v.Lock()
	defer v.Unlock()
	v.rebuildAll()
}

func (v *VirtualWidgetObject) OnWidgetUpdate(entry personalfavorites.WidgetEntry) {
	v.Lock()
	defer v.Unlock()
	v.rebuildAll()
}

func (v *VirtualWidgetObject) OnWidgetDelete(wrapperId string) {
	v.Lock()
	defer v.Unlock()
	v.rebuildAll()
}

func (v *VirtualWidgetObject) rebuildAll() {
	entries, err := v.service.GetWidgets(context.Background(), v.SpaceID())
	if err != nil {
		log.Warnf("rebuild all: %s", err)
		return
	}
	st := v.NewState()
	v.rebuildStateFromEntries(st, entries)
	if err := v.SmartBlock.StateRebuild(st); err != nil {
		log.Warnf("state rebuild: %s", err)
	}
}

// Unlink overrides the widget unlink to also handle wrapper blocks.
func (v *VirtualWidgetObject) Unlink(ctx session.Context, ids ...string) (err error) {
	st := v.NewStateCtx(ctx)
	widget.UnlinkWithWrapper(st, ids...)
	return v.Apply(st)
}
