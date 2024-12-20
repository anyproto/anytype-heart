package editor

import (
	"context"
	"encoding/base64"
	"path/filepath"
	"sync"
	"testing"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/commonspace/mock_commonspace"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/editor/userdataobject"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache/mock_objectcache"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/block/source/mock_source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/space/techspace"
	"github.com/anyproto/anytype-heart/tests/storechanges"
)

var (
	userDataObjectId = "userDataObject"
	id               = "identity"
	spaceId          = "techSpace"
	contactId        = domain.NewContactId("identity")
	name             = "name"
	description      = "description"
)

type fixture struct {
	*ContactObject
	source         *mock_source.MockStore
	storeFixture   *spaceindex.StoreFixture
	objectCache    *mock_objectcache.MockCache
	spaceService   *mock_space.MockService
	techSpace      techspace.TechSpace
	db             anystore.DB
	userDataObject userdataobject.UserDataObject
}

func TestContactObject_Init(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// when
		initContext := &smartblock.InitContext{}
		err := fx.ContactObject.Init(initContext)

		// then
		require.NoError(t, err)
		assert.Equal(t, int64(model.ObjectType_contact), initContext.State.Details().GetInt64(bundle.RelationKeyLayout))
		assert.Equal(t, bundle.TypeKeyContact, initContext.State.ObjectTypeKey())
	})
	t.Run("contact exist in store", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.storeFixture.AddObjects(t, []spaceindex.TestObject{{
			bundle.RelationKeyId:          domain.String(contactId),
			bundle.RelationKeySpaceId:     domain.String(spaceId),
			bundle.RelationKeyName:        domain.String(name),
			bundle.RelationKeyDescription: domain.String(description),
			bundle.RelationKeyIdentity:    domain.String(id),
		},
		})
		// when
		ctx := &smartblock.InitContext{}
		err := fx.ContactObject.Init(ctx)

		// then
		require.NoError(t, err)
		err = fx.ContactObject.Apply(ctx.State)
		require.NoError(t, err)
		details := fx.ContactObject.CombinedDetails()
		assert.Equal(t, name, details.GetString(bundle.RelationKeyName))
		assert.Equal(t, description, details.GetString(bundle.RelationKeyDescription))
		assert.Equal(t, id, details.GetString(bundle.RelationKeyIdentity))
	})
}

func TestContactObject_SetDetails(t *testing.T) {
	t.Run(" updated only description", func(t *testing.T) {
		// given
		fx := newFixture(t)
		err := fx.ContactObject.Init(&smartblock.InitContext{})
		require.NoError(t, err)
		var wg sync.WaitGroup
		wg.Add(2)

		var collection *storestate.StoreState
		fx.source.EXPECT().PushStoreChange(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, params source.PushStoreChangeParams) (string, error) {
			defer wg.Done()
			collection = params.State
			return storechanges.PushStoreChanges(ctx, params)
		})
		aesKey := crypto.NewAES()
		err = fx.userDataObject.SaveContact(context.Background(), &model.IdentityProfile{Identity: id}, aesKey)
		require.NoError(t, err)

		// when
		fx.callTechSpace(t)
		err = fx.ContactObject.SetDetails(nil, []domain.Detail{
			{
				Key:   bundle.RelationKeyName,
				Value: domain.String(name),
			},
			{
				Key:   bundle.RelationKeyDescription,
				Value: domain.String(description),
			},
		}, false)

		// then
		wg.Wait()
		require.NoError(t, err)
		details := fx.ContactObject.CombinedDetails()
		assert.Equal(t, "", details.GetString(bundle.RelationKeyName))
		assert.Equal(t, description, details.GetString(bundle.RelationKeyDescription))

		c, err := collection.Collection(context.Background(), "contacts")
		require.NoError(t, err)
		jsonContact, err := c.FindId(context.Background(), contactId)
		require.NoError(t, err)

		raw, err := aesKey.Raw()
		require.NoError(t, err)
		encodedKey := base64.StdEncoding.EncodeToString(raw)
		contact := userdataobject.NewContactFromJson(jsonContact.Value())
		expected := userdataobject.NewContact(id, "")
		assert.Equal(t, expected, contact)
	})
}

func TestContactObject_SetDetailsAndUpdateLastUsed(t *testing.T) {
	t.Run("success updated only description", func(t *testing.T) {
		// given
		fx := newFixture(t)
		err := fx.ContactObject.Init(&smartblock.InitContext{})
		require.NoError(t, err)
		var wg sync.WaitGroup
		wg.Add(2)

		var collection *storestate.StoreState
		fx.source.EXPECT().PushStoreChange(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, params source.PushStoreChangeParams) (string, error) {
			defer wg.Done()
			collection = params.State
			return storechanges.PushStoreChanges(ctx, params)
		})
		aesKey := crypto.NewAES()
		err = fx.userDataObject.SaveContact(context.Background(), &model.IdentityProfile{Identity: id}, aesKey)
		require.NoError(t, err)

		// when
		fx.callTechSpace(t)
		err = fx.ContactObject.SetDetailsAndUpdateLastUsed(nil, []domain.Detail{
			{
				Key:   bundle.RelationKeyName,
				Value: domain.String(name),
			},
			{
				Key:   bundle.RelationKeyDescription,
				Value: domain.String(description),
			},
		}, false)

		// then
		wg.Wait()
		require.NoError(t, err)
		details := fx.ContactObject.CombinedDetails()
		assert.Equal(t, "", details.GetString(bundle.RelationKeyName))
		assert.Equal(t, description, details.GetString(bundle.RelationKeyDescription))

		c, err := collection.Collection(context.Background(), "contacts")
		require.NoError(t, err)
		jsonContact, err := c.FindId(context.Background(), contactId)
		require.NoError(t, err)

		raw, err := aesKey.Raw()
		require.NoError(t, err)
		encodedKey := base64.StdEncoding.EncodeToString(raw)
		contact := userdataobject.NewContactFromJson(jsonContact.Value())
		expected := userdataobject.NewContact(id, "")
		assert.Equal(t, expected, contact)
	})
}

func newFixture(t *testing.T) *fixture {
	techSpace := techspace.New()

	space := mock_commonspace.NewMockSpace(gomock.NewController(t))
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
	objectGetter := mock_cache.NewMockObjectGetter(t)
	objectGetter.EXPECT().GetObjectByFullID(mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	userDataObject := userdataobject.New(smarttest.New(userDataObjectId), db, objectGetter)
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

	sb := smarttest.New(contactId)
	storeFixture := spaceindex.NewStoreFixture(t)
	fx := &fixture{
		ContactObject:  NewContactObject(sb, storeFixture, spaceService),
		source:         source,
		storeFixture:   storeFixture,
		objectCache:    objectCache,
		spaceService:   spaceService,
		techSpace:      techSpace,
		db:             db,
		userDataObject: userDataObject,
	}
	return fx
}

func (fx *fixture) callTechSpace(t *testing.T) {
	wallet := mock_wallet.NewMockWallet(t)
	keys, err := accountdata.NewRandom()
	require.NoError(t, err)
	wallet.EXPECT().Account().Return(keys)
	fx.spaceService.EXPECT().TechSpace().Return(clientspace.NewTechSpace(clientspace.TechSpaceDeps{
		AccountService: wallet,
		TechSpace:      fx.techSpace,
	}))
}
