package change

import (
	"testing"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/merge/change/chmodel"
)

func TestBuildState_SingleBranch(t *testing.T) {
	doc := state.NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id: "root",
		}),
	})
	doc.(*state.State).SetChangeId("0")
	tr := new(Tree)
	tr.Add(&Change{
		Id: "0",
		Model: chmodel.Change{
			Type: chmodel.TypeAdd,
		},
	}, &Change{
		Id: "1",
		PreviousIds: []string{"0"},
		Model: chmodel.Change{
			Type: chmodel.TypeAdd,
			Value: chmodel.ChangeValueBlockPosition{
				Blocks: []*model.Block{
					{Id: "A"},
				},
				TargetId: "root",
				Position: model.Block_Inner,
			},
		},
	}, &Change{
		Id: "2",
		PreviousIds: []string{"1"},
		Model: chmodel.Change{
			Type: chmodel.TypeAdd,
			Value: chmodel.ChangeValueBlockPosition{
				Blocks: []*model.Block{
					{Id: "B"},
				},
				TargetId: "A",
				Position: model.Block_Bottom,
			},
		},
	})

	s, _ := BuildState(doc.(*state.State), tr)
	t.Log(s.String())
}


func TestBuildState_MultiBranch(t *testing.T) {
	doc := state.NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id: "root",
		}),
	})
	doc.(*state.State).SetChangeId("0")
	tr := new(Tree)
	tr.Add(&Change{
		Id: "0",
		Model: chmodel.Change{
			Type: chmodel.TypeAdd,
		},
	}, &Change{
		Id: "1",
		PreviousIds: []string{"0"},
		Model: chmodel.Change{
			Type: chmodel.TypeAdd,
			Value: chmodel.ChangeValueBlockPosition{
				Blocks: []*model.Block{
					{Id: "A"},
				},
				TargetId: "root",
				Position: model.Block_Inner,
			},
		},
	}, &Change{
		Id: "2",
		PreviousIds: []string{"1"},
		Model: chmodel.Change{
			Type: chmodel.TypeAdd,
			Value: chmodel.ChangeValueBlockPosition{
				Blocks: []*model.Block{
					{Id: "B"},
				},
				TargetId: "A",
				Position: model.Block_Bottom,
			},
		},
	}, &Change{
		Id: "1.1",
		PreviousIds: []string{"1"},
		Model: chmodel.Change{
			Type: chmodel.TypeAdd,
			Value: chmodel.ChangeValueBlockPosition{
				Blocks: []*model.Block{
					{Id: "B.1"},
				},
				TargetId: "A",
				Position: model.Block_Bottom,
			},
		},
	})

	s, _ := BuildState(doc.(*state.State), tr)
	t.Log(s.String())
}
