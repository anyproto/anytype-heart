package accountobject

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/globalsign/mgo/bson"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/block/source/mock_source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/util/metricsid"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var ctx = context.Background()

type fixture struct {
	*accountObject
	source  *mock_source.MockStore
	storeFx *objectstore.StoreFixture
	db      anystore.DB
}

func newFixture(t *testing.T, isNewAccount bool, prepareDb func(db anystore.DB)) *fixture {
	ctx := context.Background()
	cfg := config.New(config.WithNewAccount(isNewAccount))
	db, err := anystore.Open(ctx, filepath.Join(t.TempDir(), "crdt.db"), nil)
	require.NoError(t, err)
	if prepareDb != nil {
		prepareDb(db)
	}
	t.Cleanup(func() {
		err := db.Close()
		require.NoError(t, err)
	})
	sb := smarttest.New("accountId1")
	indexStore := objectstore.NewStoreFixture(t).SpaceIndex("spaceId")
	keys, err := accountdata.NewRandom()
	require.NoError(t, err)
	object := New(sb, keys, indexStore, nil, nil, db, cfg)
	fx := &fixture{
		storeFx:       objectstore.NewStoreFixture(t),
		db:            db,
		accountObject: object.(*accountObject),
	}
	source := mock_source.NewMockStore(t)
	source.EXPECT().ReadStoreDoc(ctx, mock.Anything, mock.Anything).Return(nil)
	source.EXPECT().PushStoreChange(mock.Anything, mock.Anything).RunAndReturn(fx.applyToStore).Maybe()
	source.EXPECT().SetPushChangeHook(mock.Anything)
	fx.source = source

	err = object.Init(&smartblock.InitContext{
		Ctx:    ctx,
		Source: source,
	})
	require.NoError(t, err)

	return fx
}

func (fx *fixture) applyToStore(ctx context.Context, params source.PushStoreChangeParams) (string, error) {
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

func assertBlock(t *testing.T, st state.Doc, id string) {
	found := false
	st.Iterate(func(b simple.Block) (isContinue bool) {
		if b.Model().Id == id {
			found = true
			return false
		}
		return true
	})
	require.True(t, found)
}

func makeStoreContent(m map[string]any) source.PushChangeParams {
	changes := make([]*pb.ChangeContent, 0, len(m))
	for k, v := range m {
		changes = append(changes, &pb.ChangeContent{
			&pb.ChangeContentValueOfDetailsSet{DetailsSet: &pb.ChangeDetailsSet{
				Key:   k,
				Value: pbtypes.InterfaceToValue(v),
			},
			},
		})
	}
	return source.PushChangeParams{Changes: changes}
}

func (fx *fixture) assertStateValue(t *testing.T, val any, extract func(str *domain.Details) any) {
	require.Equal(t, val, extract(fx.SmartBlock.NewState().CombinedDetails()))
}

func TestAccountNew(t *testing.T) {
	fx := newFixture(t, true, nil)
	expectedId, err := metricsid.DeriveMetricsId(fx.keys.SignKey)
	require.NoError(t, err)
	st := fx.SmartBlock.NewState()
	assertBlock(t, st, "accountId1")
	assertBlock(t, st, "title")
	assertBlock(t, st, "header")
	assertBlock(t, st, "identity")
	res, err := fx.IsIconMigrated()
	require.NoError(t, err)
	require.True(t, res)
	id, err := fx.GetAnalyticsId()
	require.NoError(t, err)
	fmt.Println(id)
	require.Equal(t, expectedId, id)
}

func TestAccountOldInitWithData(t *testing.T) {
	fx := newFixture(t, false, func(db anystore.DB) {
		tx, _ := db.WriteTx(ctx)
		coll, err := db.CreateCollection(tx.Context(), "accountId1"+collectionName)
		require.NoError(t, err)
		err = coll.Insert(tx.Context(), anyenc.MustParseJson(fmt.Sprintf(`{"id":"%s","analyticsId":"%s","%s":"true","name":"Anna","description":"Molly"}`, accountDocumentId, "analyticsId", iconMigrationKey)))
		require.NoError(t, err)
		require.NoError(t, tx.Commit())
	})
	st := fx.SmartBlock.NewState()
	assertBlock(t, st, "accountId1")
	assertBlock(t, st, "title")
	assertBlock(t, st, "header")
	assertBlock(t, st, "identity")
	res, err := fx.IsIconMigrated()
	require.NoError(t, err)
	require.True(t, res)
	id, err := fx.GetAnalyticsId()
	require.NoError(t, err)
	require.Equal(t, "analyticsId", id)
	fx.assertStateValue(t, "Anna", func(str *domain.Details) any {
		return str.GetString("name")
	})
	fx.assertStateValue(t, "Molly", func(str *domain.Details) any {
		return str.GetString("description")
	})
	require.NotNil(t, fx)
}

func TestPushNewChanges(t *testing.T) {
	// this tests both cases when we get changes from somewhere or we push our own changes
	fx := newFixture(t, true, nil)
	_, err := fx.OnPushChange(makeStoreContent(map[string]any{"name": "Anna", "description": "Molly"}))
	require.NoError(t, err)
	fx.assertStateValue(t, "Anna", func(str *domain.Details) any {
		return str.GetString("name")
	})
	fx.assertStateValue(t, "Molly", func(str *domain.Details) any {
		return str.GetString("description")
	})
	require.NotNil(t, fx)
}

func TestIconMigrated(t *testing.T) {
	fx := newFixture(t, false, nil)
	err := fx.MigrateIconImage("image")
	require.NoError(t, err)
	res, err := fx.IsIconMigrated()
	require.NoError(t, err)
	require.True(t, res)
}

func TestSetSharedSpacesLimit(t *testing.T) {
	fx := newFixture(t, true, nil)
	err := fx.SetSharedSpacesLimit(10)
	require.NoError(t, err)
	res := fx.GetSharedSpacesLimit()
	require.Equal(t, 10, res)
}

func TestAnalyticsId(t *testing.T) {
	fx := newFixture(t, true, nil)
	err := fx.SetAnalyticsId("analyticsId")
	require.NoError(t, err)
	res, err := fx.GetAnalyticsId()
	require.NoError(t, err)
	require.Equal(t, "analyticsId", res)
}
