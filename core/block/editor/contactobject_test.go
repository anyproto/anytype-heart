package editor

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/commonspace/mock_commonspace"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/globalsign/mgo/bson"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

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
	"github.com/anyproto/anytype-heart/util/pbtypes"
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
		assert.Equal(t, int64(model.ObjectType_contact), pbtypes.GetInt64(initContext.State.Details(), bundle.RelationKeyLayout.String()))
		assert.Equal(t, bundle.TypeKeyContact, initContext.State.ObjectTypeKey())
	})
	t.Run("contact exist in store", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.storeFixture.AddObjects(t, []spaceindex.TestObject{{
			bundle.RelationKeyId:          pbtypes.String(contactId),
			bundle.RelationKeySpaceId:     pbtypes.String(spaceId),
			bundle.RelationKeyName:        pbtypes.String(name),
			bundle.RelationKeyDescription: pbtypes.String(description),
			bundle.RelationKeyIdentity:    pbtypes.String(id),
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
		assert.Equal(t, name, pbtypes.GetString(details, bundle.RelationKeyName.String()))
		assert.Equal(t, description, pbtypes.GetString(details, bundle.RelationKeyDescription.String()))
		assert.Equal(t, id, pbtypes.GetString(details, bundle.RelationKeyIdentity.String()))
	})
}

func TestContactObject_SetDetails(t *testing.T) {
	t.Run("success", func(t *testing.T) {
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
			return fx.pushStoreChanges(ctx, params)
		})
		err = fx.userDataObject.SaveContact(context.Background(), &model.IdentityProfile{Identity: id})
		require.NoError(t, err)

		// when
		fx.callTechSpace(t)
		err = fx.ContactObject.SetDetails(nil, []*model.Detail{
			{
				Key:   bundle.RelationKeyName.String(),
				Value: pbtypes.String(name),
			},
			{
				Key:   bundle.RelationKeyDescription.String(),
				Value: pbtypes.String(description),
			},
		}, false)

		// then
		wg.Wait()
		require.NoError(t, err)
		details := fx.ContactObject.CombinedDetails()
		assert.Equal(t, name, pbtypes.GetString(details, bundle.RelationKeyName.String()))
		assert.Equal(t, description, pbtypes.GetString(details, bundle.RelationKeyDescription.String()))

		c, err := collection.Collection(context.Background(), "contacts")
		require.NoError(t, err)
		jsonContact, err := c.FindId(context.Background(), contactId)
		require.NoError(t, err)
		contact := userdataobject.NewContactFromJson(jsonContact.Value())
		assert.Equal(t, userdataobject.NewContact(id, name, description, ""), contact)
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
	return changeId, tx.Commit()
}
