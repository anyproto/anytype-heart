package anystoredebug

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/objecttreebuilder"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/source/sourceimpl"
)

type DebugChange struct {
	ChangeId string
	OrderId  string
	Change   *types.Struct
	Error    error
}

type AnystoreDebug interface {
	DebugChanges(ctx context.Context) ([]*DebugChange, error)
}

type debugComponent struct {
	smartblock.SmartBlock
	store *storestate.StoreState
}

func New(sb smartblock.SmartBlock, store *storestate.StoreState) AnystoreDebug {
	return &debugComponent{
		SmartBlock: sb,
		store:      store,
	}
}

func (s *debugComponent) DebugChanges(ctx context.Context) ([]*DebugChange, error) {
	historyTree, err := s.SmartBlock.Space().TreeBuilder().BuildHistoryTree(context.Background(), s.Id(), objecttreebuilder.HistoryTreeOpts{
		Heads:   nil,
		Include: true,
	})
	if err != nil {
		return nil, fmt.Errorf("build history tree: %w", err)
	}

	tx, err := s.store.NewTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("new tx: %w", err)
	}
	// todo: replace with readonly tx
	defer tx.Rollback()

	var result []*DebugChange
	err = historyTree.IterateFrom(historyTree.Root().Id, sourceimpl.UnmarshalStoreChange, func(change *objecttree.Change) bool {
		orderId, err := tx.GetOrder(change.Id)
		if err != nil {
			result = append(result, &DebugChange{
				ChangeId: change.Id,
				Error:    fmt.Errorf("get order: %w", err),
			})
		}

		raw, err := json.Marshal(change.Model)
		if err != nil {
			result = append(result, &DebugChange{
				ChangeId: change.Id,
				OrderId:  orderId,
				Error:    fmt.Errorf("marshal json: %w", err),
			})
			return true
		}
		changeStruct := &types.Struct{Fields: map[string]*types.Value{}}
		err = jsonpb.UnmarshalString(string(raw), changeStruct)
		if err != nil {
			result = append(result, &DebugChange{
				ChangeId: change.Id,
				OrderId:  orderId,
				Error:    fmt.Errorf("unmarshal to struct: %w", err),
			})
			return true
		}

		result = append(result, &DebugChange{
			ChangeId: change.Id,
			OrderId:  orderId,
			Change:   changeStruct,
		})
		return true
	})
	if err != nil {
		return nil, fmt.Errorf("iterate root: %w", err)
	}

	return result, nil
}
