package userdataobject

import (
	"context"
	"path/filepath"
	"testing"

	anystore "github.com/anyproto/any-store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/block/source/mock_source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/tests/storechanges"
)

func TestUserDataObject_Init(t *testing.T) {
	t.Run("success init", func(t *testing.T) {
		// given
		fx := newFixture(t, make(chan struct{}))

		// when
		fx.source.EXPECT().ReadStoreDoc(context.Background(), mock.Anything, mock.Anything).Return(nil)
		err := fx.Init(&smartblock.InitContext{
			Ctx:    context.Background(),
			Source: fx.source,
		})

		// then
		require.NoError(t, err)
		require.NoError(t, fx.Close())
	})
}

func TestUserDataObject_SaveContact(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// given
		initChan := make(chan struct{})
		fx := newFixture(t, initChan)
		fx.source.EXPECT().ReadStoreDoc(context.Background(), mock.Anything, mock.Anything).Return(nil)
		fx.source.EXPECT().PushStoreChange(context.Background(), mock.Anything).RunAndReturn(fx.pushStoreChanges)
		err := fx.Init(&smartblock.InitContext{
			Ctx:    context.Background(),
			Source: fx.source,
		})
		require.NoError(t, err)
		<-initChan

		identity := "identity"
		contactId := domain.NewContactId(identity)
		test := smarttest.New(contactId)
		fx.objectGetter.EXPECT().GetObjectByFullID(mock.Anything, domain.FullID{ObjectID: contactId}).Return(test, nil)
		name := "name"
		iconCid := "cid"
		description := "description"

		// when
		err = fx.SaveContact(context.Background(), &model.IdentityProfile{
			Identity:    identity,
			Name:        name,
			IconCid:     iconCid,
			Description: description,
		})

		// then
		require.NoError(t, err)
		details := test.CombinedDetails()
		assert.Equal(t, name, details.GetString(bundle.RelationKeyName))
		assert.Equal(t, description, details.GetString(bundle.RelationKeyDescription))
		assert.Equal(t, identity, details.GetString(bundle.RelationKeyIdentity))
		assert.Equal(t, iconCid, details.GetString(bundle.RelationKeyIconImage))
	})
	t.Run("contact exists", func(t *testing.T) {
		// given
		initChan := make(chan struct{})
		fx := newFixture(t, initChan)
		fx.source.EXPECT().ReadStoreDoc(context.Background(), mock.Anything, mock.Anything).Return(nil)
		fx.source.EXPECT().PushStoreChange(context.Background(), mock.Anything).RunAndReturn(fx.pushStoreChanges).Times(2)
		err := fx.Init(&smartblock.InitContext{
			Ctx:    context.Background(),
			Source: fx.source,
		})
		require.NoError(t, err)
		<-initChan
		identity := "identity"
		contactId := domain.NewContactId(identity)
		test := smarttest.New(contactId)
		fx.objectGetter.EXPECT().GetObjectByFullID(mock.Anything, domain.FullID{ObjectID: contactId}).Return(test, nil)
		name := "name"
		iconCid := "cid"
		description := "description"

		// when
		err = fx.SaveContact(context.Background(), &model.IdentityProfile{
			Identity:    identity,
			Name:        name,
			IconCid:     iconCid,
			Description: description,
		})
		err = fx.SaveContact(context.Background(), &model.IdentityProfile{
			Identity:    identity,
			Name:        name,
			IconCid:     iconCid,
			Description: description,
		})

		// then
		require.NoError(t, err)
		contacts, err := fx.ListContacts(context.Background())
		require.NoError(t, err)
		assert.Len(t, contacts, 1)
	})
}

func TestUserDataObject_DeleteContact(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// given
		initChan := make(chan struct{})
		fx := newFixture(t, initChan)
		fx.source.EXPECT().ReadStoreDoc(context.Background(), mock.Anything, mock.Anything).Return(nil)
		fx.source.EXPECT().PushStoreChange(context.Background(), mock.Anything).RunAndReturn(fx.pushStoreChanges)
		err := fx.Init(&smartblock.InitContext{
			Ctx:    context.Background(),
			Source: fx.source,
		})
		require.NoError(t, err)
		<-initChan
		identity := "identity"
		contactId := domain.NewContactId(identity)
		test := smarttest.New(contactId)
		fx.objectGetter.EXPECT().GetObjectByFullID(mock.Anything, domain.FullID{ObjectID: contactId}).Return(test, nil)
		name := "name"
		iconCid := "cid"
		description := "description"
		err = fx.SaveContact(context.Background(), &model.IdentityProfile{
			Identity:    identity,
			Name:        name,
			IconCid:     iconCid,
			Description: description,
		})
		require.NoError(t, err)

		// when
		err = fx.DeleteContact(context.Background(), contactId)

		// then
		require.NoError(t, err)
		contacts, err := fx.ListContacts(context.Background())
		require.NoError(t, err)
		assert.Len(t, contacts, 0)
	})
	t.Run("contact not exists", func(t *testing.T) {
		// given
		initChan := make(chan struct{})
		fx := newFixture(t, initChan)
		fx.source.EXPECT().ReadStoreDoc(context.Background(), mock.Anything, mock.Anything).Return(nil)
		fx.source.EXPECT().PushStoreChange(context.Background(), mock.Anything).RunAndReturn(fx.pushStoreChanges)

		err := fx.Init(&smartblock.InitContext{
			Ctx:    context.Background(),
			Source: fx.source,
		})
		require.NoError(t, err)
		<-initChan
		contactId := domain.NewContactId("identity")

		// when
		err = fx.DeleteContact(context.Background(), contactId)

		// then
		require.Error(t, err)
		assert.ErrorIs(t, err, anystore.ErrDocNotFound)
	})
}

func TestUserDataObject_UpdateContactByIdentity(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// given
		initChan := make(chan struct{})
		fx := newFixture(t, initChan)
		fx.source.EXPECT().ReadStoreDoc(context.Background(), mock.Anything, mock.Anything).Return(nil)
		fx.source.EXPECT().PushStoreChange(context.Background(), mock.Anything).RunAndReturn(fx.pushStoreChanges).Times(2)
		identity := "identity"
		contactId := domain.NewContactId(identity)
		test := smarttest.New(contactId)
		fx.objectGetter.EXPECT().GetObjectByFullID(mock.Anything, domain.FullID{ObjectID: contactId}).Return(test, nil).Times(2)
		err := fx.Init(&smartblock.InitContext{
			Ctx:    context.Background(),
			Source: fx.source,
		})
		require.NoError(t, err)
		<-initChan
		name := "name"
		iconCid := "cid"
		description := "description"
		err = fx.SaveContact(context.Background(), &model.IdentityProfile{
			Identity:    identity,
			Name:        name,
			IconCid:     iconCid,
			Description: description,
		})
		require.NoError(t, err)

		// when
		newName := "newName"
		newCid := "newCid"
		err = fx.UpdateContactByIdentity(context.Background(), &model.IdentityProfile{
			Identity: identity,
			Name:     newName,
			IconCid:  newCid,
		})

		// then
		require.NoError(t, err)
		details := test.CombinedDetails()
		assert.Equal(t, newName, details.GetString(bundle.RelationKeyName))
		assert.Equal(t, newCid, details.GetString(bundle.RelationKeyIconImage))
	})
	t.Run("contact not exists", func(t *testing.T) {
		// given
		initChan := make(chan struct{})
		fx := newFixture(t, initChan)
		fx.source.EXPECT().ReadStoreDoc(context.Background(), mock.Anything, mock.Anything).Return(nil)
		fx.source.EXPECT().PushStoreChange(context.Background(), mock.Anything).RunAndReturn(fx.pushStoreChanges).Times(1)
		err := fx.Init(&smartblock.InitContext{
			Ctx:    context.Background(),
			Source: fx.source,
		})
		require.NoError(t, err)
		<-initChan
		identity := "identity"
		newName := "newName"
		newCid := "newCid"

		// when
		err = fx.UpdateContactByIdentity(context.Background(), &model.IdentityProfile{
			Identity: identity,
			Name:     newName,
			IconCid:  newCid,
		})

		// then
		require.NoError(t, err)
		contacts, err := fx.ListContacts(context.Background())
		require.NoError(t, err)
		assert.Len(t, contacts, 0)
	})
}

func TestUserDataObject_UpdateContactByDetails(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// given
		initChan := make(chan struct{})
		fx := newFixture(t, initChan)
		fx.source.EXPECT().ReadStoreDoc(context.Background(), mock.Anything, mock.Anything).Return(nil)
		fx.source.EXPECT().PushStoreChange(context.Background(), mock.Anything).RunAndReturn(fx.pushStoreChanges).Times(2)
		err := fx.Init(&smartblock.InitContext{
			Ctx:    context.Background(),
			Source: fx.source,
		})
		require.NoError(t, err)
		<-initChan
		identity := "identity"
		contactId := domain.NewContactId(identity)
		test := smarttest.New(contactId)
		fx.objectGetter.EXPECT().GetObjectByFullID(mock.Anything, domain.FullID{ObjectID: contactId}).Return(test, nil).Times(1)
		name := "name"
		iconCid := "cid"
		description := "description"
		err = fx.SaveContact(context.Background(), &model.IdentityProfile{
			Identity:    identity,
			Name:        name,
			IconCid:     iconCid,
			Description: description,
		})
		require.NoError(t, err)

		// when
		newName := "newName"
		newCid := "newCid"
		err = fx.UpdateContactByDetails(context.Background(), contactId, domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName:      domain.String(newName),
			bundle.RelationKeyIconImage: domain.String(newCid),
		}))

		// then
		require.NoError(t, err)
		contacts, err := fx.ListContacts(context.Background())
		assert.Len(t, contacts, 1)
		assert.Equal(t, newName, contacts[0].name)
		assert.Equal(t, newCid, contacts[0].icon)
		assert.Equal(t, description, contacts[0].description)
	})
	t.Run("update description", func(t *testing.T) {
		// given
		initChan := make(chan struct{})
		fx := newFixture(t, initChan)
		fx.source.EXPECT().ReadStoreDoc(context.Background(), mock.Anything, mock.Anything).Return(nil)
		fx.source.EXPECT().PushStoreChange(context.Background(), mock.Anything).RunAndReturn(fx.pushStoreChanges).Times(2)
		err := fx.Init(&smartblock.InitContext{
			Ctx:    context.Background(),
			Source: fx.source,
		})
		require.NoError(t, err)
		<-initChan
		identity := "identity"
		contactId := domain.NewContactId(identity)
		test := smarttest.New(contactId)
		fx.objectGetter.EXPECT().GetObjectByFullID(mock.Anything, domain.FullID{ObjectID: contactId}).Return(test, nil).Times(1)
		name := "name"
		iconCid := "cid"
		description := "description"
		err = fx.SaveContact(context.Background(), &model.IdentityProfile{
			Identity:    identity,
			Name:        name,
			IconCid:     iconCid,
			Description: description,
		})
		require.NoError(t, err)

		// when
		newDescription := "newDescription"
		err = fx.UpdateContactByDetails(context.Background(), contactId, domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyDescription: domain.String(newDescription),
		}))

		// then
		require.NoError(t, err)
		contacts, err := fx.ListContacts(context.Background())
		assert.Len(t, contacts, 1)
		assert.Equal(t, name, contacts[0].name)
		assert.Equal(t, iconCid, contacts[0].icon)
		assert.Equal(t, newDescription, contacts[0].description)
	})
	t.Run("contact not exists", func(t *testing.T) {
		// given
		initChan := make(chan struct{})
		fx := newFixture(t, initChan)
		fx.source.EXPECT().ReadStoreDoc(context.Background(), mock.Anything, mock.Anything).Return(nil)
		fx.source.EXPECT().PushStoreChange(context.Background(), mock.Anything).RunAndReturn(fx.pushStoreChanges).Times(1)
		err := fx.Init(&smartblock.InitContext{
			Ctx:    context.Background(),
			Source: fx.source,
		})
		require.NoError(t, err)
		<-initChan
		identity := "identity"
		newName := "newName"

		// when
		err = fx.UpdateContactByDetails(context.Background(), domain.NewContactId(identity), domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName: domain.String(newName),
		}))

		// then
		require.NoError(t, err)
		contacts, err := fx.ListContacts(context.Background())
		require.NoError(t, err)
		assert.Len(t, contacts, 0)
	})
}

type fixture struct {
	*userDataObject
	source       *mock_source.MockStore
	db           anystore.DB
	objectGetter *mock_cache.MockObjectGetter
}

func newFixture(t *testing.T, initChan chan struct{}) *fixture {
	ctx := context.Background()
	db, err := anystore.Open(ctx, filepath.Join(t.TempDir(), "crdt.db"), nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		err := db.Close()
		require.NoError(t, err)
	})
	sb := smarttest.New("userDataObjectId")
	require.NoError(t, err)
	objectGetter := mock_cache.NewMockObjectGetter(t)
	source := mock_source.NewMockStore(t)
	object := New(sb, db, objectGetter)
	dataObject := object.(*userDataObject)
	dataObject.onUpdateCallback = func() {
		dataObject.onUpdate()
		close(initChan)
	}
	fx := &fixture{
		db:             db,
		userDataObject: dataObject,
		source:         source,
		objectGetter:   objectGetter,
	}
	fx.source = source
	return fx
}

func (fx *fixture) pushStoreChanges(ctx context.Context, params source.PushStoreChangeParams) (string, error) {
	changeId, err := storechanges.PushStoreChanges(ctx, params)
	if err != nil {
		return "", err
	}
	if !params.NoOnUpdateHook {
		fx.onUpdate()
	}
	return changeId, nil
}
