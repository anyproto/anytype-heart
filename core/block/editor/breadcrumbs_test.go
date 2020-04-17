package editor

import (
	"testing"

	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBreadcrumbs_Init(t *testing.T) {
	b := NewBreadcrumbs()
	err := b.Init(source.NewVirtual(nil, nil, pb.SmartBlockType_Breadcrumbs))
	require.NoError(t, err)
	assert.NotEmpty(t, b.Id())
	assert.NotEmpty(t, b.RootId())
	assert.Len(t, b.Blocks(), 1)
}

func TestBreadcrumbs_OnSmartOpen(t *testing.T) {
	t.Run("add pages", func(t *testing.T) {
		b := NewBreadcrumbs()
		err := b.Init(source.NewVirtual(nil, nil, pb.SmartBlockType_Breadcrumbs))
		require.NoError(t, err)
		var events []*pb.Event
		b.SetEventFunc(func(e *pb.Event) {
			events = append(events, e)
		})
		b.OnSmartOpen("1")
		require.Len(t, events, 1)
		b.OnSmartOpen("2")
		require.Len(t, events, 2)

		s := b.NewState()
		root := s.Get(b.RootId())
		require.Len(t, root.Model().ChildrenIds, 2)
		assert.Equal(t, "1", s.Get(root.Model().ChildrenIds[0]).Model().GetLink().TargetBlockId)
		assert.Equal(t, "2", s.Get(root.Model().ChildrenIds[1]).Model().GetLink().TargetBlockId)
	})
	t.Run("add existing page", func(t *testing.T) {
		b := NewBreadcrumbs()
		err := b.Init(source.NewVirtual(nil, nil, pb.SmartBlockType_Breadcrumbs))
		require.NoError(t, err)
		var events []*pb.Event
		b.SetEventFunc(func(e *pb.Event) {
			events = append(events, e)
		})
		b.OnSmartOpen("1")
		b.OnSmartOpen("2")
		require.Len(t, events, 2)
		b.OnSmartOpen("1")
		assert.Len(t, events, 2)
		b.OnSmartOpen("2")
		assert.Len(t, events, 2)
	})
}

func TestBreadcrumbs_ChainCut(t *testing.T) {
	t.Run("negative index", func(t *testing.T) {
		b := NewBreadcrumbs()
		err := b.Init(source.NewVirtual(nil, nil, pb.SmartBlockType_Breadcrumbs))
		require.NoError(t, err)
		b.OnSmartOpen("1")
		var events []*pb.Event
		b.SetEventFunc(func(e *pb.Event) {
			events = append(events, e)
		})
		b.ChainCut(-1)
		assert.Len(t, events, 0)
	})
	t.Run("overflow index", func(t *testing.T) {
		b := NewBreadcrumbs()
		err := b.Init(source.NewVirtual(nil, nil, pb.SmartBlockType_Breadcrumbs))
		require.NoError(t, err)
		b.OnSmartOpen("1")
		var events []*pb.Event
		b.SetEventFunc(func(e *pb.Event) {
			events = append(events, e)
		})
		b.ChainCut(10)
		assert.Len(t, events, 0)
	})
	t.Run("cut", func(t *testing.T) {
		b := NewBreadcrumbs()
		err := b.Init(source.NewVirtual(nil, nil, pb.SmartBlockType_Breadcrumbs))
		require.NoError(t, err)
		b.OnSmartOpen("1")
		b.OnSmartOpen("2")
		var events []*pb.Event
		b.SetEventFunc(func(e *pb.Event) {
			events = append(events, e)
		})
		b.ChainCut(1)
		assert.Len(t, events, 1)
		assert.Len(t, b.NewState().Get(b.RootId()).Model().ChildrenIds, 1)
		b.ChainCut(0)
		assert.Len(t, events, 2)
		assert.Len(t, b.NewState().Get(b.RootId()).Model().ChildrenIds, 0)
		assert.Len(t, b.Blocks(), 1)
	})
}
