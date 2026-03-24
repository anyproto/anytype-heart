package spaceindex

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
)

func TestUpdateObjectDetails(t *testing.T) {
	t.Run("with nil field expect error", func(t *testing.T) {
		s := NewStoreFixture(t)

		err := s.UpdateObjectDetails(context.Background(), "id1", nil)
		require.Error(t, err)
	})

	t.Run("with empty details expect error", func(t *testing.T) {
		s := NewStoreFixture(t)

		err := s.UpdateObjectDetails(context.Background(), "id1", domain.NewDetails())
		require.Error(t, err)
	})

	t.Run("with no id in details expect id is added on write", func(t *testing.T) {
		s := NewStoreFixture(t)

		err := s.UpdateObjectDetails(context.Background(), "id1", makeDetails(TestObject{
			bundle.RelationKeyName: domain.String("some name"),
		}))
		require.NoError(t, err)

		want := makeDetails(TestObject{
			bundle.RelationKeyId:   domain.String("id1"),
			bundle.RelationKeyName: domain.String("some name"),
		})
		got, err := s.GetDetails("id1")
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("with no existing details try to write nil details and expect nothing is changed", func(t *testing.T) {
		s := NewStoreFixture(t)

		err := s.UpdateObjectDetails(context.Background(), "id1", nil)
		require.Error(t, err)

		det, err := s.GetDetails("id1")
		assert.NoError(t, err)
		assert.Equal(t, domain.NewDetails(), det)
	})

	t.Run("with existing details write nil details and expect nothing is changed", func(t *testing.T) {
		s := NewStoreFixture(t)
		obj := makeObjectWithName("id1", "foo")
		s.AddObjects(t, []TestObject{obj})

		err := s.UpdateObjectDetails(context.Background(), "id1", nil)
		require.Error(t, err)

		det, err := s.GetDetails("id1")
		assert.NoError(t, err)
		assert.Equal(t, makeDetails(obj), det)
	})

	t.Run("with write same details expect no error", func(t *testing.T) {
		s := NewStoreFixture(t)
		obj := makeObjectWithName("id1", "foo")
		s.AddObjects(t, []TestObject{obj})

		err := s.UpdateObjectDetails(context.Background(), "id1", makeDetails(obj))
		require.NoError(t, err)
	})

	t.Run("with updated details just store them", func(t *testing.T) {
		s := NewStoreFixture(t)
		obj := makeObjectWithName("id1", "foo")
		s.AddObjects(t, []TestObject{obj})

		newObj := makeObjectWithNameAndDescription("id1", "foo", "bar")
		err := s.UpdateObjectDetails(context.Background(), "id1", makeDetails(newObj))
		require.NoError(t, err)

		det, err := s.GetDetails("id1")
		assert.NoError(t, err)
		assert.Equal(t, makeDetails(newObj), det)
	})
}

func TestSendUpdatesToSubscriptions(t *testing.T) {
	t.Run("with details are not changed expect no updates are sent", func(t *testing.T) {
		s := NewStoreFixture(t)
		s.AddObjects(t, []TestObject{makeObjectWithName("id1", "foo")})

		s.SubscribeForAll(func(rec database.Record) {
			require.Fail(t, "unexpected call")
		})

		err := s.UpdateObjectDetails(context.Background(), "id1", makeDetails(makeObjectWithName("id1", "foo")))
		require.NoError(t, err)
	})

	t.Run("with new details", func(t *testing.T) {
		s := NewStoreFixture(t)
		obj := makeObjectWithName("id1", "foo")

		var called int
		s.SubscribeForAll(func(rec database.Record) {
			called++
			assert.Equal(t, makeDetails(obj), rec.Details)
		})

		s.AddObjects(t, []TestObject{obj})
		assert.Equal(t, 1, called)
	})

	t.Run("with updated details", func(t *testing.T) {
		s := NewStoreFixture(t)
		s.AddObjects(t, []TestObject{makeObjectWithName("id1", "foo")})

		updatedObj := makeObjectWithNameAndDescription("id1", "foobar", "bar")
		var called int
		s.SubscribeForAll(func(rec database.Record) {
			called++
			assert.Equal(t, makeDetails(updatedObj), rec.Details)
		})

		s.AddObjects(t, []TestObject{updatedObj})
		assert.Equal(t, 1, called)
	})
}

func TestUpdatePendingLocalDetails(t *testing.T) {
	t.Run("with error in process function expect previous details are not touched", func(t *testing.T) {
		s := NewStoreFixture(t)
		s.givenPendingLocalDetails(t)

		err := s.UpdatePendingLocalDetails("id1", func(details *domain.Details) (*domain.Details, error) {
			return nil, fmt.Errorf("serious error")
		})
		require.Error(t, err)

		s.assertPendingLocalDetails(t)
	})

	t.Run("with empty pending details", func(t *testing.T) {
		s := NewStoreFixture(t)

		s.givenPendingLocalDetails(t)

		s.assertPendingLocalDetails(t)
	})

	t.Run("with nil result of process function expect delete pending details", func(t *testing.T) {
		s := NewStoreFixture(t)
		s.givenPendingLocalDetails(t)

		err := s.UpdatePendingLocalDetails("id1", func(details *domain.Details) (*domain.Details, error) {
			return nil, nil
		})
		require.NoError(t, err)

		err = s.UpdatePendingLocalDetails("id1", func(details *domain.Details) (*domain.Details, error) {
			assert.Equal(t, domain.NewDetails().SetString("id", "id1"), details)
			return nil, nil
		})
		require.NoError(t, err)
	})

	t.Run("with parallel updates expect that transaction conflicts are resolved: last write wins", func(t *testing.T) {
		s := NewStoreFixture(t)

		var lastOpenedDate int64
		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				err := s.UpdatePendingLocalDetails("id1", func(details *domain.Details) (*domain.Details, error) {
					now := time.Now().UnixNano()
					atomic.StoreInt64(&lastOpenedDate, now)
					details.Set(bundle.RelationKeyLastOpenedDate, domain.Int64(now))
					return details, nil
				})
				require.NoError(t, err)
				wg.Done()
			}()
		}
		wg.Wait()

		err := s.UpdatePendingLocalDetails("id1", func(details *domain.Details) (*domain.Details, error) {
			assert.Equal(t, domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				// ID is added automatically
				bundle.RelationKeyId:             domain.String("id1"),
				bundle.RelationKeyLastOpenedDate: domain.Int64(atomic.LoadInt64(&lastOpenedDate)),
			}), details)
			return details, nil
		})
		require.NoError(t, err)
	})
}

func (fx *StoreFixture) givenPendingLocalDetails(t *testing.T) {
	err := fx.UpdatePendingLocalDetails("id1", func(details *domain.Details) (*domain.Details, error) {
		details.Set(bundle.RelationKeyIsFavorite, domain.Bool(true))
		return details, nil
	})
	require.NoError(t, err)
}

func (fx *StoreFixture) assertPendingLocalDetails(t *testing.T) {
	err := fx.UpdatePendingLocalDetails("id1", func(details *domain.Details) (*domain.Details, error) {
		assert.Equal(t, domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			// ID is added automatically
			bundle.RelationKeyId:         domain.String("id1"),
			bundle.RelationKeyIsFavorite: domain.Bool(true),
		}), details)
		return details, nil
	})
	require.NoError(t, err)
}

func TestUpdateObjectLinks(t *testing.T) {
	t.Run("with no links added", func(t *testing.T) {
		s := NewStoreFixture(t)
		ctx := context.Background()

		err := s.UpdateObjectLinks(ctx, "id1", []string{})
		require.NoError(t, err)

		out, err := s.GetOutboundLinksById("id1")
		require.NoError(t, err)
		assert.Empty(t, out)
	})

	t.Run("with some links added", func(t *testing.T) {
		s := NewStoreFixture(t)
		ctx := context.Background()

		err := s.UpdateObjectLinks(ctx, "id1", []string{"id2", "id3"})
		require.NoError(t, err)

		s.assertOutboundLinks(t, "id1", []string{"id2", "id3"})
		s.assertInboundLinks(t, "id2", []string{"id1"})
		s.assertInboundLinks(t, "id3", []string{"id1"})
	})

	t.Run("with some existing links, add new links", func(t *testing.T) {
		s := NewStoreFixture(t)
		ctx := context.Background()

		s.givenExistingLinks(t)

		err := s.UpdateObjectLinks(ctx, "id1", []string{"id2", "id3"})
		require.NoError(t, err)

		s.assertOutboundLinks(t, "id1", []string{"id2", "id3"})
		s.assertInboundLinks(t, "id2", []string{"id1"})
		s.assertInboundLinks(t, "id3", []string{"id1"})
	})

	t.Run("with some existing links, remove links", func(t *testing.T) {
		s := NewStoreFixture(t)
		ctx := context.Background()

		s.givenExistingLinks(t)

		err := s.UpdateObjectLinks(ctx, "id1", []string{})
		require.NoError(t, err)

		s.assertOutboundLinks(t, "id1", nil)
		s.assertInboundLinks(t, "id2", nil)
		s.assertInboundLinks(t, "id3", nil)
	})
}

func TestUpdateObjectLinksDetailed(t *testing.T) {
	t.Run("detailed info is stored and retrieved", func(t *testing.T) {
		s := NewStoreFixture(t)
		ctx := context.Background()

		err := s.UpdateObjectLinksDetailed(ctx, "obj1", []OutgoingLink{
			{TargetID: "file1", BlockID: "block1"},
			{TargetID: "obj2", RelationKey: "assignee"},
		})
		require.NoError(t, err)

		// Check outbound detailed
		out, err := s.GetOutboundLinksDetailedById("obj1")
		require.NoError(t, err)
		require.Len(t, out, 2)
		assert.Equal(t, OutgoingLink{TargetID: "file1", BlockID: "block1"}, out[0])
		assert.Equal(t, OutgoingLink{TargetID: "obj2", RelationKey: "assignee"}, out[1])

		// Check inbound detailed
		in, err := s.GetInboundLinksDetailedById("file1")
		require.NoError(t, err)
		require.Len(t, in, 1)
		assert.Equal(t, IncomingLink{SourceID: "obj1", BlockID: "block1"}, in[0])

		in, err = s.GetInboundLinksDetailedById("obj2")
		require.NoError(t, err)
		require.Len(t, in, 1)
		assert.Equal(t, IncomingLink{SourceID: "obj1", RelationKey: "assignee"}, in[0])
	})

	t.Run("block id updated when targets stay the same", func(t *testing.T) {
		s := NewStoreFixture(t)
		ctx := context.Background()

		// Initial index: file1 linked from block1
		err := s.UpdateObjectLinksDetailed(ctx, "obj1", []OutgoingLink{
			{TargetID: "file1", BlockID: "block1"},
		})
		require.NoError(t, err)

		// Re-index: same target, but block changed (block was replaced)
		err = s.UpdateObjectLinksDetailed(ctx, "obj1", []OutgoingLink{
			{TargetID: "file1", BlockID: "block2"},
		})
		require.NoError(t, err)

		// Block ID must reflect the new value
		in, err := s.GetInboundLinksDetailedById("file1")
		require.NoError(t, err)
		require.Len(t, in, 1)
		assert.Equal(t, "block2", in[0].BlockID)
	})

	t.Run("relation key updated when targets stay the same", func(t *testing.T) {
		s := NewStoreFixture(t)
		ctx := context.Background()

		// Initial index: obj2 linked via "assignee" relation
		err := s.UpdateObjectLinksDetailed(ctx, "obj1", []OutgoingLink{
			{TargetID: "obj2", RelationKey: "assignee"},
		})
		require.NoError(t, err)

		// Re-index: same target, but relation changed
		err = s.UpdateObjectLinksDetailed(ctx, "obj1", []OutgoingLink{
			{TargetID: "obj2", RelationKey: "creator"},
		})
		require.NoError(t, err)

		in, err := s.GetInboundLinksDetailedById("obj2")
		require.NoError(t, err)
		require.Len(t, in, 1)
		assert.Equal(t, "creator", in[0].RelationKey)
	})

	t.Run("upgrade from simple to detailed links", func(t *testing.T) {
		s := NewStoreFixture(t)
		ctx := context.Background()

		// Simulate old-style indexing (no detailed info)
		err := s.UpdateObjectLinks(ctx, "obj1", []string{"file1"})
		require.NoError(t, err)

		// Now re-index with detailed info, same targets
		err = s.UpdateObjectLinksDetailed(ctx, "obj1", []OutgoingLink{
			{TargetID: "file1", BlockID: "block1"},
		})
		require.NoError(t, err)

		in, err := s.GetInboundLinksDetailedById("file1")
		require.NoError(t, err)
		require.Len(t, in, 1)
		assert.Equal(t, "block1", in[0].BlockID)
	})
}

func (fx *StoreFixture) assertInboundLinks(t *testing.T, id string, links []string) {
	in, err := fx.GetInboundLinksById(id)
	assert.NoError(t, err)
	if len(links) == 0 {
		assert.Empty(t, in)
		return
	}
	assert.Equal(t, links, in)
}

func (fx *StoreFixture) assertOutboundLinks(t *testing.T, id string, links []string) {
	out, err := fx.GetOutboundLinksById(id)
	assert.NoError(t, err)
	if len(links) == 0 {
		assert.Empty(t, out)
		return
	}
	assert.Equal(t, links, out)
}

func (fx *StoreFixture) givenExistingLinks(t *testing.T) {
	ctx := context.Background()
	err := fx.UpdateObjectLinks(ctx, "id1", []string{"id2"})
	require.NoError(t, err)

	fx.assertOutboundLinks(t, "id1", []string{"id2"})
	fx.assertInboundLinks(t, "id2", []string{"id1"})
	fx.assertInboundLinks(t, "id3", nil)
}

func TestDsObjectStore_ModifyObjectDetails(t *testing.T) {
	t.Run("when nil modifier passed - nothing changes", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)

		// when
		err := s.ModifyObjectDetails("id", nil)

		// then
		assert.NoError(t, err)
		got, err := s.GetDetails("id")
		assert.NoError(t, err)
		assert.Equal(t, 0, got.Len())
	})

	t.Run("modifier modifies details", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		s.AddObjects(t, []TestObject{makeObjectWithName("id", "foo")})

		// when
		err := s.ModifyObjectDetails("id", func(details *domain.Details) (*domain.Details, bool, error) {
			details.Set(bundle.RelationKeyName, domain.String("bar"))
			return details, true, nil
		})

		// then
		assert.NoError(t, err)
		want := makeDetails(TestObject{
			bundle.RelationKeyId:      domain.String("id"),
			bundle.RelationKeyName:    domain.String("bar"),
			bundle.RelationKeySpaceId: domain.String(spaceName),
		})
		got, err := s.GetDetails("id")
		assert.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("if modifier wipes details - id remains", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		s.AddObjects(t, []TestObject{makeObjectWithName("id", "foo")})

		// when
		err := s.ModifyObjectDetails("id", func(_ *domain.Details) (*domain.Details, bool, error) {
			return nil, true, nil
		})

		// then
		assert.NoError(t, err)
		want := makeDetails(TestObject{
			bundle.RelationKeyId: domain.String("id"),
		})
		got, err := s.GetDetails("id")
		assert.NoError(t, err)
		assert.Equal(t, want, got)
	})
}
