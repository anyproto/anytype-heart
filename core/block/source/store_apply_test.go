package source

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree/mock_objecttree"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/pb"
)

var ctx = context.Background()

func TestStoreApply_Apply(t *testing.T) {
	t.Run("new tree", func(t *testing.T) {
		fx := newStoreFx(t)
		tx := fx.RequireTx(t)
		changes := []*objecttree.Change{
			testChange("1", false),
			testChange("2", false),
			testChange("3", false),
		}
		fx.ApplyChanges(t, tx, changes...)
		require.NoError(t, tx.Commit())
	})
	t.Run("insert middle", func(t *testing.T) {
		fx := newStoreFx(t)
		tx := fx.RequireTx(t)
		changes := []*objecttree.Change{
			testChange("1", false),
			testChange("2", false),
			testChange("3", false),
		}
		fx.ApplyChanges(t, tx, changes...)
		require.NoError(t, tx.Commit())

		tx = fx.RequireTx(t)
		changes = []*objecttree.Change{
			testChange("1", false),
			testChange("1.1", true),
			testChange("1.2", true),
			testChange("1.3", true),
			testChange("2", false),
			testChange("2.2", true),
			testChange("3", false),
		}
		fx.ExpectTreeFrom("1.1", changes[1:]...)
		fx.ExpectTreeFrom("2.2", changes[6:]...)
		fx.ApplyChanges(t, tx, changes...)
		require.NoError(t, tx.Commit())
	})
	t.Run("append", func(t *testing.T) {
		fx := newStoreFx(t)
		tx := fx.RequireTx(t)
		changes := []*objecttree.Change{
			testChange("1", false),
			testChange("2", false),
			testChange("3", false),
		}
		fx.ApplyChanges(t, tx, changes...)
		require.NoError(t, tx.Commit())

		tx = fx.RequireTx(t)
		changes = []*objecttree.Change{
			testChange("1", false),
			testChange("2", false),
			testChange("3", false),
			testChange("4", true),
			testChange("5", true),
			testChange("6", true),
		}
		fx.ApplyChanges(t, tx, changes...)
		require.NoError(t, tx.Commit())
	})
}

func TestStoreApply_Apply10000(t *testing.T) {
	fx := newStoreFx(t)
	tx := fx.RequireTx(t)
	changes := make([]*objecttree.Change, 100000)
	for i := range changes {
		changes[i] = testChange(fmt.Sprint(i), false)
	}
	st := time.Now()
	applier := &storeApply{
		tx: tx,
		ot: fx.tree,
	}
	fx.ExpectTree(changes...)
	require.NoError(t, applier.Apply())
	t.Logf("apply dur: %v;", time.Since(st))
	st = time.Now()
	require.NoError(t, tx.Commit())
	t.Logf("commit dur: %v;", time.Since(st))

}

type storeFx struct {
	state *storestate.StoreState
	tree  *mock_objecttree.MockObjectTree
	db    anystore.DB
}

func (fx *storeFx) ExpectTree(changes ...*objecttree.Change) {
	fx.tree.EXPECT().IterateRoot(gomock.Any(), gomock.Any()).DoAndReturn(func(_ objecttree.ChangeConvertFunc, f objecttree.ChangeIterateFunc) error {
		for _, ch := range changes {
			if !f(ch) {
				return nil
			}
		}
		return nil
	})
}

func (fx *storeFx) ExpectTreeFrom(fromId string, changes ...*objecttree.Change) {
	fx.tree.EXPECT().IterateFrom(fromId, gomock.Any(), gomock.Any()).DoAndReturn(func(_ string, _ objecttree.ChangeConvertFunc, f objecttree.ChangeIterateFunc) error {
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
		ot: fx.tree,
	}
	fx.ExpectTree(changes...)
	require.NoError(t, applier.Apply())
	fx.AssertOrder(t, tx, changes...)
}

func newStoreFx(t testing.TB) *storeFx {
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

	tree := mock_objecttree.NewMockObjectTree(ctrl)
	tree.EXPECT().Id().Return("root").AnyTimes()

	return &storeFx{
		state: state,
		tree:  tree,
		db:    db,
	}
}

func testChange(id string, isNew bool) *objecttree.Change {
	_, pub, _ := crypto.GenerateRandomEd25519KeyPair()

	return &objecttree.Change{
		Id:       id,
		IsNew:    isNew,
		Model:    &pb.StoreChange{},
		Identity: pub,
	}
}
