package userdataobject

import (
	"context"
	"fmt"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/any-sync/util/crypto/cryptoproto"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/identity"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var log = logging.Logger("core.block.editor.userdata")

type UserDataObject interface {
	smartblock.SmartBlock

	SaveContact(ctx context.Context, identity string, profileSymKey []byte) error
	DeleteContact(ctx context.Context, identity string) error
	UpdateContact(ctx context.Context, details *types.Struct) error
}

type userDataObject struct {
	smartblock.SmartBlock
	basic.DetailsSettable
	state       *storestate.StoreState
	storeSource source.Store
	crdtDb      anystore.DB
	arenaPool   *anyenc.ArenaPool

	identityService identity.Service
	objectCache     cache.ObjectGetter
	ctx             context.Context
	cancel          context.CancelFunc
}

func New(sb smartblock.SmartBlock, identityService identity.Service, crdtDb anystore.DB, objectCache cache.ObjectGetter) UserDataObject {
	return &userDataObject{
		SmartBlock:      sb,
		identityService: identityService,
		crdtDb:          crdtDb,
		arenaPool:       &anyenc.ArenaPool{},
		objectCache:     objectCache,
	}
}

func (u *userDataObject) Init(ctx *smartblock.InitContext) error {
	u.ctx, u.cancel = context.WithCancel(context.Background())
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
	err = storeSource.ReadStoreDoc(ctx.Ctx, stateStore, u.onUpdate)
	if err != nil {
		return fmt.Errorf("read store doc: %w", err)
	}
	u.onUpdate()
	return nil
}

func (u *userDataObject) onUpdate() {
	contacts, err := u.listContacts(u.ctx)
	if err != nil {
		log.Errorf("list contacts: %v", err)
		return
	}
	for _, contact := range contacts {
		err = cache.Do(u.objectCache, domain.NewContactId(contact.identity), func(contactObject smartblock.SmartBlock) error {
			state := contactObject.NewState()
			state.SetDetails(contact.Details())
			return contactObject.Apply(state)
		})
		if err != nil {
			log.Errorf("update contact: %v", err)
		}
	}
}

func (u *userDataObject) listContacts(ctx context.Context) ([]*Contact, error) {
	coll, err := u.state.Collection(ctx, contactsCollection)
	if err != nil {
		return nil, fmt.Errorf("get collection: %w", err)
	}
	qry := coll.Find(nil)
	contacts, err := u.queryContacts(ctx, qry)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	return contacts, nil
}

func (u *userDataObject) queryContacts(ctx context.Context, query anystore.Query) ([]*Contact, error) {
	iter, err := query.Iter(ctx)
	if err != nil {
		return nil, fmt.Errorf("find iter: %w", err)
	}
	defer iter.Close()

	var res []*Contact
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, fmt.Errorf("get doc: %w", err)
		}
		message := NewContactFromJson(doc.Value())
		res = append(res, message)
	}
	return res, nil
}

func (u *userDataObject) Close() error {
	u.cancel()
	return u.SmartBlock.Close()
}

func (u *userDataObject) SaveContact(ctx context.Context, identity string, profileSymKey []byte) error {
	err := u.registerIdentity(identity, profileSymKey)
	if err != nil {
		return err
	}
	profile := u.identityService.WaitProfile(ctx, identity)
	if profile == nil {
		return fmt.Errorf("no profile for identity %s", identity)
	}
	arena := u.arenaPool.Get()
	defer func() {
		arena.Reset()
		u.arenaPool.Put(arena)
	}()
	err = u.saveContactInStore(ctx, identity, profile, arena)
	if err != nil {
		return err
	}
	return nil
}

func (u *userDataObject) registerIdentity(identity string, profileSymKey []byte) error {
	handleIdentityUpdate := func(identity string, identityProfile *model.IdentityProfile) {
		err := u.update(u.ctx, identityProfile)
		if err != nil {
			log.Errorf("update contact for identity %s: %v", identity, err)
		}
	}
	if len(profileSymKey) == 0 {
		u.identityService.AddObserver(u.SpaceID(), identity, handleIdentityUpdate)
	} else {
		key, err := getAesKey(profileSymKey)
		if err != nil {
			return fmt.Errorf("get aes key for identity %s: %w", identity, err)
		}
		err = u.identityService.RegisterIdentity(u.SpaceID(), identity, key, handleIdentityUpdate)
		if err != nil {
			return fmt.Errorf("register identity %s: %v", identity, err)
		}
	}
	return nil
}

func (u *userDataObject) update(ctx context.Context, profile *model.IdentityProfile) (err error) {
	arena := u.arenaPool.Get()
	defer func() {
		arena.Reset()
		u.arenaPool.Put(arena)
	}()

	builder := storestate.Builder{}
	contactId := domain.NewContactId(profile.Identity)
	err = builder.Modify(contactsCollection, contactId, []string{nameField}, pb.ModifyOp_Set, profile.Name)
	err = builder.Modify(contactsCollection, contactId, []string{iconField}, pb.ModifyOp_Set, profile.IconCid)
	if err != nil {
		return fmt.Errorf("modify contact: %w", err)
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

func getAesKey(profileSymKey []byte) (*crypto.AESKey, error) {
	keyProto := &cryptoproto.Key{}
	err := keyProto.Unmarshal(profileSymKey)
	if err != nil {
		return nil, err
	}
	key, err := crypto.UnmarshallAESKey(keyProto.Data)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func (u *userDataObject) saveContactInStore(ctx context.Context, identity string, profile *model.IdentityProfile, arena *anyenc.Arena) error {
	contact := NewContact(identity, profile.Identity, profile.Description, profile.IconCid)

	builder := storestate.Builder{}
	err := builder.Create(contactsCollection, domain.NewContactId(identity), contact.ToJson(arena))
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
	u.identityService.UnregisterIdentity(u.SpaceID(), identity)
	return nil
}

func (u *userDataObject) UpdateContact(ctx context.Context, details *types.Struct) error {
	arena := u.arenaPool.Get()
	defer func() {
		arena.Reset()
		u.arenaPool.Put(arena)
	}()
	jsonContact := ModelToJson(arena, details)
	builder := storestate.Builder{}
	contactId := domain.NewContactId(pbtypes.GetString(details, bundle.RelationKeyIdentity.String()))
	err := builder.Modify(contactsCollection, contactId, []string{identityField, nameField, iconField, descriptionField}, pb.ModifyOp_Set, jsonContact)
	if err != nil {
		return fmt.Errorf("modify contact: %w", err)
	}
	_, err = u.storeSource.PushStoreChange(ctx, source.PushStoreChangeParams{
		Changes:        builder.ChangeSet,
		State:          u.state,
		Time:           time.Now(),
		NoOnUpdateHook: true,
	})
	if err != nil {
		return fmt.Errorf("push change: %w", err)
	}
	return nil
}
