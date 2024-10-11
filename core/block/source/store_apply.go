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

	prevOrder  string
	prevChange *objecttree.Change

	nextCachedOrder string
	nextCacheChange map[string]struct{}
}

func (a *storeApply) Apply() (err error) {
	maxOrder := a.tx.GetMaxOrder()
	isEmpty := maxOrder == ""
	iterErr := a.ot.IterateRoot(UnmarshalStoreChange, func(change *objecttree.Change) bool {
		// not a new change - remember and continue
		if !a.allIsNew && !change.IsNew && !isEmpty {
			a.prevChange = change
			a.prevOrder = ""
			return true
		}
		currOrder, curOrdErr := a.tx.GetOrder(change.Id)
		if curOrdErr != nil {
			if !errors.Is(curOrdErr, storestate.ErrOrderNotFound) {
				err = curOrdErr
				return false
			}
		} else {
			// change has been handled before
			a.prevChange = change
			a.prevOrder = currOrder
			return true
		}

		prevOrder, prevOrdErr := a.getPrevOrder()
		if prevOrdErr != nil {
			if !errors.Is(prevOrdErr, storestate.ErrOrderNotFound) {
				err = prevOrdErr
				return false
			}
			if !isEmpty {
				// it should not happen, consistency with tree and store broken
				err = fmt.Errorf("unable to find previous order")
				return false
			}
		}

		if prevOrder == a.tx.GetMaxOrder() {
			// insert on top - just create next id
			currOrder = a.tx.NextOrder(prevOrder)
		} else {
			// insert in the middle - find next order and create id between
			nextOrder, nextOrdErr := a.findNextOrder(change.Id)
			if nextOrdErr != nil {
				// it should not happen, consistency with tree and store broken
				err = errors.Join(nextOrdErr, fmt.Errorf("unable to find next order"))
				return false
			}
			if currOrder, err = a.tx.NextBeforeOrder(prevOrder, nextOrder); err != nil {
				return false
			}
		}

		if err = a.applyChange(change, currOrder); err != nil {
			return false
		}
		a.prevOrder = currOrder
		a.prevChange = change
		return true
	})
	if err == nil && iterErr != nil {
		return iterErr
	}
	return
}

func (a *storeApply) applyChange(change *objecttree.Change, order string) (err error) {
	storeChange, ok := change.Model.(*pb.StoreChange)
	if !ok {
		// if it is root
		if _, ok := change.Model.(*treechangeproto.TreeChangeInfo); ok {
			return a.tx.SetOrder(change.Id, order)
		}
		return fmt.Errorf("unexpected change content type: %T", change.Model)
	}
	set := storestate.ChangeSet{
		Id:        change.Id,
		Order:     order,
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

func (a *storeApply) getPrevOrder() (order string, err error) {
	if a.prevOrder != "" {
		return a.prevOrder, nil
	}
	if a.prevChange == nil {
		return "", nil
	}
	if order, err = a.tx.GetOrder(a.prevChange.Id); err != nil {
		return
	}
	a.prevOrder = order
	return
}

func (a *storeApply) findNextOrder(changeId string) (order string, err error) {
	if order = a.findNextInCache(changeId); order != "" {
		return
	}

	a.nextCacheChange = map[string]struct{}{}
	iterErr := a.ot.IterateFrom(changeId, UnmarshalStoreChange, func(change *objecttree.Change) bool {
		order, err = a.tx.GetOrder(change.Id)
		if err != nil {
			if errors.Is(err, storestate.ErrOrderNotFound) {
				// no order - remember id and move forward
				a.nextCacheChange[change.Id] = struct{}{}
				return true
			} else {
				return false
			}
		}
		// order found
		a.nextCachedOrder = order
		return false
	})
	if err == nil && iterErr != nil {
		return "", iterErr
	}
	return
}

func (a *storeApply) findNextInCache(changeId string) (order string) {
	if a.nextCacheChange == nil {
		return ""
	}
	if _, ok := a.nextCacheChange[changeId]; ok {
		return a.nextCachedOrder
	}
	a.nextCachedOrder = ""
	a.nextCacheChange = nil
	return ""
}
