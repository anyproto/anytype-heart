package canvas_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/canvas"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type fixture struct {
	canvas.CanvasEditor
	sb *smarttest.SmartTest
}

func newFixture(t *testing.T) *fixture {
	sb := smarttest.New("root")
	sb.AddBlock(simple.New(&model.Block{Id: "root"}))
	return &fixture{
		CanvasEditor: canvas.NewEditor(sb),
		sb:           sb,
	}
}

func newState(f *fixture) *state.State {
	return f.sb.NewState()
}

func TestCanvasNodeCreate(t *testing.T) {
	t.Run("object node is created", func(t *testing.T) {
		// given
		f := newFixture(t)
		s := newState(f)
		want := &model.BlockContentCanvasNode{
			Type:           model.BlockContentCanvasNode_Object,
			TargetObjectId: "obj-1",
			X:              100,
			Y:              200,
			Width:          300,
			Height:         150,
		}

		// when
		id, err := f.CanvasNodeCreate(s, pb.RpcBlockCanvasNodeCreateRequest{
			ContextId: "root",
			Node:      want,
		})

		// then
		require.NoError(t, err)
		assert.NotEmpty(t, id)

		block := s.Get(id)
		require.NotNil(t, block)
		got := block.Model().GetCanvasNode()
		require.NotNil(t, got)
		assert.Equal(t, want.TargetObjectId, got.TargetObjectId)
		assert.Equal(t, want.X, got.X)
		assert.Equal(t, want.Y, got.Y)
	})

	t.Run("text node is created", func(t *testing.T) {
		// given
		f := newFixture(t)
		s := newState(f)

		// when
		id, err := f.CanvasNodeCreate(s, pb.RpcBlockCanvasNodeCreateRequest{
			ContextId: "root",
			Node: &model.BlockContentCanvasNode{
				Type:  model.BlockContentCanvasNode_Text,
				Text:  "Hello canvas",
				X:     0,
				Y:     0,
				Width: 200,
			},
		})

		// then
		require.NoError(t, err)
		assert.NotEmpty(t, id)
	})

	t.Run("nil node returns error", func(t *testing.T) {
		// given
		f := newFixture(t)
		s := newState(f)

		// when
		_, err := f.CanvasNodeCreate(s, pb.RpcBlockCanvasNodeCreateRequest{
			ContextId: "root",
			Node:      nil,
		})

		// then
		require.Error(t, err)
	})
}

func TestCanvasNodeUpdate(t *testing.T) {
	t.Run("node position is updated", func(t *testing.T) {
		// given
		f := newFixture(t)
		s := newState(f)
		id, err := f.CanvasNodeCreate(s, pb.RpcBlockCanvasNodeCreateRequest{
			ContextId: "root",
			Node:      &model.BlockContentCanvasNode{Type: model.BlockContentCanvasNode_Object, X: 0, Y: 0},
		})
		require.NoError(t, err)

		// when
		err = f.CanvasNodeUpdate(s, pb.RpcBlockCanvasNodeUpdateRequest{
			ContextId: "root",
			BlockId:   id,
			Node:      &model.BlockContentCanvasNode{Type: model.BlockContentCanvasNode_Object, X: 50, Y: 75},
		})

		// then
		require.NoError(t, err)
		got := s.Get(id).Model().GetCanvasNode()
		assert.Equal(t, 50.0, got.X)
		assert.Equal(t, 75.0, got.Y)
	})

	t.Run("non-existent block returns error", func(t *testing.T) {
		// given
		f := newFixture(t)
		s := newState(f)

		// when
		err := f.CanvasNodeUpdate(s, pb.RpcBlockCanvasNodeUpdateRequest{
			ContextId: "root",
			BlockId:   "missing",
			Node:      &model.BlockContentCanvasNode{},
		})

		// then
		require.Error(t, err)
	})
}

func TestCanvasNodeDelete(t *testing.T) {
	t.Run("node and its edges are removed", func(t *testing.T) {
		// given
		f := newFixture(t)
		s := newState(f)
		nodeA, _ := f.CanvasNodeCreate(s, pb.RpcBlockCanvasNodeCreateRequest{
			ContextId: "root",
			Node:      &model.BlockContentCanvasNode{Type: model.BlockContentCanvasNode_Object, TargetObjectId: "a"},
		})
		nodeB, _ := f.CanvasNodeCreate(s, pb.RpcBlockCanvasNodeCreateRequest{
			ContextId: "root",
			Node:      &model.BlockContentCanvasNode{Type: model.BlockContentCanvasNode_Object, TargetObjectId: "b"},
		})
		edgeId, _ := f.CanvasEdgeCreate(s, pb.RpcBlockCanvasEdgeCreateRequest{
			ContextId: "root",
			Edge:      &model.BlockContentCanvasEdge{FromNodeId: nodeA, ToNodeId: nodeB},
		})

		// when
		err := f.CanvasNodeDelete(s, pb.RpcBlockCanvasNodeDeleteRequest{
			ContextId: "root",
			BlockIds:  []string{nodeA},
		})

		// then
		require.NoError(t, err)
		assert.Nil(t, s.Get(nodeA), "node A should be removed")
		assert.Nil(t, s.Get(edgeId), "edge referencing node A should be removed")
		assert.NotNil(t, s.Get(nodeB), "node B should still exist")
	})
}

func TestCanvasEdgeCreate(t *testing.T) {
	t.Run("edge connects two nodes", func(t *testing.T) {
		// given
		f := newFixture(t)
		s := newState(f)
		nodeA, _ := f.CanvasNodeCreate(s, pb.RpcBlockCanvasNodeCreateRequest{
			ContextId: "root",
			Node:      &model.BlockContentCanvasNode{Type: model.BlockContentCanvasNode_Object},
		})
		nodeB, _ := f.CanvasNodeCreate(s, pb.RpcBlockCanvasNodeCreateRequest{
			ContextId: "root",
			Node:      &model.BlockContentCanvasNode{Type: model.BlockContentCanvasNode_Object},
		})

		// when
		edgeId, err := f.CanvasEdgeCreate(s, pb.RpcBlockCanvasEdgeCreateRequest{
			ContextId: "root",
			Edge: &model.BlockContentCanvasEdge{
				FromNodeId: nodeA,
				ToNodeId:   nodeB,
				Label:      "relates to",
			},
		})

		// then
		require.NoError(t, err)
		assert.NotEmpty(t, edgeId)
		got := s.Get(edgeId).Model().GetCanvasEdge()
		assert.Equal(t, nodeA, got.FromNodeId)
		assert.Equal(t, nodeB, got.ToNodeId)
		assert.Equal(t, "relates to", got.Label)
	})

	t.Run("edge with missing from-node returns error", func(t *testing.T) {
		// given
		f := newFixture(t)
		s := newState(f)
		nodeB, _ := f.CanvasNodeCreate(s, pb.RpcBlockCanvasNodeCreateRequest{
			ContextId: "root",
			Node:      &model.BlockContentCanvasNode{Type: model.BlockContentCanvasNode_Object},
		})

		// when
		_, err := f.CanvasEdgeCreate(s, pb.RpcBlockCanvasEdgeCreateRequest{
			ContextId: "root",
			Edge:      &model.BlockContentCanvasEdge{FromNodeId: "ghost", ToNodeId: nodeB},
		})

		// then
		require.Error(t, err)
	})
}

func TestCanvasEdgeDelete(t *testing.T) {
	t.Run("edge is removed", func(t *testing.T) {
		// given
		f := newFixture(t)
		s := newState(f)
		nodeA, _ := f.CanvasNodeCreate(s, pb.RpcBlockCanvasNodeCreateRequest{
			ContextId: "root",
			Node:      &model.BlockContentCanvasNode{Type: model.BlockContentCanvasNode_Object},
		})
		nodeB, _ := f.CanvasNodeCreate(s, pb.RpcBlockCanvasNodeCreateRequest{
			ContextId: "root",
			Node:      &model.BlockContentCanvasNode{Type: model.BlockContentCanvasNode_Object},
		})
		edgeId, _ := f.CanvasEdgeCreate(s, pb.RpcBlockCanvasEdgeCreateRequest{
			ContextId: "root",
			Edge:      &model.BlockContentCanvasEdge{FromNodeId: nodeA, ToNodeId: nodeB},
		})

		// when
		err := f.CanvasEdgeDelete(s, pb.RpcBlockCanvasEdgeDeleteRequest{
			ContextId: "root",
			BlockIds:  []string{edgeId},
		})

		// then
		require.NoError(t, err)
		assert.Nil(t, s.Get(edgeId))
	})
}
