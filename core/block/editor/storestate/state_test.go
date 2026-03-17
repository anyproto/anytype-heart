package storestate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
)

var ctx = context.Background()

func TestStoreStateTx_GetOrder(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		fx := newFixture(t, "test", DefaultHandler{Name: "tcoll"})
		tx, err := fx.NewTx(ctx)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, tx.Commit())
		}()
		order, err := tx.GetOrder("changeId")
		assert.ErrorIs(t, err, ErrOrderNotFound)
		assert.Empty(t, order)
	})
	t.Run("set-get", func(t *testing.T) {
		fx := newFixture(t, "test", DefaultHandler{Name: "tcoll"})
		tx, err := fx.NewTx(ctx)
		require.NoError(t, err)
		require.NoError(t, tx.SetOrder("changeId", "1"))
		order, err := tx.GetOrder("changeId")
		require.NoError(t, err)
		assert.Equal(t, "1", order)
		assert.Equal(t, "1", tx.GetMaxOrder())
		require.NoError(t, tx.Commit())

		tx, err = fx.NewTx(ctx)
		require.NoError(t, err)
		assert.Equal(t, "1", tx.GetMaxOrder())
		require.NoError(t, tx.SetOrder("changeId2", "2"))
		assert.Equal(t, "2", tx.GetMaxOrder())
		require.NoError(t, tx.Commit())
	})
}

func TestStoreStateTx_ApplyChangeSet(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		fx := newFixture(t, "objId", DefaultHandler{Name: "testColl"})
		tx, err := fx.NewTx(ctx)
		require.NoError(t, err)
		build := &Builder{}
		assert.NoError(t, build.Create("testColl", "1", `{"key":"value"}`))
		require.NoError(t, tx.ApplyChangeSet(ChangeSet{
			Id:      "1",
			Order:   "1",
			Changes: build.ChangeSet,
		}))
		require.NoError(t, tx.Commit())

		coll, err := fx.Collection(ctx, "testColl")
		require.NoError(t, err)
		doc, err := coll.FindId(ctx, "1")
		require.NoError(t, err)
		assert.Equal(t, "value", string(doc.Value().GetStringBytes("key")))

	})
	t.Run("modify", func(t *testing.T) {
		fx := newFixture(t, "objId", DefaultHandler{Name: "testColl"})
		tx, err := fx.NewTx(ctx)
		require.NoError(t, err)
		build := &Builder{}
		assert.NoError(t, build.Create("testColl", "1", `{"key":"value"}`))
		assert.NoError(t, build.Modify("testColl", "1", []string{"key"}, pb.ModifyOp_Set, `"valueChanged"`))
		assert.NoError(t, build.Modify("testColl", "1", []string{"num"}, pb.ModifyOp_Inc, `2`))

		require.NoError(t, tx.ApplyChangeSet(ChangeSet{
			Id:      "1",
			Order:   "1",
			Changes: build.ChangeSet,
		}))
		require.NoError(t, tx.Commit())

		coll, err := fx.Collection(ctx, "testColl")
		require.NoError(t, err)
		count, err := coll.Count(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
		doc, err := coll.FindId(ctx, "1")
		require.NoError(t, err)
		assert.Equal(t, "valueChanged", string(doc.Value().GetStringBytes("key")))
		assert.Equal(t, float64(2), doc.Value().GetFloat64("num"))
	})
	t.Run("delete", func(t *testing.T) {
		fx := newFixture(t, "objId", DefaultHandler{Name: "testColl"})
		tx, err := fx.NewTx(ctx)
		require.NoError(t, err)
		build := &Builder{}
		assert.NoError(t, build.Create("testColl", "1", `{"key":"value"}`))
		require.NoError(t, tx.ApplyChangeSet(ChangeSet{
			Id:      "1",
			Order:   "1",
			Changes: build.ChangeSet,
		}))
		require.NoError(t, tx.Commit())

		tx, err = fx.NewTx(ctx)
		require.NoError(t, err)
		build = &Builder{}
		build.Delete("testColl", "1")
		require.NoError(t, tx.ApplyChangeSet(ChangeSet{
			Id:      "1",
			Order:   "1",
			Changes: build.ChangeSet,
		}))
		require.NoError(t, tx.Commit())

		coll, err := fx.Collection(ctx, "testColl")
		require.NoError(t, err)
		count, err := coll.Count(ctx)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})
}

func TestApplyChangeSetErrorHandling(t *testing.T) {
	t.Run("unknown handler error is logged and skipped", func(t *testing.T) {
		handler := &errorHandler{
			Name:      "testColl",
			createErr: fmt.Errorf("some unknown error from handler"),
		}
		fx := newFixture(t, "objId", handler)
		tx, err := fx.NewTx(ctx)
		require.NoError(t, err)

		build := &Builder{}
		// First change will fail with unknown error (should be skipped)
		assert.NoError(t, build.Create("testColl", "1", `{"key":"value1"}`))
		// Second change should still be applied
		handler.createErr = nil
		assert.NoError(t, build.Create("testColl", "2", `{"key":"value2"}`))

		err = tx.ApplyChangeSet(ChangeSet{
			Id:      "1",
			Order:   "1",
			Changes: build.ChangeSet,
		})
		require.NoError(t, err)
		require.NoError(t, tx.Commit())

		// Only second document should exist
		coll, err := fx.Collection(ctx, "testColl")
		require.NoError(t, err)
		_, err = coll.FindId(ctx, "2")
		require.NoError(t, err)
	})

	t.Run("ErrCritical breaks the loop", func(t *testing.T) {
		handler := &errorHandler{
			Name:      "testColl",
			createErr: fmt.Errorf("db failure: %w", ErrCritical),
		}
		fx := newFixture(t, "objId", handler)
		tx, err := fx.NewTx(ctx)
		require.NoError(t, err)
		defer func() {
			_ = tx.Rollback()
		}()

		build := &Builder{}
		assert.NoError(t, build.Create("testColl", "1", `{"key":"value1"}`))
		assert.NoError(t, build.Create("testColl", "2", `{"key":"value2"}`))

		err = tx.ApplyChangeSet(ChangeSet{
			Id:      "1",
			Order:   "1",
			Changes: build.ChangeSet,
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrCritical)
	})

	t.Run("ErrIgnore continues silently", func(t *testing.T) {
		handler := &errorHandler{
			Name:      "testColl",
			createErr: ErrIgnore,
		}
		fx := newFixture(t, "objId", handler)
		tx, err := fx.NewTx(ctx)
		require.NoError(t, err)

		build := &Builder{}
		assert.NoError(t, build.Create("testColl", "1", `{"key":"value1"}`))

		err = tx.ApplyChangeSet(ChangeSet{
			Id:      "1",
			Order:   "1",
			Changes: build.ChangeSet,
		})
		require.NoError(t, err)
		require.NoError(t, tx.Commit())
	})

	t.Run("ErrValidation is logged and skipped", func(t *testing.T) {
		handler := &errorHandler{
			Name:      "testColl",
			createErr: fmt.Errorf("validation failed: %w", ErrValidation),
		}
		fx := newFixture(t, "objId", handler)
		tx, err := fx.NewTx(ctx)
		require.NoError(t, err)

		build := &Builder{}
		assert.NoError(t, build.Create("testColl", "1", `{"key":"value1"}`))

		err = tx.ApplyChangeSet(ChangeSet{
			Id:      "1",
			Order:   "1",
			Changes: build.ChangeSet,
		})
		require.NoError(t, err)
		require.NoError(t, tx.Commit())
	})
}

// errorHandler is a test handler that returns configurable errors
type errorHandler struct {
	Name      string
	createErr error
}

func (h *errorHandler) CollectionName() string { return h.Name }

func (h *errorHandler) Init(ctx context.Context, s *StoreState) error {
	_, err := s.Collection(ctx, h.Name)
	return err
}

func (h *errorHandler) BeforeCreate(_ context.Context, _ ChangeOp) error {
	return h.createErr
}

func (h *errorHandler) BeforeModify(_ context.Context, _ ChangeOp) (ModifyMode, error) {
	return ModifyModeUpdate, nil
}

func (h *errorHandler) BeforeDelete(_ context.Context, _ ChangeOp) (DeleteMode, error) {
	return DeleteModeDelete, nil
}

func (h *errorHandler) UpgradeKeyModifier(_ ChangeOp, _ *pb.KeyModify, mod query.Modifier) query.Modifier {
	return mod
}

func TestBenchCreate(t *testing.T) {
	// t.Skip()
	var n = 100000
	fx := newFixture(t, "objId", DefaultHandler{Name: "testColl"})
	st := time.Now()
	var changes = make([]ChangeSet, n)
	for i := range n {
		build := &Builder{}
		assert.NoError(t, build.Create("testColl", fmt.Sprint(i), `{"some":"json"}`))
		changes[i] = ChangeSet{
			Id:      fmt.Sprint(i),
			Order:   fmt.Sprint(i),
			Changes: build.ChangeSet,
		}
	}
	t.Logf("created %d changes for a %v", n, time.Since(st))
	st = time.Now()
	tx, err := fx.NewTx(ctx)
	require.NoError(t, err)
	for _, ch := range changes {
		assert.NoError(t, tx.ApplyChangeSet(ch))
	}
	t.Logf("applied for a %v", time.Since(st))
	st = time.Now()
	assert.NoError(t, tx.Commit())
	t.Logf("commited for a %v", time.Since(st))
}

type fixture struct {
	*StoreState
	db     anystore.DB
	tmpDir string
}

func newFixture(t testing.TB, id string, handlers ...Handler) *fixture {
	fx := &fixture{}
	var err error
	fx.tmpDir, err = os.MkdirTemp("", "storestate_*")
	require.NoError(t, err)
	fx.db, err = anystore.Open(ctx, filepath.Join(fx.tmpDir, "db.db"), nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		fx.finish(t)
	})
	fx.StoreState, err = New(ctx, id, fx.db, handlers...)
	require.NoError(t, err)
	return fx
}

func (fx *fixture) finish(t testing.TB) {
	if fx.db != nil {
		require.NoError(t, fx.db.Close())
	}
	if fx.tmpDir != "" {
		_ = os.RemoveAll(fx.tmpDir)
	}
}
