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

// Apply overrides SmartBlock.Apply to intercept state changes and publish
// them to the CRDT store as operations. We disable tree push because the
// store is the source of truth.
//
// The sync model is op-based, not snapshot-sync: we diff the caller's
// pre-mutation VW state against the post-mutation state and emit one
// Create/Delete/Update per touched entry. We never read the store here —
// that would race with concurrent remote changes (the store's observer
// would be blocked on VW's lock while syncToStore reads the CRDT state,
// see-ing remote entries that VW hasn't absorbed yet; diffing against them
// would incorrectly delete them). Remote changes flow in via the observer
// path (OnWidgetCreate/Update/Delete → rebuildAll), unchanged.
//
// Field-level diffing: diffEntry only flags fields where prev differs from
// desired, so concurrent remote edits to untouched fields survive.
func (v *VirtualWidgetObject) Apply(s *state.State, flags ...smartblock.ApplyFlag) error {
	spaceId := v.SpaceID()
	parent := s.ParentState()
	if parent == nil {
		// Defensive: no pre-mutation state available, so we can't compute a
		// delta safely. Skip the push rather than risk treating prev as empty
		// (which would re-create every entry and potentially delete the rest).
		log.Warnf("apply without parent state; skipping delta push for space %s", spaceId)
		return v.SmartBlock.Apply(s, append(flags, smartblock.NotPushChanges)...)
	}

	prev := extractEntriesFromState(parent, spaceId)
	desired := extractEntriesFromState(s, spaceId)

	if err := v.SmartBlock.Apply(s, append(flags, smartblock.NotPushChanges)...); err != nil {
		return fmt.Errorf("apply smartblock: %w", err)
	}
	v.pushDelta(prev, desired, spaceId)
	return nil
}

// pushDelta publishes the entry-level ops that take prev to desired. It does
// not read the store, so it cannot interpret a concurrent remote change as a
// local deletion.
func (v *VirtualWidgetObject) pushDelta(prev, desired []personalfavorites.WidgetEntry, spaceId string) {
	ctx := context.Background()

	prevMap := make(map[string]personalfavorites.WidgetEntry, len(prev))
	for _, e := range prev {
		prevMap[e.Id] = e
	}
	desiredMap := make(map[string]personalfavorites.WidgetEntry, len(desired))
	for _, e := range desired {
		desiredMap[e.Id] = e
	}

	// Deletes: entries the caller removed from VW.
	for id := range prevMap {
		if _, ok := desiredMap[id]; ok {
			continue
		}
		if err := v.service.DeleteWidget(ctx, id); err != nil {
			log.Warnf("push delta: delete %s: %s", id, err)
		}
	}

	// Creates + updates: walk desired in order and emit the per-entry op.
	for _, d := range desired {
		d.SpaceId = spaceId
		p, exists := prevMap[d.Id]
		if !exists {
			if err := v.service.CreateWidget(ctx, d); err != nil {
				log.Warnf("push delta: create %s: %s", d.Id, err)
			}
			continue
		}
		update, changed := diffEntry(p, d)
		if changed {
			if err := v.service.UpdateWidget(ctx, d.Id, update); err != nil {
				log.Warnf("push delta: update %s: %s", d.Id, err)
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
