package sourceimpl

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"

	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/pb"
)

type storeApply struct {
	tx   *storestate.StoreStateTx
	ot   objecttree.ObjectTree
	hook source.ReadStoreTreeHook
}

func (a *storeApply) Apply(ctx context.Context) error {
	var lastErr error

	if a.hook != nil {
		a.hook.BeforeIteration(a.ot)
	}

	iterate := func(change *objecttree.Change) bool {
		a.tx.UpdateMaxAddSeq(change.AddSeq)

		if a.hook != nil {
			a.hook.OnIteration(a.ot, change)
		}

		lastErr = a.applyChange(change)
		return lastErr == nil
	}

	if a.tx.IsFullyReplayed() {
		err := a.ot.IterateAfterAddSeq(ctx, a.tx.GetMaxAddSeq(), UnmarshalStoreChange, iterate)
		return errors.Join(err, lastErr)
	}

	// Nothing has been replayed into the store yet, so the addSeq watermark cannot be used
	// to pick up where we left off: addSeq is a local per-insert sequence that older builds
	// never assigned, and IterateAfterAddSeq only yields changes whose addSeq is strictly
	// greater than the watermark. Starting from 0 therefore silently skips every change
	// stored before addSeq existed. Walk the tree instead — it yields the complete history
	// in order (store-backed objects never snapshot, so the tree spans the genesis root).
	err := a.ot.IterateRoot(UnmarshalStoreChange, iterate)
	if err = errors.Join(err, lastErr); err != nil {
		return err
	}
	a.tx.MarkFullyReplayed()
	return nil
}

func (a *storeApply) applyChange(change *objecttree.Change) (err error) {
	storeChange, ok := change.Model.(*pb.StoreChange)
	if !ok {
		// if it is root, skip — no order tracking needed
		if _, ok := change.Model.(*treechangeproto.TreeChangeInfo); ok {
			return nil
		}
		return fmt.Errorf("unexpected change content type: %T", change.Model)
	}
	set := storestate.ChangeSet{
		Id:        change.Id,
		Order:     change.OrderId,
		AclHeadId: change.AclHeadId,
		Changes:   storeChange.ChangeSet,
		Creator:   change.Identity.Account(),
		Timestamp: change.Timestamp,
	}
	err = a.tx.ApplyChangeSet(set)
	// Skip invalid changes
	if errors.Is(err, storestate.ErrValidation) {
		return nil
	}
	return err
}
