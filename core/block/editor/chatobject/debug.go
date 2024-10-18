package chatobject

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/objecttreebuilder"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/source"
)

type DebugChange struct {
	ChangeId string
	OrderId  string
	Change   *types.Struct
	Error    error
}

func (s *storeObject) DebugChanges(ctx context.Context) ([]*DebugChange, error) {
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
	defer tx.Commit()

	var result []*DebugChange
	err = historyTree.IterateFrom(historyTree.Root().Id, source.UnmarshalStoreChange, func(change *objecttree.Change) bool {
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
