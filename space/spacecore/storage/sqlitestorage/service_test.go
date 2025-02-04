package sqlitestorage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

func TestStorageService_BindSpaceID(t *testing.T) {
	fx := newFixture(t)
	defer fx.finish(t)

	require.NoError(t, fx.BindSpaceID("spaceId", "objectId"))
	spaceId, err := fx.GetSpaceID("objectId")
	require.NoError(t, err)
	assert.Equal(t, "spaceId", spaceId)
	require.NoError(t, fx.BindSpaceID("spaceId2", "objectId"))
	spaceId, err = fx.GetSpaceID("objectId")
	require.NoError(t, err)
	assert.Equal(t, "spaceId2", spaceId)
}

func TestStorageService_GetBoundObjectIds(t *testing.T) {
	t.Run("no bindings", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		spaceId := "spaceId"

		ids, err := fx.GetBoundObjectIds(spaceId)
		require.NoError(t, err)

		assert.Empty(t, ids)
	})

	t.Run("ok", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		spaceId := "spaceId"
		require.NoError(t, fx.BindSpaceID(spaceId, "objectId1"))
		require.NoError(t, fx.BindSpaceID(spaceId, "objectId2"))

		ids, err := fx.GetBoundObjectIds(spaceId)
		require.NoError(t, err)

		assert.ElementsMatch(t, []string{"objectId1", "objectId2"}, ids)
	})
}

func TestStorageService_DeleteSpaceStorage(t *testing.T) {
	fx := newFixture(t)
	defer fx.finish(t)

	payload := spaceTestPayload()
	spaceId := payload.SpaceHeaderWithId.Id
	store, err := createSpaceStorage(fx.storageService, payload)
	require.NoError(t, err)
	_, err = store.CreateTreeStorage(treeTestPayload())
	require.NoError(t, err)
	require.NoError(t, store.Close(ctx))
	require.NoError(t, fx.BindSpaceID("spaceId", "objectId"))

	require.NoError(t, fx.DeleteSpaceStorage(ctx, spaceId))

	var expect0 = func(q string) {
		var count int
		require.NoError(t, fx.writeDb.QueryRow(q, spaceId).Scan(&count))
		assert.Equal(t, 0, count)
	}

	expect0("SELECT COUNT(*) FROM spaces WHERE id = ?")
	expect0("SELECT COUNT(*) FROM trees WHERE spaceId = ?")
	expect0("SELECT COUNT(*) FROM changes WHERE spaceId = ?")
	expect0("SELECT COUNT(*) FROM binds WHERE spaceId = ?")

}

func TestStorageService_MarkSpaceCreated(t *testing.T) {
	fx := newFixture(t)
	defer fx.finish(t)

	payload := spaceTestPayload()
	ss, err := fx.CreateSpaceStorage(payload)
	require.NoError(t, err)
	require.NoError(t, ss.Close(ctx))

	assert.False(t, fx.IsSpaceCreated(payload.SpaceHeaderWithId.Id))
	require.NoError(t, fx.MarkSpaceCreated(payload.SpaceHeaderWithId.Id))
	assert.True(t, fx.IsSpaceCreated(payload.SpaceHeaderWithId.Id))
	require.NoError(t, fx.UnmarkSpaceCreated(payload.SpaceHeaderWithId.Id))
	assert.False(t, fx.IsSpaceCreated(payload.SpaceHeaderWithId.Id))
}

func TestStorageService_SpaceExists(t *testing.T) {
	fx := newFixture(t)
	defer fx.finish(t)

	payload := spaceTestPayload()

	assert.False(t, fx.SpaceExists(payload.SpaceHeaderWithId.Id))

	ss, err := fx.CreateSpaceStorage(payload)
	require.NoError(t, err)
	require.NoError(t, ss.Close(ctx))

	assert.True(t, fx.SpaceExists(payload.SpaceHeaderWithId.Id))
}

func TestStorageService_AllSpaceIds(t *testing.T) {
	fx := newFixture(t)
	defer fx.finish(t)

	payload := spaceTestPayload()
	ss, err := fx.CreateSpaceStorage(payload)
	require.NoError(t, err)
	require.NoError(t, ss.Close(ctx))

	spaceIds, err := fx.AllSpaceIds()
	require.NoError(t, err)
	assert.Equal(t, []string{payload.SpaceHeaderWithId.Id}, spaceIds)
}

func TestStorageService_WaitSpaceStorage(t *testing.T) {
	fx := newFixture(t)
	defer fx.finish(t)

	payload := spaceTestPayload()
	ss, err := fx.CreateSpaceStorage(payload)
	require.NoError(t, err)
	require.NoError(t, ss.Close(ctx))

	ss, err = fx.WaitSpaceStorage(ctx, payload.SpaceHeaderWithId.Id)
	require.NoError(t, err)

	var gotStorage = make(chan struct{})
	go func() {
		defer close(gotStorage)
		ss2, err := fx.WaitSpaceStorage(ctx, payload.SpaceHeaderWithId.Id)
		require.NoError(t, err)
		defer func() {
			_ = ss2.Close(ctx)
		}()
	}()

	time.Sleep(100 * time.Millisecond)

	select {
	case <-gotStorage:
		require.Fail(t, "second storage is opened")
	default:
	}

	require.NoError(t, ss.Close(ctx))

	select {
	case <-gotStorage:
	case <-time.After(time.Second):
		require.Fail(t, "second storage is not opened")
	}
}

func TestCheckpoint(t *testing.T) {
	fx := newFixture(t, func(fx *fixture) {
		fx.storageService.checkpointAfterWrite = time.Second / 3
		fx.storageService.checkpointForce = time.Second
	})
	defer fx.finish(t)

	assert.Empty(t, fx.lastCheckpoint.Load())
	assert.Empty(t, fx.lastWrite.Load())
	require.NoError(t, fx.BindSpaceID("1", "2"))
	assert.NotEmpty(t, fx.lastWrite.Load())
	time.Sleep((time.Second / 3) * 2)
	firstCheckpoint := fx.lastCheckpoint.Load()
	assert.NotEmpty(t, firstCheckpoint)
	time.Sleep(time.Second * 2)
	secondCheckpoint := fx.lastCheckpoint.Load()
	assert.NotEqual(t, firstCheckpoint, secondCheckpoint)
}

type fixture struct {
	*storageService
	a      *app.App
	tmpDir string
}

func newFixture(t require.TestingT, beforeStart ...func(fx *fixture)) *fixture {
	tmpDir, e := os.MkdirTemp("", "")
	require.NoError(t, e)
	fx := &fixture{
		storageService: New(),
		a:              new(app.App),
		tmpDir:         tmpDir,
	}
	fx.a.Register(&testConfig{tmpDir: tmpDir}).Register(fx.storageService)
	for _, b := range beforeStart {
		b(fx)
	}
	require.NoError(t, fx.a.Start(ctx))
	return fx
}

func (fx *fixture) finish(t require.TestingT) {
	require.NoError(t, fx.a.Close(ctx))
	if fx.tmpDir != "" {
		_ = os.RemoveAll(fx.tmpDir)
	}
}

type testConfig struct {
	tmpDir string
}

func (t *testConfig) GetSqliteStorePath() string {
	return filepath.Join(t.tmpDir, "spaceStore.db")
}
func (t *testConfig) GetTempDirPath() string {
	return ""
}

func (t *testConfig) Init(a *app.App) (err error) {
	return nil
}

func (t *testConfig) Name() (name string) {
	return "config"
}
