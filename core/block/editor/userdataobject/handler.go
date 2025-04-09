package userdataobject

import (
	"context"
	"fmt"

	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"

	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/pb"
)

const ContactsCollection = "contacts"

type contactsHandler struct {
}

func (h contactsHandler) CollectionName() string {
	return ContactsCollection
}

func (h contactsHandler) Init(ctx context.Context, s *storestate.StoreState) (err error) {
	_, err = s.Collection(ctx, ContactsCollection)
	return
}

func (h contactsHandler) BeforeCreate(ctx context.Context, ch storestate.ChangeOp) (err error) {
	return
}

func (h contactsHandler) BeforeModify(_ context.Context, _ storestate.ChangeOp) (mode storestate.ModifyMode, err error) {
	return storestate.ModifyModeUpdate, nil
}

func (h contactsHandler) BeforeDelete(ctx context.Context, ch storestate.ChangeOp) (mode storestate.DeleteMode, err error) {
	_, err = ch.State.Collection(ctx, ContactsCollection)
	if err != nil {
		return storestate.DeleteModeDelete, fmt.Errorf("get collection: %w", err)
	}
	return storestate.DeleteModeDelete, nil
}

func (h contactsHandler) UpgradeKeyModifier(_ storestate.ChangeOp, _ *pb.KeyModify, mod query.Modifier) query.Modifier {
	return query.ModifyFunc(func(a *anyenc.Arena, v *anyenc.Value) (result *anyenc.Value, modified bool, err error) {
		return mod.Modify(a, v)
	})
}
