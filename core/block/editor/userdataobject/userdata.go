package userdataobject

import (
	"context"
	"fmt"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
)

type UserDataObject interface {
	smartblock.SmartBlock

	SaveContact(ctx context.Context, profile *Contact) error
	DeleteContact(ctx context.Context, identity string) error
	UpdateContactByDetails(ctx context.Context, id string, details *domain.Details) error
	ListContacts(ctx context.Context) ([]*Contact, error)
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
	u := &userDataObject{
		SmartBlock:  sb,
		crdtDb:      crdtDb,
		arenaPool:   &anyenc.ArenaPool{},
		objectCache: objectCache,
	}
	return u
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
		return fmt.Errorf("source is not store")
	}
	u.storeSource = storeSource
	err = storeSource.ReadStoreDoc(ctx.Ctx, stateStore, nil)
	if err != nil {
		return fmt.Errorf("read store doc: %w", err)
	}
	return nil
}

func (u *userDataObject) Close() error {
	u.cancel()
	return u.SmartBlock.Close()
}

func (u *userDataObject) SaveContact(ctx context.Context, contact *Contact) error {
	arena := u.arenaPool.Get()
	defer func() {
		arena.Reset()
		u.arenaPool.Put(arena)
	}()
	builder := &storestate.Builder{}
	err := builder.Create(ContactsCollection, domain.NewContactId(contact.Identity()), contact.ToJson(arena))
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
	builder := &storestate.Builder{}
	contactId := domain.NewContactId(identity)
	builder.Delete(ContactsCollection, contactId)
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

func (u *userDataObject) UpdateContactByDetails(ctx context.Context, contactId string, details *domain.Details) error {
	arena := u.arenaPool.Get()
	defer func() {
		arena.Reset()
		u.arenaPool.Put(arena)
	}()

	builder := &storestate.Builder{}
	err := u.updateContactFieldsFromDetails(contactId, details, builder)
	if err != nil {
		return err
	}
	if builder.StoreChange == nil {
		return nil
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

func (u *userDataObject) updateContactFieldsFromDetails(contactId string, details *domain.Details, builder *storestate.Builder) error {
	for key, value := range details.Iterate() {
		if !slices.Contains(AllowedDetailsToChange(), key) {
			continue
		}
		err := builder.Modify(ContactsCollection, contactId, []string{key.String()}, pb.ModifyOp_Set, fmt.Sprintf(`"%s"`, value.String()))
		if err != nil {
			return fmt.Errorf("modify contact: %w", err)
		}
	}
	return nil
}

func (u *userDataObject) ListContacts(ctx context.Context) ([]*Contact, error) {
	collection, err := u.state.Collection(ctx, ContactsCollection)
	if err != nil {
		return nil, fmt.Errorf("get collection: %w", err)
	}
	qry := collection.Find(nil)
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
