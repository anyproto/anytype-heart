package sourceimpl

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
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
	// addChanges persists changes to tree storage, then apply reads from storage
	// (matches the production flow: sync persists, then Update listener applies)
	addChanges := func(fx *storeFx, heads []string, chs []*treechangeproto.RawTreeChangeWithId) {
		_, err := fx.realTree.AddRawChanges(ctx, objecttree.RawChangesPayload{
			NewHeads:   heads,
			RawChanges: chs,
		})
		require.NoError(t, err)
	}
	apply := func(fx *storeFx) {
		tx := fx.RequireTx(t)
		defer tx.Rollback()
		applier := &storeApply{
			tx: tx,
			ot: fx.realTree,
		}
		require.NoError(t, applier.Apply(ctx))
		require.NoError(t, tx.Commit())
	}
	t.Run("new real tree - 1,2,3 then 4,5", func(t *testing.T) {
		fx := newRealTreeStoreFx(t)
		addChanges(fx, []string{"3"}, []*treechangeproto.RawTreeChangeWithId{
			fx.changeCreator.CreateRaw("1", fx.aclList.Head().Id, "0", false, "0"),
			fx.changeCreator.CreateRaw("2", fx.aclList.Head().Id, "0", false, "1"),
			fx.changeCreator.CreateRaw("3", fx.aclList.Head().Id, "0", true, "2"),
		})
		apply(fx)

		addChanges(fx, []string{"3", "5"}, []*treechangeproto.RawTreeChangeWithId{
			fx.changeCreator.CreateRaw("4", fx.aclList.Head().Id, "0", false, "0"),
			fx.changeCreator.CreateRaw("5", fx.aclList.Head().Id, "0", true, "4"),
		})
		apply(fx)

		tx := fx.RequireTx(t)
		defer tx.Rollback()
		assert.True(t, tx.GetMaxAddSeq() > 0)
	})
	t.Run("new real tree - 4,5 then 1,2,3", func(t *testing.T) {
		fx := newRealTreeStoreFx(t)
		addChanges(fx, []string{"5"}, []*treechangeproto.RawTreeChangeWithId{
			fx.changeCreator.CreateRaw("4", fx.aclList.Head().Id, "0", false, "0"),
			fx.changeCreator.CreateRaw("5", fx.aclList.Head().Id, "0", true, "4"),
		})
		apply(fx)

		addChanges(fx, []string{"3", "5"}, []*treechangeproto.RawTreeChangeWithId{
			fx.changeCreator.CreateRaw("1", fx.aclList.Head().Id, "0", false, "0"),
			fx.changeCreator.CreateRaw("2", fx.aclList.Head().Id, "0", false, "1"),
			fx.changeCreator.CreateRaw("3", fx.aclList.Head().Id, "0", true, "2"),
		})
		apply(fx)

		tx := fx.RequireTx(t)
		defer tx.Rollback()
		assert.True(t, tx.GetMaxAddSeq() > 0)
	})
	t.Run("new real tree - 1,2,3,4,5 in one batch", func(t *testing.T) {
		fx := newRealTreeStoreFx(t)
		addChanges(fx, []string{"3", "4", "5"}, []*treechangeproto.RawTreeChangeWithId{
			fx.changeCreator.CreateRaw("1", fx.aclList.Head().Id, "0", false, "0"),
			fx.changeCreator.CreateRaw("2", fx.aclList.Head().Id, "0", false, "1"),
			fx.changeCreator.CreateRaw("3", fx.aclList.Head().Id, "0", true, "2"),
			fx.changeCreator.CreateRaw("4", fx.aclList.Head().Id, "0", false, "0"),
			fx.changeCreator.CreateRaw("5", fx.aclList.Head().Id, "0", true, "4"),
		})
		apply(fx)

		tx := fx.RequireTx(t)
		defer tx.Rollback()
		assert.True(t, tx.GetMaxAddSeq() > 0)
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
	fx.mockTree.EXPECT().IterateAfterAddSeq(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, addSeq uint64, _ objecttree.ChangeConvertFunc, f objecttree.ChangeIterateFunc) error {
			for _, ch := range changes {
				if ch.AddSeq > addSeq {
					if !f(ch) {
						return nil
					}
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

func (fx *storeFx) ApplyChanges(t *testing.T, tx *storestate.StoreStateTx, changes ...*objecttree.Change) {
	applier := &storeApply{
		tx: tx,
		ot: fx.mockTree,
	}
	fx.ExpectTree(changes...)
	require.NoError(t, applier.Apply(ctx))
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
		tx: tx,
		ot: fx.realTree,
	}
	require.NoError(t, applier.Apply(ctx))
	require.NoError(t, tx.Commit())
	return fx
}

func testChange(id string, addSeq uint64) *objecttree.Change {
	_, pub, _ := crypto.GenerateRandomEd25519KeyPair()

	return &objecttree.Change{
		Id:       id,
		OrderId:  id,
		AddSeq:   addSeq,
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
	// Set up AddSeq counter for storage so IterateAfterAddSeq works
	if setter, ok := treeStorage.(interface{ SetAddSeq(seq *atomic.Uint64) }); ok {
		setter.SetAddSeq(&atomic.Uint64{})
	}
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
