package testutil

import (
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/tests/blockbuilder"
)

func BuildStateFromAST(root *blockbuilder.Block) *state.State {
	st := state.NewDocFromSnapshot("", &pb.ChangeSnapshot{
		Data: &model.SmartBlockSnapshotBase{
			Blocks: root.Build(),
		},
	}).(*state.State)
	state.ApplyState(st, true)
	return st.NewState()
}
