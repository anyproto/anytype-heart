package userdataobject

import (
	"context"
	"encoding/base64"
	"path/filepath"
	"testing"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/source/mock_source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/tests/storechanges"
)

func TestUserDataObject_Init(t *testing.T) {
	t.Run("success init", func(t *testing.T) {
		// given
		fx := newFixture(t)

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
		fx := newFixture(t)
		fx.source.EXPECT().ReadStoreDoc(context.Background(), mock.Anything, mock.Anything).Return(nil)
		fx.source.EXPECT().PushStoreChange(context.Background(), mock.Anything).RunAndReturn(storechanges.PushStoreChanges)
		err := fx.Init(&smartblock.InitContext{
			Ctx:    context.Background(),
			Source: fx.source,
		})
		require.NoError(t, err)

		identity := "identity"
		aesKey := crypto.NewAES()
		rawKey, err := aesKey.Raw()
		require.NoError(t, err)
		encodedKey := base64.StdEncoding.EncodeToString(rawKey)
		contact := NewContact(identity, encodedKey)

		// when
		err = fx.SaveContact(context.Background(), contact)

		// then
		require.NoError(t, err)
		contacts, err := fx.ListContacts(context.Background())
		require.NoError(t, err)
		assert.Len(t, contacts, 1)
		assert.Equal(t, identity, contacts[0].Identity())
		assert.Equal(t, encodedKey, contacts[0].Key())
		assert.Equal(t, "", contacts[0].Description())
	})
	t.Run("contact exists", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.source.EXPECT().ReadStoreDoc(context.Background(), mock.Anything, mock.Anything).Return(nil)
		fx.source.EXPECT().PushStoreChange(context.Background(), mock.Anything).RunAndReturn(storechanges.PushStoreChanges).Times(2)
		err := fx.Init(&smartblock.InitContext{
			Ctx:    context.Background(),
			Source: fx.source,
		})
		require.NoError(t, err)
		identity := "identity"

		// when
		err = fx.SaveContact(context.Background(), NewContact(identity, ""))
		require.NoError(t, err)
		err = fx.SaveContact(context.Background(), NewContact(identity, "key"))

		// then
		require.NoError(t, err)
		contacts, err := fx.ListContacts(context.Background())
		require.NoError(t, err)
		assert.Len(t, contacts, 1)
		assert.Equal(t, identity, contacts[0].Identity())
		assert.Equal(t, "", contacts[0].Key())
	})
}

func TestUserDataObject_DeleteContact(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.source.EXPECT().ReadStoreDoc(context.Background(), mock.Anything, mock.Anything).Return(nil)
		fx.source.EXPECT().PushStoreChange(context.Background(), mock.Anything).RunAndReturn(storechanges.PushStoreChanges)
		err := fx.Init(&smartblock.InitContext{
			Ctx:    context.Background(),
			Source: fx.source,
		})
		require.NoError(t, err)
		identity := "identity"

		err = fx.SaveContact(context.Background(), NewContact(identity, ""))
		require.NoError(t, err)

		// when
		err = fx.DeleteContact(context.Background(), identity)

		// then
		require.NoError(t, err)
		contacts, err := fx.ListContacts(context.Background())
		require.NoError(t, err)
		assert.Len(t, contacts, 0)
	})
	t.Run("contact not exists", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.source.EXPECT().ReadStoreDoc(context.Background(), mock.Anything, mock.Anything).Return(nil)
		fx.source.EXPECT().PushStoreChange(context.Background(), mock.Anything).RunAndReturn(storechanges.PushStoreChanges)

		err := fx.Init(&smartblock.InitContext{
			Ctx:    context.Background(),
			Source: fx.source,
		})
		require.NoError(t, err)

		// when
		err = fx.DeleteContact(context.Background(), "identity")

		// then
		require.Error(t, err)
		assert.ErrorIs(t, err, anystore.ErrDocNotFound)
	})
}

func TestUserDataObject_UpdateContactByDetails(t *testing.T) {
	t.Run("update name, icon - details remain the same", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.source.EXPECT().ReadStoreDoc(context.Background(), mock.Anything, mock.Anything).Return(nil)
		fx.source.EXPECT().PushStoreChange(context.Background(), mock.Anything).RunAndReturn(storechanges.PushStoreChanges).Times(1)
		err := fx.Init(&smartblock.InitContext{
			Ctx:    context.Background(),
			Source: fx.source,
		})
		require.NoError(t, err)
		identity := "identity"
		contactId := domain.NewContactId(identity)
		err = fx.SaveContact(context.Background(), NewContact(identity, ""))
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
		assert.Equal(t, "", contacts[0].Description())
		assert.Equal(t, identity, contacts[0].Identity())
		assert.Equal(t, "", contacts[0].Key())
	})
	t.Run("update description", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.source.EXPECT().ReadStoreDoc(context.Background(), mock.Anything, mock.Anything).Return(nil)
		fx.source.EXPECT().PushStoreChange(context.Background(), mock.Anything).RunAndReturn(storechanges.PushStoreChanges).Times(2)
		err := fx.Init(&smartblock.InitContext{
			Ctx:    context.Background(),
			Source: fx.source,
		})
		require.NoError(t, err)
		identity := "identity"
		contactId := domain.NewContactId(identity)
		err = fx.SaveContact(context.Background(), NewContact(identity, ""))
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
		assert.Equal(t, newDescription, contacts[0].Description())
		assert.Equal(t, identity, contacts[0].Identity())
		assert.Equal(t, "", contacts[0].Key())
	})
	t.Run("contact not exists", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.source.EXPECT().ReadStoreDoc(context.Background(), mock.Anything, mock.Anything).Return(nil)
		err := fx.Init(&smartblock.InitContext{
			Ctx:    context.Background(),
			Source: fx.source,
		})
		require.NoError(t, err)
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
	source *mock_source.MockStore
}

func newFixture(t *testing.T) *fixture {
	ctx := context.Background()
	db, err := anystore.Open(ctx, filepath.Join(t.TempDir(), "crdt.db"), nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		err := db.Close()
		require.NoError(t, err)
	})
	sb := smarttest.New("userDataObjectId")
	require.NoError(t, err)
	source := mock_source.NewMockStore(t)
	object := New(sb, db)
	dataObject := object.(*userDataObject)
	fx := &fixture{
		userDataObject: dataObject,
		source:         source,
	}
	fx.source = source
	return fx
}
