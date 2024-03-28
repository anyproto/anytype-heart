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
