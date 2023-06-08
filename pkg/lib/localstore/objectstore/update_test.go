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

func TestUpdateObjectLinks(t *testing.T) {
	t.Run("with no links added", func(t *testing.T) {
		s := newStoreFixture(t)

		err := s.UpdateObjectLinks("id1", []string{})
		require.NoError(t, err)

		out, err := s.GetOutboundLinksById("id1")
		require.NoError(t, err)
		assert.Empty(t, out)
	})

	t.Run("with some links added", func(t *testing.T) {
		s := newStoreFixture(t)

		err := s.UpdateObjectLinks("id1", []string{"id2", "id3"})
		require.NoError(t, err)

		s.assertOutboundLinks(t, "id1", []string{"id2", "id3"})
		s.assertInboundLinks(t, "id2", []string{"id1"})
		s.assertInboundLinks(t, "id3", []string{"id1"})
	})

	t.Run("with some existing links, add new links", func(t *testing.T) {
		s := newStoreFixture(t)

		s.givenExistingLinks(t)

		err := s.UpdateObjectLinks("id1", []string{"id2", "id3"})
		require.NoError(t, err)

		s.assertOutboundLinks(t, "id1", []string{"id2", "id3"})
		s.assertInboundLinks(t, "id2", []string{"id1"})
		s.assertInboundLinks(t, "id3", []string{"id1"})
	})

	t.Run("with some existing links, remove links", func(t *testing.T) {
		s := newStoreFixture(t)

		s.givenExistingLinks(t)

		err := s.UpdateObjectLinks("id1", []string{})
		require.NoError(t, err)

		s.assertOutboundLinks(t, "id1", nil)
		s.assertInboundLinks(t, "id2", nil)
		s.assertInboundLinks(t, "id3", nil)
	})
}

func (fx *storeFixture) assertInboundLinks(t *testing.T, id string, links []string) {
	in, err := fx.GetInboundLinksById(id)
	assert.NoError(t, err)
	if len(links) == 0 {
		assert.Empty(t, in)
		return
	}
	assert.Equal(t, links, in)
}

func (fx *storeFixture) assertOutboundLinks(t *testing.T, id string, links []string) {
	out, err := fx.GetOutboundLinksById(id)
	assert.NoError(t, err)
	if len(links) == 0 {
		assert.Empty(t, out)
		return
	}
	assert.Equal(t, links, out)
}

func (fx *storeFixture) givenExistingLinks(t *testing.T) {
	err := fx.UpdateObjectLinks("id1", []string{"id2"})
	require.NoError(t, err)

	fx.assertOutboundLinks(t, "id1", []string{"id2"})
	fx.assertInboundLinks(t, "id2", []string{"id1"})
	fx.assertInboundLinks(t, "id3", nil)
}
