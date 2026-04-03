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

	maxAddSeq := a.tx.GetMaxAddSeq()

	err := a.ot.IterateAfterAddSeq(ctx, maxAddSeq, UnmarshalStoreChange, func(change *objecttree.Change) bool {
		a.tx.UpdateMaxAddSeq(change.AddSeq)

		if a.hook != nil {
			a.hook.OnIteration(a.ot, change)
		}

		lastErr = a.applyChange(change)
		if lastErr != nil {
			return false
		}

		return true
	})

	return errors.Join(err, lastErr)
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
