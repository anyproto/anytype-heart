package userdataobject

import (
	"context"
	"fmt"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/identity"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("core.block.editor.userdata")

type UserDataObject interface {
	smartblock.SmartBlock

	SaveContact(ctx context.Context, identity string, profileSymKey []byte) error
	DeleteContact(ctx context.Context, identity string) error
}

type userDataObject struct {
	smartblock.SmartBlock
	basic.DetailsSettable
	state       *storestate.StoreState
	storeSource source.Store
	crdtDb      anystore.DB
	arenaPool   *anyenc.ArenaPool

	identityService identity.Service
}

func New(sb smartblock.SmartBlock, identityService identity.Service, crdtDb anystore.DB) UserDataObject {
	return &userDataObject{
		SmartBlock:      sb,
		identityService: identityService,
		crdtDb:          crdtDb,
		arenaPool:       &anyenc.ArenaPool{},
	}
}

func (u *userDataObject) Init(ctx *smartblock.InitContext) error {
	err := u.SmartBlock.Init(ctx)
	if err != nil {
		return err
	}
	stateStore, err := storestate.New(ctx.Ctx, u.Id(), u.crdtDb, contactsHandler{})
	if err != nil {
		return fmt.Errorf("create state store: %w", err)
	}
	u.state = stateStore

	storeSource, ok := ctx.Source.(source.Store)
	if !ok {
		return fmt.Errorf("source is not u store")
	}
	u.storeSource = storeSource
	err = storeSource.ReadStoreDoc(ctx.Ctx, stateStore, nil)
	if err != nil {
		return fmt.Errorf("read store doc: %w", err)
	}
	return nil
}

func (u *userDataObject) SaveContact(ctx context.Context, identity string, profileSymKey []byte) error {
	if len(profileSymKey) == 0 {

	}
	arena := u.arenaPool.Get()
	defer func() {
		arena.Reset()
		u.arenaPool.Put(arena)
	}()
	contact := NewJsonContact(identity, "", "", "")

	builder := storestate.Builder{}
	err := builder.Create(contactsCollection, identity, contact.ToJson(arena))
	if err != nil {
		return fmt.Errorf("create chat: %w", err)
	}
	_, err = u.storeSource.PushStoreChange(ctx, source.PushStoreChangeParams{
		Changes: builder.ChangeSet,
		State:   u.state,
		Time:    time.Now(),
	})
	if err != nil {
		return fmt.Errorf("push change: %w", err)
	}
	return nil
}

func (u *userDataObject) DeleteContact(ctx context.Context, identity string) error {
	builder := storestate.Builder{}
	builder.Delete(contactsCollection, identity)
	_, err := u.storeSource.PushStoreChange(ctx, source.PushStoreChangeParams{
		Changes: builder.ChangeSet,
		State:   u.state,
		Time:    time.Now(),
	})
	if err != nil {
		return fmt.Errorf("push change: %w", err)
	}
	return nil
}

func (u *userDataObject) updateDescription(ctx context.Context, identity, description string) error {
	builder := storestate.Builder{}
	err := builder.Modify(contactsCollection, identity, []string{"description"}, pb.ModifyOp_Set, description)
	if err != nil {
		return fmt.Errorf("modify content: %w", err)
	}
	_, err = u.storeSource.PushStoreChange(ctx, source.PushStoreChangeParams{
		Changes: builder.ChangeSet,
		State:   u.state,
		Time:    time.Now(),
	})
	if err != nil {
		return fmt.Errorf("push change: %w", err)
	}
	return nil
}
