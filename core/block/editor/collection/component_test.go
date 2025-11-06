package collection

import (
	"context"
	"sync"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/pb"
)

type testFlusher struct{}

func (tf *testFlusher) Name() string { return "test.flusher" }

func (tf *testFlusher) Init(*app.App) error { return nil }

func (tf *testFlusher) Run(context.Context) error { return nil }

func (tf *testFlusher) Close(context.Context) error { return nil }

func (tf *testFlusher) FlushUpdates() {}

type fixture struct {
	*component
}

func newFixture(t *testing.T) *fixture {
	sb := smarttest.New("id1")
	flusher := &testFlusher{}

	coll := New(sb, flusher)

	return &fixture{component: coll.(*component)}
}

func TestBroadcast(t *testing.T) {
	s := newFixture(t)

	_, subCh1, err := s.SubscribeForCollection("sub1")
	require.NoError(t, err)
	_, subCh2, err := s.SubscribeForCollection("sub2")
	require.NoError(t, err)

	var wg sync.WaitGroup

	var sub1Results [][]string
	wg.Add(1)
	go func() {
		defer wg.Done()
		for c := range subCh1 {
			sub1Results = append(sub1Results, c)
		}

	}()

	var sub2Results [][]string
	wg.Add(1)
	go func() {
		defer wg.Done()
		for c := range subCh2 {
			sub2Results = append(sub2Results, c)
		}
	}()

	changeCollection := func(ids []string) {
		st := s.NewState()
		st.UpdateStoreSlice(template.CollectionStoreKey, ids)
		err := s.Apply(st)

		require.NoError(t, err)
	}

	changeCollection([]string{"1", "2", "3"})
	changeCollection([]string{"3", "2"})
	changeCollection([]string{"1", "2", "3", "4"})
	s.UnsubscribeFromCollection("sub1")
	changeCollection([]string{"1", "4"})
	s.UnsubscribeFromCollection("sub2")
	changeCollection([]string{"5"})

	wg.Wait()

	assert.Equal(t, [][]string{
		{"1", "2", "3"},
		{"3", "2"},
		{"1", "2", "3", "4"},
	}, sub1Results)
	assert.Equal(t, [][]string{
		{"1", "2", "3"},
		{"3", "2"},
		{"1", "2", "3", "4"},
		{"1", "4"},
	}, sub2Results)
}

func TestService_Add(t *testing.T) {
	t.Run("add new objects to collection", func(t *testing.T) {
		s := newFixture(t)

		// when
		err := s.AddToCollection(nil, &pb.RpcObjectCollectionAddRequest{
			ObjectIds: []string{"obj1", "obj2"},
		})

		// then
		assert.NoError(t, err)
		assert.Equal(t, []string{"obj1", "obj2"}, s.NewState().GetStoreSlice(template.CollectionStoreKey))
	})
}
