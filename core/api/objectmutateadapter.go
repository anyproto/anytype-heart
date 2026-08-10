package api

import (
	"context"
	"fmt"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// objectMutateAdapter implements apicore.ObjectMutator over the block
// service's object cache: the API v2 edit path (APIV2.md §2 Phase 3). The
// whole mutation happens under one object lock — the consistent read handed
// to the callback, the edit, and the post-apply heads — so the If-Match
// check, the edit, and the new etag are all consistent.
//
// PATCH (MutateObject) is the ordinary editor apply, and since the PUT
// removal it is the whole adapter: a child state of the live doc is handed
// to the op applier and committed with ONE plain sb.Apply — exactly what
// the Block* RPC handlers do. Apply gives per-block restriction checks,
// undo recording, hooks/events, and the minimal id-matched change diff;
// nothing the state already owns (relation links, structural blocks,
// resolvedLayout, extra object types) is ever dropped, because a child
// state inherits it all (review findings A1–A3/A5 fixed by construction).
// That inheritance is why the reset path's preserveEditorOwnedState repair
// left with it — the ops path never needed it (APIV2.md §8.27).
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
// path (A4): object-level Blocks/Details restrictions are not what Apply
// checks, so without this an agent could rewrite objects the editor itself
// refuses to edit.
//
// The check is per-axis (surface review M1): a set and a collection carry
// Restrictions_Blocks but NOT Restrictions_Details, so demanding both of
// every edit made renaming a set — and every addItems/removeItems, the only
// v2 route into an existing collection — permanently refuse. needs comes
// from the ops the batch actually contains.
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
