package sourceimpl

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree/mock_objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/sys/unix"

	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/pb"
)

var ctx = context.Background()

func TestStoreApply_RealTree(t *testing.T) {
	update := func(fx *storeFx, heads []string, chs []*treechangeproto.RawTreeChangeWithId) {
		tx := fx.RequireTx(t)
		defer tx.Rollback()

		_, err := fx.realTree.AddRawChangesWithUpdater(ctx, objecttree.RawChangesPayload{
			NewHeads:   heads,
			RawChanges: chs,
		}, func(tree objecttree.ObjectTree, md objecttree.Mode) error {
			applier := &storeApply{
				tx: tx,
				ot: fx.realTree,
			}
			return applier.Apply()
		})
		require.NoError(t, err)
		require.NoError(t, tx.Commit())
	}
	assertOrder := func(fx *storeFx, orders []string) {
		var changes []*objecttree.Change
		for _, order := range orders {
			changes = append(changes, testChange(order, false))
		}
		tx := fx.RequireTx(t)
		defer tx.Rollback()

		fx.AssertOrder(t, tx, changes...)
	}
	t.Run("new real tree - 1,2,3 then 4,5", func(t *testing.T) {
		fx := newRealTreeStoreFx(t)
		newChanges := []*treechangeproto.RawTreeChangeWithId{
			fx.changeCreator.CreateRaw("1", fx.aclList.Head().Id, "0", false, "0"),
			fx.changeCreator.CreateRaw("2", fx.aclList.Head().Id, "0", false, "1"),
			fx.changeCreator.CreateRaw("3", fx.aclList.Head().Id, "0", true, "2"),
		}
		update(fx, []string{"3"}, newChanges)
		newChanges = []*treechangeproto.RawTreeChangeWithId{
			fx.changeCreator.CreateRaw("4", fx.aclList.Head().Id, "0", false, "0"),
			fx.changeCreator.CreateRaw("5", fx.aclList.Head().Id, "0", true, "4"),
		}
		update(fx, []string{"3", "5"}, newChanges)
		assertOrder(fx, []string{"0", "1", "2", "3", "4", "5"})
	})
	t.Run("new real tree - 4,5 then 1,2,3", func(t *testing.T) {
		fx := newRealTreeStoreFx(t)
		newChanges := []*treechangeproto.RawTreeChangeWithId{
			fx.changeCreator.CreateRaw("4", fx.aclList.Head().Id, "0", false, "0"),
			fx.changeCreator.CreateRaw("5", fx.aclList.Head().Id, "0", true, "4"),
		}
		update(fx, []string{"5"}, newChanges)
		newChanges = []*treechangeproto.RawTreeChangeWithId{
			fx.changeCreator.CreateRaw("1", fx.aclList.Head().Id, "0", false, "0"),
			fx.changeCreator.CreateRaw("2", fx.aclList.Head().Id, "0", false, "1"),
			fx.changeCreator.CreateRaw("3", fx.aclList.Head().Id, "0", true, "2"),
		}
		update(fx, []string{"3", "5"}, newChanges)
		assertOrder(fx, []string{"0", "1", "2", "3", "4", "5"})
	})
	t.Run("new real tree - 1,2,3,4,5 in one batch", func(t *testing.T) {
		fx := newRealTreeStoreFx(t)
		newChanges := []*treechangeproto.RawTreeChangeWithId{
			fx.changeCreator.CreateRaw("1", fx.aclList.Head().Id, "0", false, "0"),
			fx.changeCreator.CreateRaw("2", fx.aclList.Head().Id, "0", false, "1"),
			fx.changeCreator.CreateRaw("3", fx.aclList.Head().Id, "0", true, "2"),
			fx.changeCreator.CreateRaw("4", fx.aclList.Head().Id, "0", false, "0"),
			fx.changeCreator.CreateRaw("5", fx.aclList.Head().Id, "0", true, "4"),
		}
		update(fx, []string{"3", "4", "5"}, newChanges)
		assertOrder(fx, []string{"0", "1", "2", "3", "4", "5"})
	})
}

type storeFx struct {
	state         *storestate.StoreState
	mockTree      *mock_objecttree.MockObjectTree
	realTree      objecttree.ObjectTree
	changeCreator *objecttree.MockChangeCreator
	aclList       list.AclList
	db            anystore.DB
}

func (fx *storeFx) ExpectTree(changes ...*objecttree.Change) {
	fx.mockTree.EXPECT().IterateRoot(gomock.Any(), gomock.Any()).DoAndReturn(func(_ objecttree.ChangeConvertFunc, f objecttree.ChangeIterateFunc) error {
		for _, ch := range changes {
			if !f(ch) {
				return nil
			}
		}
		return nil
	})
}

func (fx *storeFx) ExpectTreeFrom(fromId string, changes ...*objecttree.Change) {
	fx.mockTree.EXPECT().IterateFrom(fromId, gomock.Any(), gomock.Any()).DoAndReturn(func(_ string, _ objecttree.ChangeConvertFunc, f objecttree.ChangeIterateFunc) error {
		for _, ch := range changes {
			if !f(ch) {
				return nil
			}
		}
		return nil
	})
}

func (fx *storeFx) RequireTx(t testing.TB) *storestate.StoreStateTx {
	tx, err := fx.state.NewTx(ctx)
	require.NoError(t, err)
	return tx
}

func (fx *storeFx) AssertOrder(t testing.TB, tx *storestate.StoreStateTx, changes ...*objecttree.Change) {
	var expectedIds = make([]string, len(changes))
	var storeOrders = make([]string, len(changes))
	var err error
	for i, ch := range changes {
		expectedIds[i] = ch.Id
		storeOrders[i], err = tx.GetOrder(ch.Id)
		require.NoError(t, err)
	}
	assert.Equal(t, len(expectedIds), len(storeOrders))
	assert.True(t, sort.StringsAreSorted(storeOrders))
	t.Log(storeOrders)
}

func (fx *storeFx) ApplyChanges(t *testing.T, tx *storestate.StoreStateTx, changes ...*objecttree.Change) {
	applier := &storeApply{
		tx: tx,
		ot: fx.mockTree,
	}
	fx.ExpectTree(changes...)
	require.NoError(t, applier.Apply())
	fx.AssertOrder(t, tx, changes...)
}

func newRealTreeStoreFx(t testing.TB) *storeFx {
	tmpDir, err := os.MkdirTemp("", "source_store_*")
	require.NoError(t, err)

	db, err := anystore.Open(ctx, filepath.Join(tmpDir, "test.db"), nil)
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	t.Cleanup(func() {
		if db != nil {
			_ = db.Close()
		}
		ctrl.Finish()
		if tmpDir != "" {
			_ = os.RemoveAll(tmpDir)
		}
	})

	state, err := storestate.New(ctx, "source_test", db, storestate.DefaultHandler{Name: "default"})
	require.NoError(t, err)
	aclList, _ := prepareAclList(t)
	objTree, err := buildTree(t, aclList)
	require.NoError(t, err)
	fx := &storeFx{
		state:    state,
		realTree: objTree,
		aclList:  aclList,
		changeCreator: objecttree.NewMockChangeCreator(func() anystore.DB {
			return createStore(ctx, t)
		}),
		db: db,
	}
	tx := fx.RequireTx(t)
	defer tx.Rollback()
	applier := &storeApply{
		tx:       tx,
		allIsNew: true,
		ot:       fx.realTree,
	}
	require.NoError(t, applier.Apply())
	require.NoError(t, tx.Commit())
	return fx
}

func testChange(id string, isNew bool) *objecttree.Change {
	_, pub, _ := crypto.GenerateRandomEd25519KeyPair()

	return &objecttree.Change{
		Id:       id,
		OrderId:  id,
		IsNew:    isNew,
		Model:    &pb.StoreChange{},
		Identity: pub,
	}
}

func prepareAclList(t testing.TB) (list.AclList, *accountdata.AccountKeys) {
	randKeys, err := accountdata.NewRandom()
	require.NoError(t, err)
	aclList, err := list.NewInMemoryDerivedAcl("spaceId", randKeys)
	require.NoError(t, err, "building acl list should be without error")

	return aclList, randKeys
}

func buildTree(t testing.TB, aclList list.AclList) (objecttree.ObjectTree, error) {
	changeCreator := objecttree.NewMockChangeCreator(func() anystore.DB {
		return createStore(ctx, t)
	})
	treeStorage := changeCreator.CreateNewTreeStorage(t.(*testing.T), "0", aclList.Head().Id, false)
	tree, err := objecttree.BuildTestableTree(treeStorage, aclList)
	if err != nil {
		return nil, err
	}
	tree.SetFlusher(objecttree.MarkNewChangeFlusher())
	return tree, nil
}

func createStore(ctx context.Context, t testing.TB) anystore.DB {
	return createNamedStore(ctx, t, "changes.db")
}

func createNamedStore(ctx context.Context, t testing.TB, name string) anystore.DB {
	path := filepath.Join(t.TempDir(), name)
	db, err := anystore.Open(ctx, path, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		err := db.Close()
		require.NoError(t, err)
		unix.Rmdir(path)
	})
	return objecttree.TestStore{
		DB:   db,
		Path: path,
	}
}
