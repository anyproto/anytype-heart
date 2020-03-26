package core

import (
	"context"
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

	thrd, err := s.(*Anytype).ts.GetThread(context.Background(), block.(*smartBlock).thread.ID)
	require.NoError(t, err)
	require.Len(t, thrd.Logs, 1)

	/*a1 := s.(*Anytype).device.Address()
	a2 := thrd.Logs[0].ID.String()

	require.Equal(t, a1, a2)*/

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

	creator, err := lastSnap.Creator()
	require.NoError(t, err)
	require.Equal(t, s.Account(), creator)

	lastSnapMeta, _ := lastSnap.Meta()

	require.Equal(t, &SmartBlockMeta{Details: &types.Struct{Fields: map[string]*types.Value{"name": structs.String("name1")}}}, lastSnapMeta)
}
