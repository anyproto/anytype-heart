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
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestUpdateObjectDetails(t *testing.T) {
	t.Run("with nil field expect error", func(t *testing.T) {
		s := newStoreFixture(t)

		err := s.UpdateObjectDetails("id1", &types.Struct{})
		require.Error(t, err)
	})

	t.Run("with empty details expect just id detail is written", func(t *testing.T) {
		s := newStoreFixture(t)

		err := s.UpdateObjectDetails("id1", &types.Struct{Fields: map[string]*types.Value{}})
		require.NoError(t, err)

		want := makeDetails(testObject{
			bundle.RelationKeyId: pbtypes.String("id1"),
		})
		got, err := s.GetDetails("id1")
		require.NoError(t, err)
		assert.Equal(t, want, got.GetDetails())
	})

	t.Run("with no id in details expect id is added on write", func(t *testing.T) {
		s := newStoreFixture(t)

		err := s.UpdateObjectDetails("id1", makeDetails(testObject{
			bundle.RelationKeyName: pbtypes.String("some name"),
		}))
		require.NoError(t, err)

		want := makeDetails(testObject{
			bundle.RelationKeyId:   pbtypes.String("id1"),
			bundle.RelationKeyName: pbtypes.String("some name"),
		})
		got, err := s.GetDetails("id1")
		require.NoError(t, err)
		assert.Equal(t, want, got.GetDetails())
	})

	t.Run("with no existing details try to write nil details and expect nothing is changed", func(t *testing.T) {
		s := newStoreFixture(t)

		err := s.UpdateObjectDetails("id1", nil)
		require.NoError(t, err)

		det, err := s.GetDetails("id1")
		assert.NoError(t, err)
		assert.Equal(t, &types.Struct{Fields: map[string]*types.Value{}}, det.GetDetails())
	})

	t.Run("with existing details write nil details and expect nothing is changed", func(t *testing.T) {
		s := newStoreFixture(t)
		obj := makeObjectWithName("id1", "foo")
		s.addObjects(t, []testObject{obj})

		err := s.UpdateObjectDetails("id1", nil)
		require.NoError(t, err)

		det, err := s.GetDetails("id1")
		assert.NoError(t, err)
		assert.Equal(t, makeDetails(obj), det.GetDetails())
	})

	t.Run("with write same details expect error", func(t *testing.T) {
		s := newStoreFixture(t)
		obj := makeObjectWithName("id1", "foo")
		s.addObjects(t, []testObject{obj})

		err := s.UpdateObjectDetails("id1", makeDetails(obj))
		require.Equal(t, ErrDetailsNotChanged, err)
	})

	t.Run("with updated details just store them", func(t *testing.T) {
		s := newStoreFixture(t)
		obj := makeObjectWithName("id1", "foo")
		s.addObjects(t, []testObject{obj})

		newObj := makeObjectWithNameAndDescription("id1", "foo", "bar")
		err := s.UpdateObjectDetails("id1", makeDetails(newObj))
		require.NoError(t, err)

		det, err := s.GetDetails("id1")
		assert.NoError(t, err)
		assert.Equal(t, makeDetails(newObj), det.GetDetails())
	})
}

func TestSendUpdatesToSubscriptions(t *testing.T) {
	t.Run("with details are not changed expect no updates are sent", func(t *testing.T) {
		s := newStoreFixture(t)
		s.addObjects(t, []testObject{makeObjectWithName("id1", "foo")})

		s.SubscribeForAll(func(rec database.Record) {
			require.Fail(t, "unexpected call")
		})

		err := s.UpdateObjectDetails("id1", makeDetails(makeObjectWithName("id1", "foo")))
		require.Equal(t, ErrDetailsNotChanged, err)
	})

	t.Run("with new details", func(t *testing.T) {
		s := newStoreFixture(t)
		obj := makeObjectWithName("id1", "foo")

		var called int
		s.SubscribeForAll(func(rec database.Record) {
			called++
			assert.Equal(t, makeDetails(obj), rec.Details)
		})

		s.addObjects(t, []testObject{obj})
		assert.Equal(t, 1, called)
	})

	t.Run("with updated details", func(t *testing.T) {
		s := newStoreFixture(t)
		s.addObjects(t, []testObject{makeObjectWithName("id1", "foo")})

		updatedObj := makeObjectWithNameAndDescription("id1", "foobar", "bar")
		var called int
		s.SubscribeForAll(func(rec database.Record) {
			called++
			assert.Equal(t, makeDetails(updatedObj), rec.Details)
		})

		s.addObjects(t, []testObject{updatedObj})
		assert.Equal(t, 1, called)
	})
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

		out, err := s.GetOutboundLinksByID("id1")
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
	in, err := fx.GetInboundLinksByID(id)
	assert.NoError(t, err)
	if len(links) == 0 {
		assert.Empty(t, in)
		return
	}
	assert.Equal(t, links, in)
}

func (fx *storeFixture) assertOutboundLinks(t *testing.T, id string, links []string) {
	out, err := fx.GetOutboundLinksByID(id)
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
