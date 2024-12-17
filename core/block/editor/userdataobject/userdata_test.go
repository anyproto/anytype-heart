package userdataobject

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	anystore "github.com/anyproto/any-store"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/block/source/mock_source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
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
		fx.source.EXPECT().PushStoreChange(context.Background(), mock.Anything).RunAndReturn(fx.pushStoreChanges)
		err := fx.Init(&smartblock.InitContext{
			Ctx:    context.Background(),
			Source: fx.source,
		})
		require.NoError(t, err)

		identity := "identity"
		contactId := domain.NewContactId(identity)
		test := smarttest.New(contactId)
		fx.objectGetter.EXPECT().GetObject(context.Background(), contactId).Return(test, nil)
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
		assert.Equal(t, name, pbtypes.GetString(details, bundle.RelationKeyName.String()))
		assert.Equal(t, description, pbtypes.GetString(details, bundle.RelationKeyDescription.String()))
		assert.Equal(t, identity, pbtypes.GetString(details, bundle.RelationKeyIdentity.String()))
		assert.Equal(t, iconCid, pbtypes.GetString(details, bundle.RelationKeyIconImage.String()))
	})
	t.Run("contact exists", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.source.EXPECT().ReadStoreDoc(context.Background(), mock.Anything, mock.Anything).Return(nil)
		fx.source.EXPECT().PushStoreChange(context.Background(), mock.Anything).RunAndReturn(fx.pushStoreChanges).Times(2)
		err := fx.Init(&smartblock.InitContext{
			Ctx:    context.Background(),
			Source: fx.source,
		})
		require.NoError(t, err)

		identity := "identity"
		contactId := domain.NewContactId(identity)
		test := smarttest.New(contactId)
		fx.objectGetter.EXPECT().GetObject(context.Background(), contactId).Return(test, nil)
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
		fx := newFixture(t)
		fx.source.EXPECT().ReadStoreDoc(context.Background(), mock.Anything, mock.Anything).Return(nil)
		fx.source.EXPECT().PushStoreChange(context.Background(), mock.Anything).RunAndReturn(fx.pushStoreChanges)
		err := fx.Init(&smartblock.InitContext{
			Ctx:    context.Background(),
			Source: fx.source,
		})
		require.NoError(t, err)
		identity := "identity"
		contactId := domain.NewContactId(identity)
		test := smarttest.New(contactId)
		fx.objectGetter.EXPECT().GetObject(context.Background(), contactId).Return(test, nil)
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
		fx := newFixture(t)
		fx.source.EXPECT().ReadStoreDoc(context.Background(), mock.Anything, mock.Anything).Return(nil)
		fx.source.EXPECT().PushStoreChange(context.Background(), mock.Anything).RunAndReturn(fx.pushStoreChanges)

		err := fx.Init(&smartblock.InitContext{
			Ctx:    context.Background(),
			Source: fx.source,
		})
		require.NoError(t, err)
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
		fx := newFixture(t)
		fx.source.EXPECT().ReadStoreDoc(context.Background(), mock.Anything, mock.Anything).Return(nil)
		fx.source.EXPECT().PushStoreChange(context.Background(), mock.Anything).RunAndReturn(fx.pushStoreChanges).Times(2)
		err := fx.Init(&smartblock.InitContext{
			Ctx:    context.Background(),
			Source: fx.source,
		})
		require.NoError(t, err)

		identity := "identity"
		contactId := domain.NewContactId(identity)
		test := smarttest.New(contactId)
		fx.objectGetter.EXPECT().GetObject(context.Background(), contactId).Return(test, nil).Times(2)
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
		assert.Equal(t, newName, pbtypes.GetString(details, bundle.RelationKeyName.String()))
		assert.Equal(t, newCid, pbtypes.GetString(details, bundle.RelationKeyIconImage.String()))
	})
	t.Run("contact not exists", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.source.EXPECT().ReadStoreDoc(context.Background(), mock.Anything, mock.Anything).Return(nil)
		fx.source.EXPECT().PushStoreChange(context.Background(), mock.Anything).RunAndReturn(fx.pushStoreChanges).Times(1)
		err := fx.Init(&smartblock.InitContext{
			Ctx:    context.Background(),
			Source: fx.source,
		})
		require.NoError(t, err)

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
		fx := newFixture(t)
		fx.source.EXPECT().ReadStoreDoc(context.Background(), mock.Anything, mock.Anything).Return(nil)
		fx.source.EXPECT().PushStoreChange(context.Background(), mock.Anything).RunAndReturn(fx.pushStoreChanges).Times(2)
		err := fx.Init(&smartblock.InitContext{
			Ctx:    context.Background(),
			Source: fx.source,
		})
		require.NoError(t, err)

		identity := "identity"
		contactId := domain.NewContactId(identity)
		test := smarttest.New(contactId)
		fx.objectGetter.EXPECT().GetObject(context.Background(), contactId).Return(test, nil).Times(2)
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
		err = fx.UpdateContactByDetails(context.Background(), contactId, &types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyName.String():      pbtypes.String(newName),
				bundle.RelationKeyIconImage.String(): pbtypes.String(newCid),
			},
		})

		// then
		require.NoError(t, err)
		details := test.CombinedDetails()
		assert.Equal(t, newName, pbtypes.GetString(details, bundle.RelationKeyName.String()))
		assert.Equal(t, newCid, pbtypes.GetString(details, bundle.RelationKeyIconImage.String()))
		assert.Equal(t, description, pbtypes.GetString(details, bundle.RelationKeyDescription.String()))
	})
	t.Run("update description", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.source.EXPECT().ReadStoreDoc(context.Background(), mock.Anything, mock.Anything).Return(nil)
		fx.source.EXPECT().PushStoreChange(context.Background(), mock.Anything).RunAndReturn(fx.pushStoreChanges).Times(2)
		err := fx.Init(&smartblock.InitContext{
			Ctx:    context.Background(),
			Source: fx.source,
		})
		require.NoError(t, err)

		identity := "identity"
		contactId := domain.NewContactId(identity)
		test := smarttest.New(contactId)
		fx.objectGetter.EXPECT().GetObject(context.Background(), contactId).Return(test, nil).Times(2)
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
		err = fx.UpdateContactByDetails(context.Background(), contactId, &types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyDescription.String(): pbtypes.String(newDescription),
			},
		})

		// then
		require.NoError(t, err)
		details := test.CombinedDetails()
		assert.Equal(t, name, pbtypes.GetString(details, bundle.RelationKeyName.String()))
		assert.Equal(t, iconCid, pbtypes.GetString(details, bundle.RelationKeyIconImage.String()))
		assert.Equal(t, newDescription, pbtypes.GetString(details, bundle.RelationKeyDescription.String()))
	})
	t.Run("contact not exists", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.source.EXPECT().ReadStoreDoc(context.Background(), mock.Anything, mock.Anything).Return(nil)
		fx.source.EXPECT().PushStoreChange(context.Background(), mock.Anything).RunAndReturn(fx.pushStoreChanges).Times(1)
		err := fx.Init(&smartblock.InitContext{
			Ctx:    context.Background(),
			Source: fx.source,
		})
		require.NoError(t, err)

		identity := "identity"
		newName := "newName"

		// when
		err = fx.UpdateContactByDetails(context.Background(), domain.NewContactId(identity), &types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyName.String(): pbtypes.String(newName),
			},
		})

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
	objectGetter := mock_cache.NewMockObjectGetter(t)
	source := mock_source.NewMockStore(t)
	object := New(sb, db, objectGetter)
	fx := &fixture{
		db:             db,
		userDataObject: object.(*userDataObject),
		source:         source,
		objectGetter:   objectGetter,
	}
	fx.source = source
	return fx
}

func (fx *fixture) pushStoreChanges(ctx context.Context, params source.PushStoreChangeParams) (string, error) {
	changeId := bson.NewObjectId().Hex()
	tx, err := params.State.NewTx(ctx)
	if err != nil {
		return "", fmt.Errorf("new tx: %w", err)
	}
	order := tx.NextOrder(tx.GetMaxOrder())
	err = tx.ApplyChangeSet(storestate.ChangeSet{
		Id:        changeId,
		Order:     order,
		Changes:   params.Changes,
		Creator:   "creator",
		Timestamp: params.Time.Unix(),
	})
	if err != nil {
		return "", errors.Join(tx.Rollback(), fmt.Errorf("apply change set: %w", err))
	}
	err = tx.Commit()
	if err != nil {
		return "", err
	}
	fx.onUpdate()
	return changeId, nil
}
