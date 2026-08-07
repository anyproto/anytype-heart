package api

import (
	"context"
	"fmt"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/history"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// objectMutateAdapter implements apicore.ObjectMutator over the block
// service's object cache: the API v2 edit path (APIV2.md §2 Phase 3). The
// whole mutation happens under one object lock — the consistent read handed
// to the callback, the edit, and the post-apply heads — so the If-Match
// check, the edit, and the new etag are all consistent.
//
// PATCH (MutateObject) is the ordinary editor apply: a child state of the
// live doc is handed to the op applier and committed with ONE plain
// sb.Apply — exactly what the Block* RPC handlers do. Apply gives per-block
// restriction checks, undo recording, hooks/events, and the minimal
// id-matched change diff; nothing the state already owns (relation links,
// structural blocks, resolvedLayout, extra object types) is ever dropped,
// because a child state inherits it all (review findings A1–A3/A5 fixed by
// construction).
//
// PUT (ResetObject) still runs the document-replacement machinery
// (NewDocFromSnapshot → ResetToVersion, the import/history pattern) until
// its stage-3 rework; preserveEditorOwnedState repairs what the AnyBlock
// format deliberately does not carry.
type objectMutateAdapter struct {
	getter cache.ObjectGetter
}

func newObjectMutateAdapter(getter cache.ObjectGetter) apicore.ObjectMutator {
	return &objectMutateAdapter{getter: getter}
}

// MutateObject is the PATCH path: one lock, one child state, one ordinary
// Apply.
func (a *objectMutateAdapter) MutateObject(ctx context.Context, spaceId string, objectId string, needs apicore.EditNeeds, apply func(edit apicore.ObjectEdit) error) ([]string, error) {
	var heads []string
	err := cache.DoContextFullID(a.getter, ctx, domain.FullID{SpaceID: spaceId, ObjectID: objectId}, func(sb smartblock.SmartBlock) error {
		// object-level Blocks/Details restrictions are a service-layer concern
		// (Apply checks per-block restrictions, not these). Only the axes the
		// batch actually touches are demanded — see checkObjectEditable.
		if err := checkObjectEditable(sb, needs); err != nil {
			return err
		}
		st := sb.NewState()
		edit := apicore.ObjectEdit{
			SbType: sb.Type().ToProto(),
			Heads:  append([]string(nil), sb.GetDocInfo().Heads...),
			State:  st,
		}
		if err := apply(edit); err != nil {
			return err
		}
		// the bundled-revision guard still applies: an op that downgrades an
		// installed object's revision must be refused, exactly like the import
		// mirror. Untouched revision/sourceObject are inherited by the child
		// state, so the guard is a no-op on ordinary PATCHes.
		if err := guardBundledRevision(sb, st); err != nil {
			return err
		}
		if err := sb.Apply(st); err != nil {
			return fmt.Errorf("apply edit state: %w", err)
		}
		heads = append([]string(nil), sb.GetDocInfo().Heads...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return heads, nil
}

// ResetObject is the PUT path (stage-3 rework pending): document-level
// replace through the reset-to-version machinery.
func (a *objectMutateAdapter) ResetObject(ctx context.Context, spaceId string, objectId string, build func(cur apicore.ObjectRead) (*model.SmartBlockSnapshotBase, error)) ([]string, error) {
	var heads []string
	err := cache.DoContextFullID(a.getter, ctx, domain.FullID{SpaceID: spaceId, ObjectID: objectId}, func(sb smartblock.SmartBlock) error {
		// A4: the apply below runs with NoRestrictions (the reset machinery
		// rewrites structural blocks), so the object's own restrictions must be
		// checked here — otherwise the API can edit objects the app forbids
		// (workspace, archive, widgets, a set's dataview…). PUT replaces the
		// whole document, so it demands both axes.
		if err := checkObjectEditable(sb, apicore.EditNeeds{Blocks: true, Details: true}); err != nil {
			return err
		}
		cur := readLiveState(sb)
		snapshot, err := build(cur)
		if err != nil {
			return err
		}
		if snapshot == nil {
			heads = cur.Heads
			return nil
		}
		st, err := state.NewDocFromSnapshot(objectId, &pb.ChangeSnapshot{Data: snapshot})
		if err != nil {
			return fmt.Errorf("state from snapshot: %w", err)
		}
		// A1–A3: the AnyBlock format deliberately drops state the editor owns
		// (relation links, structural blocks, resolvedLayout, extra object
		// types). Resetting to a snapshot without them turns each absence into
		// a real change — wiping custom property values on replay, eating an
		// unnamed page's first paragraph, deleting the featured-properties row.
		// Carry them over from the live state so the diff stays what the ops
		// actually changed.
		preserveEditorOwnedState(sb, st)
		if err := guardBundledRevision(sb, st); err != nil {
			return err
		}
		if err := history.ResetToVersion(sb, st); err != nil {
			return fmt.Errorf("apply snapshot: %w", err)
		}
		heads = append([]string(nil), sb.GetDocInfo().Heads...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return heads, nil
}

// checkRestriction returns the object's verdict on ONE restriction axis, or
// nil when that axis is editable. The message names the axis in the API's own
// vocabulary, and the error wraps restriction.ErrRestricted so the service
// layer can classify it (403, not 500).
func checkRestriction(sb smartblock.SmartBlock, r model.RestrictionsObjectRestriction) error {
	if err := sb.Restrictions().Object.Check(r); err != nil {
		switch r {
		case model.Restrictions_Blocks:
			return fmt.Errorf("%w: this object's blocks cannot be edited through the API", err)
		case model.Restrictions_Details:
			return fmt.Errorf("%w: this object's properties cannot be edited through the API", err)
		}
		return err
	}
	return nil
}

// checkObjectEditable enforces the object's own restrictions on the API edit
// path (A4). The reset apply passes NoRestrictions, so without this an agent
// could rewrite objects the editor itself refuses to edit.
//
// The check is per-axis (surface review M1): a set and a collection carry
// Restrictions_Blocks but NOT Restrictions_Details, so demanding both of
// every edit made renaming a set — and every addItems/removeItems, the only
// v2 route into an existing collection — permanently refuse. needs comes
// from the ops the batch actually contains; PUT passes both, because a
// document replace rewrites blocks and details alike.
func checkObjectEditable(sb smartblock.SmartBlock, needs apicore.EditNeeds) error {
	if needs.Blocks {
		if err := checkRestriction(sb, model.Restrictions_Blocks); err != nil {
			return err
		}
	}
	if needs.Details {
		if err := checkRestriction(sb, model.Restrictions_Details); err != nil {
			return err
		}
	}
	return nil
}

// preserveEditorOwnedState copies into the reset state everything the AnyBlock
// format does not carry but the live object owns, so ResetToVersion diffs only
// the edit itself (APIV2.md §8.2; review findings A1–A3 and the multi-type
// case). Anything the format DOES carry stays untouched — the ops are
// authoritative for blocks, details and the primary type.
func preserveEditorOwnedState(sb smartblock.SmartBlock, st *state.State) {
	live := sb.NewState()

	// A1: the snapshot has no RelationLinks at all, so the diff would emit
	// RelationRemove for every custom relation — which deletes the detail value
	// on replay (the GO-7217 class; ResetToVersion repairs bundled keys only).
	if links := live.PickRelationLinks(); len(links) > 0 {
		st.AddRelationLinks(links...)
	}

	// A2: resolvedLayout is a local detail the format strips. Without it
	// resolveLayout sees unset→recommended as a change and runs the note
	// conversion, which moves the first paragraph into the title and unlinks it.
	if v := live.LocalDetails().Get(bundle.RelationKeyResolvedLayout); v.Ok() {
		st.SetLocalDetail(bundle.RelationKeyResolvedLayout, v)
	}

	// multi-type objects: the format carries objectTypes[0] (+ templateFor).
	// Keep any extra keys the live object had rather than dropping them.
	if liveKeys, newKeys := live.ObjectTypeKeys(), st.ObjectTypeKeys(); len(liveKeys) > len(newKeys) && len(newKeys) > 0 {
		st.SetObjectTypeKeys(append(append([]domain.TypeKey(nil), newKeys...), liveKeys[len(newKeys):]...))
	}

	// A3: structural blocks (header wrapper, title, description,
	// featuredRelations) are dropped by the format by design (SPEC §7). The
	// editor regenerates some of them, but featuredRelations only on a full
	// rebuild — so an edit would strip the featured row from open clients.
	// Re-attach the live subtree at the top of the root's children.
	preserveStructuralBlocks(live, st)
}

// preserveStructuralBlocks re-adds the live state's structural top-level
// blocks (and their descendants) to st, in their original leading position.
func preserveStructuralBlocks(live, st *state.State) {
	liveRoot := live.Pick(live.RootId())
	stRoot := st.Pick(st.RootId())
	if liveRoot == nil || stRoot == nil {
		return
	}
	var restored []string
	for _, id := range liveRoot.Model().ChildrenIds {
		b := live.Pick(id)
		if b == nil || !isStructuralBlock(b.Model()) || st.Exists(id) {
			continue
		}
		if copySubtree(live, st, id) {
			restored = append(restored, id)
		}
	}
	if len(restored) == 0 {
		return
	}
	// structural blocks lead the document
	st.Set(simple.New(&model.Block{
		Id:          stRoot.Model().Id,
		ChildrenIds: append(restored, stRoot.Model().ChildrenIds...),
		Content:     stRoot.Model().Content,
		Fields:      stRoot.Model().Fields,
	}))
}

// copySubtree deep-copies the block id and its descendants from live into st.
func copySubtree(live, st *state.State, id string) bool {
	b := live.Pick(id)
	if b == nil {
		return false
	}
	if !st.Add(b.Copy()) {
		return false
	}
	for _, child := range b.Model().ChildrenIds {
		copySubtree(live, st, child)
	}
	return true
}

// isStructuralBlock mirrors anyblockjson's export-side isStructural: the
// blocks the format drops because the editor owns them (SPEC §7).
func isStructuralBlock(b *model.Block) bool {
	switch c := b.Content.(type) {
	case *model.BlockContentOfLayout:
		return c.Layout != nil && c.Layout.Style == model.BlockContentLayout_Header
	case *model.BlockContentOfText:
		if c.Text == nil {
			return false
		}
		return c.Text.Style == model.BlockContentText_Title ||
			c.Text.Style == model.BlockContentText_Description
	case *model.BlockContentOfFeaturedRelations:
		return true
	}
	return false
}

// guardBundledRevision mirrors the import path's revision guard (objectcreator
// resetState / preserveBundledIdentity): an object derived from a bundled
// definition must never be reset to an older revision, and an incoming state
// that carries neither sourceObject nor revision keeps the live object's —
// resetting drops details the new state omits, and these two tie an installed
// object back to its bundled definition.
func guardBundledRevision(sb smartblock.SmartBlock, st *state.State) error {
	incomingDerived := st.Details().GetInt64(bundle.RelationKeyRevision) > 0 ||
		st.Details().GetString(bundle.RelationKeySourceObject) != ""
	if incomingDerived {
		current := sb.Details().GetInt64(bundle.RelationKeyRevision)
		incoming := st.Details().GetInt64(bundle.RelationKeyRevision)
		if current > incoming {
			return fmt.Errorf("the live object carries revision %d, newer than the document's %d — bundled definitions are never downgraded", current, incoming)
		}
		return nil
	}
	for _, key := range []domain.RelationKey{bundle.RelationKeySourceObject, bundle.RelationKeyRevision} {
		if value := sb.Details().Get(key); value.Ok() {
			st.SetDetail(key, value)
		}
	}
	return nil
}
