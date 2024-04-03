package sqlitestorage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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
		require.NoError(t, fx.db.QueryRow(q, spaceId).Scan(&count))
		assert.Equal(t, 0, count)
	}

	expect0("SELECT COUNT(*) FROM spaces WHERE id = ?")
	expect0("SELECT COUNT(*) FROM trees WHERE spaceId = ?")
	expect0("SELECT COUNT(*) FROM changes WHERE spaceId = ?")
	expect0("SELECT COUNT(*) FROM binds WHERE spaceId = ?")

}

type fixture struct {
	*storageService
	a      *app.App
	tmpDir string
}

func newFixture(t *testing.T) *fixture {
	tmpDir, e := os.MkdirTemp("", "")
	require.NoError(t, e)
	fx := &fixture{
		storageService: New().(*storageService),
		a:              new(app.App),
		tmpDir:         tmpDir,
	}
	fx.a.Register(&testConfig{tmpDir: tmpDir}).Register(fx.storageService)
	require.NoError(t, fx.a.Start(ctx))
	return fx
}

func (fx *fixture) finish(t *testing.T) {
	require.NoError(t, fx.a.Close(ctx))
	if fx.tmpDir != "" {
		_ = os.RemoveAll(fx.tmpDir)
	}
}

type testConfig struct {
	tmpDir string
}

func (t *testConfig) GetSpaceStorePath() string {
	return filepath.Join(t.tmpDir, "spaceStore.db")
}

func (t *testConfig) Init(a *app.App) (err error) {
	return nil
}

func (t *testConfig) Name() (name string) {
	return "config"
}
