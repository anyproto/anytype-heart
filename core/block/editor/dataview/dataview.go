package dataview

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/dataview"
	"github.com/google/uuid"
	"github.com/santhosh-tekuri/jsonschema/v2"
)

const defaultViewName = "Untitled"

type Dataview interface {
	UpdateDataview(ctx *state.Context, id string, showEvent bool, apply func(t dataview.Block) error) error
	SetActiveView(ctx *state.Context, id string, activeViewId string, showEvent bool) error
	CreateView(ctx *state.Context, id string, view model.BlockContentDataviewView) (*model.BlockContentDataviewView, error)
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

	d.activeView = activeViewId

	return d.Apply(s, smartblock.NoEvent)
}

func getDefaultRelations(schema *jsonschema.Schema) []*model.BlockContentDataviewRelation {
	var relations []*model.BlockContentDataviewRelation

	if defaults, ok := schema.Default.([]interface{}); ok {
		for _, def := range defaults {
			if v, ok := def.(map[string]interface{}); ok {
				if isHided, exists := v["isHided"]; exists {
					if v, ok := isHided.(bool); ok {
						if v {
							continue
						}
					}
				}

				relations = append(relations,
					&model.BlockContentDataviewRelation{
						Id:      v["id"].(string),
						Visible: true,
					})
			}
		}
	}
	return relations
}

func (d *dataviewImpl) CreateView(ctx *state.Context, id string, view model.BlockContentDataviewView) (*model.BlockContentDataviewView, error) {
	view.Id = uuid.New().String()
	s := d.NewStateCtx(ctx)
	tb, err := getDataview(s, id)
	if err != nil {
		return nil, err
	}

	if view.Name == "" {
		view.Name = defaultViewName
	}

	if len(view.Relations) == 0 {
		sch, err := d.Anytype().GetSchema(tb.Model().GetDataview().SchemaURL)
		if err != nil {
			return nil, fmt.Errorf("failed to get schema %s for dataview: %s", tb.Model().GetDataview().SchemaURL, err.Error())
		}

		view.Relations = getDefaultRelations(sch)
	}

	if len(view.Sorts) == 0 {
		// todo: set depends on the view type
		view.Sorts = []*model.BlockContentDataviewSort{{
			RelationId: "id",
			Type:       model.BlockContentDataviewSort_Asc,
		}}
	}

	tb.AddView(view)
	return &view, d.Apply(s)
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
