package space

import (
	"github.com/dgraph-io/badger/v3"
	"github.com/dgraph-io/ristretto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
)

type fixture struct {
	Service
}

func newFixture(t *testing.T) *fixture {
	db, err := badger.Open(badger.DefaultOptions(filepath.Join(t.TempDir(), "badger")))
	require.NoError(t, err)

	spaceResolverCache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 10_000_000,
		MaxCost:     100_000_000,
		BufferItems: 64,
	})
	require.NoError(t, err)

	s := &service{
		db:                 db,
		spaceResolverCache: spaceResolverCache,
	}
	return &fixture{
		Service: s,
	}
}

func TestResolveSpaceID(t *testing.T) {
	s := newFixture(t)

	err := s.StoreSpaceID("object1", "space1")
	require.NoError(t, err)

	err = s.StoreSpaceID("object2", "space2")
	require.NoError(t, err)

	got, err := s.ResolveSpaceID("object1")
	require.NoError(t, err)
	assert.Equal(t, "space1", got)

	got, err = s.ResolveSpaceID("object2")
	require.NoError(t, err)
	assert.Equal(t, "space2", got)

	_, err = s.ResolveSpaceID("object3")
	require.Error(t, err)
}
