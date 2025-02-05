package source

import (
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"

	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/pb"
)

type storeApply struct {
	tx       *storestate.StoreStateTx
	ot       objecttree.ObjectTree
	allIsNew bool

	needFetchPrevOrderId bool
}

func (a *storeApply) Apply() error {
	return a.ot.IterateRoot(UnmarshalStoreChange, func(change *objecttree.Change) bool {
		// not a new change - remember and continue
		if !a.allIsNew && !change.IsNew {
			return true
		}

		var prevOrderId string
		if a.needFetchPrevOrderId {
			var err error
			prevOrderId, err = a.tx.GetPrevOrderId(change.OrderId)
			if err != nil {
				log.With("error", err).Error("get prev order")
				return false
			}
		}

		err := a.applyChange(prevOrderId, change)
		if err != nil {
			return false
		}

		return true
	})
}

func (a *storeApply) applyChange(prevOrderId string, change *objecttree.Change) (err error) {
	storeChange, ok := change.Model.(*pb.StoreChange)
	if !ok {
		// if it is root
		if _, ok := change.Model.(*treechangeproto.TreeChangeInfo); ok {
			return a.tx.SetOrder(change.Id, change.OrderId)
		}
		return fmt.Errorf("unexpected change content type: %T", change.Model)
	}
	set := storestate.ChangeSet{
		Id:          change.Id,
		PrevOrderId: prevOrderId,
		Order:       change.OrderId,
		Changes:     storeChange.ChangeSet,
		Creator:     change.Identity.Account(),
		Timestamp:   change.Timestamp,
	}
	err = a.tx.ApplyChangeSet(set)
	// Skip invalid changes
	if errors.Is(err, storestate.ErrValidation) {
		return nil
	}
	return err
}
