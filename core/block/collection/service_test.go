package collection

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/mock_objectstore"
	"github.com/anyproto/anytype-heart/core/session"
)

type testPicker struct {
	sb smartblock.SmartBlock
}

func (t *testPicker) PickBlock(ctx context.Context, id string) (sb smartblock.SmartBlock, err error) {
	return t.sb, nil
}

func (t *testPicker) Init(a *app.App) error { return nil }

func (t *testPicker) Name() string { return "" }

type fixture struct {
	picker *testPicker
	*Service
}

func newFixture(t *testing.T) *fixture {
	picker := &testPicker{}
	a := &app.App{}
	objectStore := mock_objectstore.NewMockObjectStore(t)
	objectStore.EXPECT().Name().Return("objectStore")
	a.Register(picker)
	a.Register(objectStore)
	s := New()

	err := s.Init(a)
	require.NoError(t, err)
	return &fixture{picker: picker, Service: s}
}

func TestBroadcast(t *testing.T) {
	const collectionID = "collectionID"
	sb := smarttest.New(collectionID)

	s := newFixture(t)
	s.picker.sb = sb

	_, subCh1, err := s.SubscribeForCollection(collectionID, "sub1")
	require.NoError(t, err)
	_, subCh2, err := s.SubscribeForCollection(collectionID, "sub2")
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
		st := sb.NewState()
		st.UpdateStoreSlice(template.CollectionStoreKey, ids)
		err := sb.Apply(st)

		require.NoError(t, err)
	}

	changeCollection([]string{"1", "2", "3"})
	changeCollection([]string{"3", "2"})
	changeCollection([]string{"1", "2", "3", "4"})
	s.UnsubscribeFromCollection(collectionID, "sub1")
	changeCollection([]string{"1", "4"})
	s.UnsubscribeFromCollection(collectionID, "sub2")
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
