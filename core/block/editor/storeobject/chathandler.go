package storeobject

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-store/query"
	"github.com/valyala/fastjson"

	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/pb"
)

type ChatHandler struct {
	MyIdentity string
}

func (d ChatHandler) CollectionName() string {
	return collectionName
}

func (d ChatHandler) Init(ctx context.Context, s *storestate.StoreState) (err error) {
	_, err = s.Collection(ctx, collectionName)
	return
}

func (d ChatHandler) BeforeCreate(ctx context.Context, ch storestate.ChangeOp) (err error) {
	return
}

func (d ChatHandler) BeforeModify(ctx context.Context, ch storestate.ChangeOp) (mode storestate.ModifyMode, err error) {
	return storestate.ModifyModeUpsert, nil
}

func (d ChatHandler) BeforeDelete(ctx context.Context, ch storestate.ChangeOp) (mode storestate.DeleteMode, err error) {
	return storestate.DeleteModeDelete, nil
}

func (d ChatHandler) UpgradeKeyModifier(ch storestate.ChangeOp, key *pb.KeyModify, mod query.Modifier) query.Modifier {
	return query.ModifyFunc(func(a *fastjson.Arena, v *fastjson.Value) (result *fastjson.Value, modified bool, err error) {
		author := v.GetStringBytes("author")
		if string(author) != d.MyIdentity {
			return v, false, errors.Join(storestate.ErrValidation, fmt.Errorf("can't modify not own message"))
		}
		return mod.Modify(a, v)
	})
}
