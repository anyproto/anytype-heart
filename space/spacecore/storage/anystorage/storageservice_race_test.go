package anystorage

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/anyproto/any-sync/commonspace/spacepayloads"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/anyproto/any-sync/util/crypto"
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
// `PRAGMA user_version = 2` that stamps it.
func waitForFile(t *testing.T, path string, done <-chan struct{}) bool {
	t.Helper()
	for {
		select {
		case <-done:
			return false
		default:
		}
		if _, err := os.Stat(path); err == nil {
			return true
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
			go func() {
				st, err := s.CreateSpaceStorage(ctx, payload)
				if err == nil {
					err = st.Close(ctx)
				}
				createErr <- err
			}()

			require.True(t, waitForFile(t, dbPath, nil))

			// when
			st, err := s.WaitSpaceStorage(ctx, spaceId)

			// then
			require.NoError(t, <-createErr, "round %d", i)
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

		// when: derived spaces re-run the create on every bootstrap
		_, err = s.CreateSpaceStorage(ctx, payload)

		// then
		require.ErrorIs(t, err, spacestorage.ErrSpaceStorageExists)
		assert.Empty(t, s.ListCorruptedBackups(), "a healthy store must never be moved aside")
		assert.True(t, s.SpaceExists(spaceId))
	})
}
