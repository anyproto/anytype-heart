package oldstore

import (
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	types "google.golang.org/protobuf/types/known/structpb"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func newTestService(t *testing.T) *service {
	db, err := badger.Open(badger.DefaultOptions("").WithInMemory(true))
	require.NoError(t, err)

	return &service{
		db: db,
	}
}

func TestGetLocalDetails(t *testing.T) {
	t.Run("no details, return error", func(t *testing.T) {
		s := newTestService(t)

		got, err := s.GetLocalDetails("id1")
		require.Error(t, err)
		require.Nil(t, got)
	})

	t.Run("details exist, return only local details", func(t *testing.T) {
		s := newTestService(t)

		now := time.Now().Unix()

		err := s.SetDetails("id1", &types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():             pbtypes.String("id1"),
				bundle.RelationKeyName.String():           pbtypes.String("name"),
				bundle.RelationKeyLastOpenedDate.String(): pbtypes.Int64(now),
				bundle.RelationKeyIsFavorite.String():     pbtypes.Bool(true),
			},
		})
		require.NoError(t, err)

		got, err := s.GetLocalDetails("id1")
		require.NoError(t, err)

		want := &types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyLastOpenedDate.String(): pbtypes.Int64(now),
				bundle.RelationKeyIsFavorite.String():     pbtypes.Bool(true),
			},
		}

		assert.Equal(t, want, got)

		t.Run("delete", func(t *testing.T) {
			err := s.DeleteDetails("id1")
			require.NoError(t, err)

			got, err := s.GetLocalDetails("id1")
			require.Error(t, err)
			require.Nil(t, got)
		})
	})
}
