package userdataobject

import (
	"context"
	"fmt"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var log = logging.Logger("core.block.editor.userdata")

type UserDataObject interface {
	smartblock.SmartBlock

	SaveContact(ctx context.Context, profile *model.IdentityProfile) error
	DeleteContact(ctx context.Context, identity string) error
	UpdateContactByDetails(ctx context.Context, id string, details *types.Struct) error
	UpdateContactByIdentity(ctx context.Context, profile *model.IdentityProfile) (err error)
}

type userDataObject struct {
	smartblock.SmartBlock
	state       *storestate.StoreState
	storeSource source.Store
	crdtDb      anystore.DB
	arenaPool   *anyenc.ArenaPool

	objectCache cache.ObjectGetter
	ctx         context.Context
	cancel      context.CancelFunc
}

func New(sb smartblock.SmartBlock, crdtDb anystore.DB, objectCache cache.ObjectGetter) UserDataObject {
	return &userDataObject{
		SmartBlock:  sb,
		crdtDb:      crdtDb,
		arenaPool:   &anyenc.ArenaPool{},
		objectCache: objectCache,
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
	coll, err := u.state.Collection(ctx, ContactsCollection)
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

func (u *userDataObject) SaveContact(ctx context.Context, profile *model.IdentityProfile) error {
	arena := u.arenaPool.Get()
	defer func() {
		arena.Reset()
		u.arenaPool.Put(arena)
	}()
	err := u.saveContactInStore(ctx, profile, arena)
	if err != nil {
		return err
	}
	return nil
}

func (u *userDataObject) saveContactInStore(ctx context.Context, profile *model.IdentityProfile, arena *anyenc.Arena) error {
	contact := NewContact(profile.Identity, profile.Name, profile.Description, profile.IconCid)

	builder := storestate.Builder{}
	err := builder.Create(ContactsCollection, domain.NewContactId(profile.Identity), contact.ToJson(arena))
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

func (u *userDataObject) UpdateContactByIdentity(ctx context.Context, profile *model.IdentityProfile) error {
	arena := u.arenaPool.Get()
	defer func() {
		arena.Reset()
		u.arenaPool.Put(arena)
	}()

	builder := storestate.Builder{}

	err := u.updateContactFields(profile, &builder)
	if err != nil {
		return err
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

func (u *userDataObject) updateContactFields(profile *model.IdentityProfile, builder *storestate.Builder) error {
	contactId := domain.NewContactId(profile.Identity)

	err := builder.Modify(ContactsCollection, contactId, []string{nameField}, pb.ModifyOp_Set, fmt.Sprintf(`"%s"`, profile.Name))
	if err != nil {
		return fmt.Errorf("modify contact: %w", err)
	}
	err = builder.Modify(ContactsCollection, contactId, []string{iconField}, pb.ModifyOp_Set, fmt.Sprintf(`"%s"`, profile.IconCid))
	if err != nil {
		return fmt.Errorf("modify contact: %w", err)
	}
	err = builder.Modify(ContactsCollection, contactId, []string{descriptionField}, pb.ModifyOp_Set, fmt.Sprintf(`"%s"`, profile.Description))
	if err != nil {
		return fmt.Errorf("modify contact: %w", err)
	}
	return nil
}

func (u *userDataObject) DeleteContact(ctx context.Context, identity string) error {
	builder := storestate.Builder{}
	builder.Delete(ContactsCollection, identity)
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

func (u *userDataObject) UpdateContactByDetails(ctx context.Context, contactId string, details *types.Struct) error {
	arena := u.arenaPool.Get()
	defer func() {
		arena.Reset()
		u.arenaPool.Put(arena)
	}()
	builder := storestate.Builder{}

	err := u.updateContactFieldsFromDetails(details, &builder, contactId)
	if err != nil {
		return err
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

func (u *userDataObject) updateContactFieldsFromDetails(details *types.Struct, builder *storestate.Builder, contactId string) error {
	if nameVal := pbtypes.Get(details, bundle.RelationKeyName.String()); nameVal != nil {
		err := builder.Modify(ContactsCollection, contactId, []string{nameField}, pb.ModifyOp_Set, fmt.Sprintf(`"%s"`, nameVal.GetStringValue()))
		if err != nil {
			return fmt.Errorf("modify contact: %w", err)
		}
	}
	if descriptionVal := pbtypes.Get(details, bundle.RelationKeyDescription.String()); descriptionVal != nil {
		err := builder.Modify(ContactsCollection, contactId, []string{descriptionField}, pb.ModifyOp_Set, fmt.Sprintf(`"%s"`, descriptionVal.GetStringValue()))
		if err != nil {
			return fmt.Errorf("modify contact: %w", err)
		}
	}
	if iconVal := pbtypes.Get(details, bundle.RelationKeyIconImage.String()); iconVal != nil {
		err := builder.Modify(ContactsCollection, contactId, []string{iconField}, pb.ModifyOp_Set, fmt.Sprintf(`"%s"`, iconVal.GetStringValue()))
		if err != nil {
			return fmt.Errorf("modify contact: %w", err)
		}
	}
	return nil
}
