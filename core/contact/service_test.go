package contact

import (
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

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/userdataobject"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache/mock_objectcache"
	"github.com/anyproto/anytype-heart/core/block/source/mock_source"
	"github.com/anyproto/anytype-heart/core/contact/mock_contact"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
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
	cid              = "iconCid"
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
	t.Run("success - add observers", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.callTechSpace(t)
		fx.source.EXPECT().PushStoreChange(mock.Anything, mock.Anything).RunAndReturn(storechanges.PushStoreChanges)
		err := fx.userDataObject.SaveContact(context.Background(), &model.IdentityProfile{
			Identity: id,
		})
		require.NoError(t, err)
		fx.spaceService.EXPECT().TechSpaceId().Return(spaceId)
		fx.identityService.EXPECT().AddObserver(spaceId, id, mock.Anything).Return()

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
		fx.source.EXPECT().PushStoreChange(mock.Anything, mock.Anything).RunAndReturn(storechanges.PushStoreChanges).Times(2)
		err = fx.userDataObject.SaveContact(context.Background(), &model.IdentityProfile{
			Identity: id,
		})
		require.NoError(t, err)

		// when
		fx.handleIdentityUpdate(id, &model.IdentityProfile{Identity: id, Name: name, Description: description})

		// then
		contacts, err := fx.userDataObject.ListContacts(context.Background())
		require.NoError(t, err)
		require.Len(t, contacts, 1)
		assert.Equal(t, id, contacts[0].Identity())
		assert.Equal(t, name, contacts[0].Name())
		assert.Equal(t, description, contacts[0].Description())
	})
}

func TestService_SaveContact(t *testing.T) {
	t.Run("no identity", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.identityService.EXPECT().AddObserver(spaceId, id, mock.Anything).Return()
		fx.identityService.EXPECT().WaitProfile(context.Background(), id).Return(nil)
		fx.spaceService.EXPECT().TechSpaceId().Return(spaceId)

		// when
		err := fx.SaveContact(context.Background(), id, "")

		// then
		require.Error(t, err)
	})
	t.Run("get profile from cache", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.identityService.EXPECT().AddObserver(spaceId, id, mock.Anything).Return()
		fx.identityService.EXPECT().WaitProfile(context.Background(), id).Return(&model.IdentityProfile{
			Identity:    id,
			Name:        name,
			IconCid:     cid,
			Description: description,
		})
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
		fx.identityService.EXPECT().WaitProfile(context.Background(), id).Return(&model.IdentityProfile{
			Identity:    id,
			Name:        name,
			IconCid:     cid,
			Description: description,
		})
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
		fx.identityService.EXPECT().UnregisterIdentity(spaceId, id).Return()
		fx.callTechSpace(t)
		fx.source.EXPECT().PushStoreChange(mock.Anything, mock.Anything).Return("changeId", nil)
		fx.spaceService.EXPECT().TechSpaceId().Return(spaceId)

		// when
		err := fx.DeleteContact(context.Background(), id)

		// then
		require.NoError(t, err)
	})
	t.Run("no identity", func(t *testing.T) {
		// given
		fx := newFixture(t)
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
}

func newFixture(t *testing.T) *fixture {
	spaceService := mock_space.NewMockService(t)
	identityService := mock_contact.NewMockidentityService(t)
	objectCache := mock_objectcache.NewMockCache(t)
	source := mock_source.NewMockStore(t)

	a := new(app.App)
	a.Register(testutil.PrepareMock(context.Background(), a, spaceService))
	a.Register(testutil.PrepareMock(context.Background(), a, identityService))

	contactService := New()
	err := contactService.Init(a)
	require.NoError(t, err)
	fx := &fixture{
		service:         contactService.(*service),
		spaceService:    spaceService,
		identityService: identityService,
		objectCache:     objectCache,
		source:          source,
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
	userDataObject := userdataobject.New(smarttest.New(userDataObjectId), db, nil)
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
