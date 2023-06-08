package objectstore

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (fx *storeFixture) givenPendingLocalDetails(t *testing.T) {
	err := fx.UpdatePendingLocalDetails("id1", func(details *types.Struct) (*types.Struct, error) {
		details.Fields[bundle.RelationKeyIsFavorite.String()] = pbtypes.Bool(true)
		return details, nil
	})
	require.NoError(t, err)
}

func (fx *storeFixture) assertPendingLocalDetails(t *testing.T) {
	err := fx.UpdatePendingLocalDetails("id1", func(details *types.Struct) (*types.Struct, error) {
		assert.Equal(t, &types.Struct{
			Fields: map[string]*types.Value{
				// ID is added automatically
				bundle.RelationKeyId.String():         pbtypes.String("id1"),
				bundle.RelationKeyIsFavorite.String(): pbtypes.Bool(true),
			},
		}, details)
		return details, nil
	})
	require.NoError(t, err)
}

func TestUpdatePendingLocalDetails(t *testing.T) {
	t.Run("with error in process function expect previous details are not touched", func(t *testing.T) {
		s := newStoreFixture(t)
		s.givenPendingLocalDetails(t)

		err := s.UpdatePendingLocalDetails("id1", func(details *types.Struct) (*types.Struct, error) {
			return nil, fmt.Errorf("serious error")
		})
		require.Error(t, err)

		s.assertPendingLocalDetails(t)
	})

	t.Run("with empty pending details", func(t *testing.T) {
		s := newStoreFixture(t)

		s.givenPendingLocalDetails(t)

		s.assertPendingLocalDetails(t)
	})

	t.Run("with nil result of process function expect delete pending details", func(t *testing.T) {
		s := newStoreFixture(t)
		s.givenPendingLocalDetails(t)

		err := s.UpdatePendingLocalDetails("id1", func(details *types.Struct) (*types.Struct, error) {
			return nil, nil
		})
		require.NoError(t, err)

		err = s.UpdatePendingLocalDetails("id1", func(details *types.Struct) (*types.Struct, error) {
			assert.Equal(t, &types.Struct{Fields: map[string]*types.Value{}}, details)
			return nil, nil
		})
		require.NoError(t, err)
	})

	t.Run("with parallel updates expect that transaction conflicts are resolved: last write wins", func(t *testing.T) {
		s := newStoreFixture(t)

		var lastOpenedDate int64
		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				err := s.UpdatePendingLocalDetails("id1", func(details *types.Struct) (*types.Struct, error) {
					now := time.Now().UnixNano()
					atomic.StoreInt64(&lastOpenedDate, now)
					details.Fields[bundle.RelationKeyLastOpenedDate.String()] = pbtypes.Int64(now)
					return details, nil
				})
				require.NoError(t, err)
				wg.Done()
			}()
		}
		wg.Wait()

		err := s.UpdatePendingLocalDetails("id1", func(details *types.Struct) (*types.Struct, error) {
			assert.Equal(t, &types.Struct{
				Fields: map[string]*types.Value{
					// ID is added automatically
					bundle.RelationKeyId.String():             pbtypes.String("id1"),
					bundle.RelationKeyLastOpenedDate.String(): pbtypes.Int64(atomic.LoadInt64(&lastOpenedDate)),
				},
			}, details)
			return details, nil
		})
		require.NoError(t, err)
	})
}
