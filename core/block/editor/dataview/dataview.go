package dataview

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/dataview"
)

type Dataview interface {
	UpdateDataview(ctx *state.Context, id string, showEvent bool, apply func(t dataview.Block) error) error
	SetActiveView(ctx *state.Context, id string, activeViewId string, showEvent bool) error
}

func NewDataview(sb smartblock.SmartBlock) Dataview {
	return &dataviewImpl{SmartBlock: sb}
}

type dataviewImpl struct {
	smartblock.SmartBlock
	activeView string
}

func (d *dataviewImpl) UpdateDataview(ctx *state.Context, id string, showEvent bool, apply func(t dataview.Block) error) error {
	s := d.NewStateCtx(ctx)
	tb, err := getDataview(s, id)
	if err != nil {
		return err
	}

	if err = apply(tb); err != nil {
		return err
	}

	if showEvent {
		return d.Apply(s)
	}
	return d.Apply(s, smartblock.NoEvent)
}

func (d *dataviewImpl) SetActiveView(ctx *state.Context, id string, activeViewId string, showEvent bool) error {
	s := d.NewStateCtx(ctx)
	tb, err := getDataview(s, id)
	if err != nil {
		return err
	}

	var found bool
	for _, view := range tb.Model().GetDataview().Views {
		if view.Id == activeViewId {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("view not found")
	}

	return d.Apply(s, smartblock.NoEvent)
}

func getDataview(s *state.State, id string) (dataview.Block, error) {
	b := s.Get(id)
	if b == nil {
		return nil, smartblock.ErrSimpleBlockNotFound
	}
	if tb, ok := b.(dataview.Block); ok {
		return tb, nil
	}
	return nil, fmt.Errorf("block '%s' not a dataview block", id)
}
