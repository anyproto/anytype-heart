package migrator

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/clientds"
	"github.com/anyproto/anytype-heart/space/spacecore/oldstorage"
	"github.com/anyproto/anytype-heart/space/spacecore/storage"
	"github.com/anyproto/anytype-heart/space/spacecore/storage/migratorfinisher"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

type fixture struct {
	migrator *migrator
	app      *app.App
	cfg      *config.Config
	storage  *failingNewStorage
}

type quicPreferenceSetterStub struct {
}

func (q *quicPreferenceSetterStub) Init(a *app.App) (err error) {
	return nil
}

func (q *quicPreferenceSetterStub) Name() (name string) {
	return "quicPreferenceSetterStub"
}

func (q *quicPreferenceSetterStub) PreferQuic(b bool) {
}

func newFixture(t *testing.T, mode storage.SpaceStorageMode) *fixture {
	return newFixtureWithPath(mode, t.TempDir())
}

func newFixtureWithPath(mode storage.SpaceStorageMode, path string) *fixture {
	cfg := config.New()
	cfg.SpaceStorageMode = mode
	cfg.RepoPath = path

	fx := &fixture{
		cfg: cfg,
	}
	return fx
}

type failingNewStorage struct {
	storage.ClientStorage
	err error
}

func newFailingNewStorage(err error) *failingNewStorage {
	return &failingNewStorage{
		ClientStorage: storage.New(),
		err:           err,
	}
}

func (f *failingNewStorage) WaitSpaceStorage(ctx context.Context, id string) (spacestorage.SpaceStorage, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.ClientStorage.WaitSpaceStorage(ctx, id)
}

func (fx *fixture) startWithError(t *testing.T, err error) {
	walletService := wallet.NewWithRepoDirAndRandomKeys(fx.cfg.RepoPath)
	oldStorage := oldstorage.New()
	newStorage := &failingNewStorage{storage.New(), err}
	processService := process.New()
	eventSender := mock_event.NewMockSender(t)
	eventSender.EXPECT().Broadcast(mock.Anything).Run(func(ev *pb.Event) {
	}).Maybe()
	eventSender.EXPECT().BroadcastExceptSessions(mock.Anything, mock.Anything).Run(func(ev *pb.Event, exceptSessions []string) {
		t.Log(ev)
	}).Maybe()

	migrator := New().(*migrator)

	ctx := context.Background()
	testApp := &app.App{}
	testApp.Register(migratorfinisher.New())
	testApp.Register(clientds.New())
	testApp.Register(testutil.PrepareMock(ctx, testApp, eventSender))
	testApp.Register(&quicPreferenceSetterStub{})
	testApp.Register(walletService)
	testApp.Register(fx.cfg)
	testApp.Register(oldStorage)
	testApp.Register(newStorage)
	testApp.Register(processService)
	testApp.Register(migrator)

	fx.app = testApp
	fx.migrator = migrator
	fx.storage = newStorage

	err = testApp.Start(ctx)
	require.NoError(t, err)
}

func (fx *fixture) start(t *testing.T) {
	fx.startWithError(t, nil)
}

func (fx *fixture) stop(t *testing.T) {
	ctx := context.Background()
	err := fx.app.Close(ctx)
	require.NoError(t, err)
}

func assertReports(t *testing.T, reports []*verificationReport) {
	for _, report := range reports {
		for _, err := range report.errors {
			assert.NoError(t, err.err, err.id)
		}
	}
}

func TestMigration(t *testing.T) {
	t.Run("no old storage", func(t *testing.T) {
		fx := newFixture(t, storage.SpaceStorageModeSqlite)

		fx.start(t)
	})

	t.Run("with sqlite, fast verification", func(t *testing.T) {
		fx := newFixture(t, storage.SpaceStorageModeSqlite)

		err := copyFile("testdata/spaceStore.db", fx.cfg.GetOldSpaceStorePath())
		require.NoError(t, err)

		// TODO Test object->space bindings were populated

		fx.start(t)

		reports, err := fx.migrator.verify(context.Background(), true)
		require.NoError(t, err)
		assertReports(t, reports)
	})

	t.Run("with sqlite, load error", func(t *testing.T) {
		// start and verify first migration
		fx := newFixture(t, storage.SpaceStorageModeSqlite)
		err := copyFile("testdata/spaceStore.db", fx.cfg.GetOldSpaceStorePath())
		require.NoError(t, err)
		fx.start(t)
		reports, err := fx.migrator.verify(context.Background(), true)
		require.NoError(t, err)
		assertReports(t, reports)
		fx.stop(t)

		// start and verify second migration where every new storage is "broken"
		otherFx := newFixtureWithPath(storage.SpaceStorageModeSqlite, fx.cfg.RepoPath)
		err = copyFile("testdata/spaceStore.db", fx.cfg.GetOldSpaceStorePath())
		require.NoError(t, err)
		otherFx.startWithError(t, fmt.Errorf("load error"))
		otherFx.storage.err = nil
		reports, err = otherFx.migrator.verify(context.Background(), true)
		require.NoError(t, err)
	})

	t.Run("with sqlite, full verification", func(t *testing.T) {
		fx := newFixture(t, storage.SpaceStorageModeSqlite)

		err := copyFile("testdata/spaceStore.db", fx.cfg.GetOldSpaceStorePath())
		require.NoError(t, err)

		// TODO Test object->space bindings were populated

		fx.start(t)

		reports, err := fx.migrator.verify(context.Background(), false)
		require.NoError(t, err)
		assertReports(t, reports)
	})

	t.Run("with badger, fast verification", func(t *testing.T) {
		fx := newFixture(t, storage.SpaceStorageModeBadger)

		err := copyDir("testdata/badger_spacestore", fx.cfg.GetOldSpaceStorePath())
		require.NoError(t, err)

		// TODO Test object->space bindings were populated

		fx.start(t)

		reports, err := fx.migrator.verify(context.Background(), true)
		require.NoError(t, err)
		assertReports(t, reports)
	})

	t.Run("with badger, full verification", func(t *testing.T) {
		fx := newFixture(t, storage.SpaceStorageModeBadger)

		err := copyDir("testdata/badger_spacestore", fx.cfg.GetOldSpaceStorePath())
		require.NoError(t, err)

		// TODO Test object->space bindings were populated

		fx.start(t)

		reports, err := fx.migrator.verify(context.Background(), false)
		require.NoError(t, err)
		assertReports(t, reports)
	})
}

func copyFile(srcPath string, destPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()
	dest, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer dest.Close()
	_, err = io.Copy(dest, src)
	return err
}

func copyDir(srcPath string, destPath string) error {
	dir, err := os.ReadDir(srcPath)
	if err != nil {
		return err
	}

	err = os.MkdirAll(destPath, os.ModePerm)
	if err != nil {
		return err
	}

	for _, entry := range dir {
		src := filepath.Join(srcPath, entry.Name())
		dst := filepath.Join(destPath, entry.Name())
		err := copyFile(src, dst)
		if err != nil {
			return err
		}
	}
	return nil
}

func TestIsDiskFull(t *testing.T) {
	for _, tc := range []struct {
		inputErr error
		expected bool
	}{
		{nil, false},
		{syscall.ENOSPC, true},
		{os.ErrInvalid, false},
		{syscall.Errno(112), true},
		{syscall.Errno(111), false},
		{fmt.Errorf("disk is full"), true},
	} {
		assert.Equal(t, tc.expected, isDiskFull(tc.inputErr))
	}
}
