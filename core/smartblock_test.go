package core

import (
	"testing"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/structs"
	"github.com/anytypeio/go-anytype-library/vclock"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/require"
)

func Test_smartBlock_GetLastSnapshot(t *testing.T) {
	s := getRunningService(t)
	block, err := s.CreateBlock(SmartBlockTypePage)
	require.NoError(t, err)

	state := vclock.New()
	snap, err := block.PushSnapshot(
		state,
		&SmartBlockMeta{Details: &types.Struct{Fields: map[string]*types.Value{"name": structs.String("name1")}}},
		[]*model.Block{
			{
				Id:      "test",
				Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "test"}},
			},
		},
	)

	lastSnap, err := block.GetLastSnapshot()
	require.NoError(t, err)

	require.Equal(t, snap.State().Hash(), lastSnap.State().Hash())

	lastSnapBlocks, _ := lastSnap.Blocks()
	require.Equal(t, []*model.Block{
		{
			Id:      "test",
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "test"}},
		},
	}, lastSnapBlocks)

	lastSnapMeta, _ := lastSnap.Meta()

	require.Equal(t, &SmartBlockMeta{Details: &types.Struct{Fields: map[string]*types.Value{"name": structs.String("name1")}}}, lastSnapMeta)

}
