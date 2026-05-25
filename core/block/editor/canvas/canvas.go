package canvas

import (
	"fmt"

	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type CanvasEditor interface {
	CanvasNodeCreate(s *state.State, req pb.RpcBlockCanvasNodeCreateRequest) (blockId string, err error)
	CanvasNodeUpdate(s *state.State, req pb.RpcBlockCanvasNodeUpdateRequest) error
	CanvasNodeDelete(s *state.State, req pb.RpcBlockCanvasNodeDeleteRequest) error

	CanvasEdgeCreate(s *state.State, req pb.RpcBlockCanvasEdgeCreateRequest) (blockId string, err error)
	CanvasEdgeUpdate(s *state.State, req pb.RpcBlockCanvasEdgeUpdateRequest) error
	CanvasEdgeDelete(s *state.State, req pb.RpcBlockCanvasEdgeDeleteRequest) error
}

type editor struct {
	sb smartblock.SmartBlock
}

var _ CanvasEditor = &editor{}

func NewEditor(sb smartblock.SmartBlock) CanvasEditor {
	return &editor{sb: sb}
}

func newId() string {
	return bson.NewObjectId().Hex()
}

// CanvasNodeCreate adds a new node block to the canvas root.
func (e *editor) CanvasNodeCreate(s *state.State, req pb.RpcBlockCanvasNodeCreateRequest) (string, error) {
	if req.Node == nil {
		return "", fmt.Errorf("node is required")
	}
	id := newId()
	b := simple.New(&model.Block{
		Id: id,
		Content: &model.BlockContentOfCanvasNode{
			CanvasNode: req.Node,
		},
	})
	s.Add(b)
	if err := s.InsertTo(s.RootId(), model.Block_Inner, id); err != nil {
		return "", fmt.Errorf("insert canvas node: %w", err)
	}
	return id, nil
}

// CanvasNodeUpdate updates position, size, text, or color of an existing node.
func (e *editor) CanvasNodeUpdate(s *state.State, req pb.RpcBlockCanvasNodeUpdateRequest) error {
	if req.Node == nil {
		return fmt.Errorf("node is required")
	}
	b := s.Get(req.BlockId)
	if b == nil {
		return fmt.Errorf("block not found: %s", req.BlockId)
	}
	if b.Model().GetCanvasNode() == nil {
		return fmt.Errorf("block %s is not a canvas node", req.BlockId)
	}
	updated := b.Copy()
	updated.Model().Content = &model.BlockContentOfCanvasNode{CanvasNode: req.Node}
	s.Set(updated)
	return nil
}

// CanvasNodeDelete removes one or more node blocks (and any edges referencing them).
func (e *editor) CanvasNodeDelete(s *state.State, req pb.RpcBlockCanvasNodeDeleteRequest) error {
	for _, id := range req.BlockIds {
		e.removeEdgesFor(s, id)
		if !s.Unlink(id) {
			return fmt.Errorf("canvas node not found: %s", id)
		}
		s.CleanupBlock(id)
	}
	return nil
}

// removeEdgesFor removes all edge blocks that reference the given node id.
func (e *editor) removeEdgesFor(s *state.State, nodeId string) {
	var toRemove []string
	s.Iterate(func(b simple.Block) bool {
		edge := b.Model().GetCanvasEdge()
		if edge != nil && (edge.FromNodeId == nodeId || edge.ToNodeId == nodeId) {
			toRemove = append(toRemove, b.Model().Id)
		}
		return true
	})
	for _, id := range toRemove {
		s.Unlink(id)
		s.CleanupBlock(id)
	}
}

// CanvasEdgeCreate adds a directed edge between two existing nodes.
func (e *editor) CanvasEdgeCreate(s *state.State, req pb.RpcBlockCanvasEdgeCreateRequest) (string, error) {
	if req.Edge == nil {
		return "", fmt.Errorf("edge is required")
	}
	if s.Get(req.Edge.FromNodeId) == nil {
		return "", fmt.Errorf("from node not found: %s", req.Edge.FromNodeId)
	}
	if s.Get(req.Edge.ToNodeId) == nil {
		return "", fmt.Errorf("to node not found: %s", req.Edge.ToNodeId)
	}
	id := newId()
	b := simple.New(&model.Block{
		Id: id,
		Content: &model.BlockContentOfCanvasEdge{
			CanvasEdge: req.Edge,
		},
	})
	s.Add(b)
	if err := s.InsertTo(s.RootId(), model.Block_Inner, id); err != nil {
		return "", fmt.Errorf("insert canvas edge: %w", err)
	}
	return id, nil
}

// CanvasEdgeUpdate updates the label or color of an existing edge.
func (e *editor) CanvasEdgeUpdate(s *state.State, req pb.RpcBlockCanvasEdgeUpdateRequest) error {
	if req.Edge == nil {
		return fmt.Errorf("edge is required")
	}
	b := s.Get(req.BlockId)
	if b == nil {
		return fmt.Errorf("block not found: %s", req.BlockId)
	}
	if b.Model().GetCanvasEdge() == nil {
		return fmt.Errorf("block %s is not a canvas edge", req.BlockId)
	}
	updated := b.Copy()
	updated.Model().Content = &model.BlockContentOfCanvasEdge{CanvasEdge: req.Edge}
	s.Set(updated)
	return nil
}

// CanvasEdgeDelete removes one or more edge blocks.
func (e *editor) CanvasEdgeDelete(s *state.State, req pb.RpcBlockCanvasEdgeDeleteRequest) error {
	for _, id := range req.BlockIds {
		if !s.Unlink(id) {
			return fmt.Errorf("canvas edge not found: %s", id)
		}
		s.CleanupBlock(id)
	}
	return nil
}
