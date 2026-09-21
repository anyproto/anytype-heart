package api

import (
	"context"
	"fmt"

	"github.com/globalsign/mgo/bson"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/widget"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
)

// widgetSpaces is the one method of space.Service the widget adapter needs:
// the loaded space, for its derived widget-object id and its ACL verdicts.
type widgetSpaces interface {
	Get(ctx context.Context, spaceId string) (clientspace.Space, error)
}

// widgetAdapter implements apicore.Widgets over the object cache and the
// space service — the API v2 sidebar-widget path (APIV2_WIDGETS.md §5.2).
//
// Every read and every mutation is one locked pass over the widget root:
// the same wrapper+link shape the desktop writes, through the same editor
// (WidgetObject for the space root, VirtualWidgetObject for the personal
// root, whose Apply override projects the state onto the tech-space store).
// Doing the work on the state rather than through the per-field RPCs is
// what makes a create-with-placement or an update-with-move ONE change and
// one event instead of two, and it is the pattern builtinobjects already
// uses to install a use case's widgets.
//
// Two invariants the service validates OUTSIDE the lock are re-checked
// under it, because between the service's read and this write another
// request can have changed the root: the target must still be absent
// (one widget per target per root), and the caller must still be allowed
// to write the root. The editor's own Apply refuses the space root for a
// plain writer too, but only after it has merged the state into the live
// document, so the check here is what keeps a refused write invisible.
type widgetAdapter struct {
	getter cache.ObjectGetter
	spaces widgetSpaces
}

func newWidgetAdapter(getter cache.ObjectGetter, spaces widgetSpaces) apicore.Widgets {
	return &widgetAdapter{getter: getter, spaces: spaces}
}

// widgetEditor is what both widget roots implement: the smartblock plus the
// widget-creating editor. Movement and unlinking are done on the state
// directly (widget.UnlinkWithWrapper, State.InsertTo), so no further editor
// interface is demanded.
type widgetEditor interface {
	smartblock.SmartBlock
	widget.Widget
}

// widgetRoot is a resolved root: its object id and the permission verdict
// for writing it, asked of the loaded space.
type widgetRoot struct {
	id      string
	canEdit func() bool
}

// resolveRoot resolves the widget root of scope in the space.
func (a *widgetAdapter) resolveRoot(ctx context.Context, spaceId string, scope apicore.WidgetScope) (widgetRoot, error) {
	spc, err := a.spaces.Get(ctx, spaceId)
	if err != nil {
		return widgetRoot{}, fmt.Errorf("get space %s: %w", spaceId, err)
	}
	switch scope {
	case apicore.WidgetScopeSpace:
		// the space root is space configuration: the editor's Apply refuses
		// it for anyone but the owner or an admin (checkSpaceConfigLock),
		// so this is the same verdict asked up front — minus the editor's
		// one-to-one exemption. A one-to-one space has no owner or admin
		// (its owner is a key derived from both identities), and the
		// desktop's pin action, gated on owner-or-admin, is never offered
		// there; the API offers no more than the desktop does
		return widgetRoot{id: spc.DerivedIDs().Widgets, canEdit: func() bool { return spc.CanManageSpace() && !spc.IsOneToOne() }}, nil
	case apicore.WidgetScopePersonal:
		// the personal root is this account's own, held in the tech space;
		// the desktop still gates favorites on being able to write in the
		// space, and so does this
		return widgetRoot{id: domain.NewPersonalWidgetsId(spaceId), canEdit: func() bool { return !spc.IsReadOnly() }}, nil
	}
	return widgetRoot{}, fmt.Errorf("unknown widget scope %q", scope)
}

func (a *widgetAdapter) CanEditWidgets(ctx context.Context, spaceId string, scope apicore.WidgetScope) (bool, error) {
	root, err := a.resolveRoot(ctx, spaceId, scope)
	if err != nil {
		return false, err
	}
	return root.canEdit(), nil
}

// doRoot runs apply on the locked widget root of scope.
func (a *widgetAdapter) doRoot(ctx context.Context, spaceId string, scope apicore.WidgetScope, apply func(root widgetRoot, sb widgetEditor) error) error {
	root, err := a.resolveRoot(ctx, spaceId, scope)
	if err != nil {
		return err
	}
	return cache.DoContextFullID(a.getter, ctx, domain.FullID{SpaceID: spaceId, ObjectID: root.id}, func(sb widgetEditor) error {
		return apply(root, sb)
	})
}

// commit applies a mutated state after the under-lock permission re-check.
func commit(root widgetRoot, sb widgetEditor, st *state.State, op string) error {
	if !root.canEdit() {
		return fmt.Errorf("%w: %s refused, this account may not write the sidebar", restriction.ErrRestricted, op)
	}
	if err := sb.Apply(st); err != nil {
		return fmt.Errorf("apply %s: %w", op, err)
	}
	return nil
}

func (a *widgetAdapter) ListWidgets(ctx context.Context, spaceId string, scope apicore.WidgetScope) ([]apicore.WidgetEntry, error) {
	var entries []apicore.WidgetEntry
	err := a.doRoot(ctx, spaceId, scope, func(_ widgetRoot, sb widgetEditor) error {
		entries = widgetEntries(sb.NewState(), scope)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read %s widgets: %w", scope, err)
	}
	return entries, nil
}

func (a *widgetAdapter) CreateWidget(ctx context.Context, spaceId string, scope apicore.WidgetScope, create apicore.WidgetCreate) (apicore.WidgetEntry, error) {
	var created apicore.WidgetEntry
	err := a.doRoot(ctx, spaceId, scope, func(root widgetRoot, sb widgetEditor) error {
		st := sb.NewState()
		for _, entry := range widgetEntries(st, scope) {
			if entry.Target == create.Target {
				return fmt.Errorf("%w: %s", apicore.ErrWidgetExists, entry.Id)
			}
		}
		anchor, position, applied, err := placementAnchor(st, scope, create.Placement)
		if err != nil {
			return err
		}
		// the link id is minted here, not left for the editor: with an
		// explicit link id the editor derives the wrapper id from it
		// (<linkId>-wrapper), which is the ONLY wrapper id the personal
		// root keeps — its store holds the link id and re-derives the
		// wrapper on every rebuild, so a random wrapper id would be renamed
		// under the caller moments after this returns it
		wrapperId, err := sb.CreateBlock(st, &pb.RpcBlockCreateWidgetRequest{
			ContextId:    st.RootId(),
			TargetId:     anchor,
			Position:     position,
			WidgetLayout: create.Layout,
			ObjectLimit:  create.Limit,
			ViewId:       create.ViewId,
			Block: &model.Block{Id: bson.NewObjectId().Hex(), Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{
				TargetBlockId: create.Target,
			}}},
		})
		if err != nil {
			return fmt.Errorf("create widget block: %w", err)
		}
		if err := commit(root, sb, st, "widget create"); err != nil {
			return err
		}
		entry, ok := widgetEntry(sb.NewState(), scope, wrapperId)
		if !ok {
			return fmt.Errorf("widget %s not readable after create", wrapperId)
		}
		entry.Placed = &applied
		created = entry
		return nil
	})
	if err != nil {
		return apicore.WidgetEntry{}, err
	}
	return created, nil
}

func (a *widgetAdapter) UpdateWidget(ctx context.Context, spaceId string, scope apicore.WidgetScope, widgetId string, plan func(current apicore.WidgetEntry) (apicore.WidgetUpdate, error)) (apicore.WidgetEntry, error) {
	var updated apicore.WidgetEntry
	err := a.doRoot(ctx, spaceId, scope, func(root widgetRoot, sb widgetEditor) error {
		st := sb.NewState()
		current, ok := widgetEntry(st, scope, widgetId)
		if !ok {
			return apicore.ErrWidgetNotFound
		}
		update, err := plan(current)
		if err != nil {
			return err
		}
		wrapper := st.Get(widgetId)
		content := wrapper.Model().GetWidget()
		if update.Layout != nil {
			content.Layout = *update.Layout
		}
		if update.Limit != nil {
			content.Limit = *update.Limit
		}
		if update.ViewId != nil {
			content.ViewId = *update.ViewId
		}
		var applied *apicore.WidgetPlacement
		if update.Placement != nil {
			// a reorder is unlink + insert on the root's children: the block
			// itself stays in the state, only its position changes
			st.Unlink(widgetId)
			anchor, position, placed, err := placementAnchor(st, scope, *update.Placement)
			if err != nil {
				return err
			}
			if err := st.InsertTo(anchor, position, widgetId); err != nil {
				return fmt.Errorf("move widget: %w", err)
			}
			applied = &placed
		}
		if err := commit(root, sb, st, "widget update"); err != nil {
			return err
		}
		entry, ok := widgetEntry(sb.NewState(), scope, widgetId)
		if !ok {
			return fmt.Errorf("widget %s not readable after update", widgetId)
		}
		entry.Placed = applied
		updated = entry
		return nil
	})
	if err != nil {
		return apicore.WidgetEntry{}, err
	}
	return updated, nil
}

func (a *widgetAdapter) DeleteWidget(ctx context.Context, spaceId string, scope apicore.WidgetScope, widgetId string, expectedTarget string) (apicore.WidgetEntry, error) {
	var removed apicore.WidgetEntry
	err := a.doRoot(ctx, spaceId, scope, func(root widgetRoot, sb widgetEditor) error {
		st := sb.NewState()
		current, ok := widgetEntry(st, scope, widgetId)
		if !ok {
			return apicore.ErrWidgetNotFound
		}
		if current.Target != expectedTarget {
			return fmt.Errorf("%w: now %s", apicore.ErrWidgetRetargeted, current.Target)
		}
		// the wrapper and its link child go together, the way the editor's
		// own Unlink does it; an empty wrapper left behind would be stripped
		// on the next load anyway, silently
		widget.UnlinkWithWrapper(st, widgetId)
		if err := commit(root, sb, st, "widget delete"); err != nil {
			return err
		}
		removed = current
		return nil
	})
	if err != nil {
		return apicore.WidgetEntry{}, err
	}
	return removed, nil
}

// placementAnchor translates a placement into the editor's anchor + position
// pair, against the root as it is under the lock. The editor's naming is the
// trap here: with an empty anchor, Block_Inner APPENDS and Block_InnerFirst
// PREPENDS (state/position.go). The bin rule is enforced here too, on the
// current root: an anchor that now targets the bin is refused, and the
// default placement lands before a trailing bin rather than after it — the
// service resolves the same rule on its own read, for the receipt; this is
// the backstop that holds when the root moved meanwhile.
func placementAnchor(st *state.State, scope apicore.WidgetScope, placement apicore.WidgetPlacement) (anchor string, position model.BlockPosition, applied apicore.WidgetPlacement, err error) {
	switch {
	case placement.AfterId != "":
		target, ok := widgetEntry(st, scope, placement.AfterId)
		if !ok {
			return "", 0, applied, fmt.Errorf("%w: after %s", apicore.ErrWidgetNotFound, placement.AfterId)
		}
		if target.Target == widget.DefaultWidgetBin {
			return "", 0, applied, fmt.Errorf("%w: after %s", apicore.ErrBinPlacement, placement.AfterId)
		}
		return placement.AfterId, model.Block_Bottom, placement, nil
	case placement.BeforeId != "":
		if st.Pick(placement.BeforeId) == nil {
			return "", 0, applied, fmt.Errorf("%w: before %s", apicore.ErrWidgetNotFound, placement.BeforeId)
		}
		return placement.BeforeId, model.Block_Top, placement, nil
	case placement.First:
		return "", model.Block_InnerFirst, placement, nil
	}
	if entries := widgetEntries(st, scope); len(entries) > 0 && entries[len(entries)-1].Target == widget.DefaultWidgetBin {
		bin := entries[len(entries)-1].Id
		return bin, model.Block_Top, apicore.WidgetPlacement{BeforeId: bin}, nil
	}
	return "", model.Block_Inner, apicore.WidgetPlacement{}, nil
}

// widgetEntries walks the root's children in sidebar order, reading each
// wrapper → link pair. A child that is not that shape is skipped rather than
// failing the read: the desktop skips it too, and a stray block must not
// make the whole sidebar unlistable.
func widgetEntries(st *state.State, scope apicore.WidgetScope) []apicore.WidgetEntry {
	root := st.Pick(st.RootId())
	if root == nil {
		return nil
	}
	var entries []apicore.WidgetEntry
	for _, wrapperId := range root.Model().ChildrenIds {
		if entry, ok := widgetEntry(st, scope, wrapperId); ok {
			entries = append(entries, entry)
		}
	}
	return entries
}

// widgetEntry reads one wrapper → link pair; false when wrapperId is not a
// widget wrapper with a link child.
func widgetEntry(st *state.State, scope apicore.WidgetScope, wrapperId string) (apicore.WidgetEntry, bool) {
	wrapper := st.Pick(wrapperId)
	if wrapper == nil {
		return apicore.WidgetEntry{}, false
	}
	content := wrapper.Model().GetWidget()
	if content == nil || len(wrapper.Model().ChildrenIds) == 0 {
		return apicore.WidgetEntry{}, false
	}
	link := st.Pick(wrapper.Model().ChildrenIds[0])
	if link == nil || link.Model().GetLink() == nil {
		return apicore.WidgetEntry{}, false
	}
	return apicore.WidgetEntry{
		Id:     wrapperId,
		LinkId: link.Model().Id,
		Scope:  scope,
		Target: link.Model().GetLink().TargetBlockId,
		Layout: content.Layout,
		Limit:  content.Limit,
		ViewId: content.ViewId,
	}, true
}
