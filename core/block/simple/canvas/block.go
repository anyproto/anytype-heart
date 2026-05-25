package canvas

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/base"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func init() {
	simple.RegisterCreator(NewNodeBlock)
	simple.RegisterCreator(NewEdgeBlock)
}

// --- CanvasNode block ---

func NewNodeBlock(b *model.Block) simple.Block {
	if b.GetCanvasNode() != nil {
		return &nodeBlock{
			Base:    base.NewBase(b).(*base.Base),
			content: b.GetCanvasNode(),
		}
	}
	return nil
}

type NodeBlock interface {
	simple.Block
}

type nodeBlock struct {
	*base.Base
	content *model.BlockContentCanvasNode
}

func (b *nodeBlock) Copy() simple.Block {
	return NewNodeBlock(pbtypes.CopyBlock(b.Model()))
}

func (b *nodeBlock) Diff(spaceId string, ob simple.Block) ([]simple.EventMessage, error) {
	other, ok := ob.(*nodeBlock)
	if !ok {
		return nil, fmt.Errorf("can't make diff with incompatible block")
	}
	msgs, err := b.Base.Diff(spaceId, other)
	if err != nil {
		return nil, err
	}
	// position or content changes are sent as a full block update via base diff
	_ = other
	return msgs, nil
}

func (b *nodeBlock) FillSmartIds(ids []string) []string {
	if b.content.TargetObjectId != "" {
		ids = append(ids, b.content.TargetObjectId)
	}
	return ids
}

func (b *nodeBlock) HasSmartIds() bool {
	return b.content.TargetObjectId != ""
}

// --- CanvasEdge block ---

func NewEdgeBlock(b *model.Block) simple.Block {
	if b.GetCanvasEdge() != nil {
		return &edgeBlock{
			Base:    base.NewBase(b).(*base.Base),
			content: b.GetCanvasEdge(),
		}
	}
	return nil
}

type EdgeBlock interface {
	simple.Block
}

type edgeBlock struct {
	*base.Base
	content *model.BlockContentCanvasEdge
}

func (b *edgeBlock) Copy() simple.Block {
	return NewEdgeBlock(pbtypes.CopyBlock(b.Model()))
}

func (b *edgeBlock) Diff(spaceId string, ob simple.Block) ([]simple.EventMessage, error) {
	other, ok := ob.(*edgeBlock)
	if !ok {
		return nil, fmt.Errorf("can't make diff with incompatible block")
	}
	msgs, err := b.Base.Diff(spaceId, other)
	if err != nil {
		return nil, err
	}
	_ = other
	return msgs, nil
}
