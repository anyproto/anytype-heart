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
	"github.com/anyproto/anytype-heart/tests/testutil"
)

var (
	userDataObjectId = "userDataObject"
	id               = "identity"
	spaceId          = "techSpace"
)

func TestService_Run(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// when
		err := fx.Run(context.Background())

		// then
		require.NoError(t, err)
	})
}

func TestService_SaveContact(t *testing.T) {
	t.Run("no identity", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.identityService.EXPECT().AddObserver(spaceId, id, mock.Anything).Return()
		fx.identityService.EXPECT().WaitProfile(context.Background(), id).Return(nil)
		fx.techCore.EXPECT().Id().Return(spaceId)

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
			Name:        "name",
			IconCid:     "iconCid",
			Description: "description",
		})
		fx.techCore.EXPECT().Id().Return(spaceId)
		fx.source.EXPECT().PushStoreChange(mock.Anything, mock.Anything).Return("changeId", nil)

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
			Name:        "name",
			IconCid:     "iconCid",
			Description: "description",
		})
		fx.techCore.EXPECT().Id().Return(spaceId)
		fx.source.EXPECT().PushStoreChange(mock.Anything, mock.Anything).Return("changeId", nil)

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

		fx.techCore.EXPECT().Id().Return(spaceId)
		fx.source.EXPECT().PushStoreChange(mock.Anything, mock.Anything).Return("changeId", nil)

		// when
		err := fx.DeleteContact(context.Background(), id)

		// then
		require.NoError(t, err)
	})
	t.Run("no identity", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.source.EXPECT().PushStoreChange(mock.Anything, mock.Anything).Return("", anystore.ErrDocNotFound)

		// when
		err := fx.DeleteContact(context.Background(), id)

		// then
		require.Error(t, err)
	})
}

type fixture struct {
	Service
	techSpace       techspace.TechSpace
	techCore        *mock_commonspace.MockSpace
	objectCache     *mock_objectcache.MockCache
	identityService *mock_contact.MockidentityService
	source          *mock_source.MockStore
}

func newFixture(t *testing.T) *fixture {
	ctrl := gomock.NewController(t)
	techSpace := techspace.New()
	space := mock_commonspace.NewMockSpace(ctrl)
	space.EXPECT().Id().Return(spaceId).Times(2)
	objectCache := mock_objectcache.NewMockCache(t)
	objectCache.EXPECT().DeriveObjectID(mock.Anything, mock.Anything).Return(userDataObjectId, nil)
	objectCache.EXPECT().DeriveTreePayload(mock.Anything, mock.Anything).Return(treestorage.TreeStorageCreatePayload{
		RootRawChange: &treechangeproto.RawTreeChangeWithId{
			Id: "accountId",
		},
	}, nil)

	db, err := anystore.Open(context.Background(), filepath.Join(t.TempDir(), "crdt.db"), nil)
	require.NoError(t, err)
	userDataObject := userdataobject.New(smarttest.New(userDataObjectId), db, nil)
	source := mock_source.NewMockStore(t)
	source.EXPECT().ReadStoreDoc(mock.Anything, mock.Anything, mock.Anything).Return(nil)
	err = userDataObject.Init(&smartblock.InitContext{
		Ctx:    context.Background(),
		Source: source,
	})
	require.NoError(t, err)
	objectCache.EXPECT().GetObject(mock.Anything, userDataObjectId).Return(userDataObject, nil).Maybe()
	objectCache.EXPECT().GetObject(mock.Anything, "accountId").Return(nil, nil)
	err = techSpace.Run(space, objectCache, false)
	require.NoError(t, err)

	spaceService := mock_space.NewMockService(t)
	wallet := mock_wallet.NewMockWallet(t)
	keys, err := accountdata.NewRandom()
	require.NoError(t, err)
	wallet.EXPECT().Account().Return(keys)
	spaceService.EXPECT().TechSpace().Return(clientspace.NewTechSpace(clientspace.TechSpaceDeps{
		AccountService: wallet,
		TechSpace:      techSpace,
	}))
	identityService := mock_contact.NewMockidentityService(t)

	a := new(app.App)
	a.Register(techSpace)
	a.Register(testutil.PrepareMock(context.Background(), a, spaceService))
	a.Register(testutil.PrepareMock(context.Background(), a, identityService))

	contactService := New()
	err = contactService.Init(a)
	require.NoError(t, err)
	fx := &fixture{
		Service:         contactService,
		techSpace:       techSpace,
		identityService: identityService,
		techCore:        space,
		objectCache:     objectCache,
		source:          source,
	}
	return fx
}
