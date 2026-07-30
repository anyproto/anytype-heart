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
// whole mutation happens under one object lock — the current read handed to
// build, the diff-apply of the snapshot build returns, and the post-apply
// heads — so the If-Match check, the edit, and the new etag are all
// consistent. The apply is the smartblock reset-to-version machinery (the
// pattern proven by core/block/import/common/objectcreator.updateExistingObject
// and core/block/history): NewDocFromSnapshot → ResetToVersion, which handles
// SetParent, local-detail injection, bundled relation links, and migrations,
// and lands the change as ONE change set.
type objectMutateAdapter struct {
	getter cache.ObjectGetter
}

func newObjectMutateAdapter(getter cache.ObjectGetter) apicore.ObjectMutator {
	return &objectMutateAdapter{getter: getter}
}

func (a *objectMutateAdapter) MutateObject(ctx context.Context, spaceId string, objectId string, build func(cur apicore.ObjectRead) (*model.SmartBlockSnapshotBase, error)) ([]string, error) {
	var heads []string
	err := cache.DoContextFullID(a.getter, ctx, domain.FullID{SpaceID: spaceId, ObjectID: objectId}, func(sb smartblock.SmartBlock) error {
		// A4: the apply below runs with NoRestrictions (the reset machinery
		// rewrites structural blocks), so the object's own restrictions must be
		// checked here — otherwise the API can edit objects the app forbids
		// (workspace, archive, widgets, a set's dataview…).
		if err := checkObjectEditable(sb); err != nil {
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

// checkObjectEditable enforces the object's own restrictions on the API edit
// path (A4). The reset apply passes NoRestrictions, so without this an agent
// could rewrite objects the editor itself refuses to edit.
func checkObjectEditable(sb smartblock.SmartBlock) error {
	r := sb.Restrictions().Object
	if err := r.Check(model.Restrictions_Blocks); err != nil {
		return fmt.Errorf("%w: this object's blocks cannot be edited through the API", err)
	}
	if err := r.Check(model.Restrictions_Details); err != nil {
		return fmt.Errorf("%w: this object's properties cannot be edited through the API", err)
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
