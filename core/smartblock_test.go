package core

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/require"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/structs"
	"github.com/anytypeio/go-anytype-library/vclock"
)

func BenchmarkSnapshot(b *testing.B) {
	b.StopTimer()
	// run the Fib function b.N times
	s := getRunningServiceB(b)
	block, err := s.CreateBlock(SmartBlockTypePage)
	state := vclock.New()
	snap, err := block.PushSnapshot(
		state,
		&SmartBlockMeta{Details: &types.Struct{Fields: map[string]*types.Value{"name": structs.String("name1")}}},
		[]*model.Block{
			{
				Id:      "test_id1",
				Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "test"}},
			},
		},
	)
	require.NoError(b, err)

	state = snap.State()
	block, err = s.GetBlock(block.ID())
	require.NoError(b, err)

	b.StartTimer()
	for n := 0; n < b.N; n++ {
		snap, _ := block.PushSnapshot(
			state,
			&SmartBlockMeta{Details: &types.Struct{Fields: map[string]*types.Value{"name": structs.String("name1")}}},
			[]*model.Block{
				{
					Id:      "test_id1",
					Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "test"}},
				},
			},
		)
		state = snap.State()
	}
	b.StopTimer()
}

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
				Id:      "test_id1",
				Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "test"}},
			},
		},
	)

	snap2, err := block.PushSnapshot(
		snap.State(),
		&SmartBlockMeta{Details: &types.Struct{Fields: map[string]*types.Value{"name": structs.String("name2")}}},
		[]*model.Block{
			{
				Id:      "test_id2",
				Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "test2"}},
			},
		},
	)

	thrd, err := s.(*Anytype).t.GetThread(context.Background(), block.(*smartBlock).thread.ID)
	require.NoError(t, err)
	require.Len(t, thrd.Logs, 1)

	/*a1 := s.(*Anytype).device.Address()
	a2 := thrd.Logs[0].ID.String()

	require.Equal(t, a1, a2)*/
	time.Sleep(time.Millisecond * 100)
	lastSnap, err := block.GetLastSnapshot()
	require.NoError(t, err)
	require.Equal(t, snap2.State().Hash(), lastSnap.State().Hash())

	lastSnapBlocks, _ := lastSnap.Blocks()
	require.Equal(t, []*model.Block{
		{
			Id:      "test_id2",
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "test2"}},
		},
	}, lastSnapBlocks)

	creator, err := lastSnap.Creator()
	require.NoError(t, err)
	require.Equal(t, s.Account(), creator)
	lastSnapMeta, _ := lastSnap.Meta()

	_, err = lastSnap.PublicWebURL()
	require.NoError(t, err)

	require.Equal(t, &SmartBlockMeta{Details: &types.Struct{Fields: map[string]*types.Value{"name": structs.String("name2")}}}, lastSnapMeta)
}

func Test_smartBlock_GetSnapshots(t *testing.T) {
	s := getRunningService(t)
	block, err := s.CreateBlock(SmartBlockTypePage)
	require.NoError(t, err)

	state := vclock.New()
	snap, err := block.PushSnapshot(
		state,
		&SmartBlockMeta{Details: &types.Struct{Fields: map[string]*types.Value{"name": structs.String("name1")}}},
		[]*model.Block{
			{
				Id:      "test_id1",
				Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "test"}},
			},
		},
	)

	snap2, err := block.PushSnapshot(
		snap.State(),
		&SmartBlockMeta{Details: &types.Struct{Fields: map[string]*types.Value{"name": structs.String("name2")}}},
		[]*model.Block{
			{
				Id:      "test_id2",
				Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "test2"}},
			},
		},
	)

	thrd, err := s.(*Anytype).t.GetThread(context.Background(), block.(*smartBlock).thread.ID)
	require.NoError(t, err)
	require.Len(t, thrd.Logs, 1)

	/*a1 := s.(*Anytype).device.Address()
	a2 := thrd.Logs[0].ID.String()

	require.Equal(t, a1, a2)*/

	snaps, err := block.(*smartBlock).GetSnapshots(vclock.Undef, 2, false)
	require.NoError(t, err)
	require.Len(t, snaps, 2)

	require.Equal(t, snap2.State().Hash(), snaps[0].State().Hash())

	lastSnapBlocks, _ := snaps[0].Blocks()
	require.Equal(t, []*model.Block{
		{
			Id:      "test_id2",
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "test2"}},
		},
	}, lastSnapBlocks)

	creator, err := snaps[0].Creator()
	require.NoError(t, err)
	require.Equal(t, s.Account(), creator)

	lastSnapMeta, _ := snaps[0].Meta()

	require.Equal(t, &SmartBlockMeta{Details: &types.Struct{Fields: map[string]*types.Value{"name": structs.String("name2")}}}, lastSnapMeta)
}

func Test_smartBlock_SnapshotFileKeys(t *testing.T) {
	s := getRunningService(t)
	block, err := s.CreateBlock(SmartBlockTypePage)
	require.NoError(t, err)

	state := vclock.New()
	f, err := s.FileAddWithReader(context.Background(), bytes.NewReader([]byte("123")), "test")
	require.NoError(t, err)

	_, err = block.PushSnapshot(
		state,
		&SmartBlockMeta{Details: &types.Struct{Fields: map[string]*types.Value{"name": structs.String("name1")}}},
		[]*model.Block{
			{
				Id:      "test_id1",
				Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "test"}},
			},
			{
				Id: "test_id2",
				Content: &model.BlockContentOfFile{
					&model.BlockContentFile{
						Hash:    f.Hash(),
						Name:    f.Meta().Name,
						Type:    model.BlockContentFile_File,
						Mime:    f.Meta().Media,
						Size_:   f.Meta().Size,
						AddedAt: time.Now().Unix(),
						State:   model.BlockContentFile_Done,
					}},
			},
		},
	)

	thrd, err := s.(*Anytype).t.GetThread(context.Background(), block.(*smartBlock).thread.ID)
	require.NoError(t, err)
	require.Len(t, thrd.Logs, 1)

	files, err := s.(*Anytype).localStore.Files.ListByTarget(f.Hash())
	require.NoError(t, err)
	require.Len(t, files, 1)

	// clear cached file info to test recovery from ipfs
	err = s.(*Anytype).localStore.Files.DeleteByHash(files[0].Hash)
	require.NoError(t, err)

	fhash := f.Hash()
	f1, err := s.FileByHash(context.Background(), fhash)
	require.NoError(t, err)
	require.NotNil(t, f1)

	// clear file keys cache
	filesKeysCache = make(map[string]map[string]string)

	snaps, err := block.(*smartBlock).GetSnapshots(vclock.Undef, 10, false)
	require.NoError(t, err)
	require.Len(t, snaps, 1)
	f2, err := s.FileByHash(context.Background(), f.Hash())
	require.NoError(t, err)
	require.NotNil(t, f2)
	require.NotNil(t, filesKeysCache[f.Hash()])
}
