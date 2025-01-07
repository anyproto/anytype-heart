package contact

import (
	"encoding/base64"
	"path/filepath"
	"testing"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/mock_commonspace"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/net/context"

	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/userdataobject"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache/mock_objectcache"
	"github.com/anyproto/anytype-heart/core/block/source/mock_source"
	"github.com/anyproto/anytype-heart/core/contact/mock_contact"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/space/techspace"
	"github.com/anyproto/anytype-heart/tests/storechanges"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

var (
	userDataObjectId = "userDataObject"
	id               = "identity"
	spaceId          = "techSpace"
	name             = "name"
	description      = "description"
	iconCid          = "iconCid"
	globalName       = "globalName"
	contactId        = domain.NewContactId(id)
)

func TestService_Run(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.callTechSpace(t)

		// when
		err := fx.Run(context.Background())

		// then
		require.NoError(t, err)
		require.NoError(t, fx.Close(context.Background()))
	})
	t.Run("success - register identity", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.callTechSpace(t)
		fx.source.EXPECT().PushStoreChange(mock.Anything, mock.Anything).RunAndReturn(storechanges.PushStoreChanges)
		aesKey := crypto.NewAES()
		raw, err := aesKey.Raw()
		require.NoError(t, err)
		base64Key := base64.StdEncoding.EncodeToString(raw)
		contact := userdataobject.NewContact(id, base64Key)
		err = fx.userDataObject.SaveContact(context.Background(), contact)
		require.NoError(t, err)
		fx.identityService.EXPECT().RegisterIdentity(spaceId, id, aesKey, mock.Anything).Return(nil)
		fx.spaceService.EXPECT().TechSpaceId().Return(spaceId)
		test := smarttest.New(contactId)
		fx.objectGetter.EXPECT().GetObjectByFullID(mock.Anything, domain.FullID{ObjectID: contactId, SpaceID: spaceId}).Return(test, nil).Times(1)

		// when
		err = fx.Run(context.Background())

		// then
		require.NoError(t, err)
		require.NoError(t, fx.Close(context.Background()))
	})
}

func TestService_handleIdentityUpdate(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.callTechSpace(t)
		err := fx.Run(context.Background())
		require.NoError(t, err)
		fx.source.EXPECT().PushStoreChange(mock.Anything, mock.Anything).RunAndReturn(storechanges.PushStoreChanges).Times(1)
		aesKey := crypto.NewAES()
		raw, err := aesKey.Raw()
		require.NoError(t, err)
		base64Key := base64.StdEncoding.EncodeToString(raw)
		contact := userdataobject.NewContact(id, base64Key)
		err = fx.userDataObject.SaveContact(context.Background(), contact)
		require.NoError(t, err)
		fx.spaceService.EXPECT().TechSpaceId().Return(spaceId)
		test := smarttest.New(contactId)
		fx.objectGetter.EXPECT().GetObjectByFullID(mock.Anything, domain.FullID{ObjectID: contactId, SpaceID: spaceId}).Return(test, nil).Times(1)

		// when
		fx.handleIdentityUpdate(id, &model.IdentityProfile{Identity: id, Name: name, Description: description, IconCid: iconCid})

		// then
		details := test.CombinedDetails()
		assert.Equal(t, name, details.GetString(bundle.RelationKeyName))
		assert.Equal(t, iconCid, details.GetString(bundle.RelationKeyIconImage))
		assert.Equal(t, id, details.GetString(bundle.RelationKeyIdentity))
		assert.Equal(t, "", details.GetString(bundle.RelationKeyGlobalName))
		assert.Equal(t, "", details.GetString(bundle.RelationKeyDescription))
	})
}

func TestService_SaveContact(t *testing.T) {
	t.Run("identity is not provided", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// when
		err := fx.SaveContact(context.Background(), "", "")

		// then
		require.Error(t, err)
	})
	t.Run("key not exists", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.identityService.EXPECT().GetIdentityKey(id).Return(nil)

		// when
		err := fx.SaveContact(context.Background(), id, "")

		// then
		require.Error(t, err)
	})
	t.Run("get profile from cache", func(t *testing.T) {
		// given
		fx := newFixture(t)

		aesKey := crypto.NewAES()
		fx.identityService.EXPECT().RegisterIdentity(spaceId, id, aesKey, mock.Anything).Return(nil)
		fx.identityService.EXPECT().GetIdentityKey(id).Return(aesKey)
		fx.callTechSpace(t)
		fx.source.EXPECT().PushStoreChange(mock.Anything, mock.Anything).Return("changeId", nil)
		fx.spaceService.EXPECT().TechSpaceId().Return(spaceId)

		// when
		err := fx.SaveContact(context.Background(), id, "")

		// then
		require.NoError(t, err)
	})
	t.Run("register identity", func(t *testing.T) {
		// given
		fx := newFixture(t)

		aesKey := crypto.NewAES()
		fx.identityService.EXPECT().RegisterIdentity(spaceId, id, aesKey, mock.Anything).Return(nil)
		fx.callTechSpace(t)
		fx.source.EXPECT().PushStoreChange(mock.Anything, mock.Anything).Return("changeId", nil)
		fx.spaceService.EXPECT().TechSpaceId().Return(spaceId)

		// when
		err := fx.SaveContact(context.Background(), id, aesKey.String())

		// then
		require.NoError(t, err)
	})
}

func TestService_DeleteContact(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:          domain.String(contactId),
				bundle.RelationKeySpaceId:     domain.String(spaceId),
				bundle.RelationKeyName:        domain.String(name),
				bundle.RelationKeyDescription: domain.String(description),
				bundle.RelationKeyIconImage:   domain.String(iconCid),
			},
		})
		fx.identityService.EXPECT().UnregisterIdentity(spaceId, id).Return()
		fx.callTechSpace(t)
		fx.source.EXPECT().PushStoreChange(mock.Anything, mock.Anything).Return("changeId", nil)
		fx.spaceService.EXPECT().TechSpaceId().Return(spaceId)

		// when
		err := fx.DeleteContact(context.Background(), id)

		// then
		require.NoError(t, err)
		ids, err := fx.store.SpaceIndex(spaceId).QueryByIds([]string{contactId})
		require.NoError(t, err)
		assert.Len(t, ids, 0)
	})

	t.Run("no identity", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.identityService.EXPECT().UnregisterIdentity(spaceId, id).Return()
		fx.spaceService.EXPECT().TechSpaceId().Return(spaceId)
		fx.source.EXPECT().PushStoreChange(mock.Anything, mock.Anything).Return("", anystore.ErrDocNotFound)
		fx.callTechSpace(t)

		// when
		err := fx.DeleteContact(context.Background(), id)

		// then
		require.Error(t, err)
	})
}

type fixture struct {
	*service
	objectCache     *mock_objectcache.MockCache
	identityService *mock_contact.MockidentityService
	source          *mock_source.MockStore
	spaceService    *mock_space.MockService
	userDataObject  userdataobject.UserDataObject
	objectGetter    *mock_cache.MockObjectGetterComponent
	store           *objectstore.StoreFixture
}

func newFixture(t *testing.T) *fixture {
	spaceService := mock_space.NewMockService(t)
	identityService := mock_contact.NewMockidentityService(t)
	objectCache := mock_objectcache.NewMockCache(t)
	source := mock_source.NewMockStore(t)
	objectGetter := mock_cache.NewMockObjectGetterComponent(t)
	store := objectstore.NewStoreFixture(t)

	a := new(app.App)
	a.Register(testutil.PrepareMock(context.Background(), a, spaceService))
	a.Register(testutil.PrepareMock(context.Background(), a, identityService))
	a.Register(testutil.PrepareMock(context.Background(), a, objectGetter))
	a.Register(store)

	contactService := New()
	err := contactService.Init(a)
	require.NoError(t, err)
	fx := &fixture{
		service:         contactService.(*service),
		spaceService:    spaceService,
		identityService: identityService,
		objectCache:     objectCache,
		source:          source,
		objectGetter:    objectGetter,
		store:           store,
	}
	return fx
}

func (fx *fixture) callTechSpace(t *testing.T) {
	ctrl := gomock.NewController(t)
	techSpace := techspace.New()
	space := mock_commonspace.NewMockSpace(ctrl)
	space.EXPECT().Id().Return(spaceId).Times(2)
	fx.objectCache.EXPECT().DeriveObjectID(mock.Anything, mock.Anything).Return(userDataObjectId, nil)
	fx.objectCache.EXPECT().DeriveTreePayload(mock.Anything, mock.Anything).Return(treestorage.TreeStorageCreatePayload{
		RootRawChange: &treechangeproto.RawTreeChangeWithId{
			Id: "accountId",
		},
	}, nil)

	db, err := anystore.Open(context.Background(), filepath.Join(t.TempDir(), "crdt.db"), nil)
	require.NoError(t, err)
	objectGetter := mock_cache.NewMockObjectGetter(t)
	objectGetter.EXPECT().GetObjectByFullID(mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	userDataObject := userdataobject.New(smarttest.New(userDataObjectId), db)
	fx.userDataObject = userDataObject
	fx.source.EXPECT().ReadStoreDoc(mock.Anything, mock.Anything, mock.Anything).Return(nil)
	err = userDataObject.Init(&smartblock.InitContext{
		Ctx:    context.Background(),
		Source: fx.source,
	})
	require.NoError(t, err)
	fx.objectCache.EXPECT().GetObject(mock.Anything, userDataObjectId).Return(userDataObject, nil).Maybe()
	fx.objectCache.EXPECT().GetObject(mock.Anything, "accountId").Return(nil, nil)
	err = techSpace.Run(space, fx.objectCache, false)
	require.NoError(t, err)
	wallet := mock_wallet.NewMockWallet(t)
	keys, err := accountdata.NewRandom()
	require.NoError(t, err)
	wallet.EXPECT().Account().Return(keys)
	fx.spaceService.EXPECT().TechSpace().Return(clientspace.NewTechSpace(clientspace.TechSpaceDeps{
		AccountService: wallet,
		TechSpace:      techSpace,
	}))
}
