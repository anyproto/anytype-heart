package accountobject

import (
	"context"
	"fmt"

	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"

	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/pb"
)

type accountHandler struct {
}

func (a accountHandler) CollectionName() string {
	return collectionName
}

func (a accountHandler) Init(ctx context.Context, s *storestate.StoreState) (err error) {
	_, err = s.Collection(ctx, collectionName)
	return
}

func (a accountHandler) BeforeCreate(ctx context.Context, ch storestate.ChangeOp) (err error) {
	return
}

func (a accountHandler) BeforeModify(ctx context.Context, ch storestate.ChangeOp) (mode storestate.ModifyMode, err error) {
	return storestate.ModifyModeUpsert, nil
}

func (a accountHandler) BeforeDelete(ctx context.Context, ch storestate.ChangeOp) (mode storestate.DeleteMode, err error) {
	_, err = ch.State.Collection(ctx, collectionName)
	if err != nil {
		return storestate.DeleteModeDelete, fmt.Errorf("get collection: %w", err)
	}
	return storestate.DeleteModeDelete, nil
}

func (a accountHandler) UpgradeKeyModifier(ch storestate.ChangeOp, key *pb.KeyModify, mod query.Modifier) query.Modifier {
	return query.ModifyFunc(func(a *anyenc.Arena, v *anyenc.Value) (result *anyenc.Value, modified bool, err error) {
		return mod.Modify(a, v)
	})
}
