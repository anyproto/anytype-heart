package anystorage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/commonspace/spacepayloads"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/go-sqlite"
	"github.com/anyproto/go-sqlite/sqlitex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/space/spacedomain"
)

func newCreatePayload(t *testing.T) spacestorage.SpaceStorageCreatePayload {
	signKey, _, err := crypto.GenerateRandomEd25519KeyPair()
	require.NoError(t, err)
	masterKey, _, err := crypto.GenerateRandomEd25519KeyPair()
	require.NoError(t, err)
	metaKey, _, err := crypto.GenerateRandomEd25519KeyPair()
	require.NoError(t, err)
	payload, err := spacepayloads.StoragePayloadForSpaceCreate(spacepayloads.SpaceCreatePayload{
		SigningKey:     signKey,
		MasterKey:      masterKey,
		ReadKey:        crypto.NewAES(),
		MetadataKey:    metaKey,
		SpaceType:      string(spacedomain.SpaceTypeRegular),
		ReplicationKey: 1,
		Metadata:       []byte("metadata"),
	})
	require.NoError(t, err)
	return payload
}

// waitForFile busy-waits until path exists, so the reader lands inside the
// window any-store leaves open between SQLite creating store.db and the
// `PRAGMA user_version = 2` that stamps it. It gives up on done or on the
// deadline, so a create that fails before it ever makes the file ends the test
// with its own error instead of spinning until the go test timeout.
func waitForFile(t *testing.T, path string, done <-chan struct{}) bool {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		select {
		case <-done:
			return false
		default:
		}
		if _, err := os.Stat(path); err == nil {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		runtime.Gosched()
	}
}

// A space store is created by one goroutine while others open the same space:
// every LAN handshake derives discovery keys over AllSpaceIds, and space
// loaders open stores independently of whoever is creating them. The store
// under construction must never be taken for a corrupted one.
func TestCreateSpaceStorage_ConcurrentOpen(t *testing.T) {
	t.Run("an open racing the creator never backs up the live store", func(t *testing.T) {
		// given
		const rounds = 10
		s := newTestService(t)
		ctx := context.Background()

		for i := 0; i < rounds; i++ {
			payload := newCreatePayload(t)
			spaceId := payload.SpaceHeaderWithId.Id
			dbPath := filepath.Join(s.rootPath, spaceId, "store.db")

			done := make(chan struct{})
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				if !waitForFile(t, dbPath, done) {
					return
				}
				for {
					select {
					case <-done:
						return
					default:
					}
					st, err := s.WaitSpaceStorage(ctx, spaceId)
					if err == nil {
						_ = st.Close(ctx)
					}
				}
			}()

			// when
			st, err := s.CreateSpaceStorage(ctx, payload)
			close(done)
			wg.Wait()

			// then
			require.NoError(t, err, "round %d: creating a space storage must not fail because another goroutine opened it", i)
			require.NoError(t, st.Close(ctx))
			assert.True(t, s.SpaceExists(spaceId), "round %d: the created space dir must still be in place", i)
		}
		assert.Empty(t, s.ListCorruptedBackups(), "a store being created must never be classified as corrupted")
	})

	t.Run("an open racing the creator returns the created storage", func(t *testing.T) {
		// given
		const rounds = 10
		s := newTestService(t)
		ctx := context.Background()

		for i := 0; i < rounds; i++ {
			payload := newCreatePayload(t)
			spaceId := payload.SpaceHeaderWithId.Id
			dbPath := filepath.Join(s.rootPath, spaceId, "store.db")

			createErr := make(chan error, 1)
			createDone := make(chan struct{})
			go func() {
				defer close(createDone)
				st, err := s.CreateSpaceStorage(ctx, payload)
				if err == nil {
					err = st.Close(ctx)
				}
				createErr <- err
			}()

			sawFile := waitForFile(t, dbPath, createDone)

			// when
			st, err := s.WaitSpaceStorage(ctx, spaceId)

			// then
			<-createDone
			require.NoError(t, <-createErr, "round %d", i)
			if !sawFile && errors.Is(err, spacestorage.ErrSpaceStorageMissing) {
				// we looked before the creator had put anything on disk, so
				// nothing raced this round and missing is the honest answer
				continue
			}
			require.NoError(t, err, "round %d: an open that races the creator must join it, not report the space missing", i)
			require.NoError(t, st.Close(ctx))
		}
		assert.Empty(t, s.ListCorruptedBackups())
	})
}

// A create interrupted before any-store stamped `user_version` leaves a
// store.db that reads back as version 0. A derived space (tech, personal) is
// created rather than fetched from peers, so the create itself has to be able
// to get past it — otherwise the account never bootstraps again.
func TestCreateSpaceStorage_UnstampedLeftover(t *testing.T) {
	t.Run("a never-stamped store is moved aside and the space is created", func(t *testing.T) {
		// given
		s := newTestService(t)
		ctx := context.Background()
		payload := newCreatePayload(t)
		spaceId := payload.SpaceHeaderWithId.Id
		dirPath := filepath.Join(s.rootPath, spaceId)
		require.NoError(t, os.MkdirAll(dirPath, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dirPath, "store.db"), nil, 0644))

		// when
		st, err := s.CreateSpaceStorage(ctx, payload)

		// then
		require.NoError(t, err)
		require.Equal(t, spaceId, st.Id())
		require.NoError(t, st.Close(ctx))

		backups := s.ListCorruptedBackups()
		require.Len(t, backups, 1)
		assert.Equal(t, spaceId, backups[0].SpaceId)

		// the space is usable from now on, without another backup
		reopened, err := s.WaitSpaceStorage(ctx, spaceId)
		require.NoError(t, err)
		require.NoError(t, reopened.Close(ctx))
		assert.Len(t, s.ListCorruptedBackups(), 1)
	})

	t.Run("an existing valid store is kept, not backed up", func(t *testing.T) {
		// given
		s := newTestService(t)
		ctx := context.Background()
		payload := newCreatePayload(t)
		spaceId := payload.SpaceHeaderWithId.Id
		st, err := s.CreateSpaceStorage(ctx, payload)
		require.NoError(t, err)
		require.NoError(t, st.Close(ctx))

		// when: Derive creates before it loads, so an account-create retry runs
		// the create again over the store the previous attempt left
		_, err = s.CreateSpaceStorage(ctx, payload)

		// then
		require.ErrorIs(t, err, spacestorage.ErrSpaceStorageExists)
		assert.Empty(t, s.ListCorruptedBackups(), "a healthy store must never be moved aside")
		assert.True(t, s.SpaceExists(spaceId))
	})
}

// setUserVersion stamps an arbitrary schema version on an existing store, the
// state a store written by an incompatible any-store would be in.
func setUserVersion(t *testing.T, dbPath string, version int) {
	t.Helper()
	conn, err := sqlite.OpenConn(dbPath, sqlite.OpenReadWrite|sqlite.OpenWAL|sqlite.OpenURI)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, conn.Close())
	}()
	require.NoError(t, sqlitex.ExecuteTransient(conn, fmt.Sprintf("PRAGMA user_version = %d", version), nil))
}

// The recovery in createDb moves a space directory aside, so it must fire only
// for a store that provably holds nothing. IsCorruptedError is far broader --
// it also covers ErrQuickCheckFailed, which any-store returns for any error the
// check hit, a cancelled context included.
func TestCreateSpaceStorage_RecoveryIsNarrow(t *testing.T) {
	t.Run("a populated store with a foreign version is left alone", func(t *testing.T) {
		// given
		s := newTestService(t)
		ctx := context.Background()
		payload := newCreatePayload(t)
		spaceId := payload.SpaceHeaderWithId.Id
		st, err := s.CreateSpaceStorage(ctx, payload)
		require.NoError(t, err)
		require.NoError(t, st.Close(ctx))
		dbPath := filepath.Join(s.rootPath, spaceId, "store.db")
		setUserVersion(t, dbPath, 99)

		// when
		_, err = s.CreateSpaceStorage(ctx, payload)

		// then
		require.ErrorIs(t, err, anystore.ErrIncompatibleVersion)
		assert.Empty(t, s.ListCorruptedBackups(), "a store holding data must never be moved aside")
		_, statErr := os.Stat(dbPath)
		assert.NoError(t, statErr, "the store must stay where it is")
	})

	t.Run("an unstamped store holding tables is left alone", func(t *testing.T) {
		// given: version 0 alone must not be enough to move a directory aside
		s := newTestService(t)
		ctx := context.Background()
		payload := newCreatePayload(t)
		spaceId := payload.SpaceHeaderWithId.Id
		dirPath := filepath.Join(s.rootPath, spaceId)
		require.NoError(t, os.MkdirAll(dirPath, 0755))
		dbPath := filepath.Join(dirPath, "store.db")
		conn, err := sqlite.OpenConn(dbPath, sqlite.OpenCreate|sqlite.OpenReadWrite|sqlite.OpenWAL|sqlite.OpenURI)
		require.NoError(t, err)
		require.NoError(t, sqlitex.ExecuteTransient(conn, "CREATE TABLE somebodys_data (v TEXT)", nil))
		require.NoError(t, sqlitex.ExecuteTransient(conn, "PRAGMA user_version = 0", nil))
		require.NoError(t, conn.Close())

		// when
		_, err = s.CreateSpaceStorage(ctx, payload)

		// then
		require.ErrorIs(t, err, anystore.ErrIncompatibleVersion)
		assert.Empty(t, s.ListCorruptedBackups(), "only a store with nothing in it may be moved aside")
		_, statErr := os.Stat(dbPath)
		assert.NoError(t, statErr)
	})

	t.Run("a backup directory is not offered as a space", func(t *testing.T) {
		// given: the state left behind once a store has been backed up
		s := newTestService(t)
		ctx := context.Background()
		payload := newCreatePayload(t)
		spaceId := payload.SpaceHeaderWithId.Id
		dirPath := filepath.Join(s.rootPath, spaceId)
		require.NoError(t, os.MkdirAll(dirPath, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dirPath, "store.db"), nil, 0644))
		st, err := s.CreateSpaceStorage(ctx, payload)
		require.NoError(t, err)
		require.NoError(t, st.Close(ctx))
		require.Len(t, s.ListCorruptedBackups(), 1)

		// when
		ids, err := s.AllSpaceIds()

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{spaceId}, ids, "discovery must not be handed backup dirs to open")
	})
}

// SQLite drops the WAL of a zero-page main file the moment the db is opened, so
// a store truncated away with its WAL still holding data has to be preserved
// before anything opens it.
func TestOrphanWal(t *testing.T) {
	writeOrphanWal := func(t *testing.T, dirPath string) (dbPath string, walBytes int64) {
		t.Helper()
		require.NoError(t, os.MkdirAll(dirPath, 0755))
		dbPath = filepath.Join(dirPath, "store.db")
		require.NoError(t, os.WriteFile(dbPath, nil, 0644))
		wal := []byte("not really a wal, but not empty either")
		require.NoError(t, os.WriteFile(dbPath+"-wal", wal, 0644))
		return dbPath, int64(len(wal))
	}

	t.Run("an open preserves the pair instead of dropping the wal", func(t *testing.T) {
		// given
		s := newTestService(t)
		const spaceId = "space1"
		_, walBytes := writeOrphanWal(t, filepath.Join(s.rootPath, spaceId))

		// when
		_, err := s.WaitSpaceStorage(context.Background(), spaceId)

		// then
		require.ErrorIs(t, err, spacestorage.ErrSpaceStorageMissing)
		backups := s.ListCorruptedBackups()
		require.Len(t, backups, 1)
		kept, statErr := os.Stat(filepath.Join(backups[0].BackupPath, "store.db-wal"))
		require.NoError(t, statErr, "the wal must survive in the backup")
		assert.Equal(t, walBytes, kept.Size())
	})

	t.Run("a create preserves the pair and still makes the space", func(t *testing.T) {
		// given
		s := newTestService(t)
		ctx := context.Background()
		payload := newCreatePayload(t)
		spaceId := payload.SpaceHeaderWithId.Id
		_, walBytes := writeOrphanWal(t, filepath.Join(s.rootPath, spaceId))

		// when
		st, err := s.CreateSpaceStorage(ctx, payload)

		// then
		require.NoError(t, err)
		require.NoError(t, st.Close(ctx))
		backups := s.ListCorruptedBackups()
		require.Len(t, backups, 1)
		kept, statErr := os.Stat(filepath.Join(backups[0].BackupPath, "store.db-wal"))
		require.NoError(t, statErr, "the wal must survive in the backup")
		assert.Equal(t, walBytes, kept.Size())
	})

	t.Run("a healthy store is untouched", func(t *testing.T) {
		// given: a live store carries a page in its main file, so it can never
		// look like the truncated pair above
		s := newTestService(t)
		ctx := context.Background()
		payload := newCreatePayload(t)
		st, err := s.CreateSpaceStorage(ctx, payload)
		require.NoError(t, err)
		require.NoError(t, st.Close(ctx))

		// when
		reopened, err := s.WaitSpaceStorage(ctx, payload.SpaceHeaderWithId.Id)

		// then
		require.NoError(t, err)
		require.NoError(t, reopened.Close(ctx))
		assert.Empty(t, s.ListCorruptedBackups())
	})
}

// collidingPayloads mints real space payloads until two of them land on one
// lock shard. Space ids are CIDs, so a collision has to be found rather than
// chosen; the birthday bound makes that a few dozen tries.
func collidingPayloads(t *testing.T) (a, b spacestorage.SpaceStorageCreatePayload) {
	t.Helper()
	seen := map[int]spacestorage.SpaceStorageCreatePayload{}
	for i := 0; i < 20000; i++ {
		p := newCreatePayload(t)
		shard := spaceLockShard(p.SpaceHeaderWithId.Id)
		if prev, ok := seen[shard]; ok {
			return prev, p
		}
		seen[shard] = p
	}
	t.Fatal("no two payloads landed on one shard")
	return
}

// A shard is reused: one space takes it, finishes and hands it back, and a
// different space that hashes to the same shard takes it next. Reuse must not
// weaken what the lock is for -- two callers for ONE id still have to be
// serialized, because one id always maps to one shard.
func TestCreateSpaceStorage_ShardReuse(t *testing.T) {
	// given: two spaces sharing a shard, the first already created and closed
	// so the shard is back in the pool
	s := newTestService(t)
	ctx := context.Background()
	first, second := collidingPayloads(t)
	require.Equal(t, spaceLockShard(first.SpaceHeaderWithId.Id), spaceLockShard(second.SpaceHeaderWithId.Id))

	firstStore, err := s.CreateSpaceStorage(ctx, first)
	require.NoError(t, err)
	require.NoError(t, firstStore.Close(ctx))

	// when: the second space is created while others open it, on that same shard
	spaceId := second.SpaceHeaderWithId.Id
	dbPath := filepath.Join(s.rootPath, spaceId, "store.db")
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if !waitForFile(t, dbPath, done) {
			return
		}
		for {
			select {
			case <-done:
				return
			default:
			}
			if st, err := s.WaitSpaceStorage(ctx, spaceId); err == nil {
				_ = st.Close(ctx)
			}
		}
	}()
	secondStore, err := s.CreateSpaceStorage(ctx, second)
	close(done)
	wg.Wait()

	// then
	require.NoError(t, err, "a reused shard must still serialize the create it is asked to protect")
	require.NoError(t, secondStore.Close(ctx))
	assert.Empty(t, s.ListCorruptedBackups())
	assert.True(t, s.SpaceExists(spaceId))
	// the space that handed the shard over is untouched
	assert.True(t, s.SpaceExists(first.SpaceHeaderWithId.Id))
	reopened, err := s.WaitSpaceStorage(ctx, first.SpaceHeaderWithId.Id)
	require.NoError(t, err)
	require.NoError(t, reopened.Close(ctx))
}
