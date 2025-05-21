package sourceimpl

import (
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/any-sync/util/slice"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/pb"
)

type storeApply struct {
	tx              *storestate.StoreStateTx
	ot              objecttree.ObjectTree
	allIsNew        bool
	currentIdentity crypto.PubKey

	needFetchPrevOrderId bool
}

func (a *storeApply) Apply() error {
	var lastErr error

	a.ot.AclList().RLock()
	joinedAclRecordId := a.ot.AclList().Head().Id
	for _, accState := range a.ot.AclList().AclState().CurrentAccounts() {
		if !accState.PubKey.Equals(a.currentIdentity) {
			continue
		}
		noPermissionsIdx := -1
		for i := len(accState.PermissionChanges) - 1; i >= 0; i-- {
			permChange := accState.PermissionChanges[i]
			if permChange.Permission.NoPermissions() {
				noPermissionsIdx = i
				break
			}
		}

		if noPermissionsIdx == -1 || noPermissionsIdx == len(accState.PermissionChanges)-1 {
			break
		}

		// Get a permission change when user was joined space successfully
		permChange := accState.PermissionChanges[noPermissionsIdx+1]
		joinedAclRecordId = permChange.RecordId
	}
	a.ot.AclList().RUnlock()

	var heads []string
	err := a.ot.IterateRoot(UnmarshalStoreChange, func(change *objecttree.Change) bool {
		// not a new change - remember and continue
		if !a.allIsNew && !change.IsNew {
			return true
		}

		if ok, _ := a.ot.AclList().IsAfter(joinedAclRecordId, change.AclHeadId); ok {
			heads = slice.DiscardFromSlice(heads, func(s string) bool {
				return slices.Contains(change.PreviousIds, s)
			})
			if !slices.Contains(heads, change.Id) {
				heads = append(heads, change.Id)
			}
		}

		lastErr = a.applyChange(change)
		if lastErr != nil {
			return false
		}

		return true
	})

	fmt.Println("HEADS=", heads)

	return errors.Join(err, lastErr)
}

func (a *storeApply) applyChange(change *objecttree.Change) (err error) {
	storeChange, ok := change.Model.(*pb.StoreChange)
	if !ok {
		// if it is root
		if _, ok := change.Model.(*treechangeproto.TreeChangeInfo); ok {
			return a.tx.SetOrder(change.Id, change.OrderId)
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
