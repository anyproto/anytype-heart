package core

import (
	"testing"

	"github.com/anytypeio/go-anytype-library/core/smartblock"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/structs"
	"github.com/anytypeio/go-anytype-library/vclock"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/require"
)

func Test_Anytype_PageInfoWithLinks(t *testing.T) {
	s := getRunningService(t)
	block1, err := s.CreateBlock(smartblock.SmartBlockTypePage)
	require.NoError(t, err)

	state1 := vclock.New()
	details1 := &types.Struct{Fields: map[string]*types.Value{"name": structs.String("block1_name")}}
	snap1, err := block1.PushSnapshot(
		state1,
		&SmartBlockMeta{Details: details1},
		[]*model.Block{
			{
				Id:      "test_id1",
				Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "Kademlia is a distributed hash table for decentralized peer-to-peer computer networks designed by Petar Maymounkov and David Mazières in 2002.[1][2] It specifies the structure of the network and the exchange of information through node lookups. Kademlia nodes communicate among themselves using UDP. A virtual or overlay network is formed by the participant nodes. Each node is identified by a number or node ID. The node ID serves not only as identification, but the Kademlia algorithm uses the node ID to locate values (usually file hashes or keywords). In fact, the node ID provides a direct map to file hashes and that node stores information on where to obtain the file or resource."}},
			},
		},
	)

	require.NoError(t, err)
	block2, err := s.CreateBlock(smartblock.SmartBlockTypePage)
	require.NoError(t, err)

	state2 := vclock.New()
	details2 := &types.Struct{Fields: map[string]*types.Value{"name": structs.String("block2_name")}}

	snap2, err := block2.PushSnapshot(
		state2,
		&SmartBlockMeta{Details: details2},
		[]*model.Block{
			{
				Id:      "test_id1",
				Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "test"}},
			},
			{
				Id:      "test_id1",
				Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: block1.ID()}},
			},
		},
	)
	require.NoError(t, err)

	info2, err := s.PageInfoWithLinks(block2.ID())
	require.NoError(t, err)

	require.NotNil(t, info2.Links)
	require.NotNil(t, info2.Links.Outbound)
	require.Len(t, info2.Links.Outbound, 1)

	require.Equal(t, block1.ID(), info2.Links.Outbound[0].Id)
	require.True(t, info2.Links.Outbound[0].Details.Compare(details1) == 0)
	require.Equal(t, snap1.State().Map(), info2.Links.Outbound[0].State.State)

	info1, err := s.PageInfoWithLinks(block1.ID())
	require.NoError(t, err)

	require.NotNil(t, info1.Links)
	require.Len(t, info1.Links.Inbound, 1)

	require.Equal(t, block2.ID(), info1.Links.Inbound[0].Id)
	require.True(t, info1.Links.Inbound[0].Details.Compare(details2) == 0)
	require.Equal(t, snap2.State().Map(), info1.Links.Inbound[0].State.State)
	require.Equal(t, "Kademlia is a distributed hash table for decentralized peer-to-peer computer networks designed by Petar Maymounkov and David Mazières in 2002.[1][2] It specifies the structure of the network and the exchange of information through node lookups. Kademlia nodes communicate among themselves using UDP. …", info1.Info.Snippet)

	// test change of existing page index
	details2Modified := &types.Struct{Fields: map[string]*types.Value{"name": structs.String("block2_name_modified")}}

	_, err = block2.PushSnapshot(
		state2,
		&SmartBlockMeta{Details: details2Modified},
		[]*model.Block{
			{
				Id:      "test_id1",
				Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "newtext"}},
			},
		},
	)
	require.NoError(t, err)

	info2Modified, err := s.PageInfoWithLinks(block2.ID())
	require.NoError(t, err)

	info1Modified, err := s.PageInfoWithLinks(block1.ID())
	require.NoError(t, err)

	require.Len(t, info1Modified.Links.Inbound, 0)
	require.Len(t, info2Modified.Links.Outbound, 0)
	require.Equal(t, "newtext", info2Modified.Info.Snippet)
	require.True(t, details2Modified.Compare(info2Modified.Info.Details) == 0)

	err = s.DeleteBlock(block1.ID())
	require.NoError(t, err)

	info1Modified, err = s.PageInfoWithLinks(block1.ID())
	require.Error(t, err)
	require.Nil(t, info1Modified)

	info2Modified, err = s.PageInfoWithLinks(block2.ID())
	require.NoError(t, err)
	require.Len(t, info2Modified.Links.Outbound, 0)
}

func Test_Anytype_PageList(t *testing.T) {
	s := getRunningService(t)
	block1, err := s.CreateBlock(smartblock.SmartBlockTypePage)
	require.NoError(t, err)

	state1 := vclock.New()
	details1 := &types.Struct{Fields: map[string]*types.Value{"name": structs.String("block1_name")}}
	snap1, err := block1.PushSnapshot(
		state1,
		&SmartBlockMeta{Details: details1},
		[]*model.Block{
			{
				Id:      "test_id1",
				Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "test"}},
			},
		},
	)

	require.NoError(t, err)
	block2, err := s.CreateBlock(smartblock.SmartBlockTypePage)
	require.NoError(t, err)

	state2 := vclock.New()
	details2 := &types.Struct{Fields: map[string]*types.Value{"name": structs.String("block2_name")}}

	snap2, err := block2.PushSnapshot(
		state2,
		&SmartBlockMeta{Details: details2},
		[]*model.Block{
			{
				Id:      "test_id1",
				Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "test"}},
			},
			{
				Id:      "test_id1",
				Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: block1.ID()}},
			},
		},
	)
	require.NoError(t, err)

	pages, err := s.PageList()
	require.NoError(t, err)

	var pageById = make(map[string]*model.PageInfo)
	for _, page := range pages {
		pageById[page.Id] = page
	}

	require.NotNil(t, pageById[block1.ID()])
	require.Equal(t, details1, pageById[block1.ID()].Details)
	require.Equal(t, snap1.State().Map(), pageById[block1.ID()].State.State)
	require.Equal(t, "test", pageById[block1.ID()].Snippet)

	require.Equal(t, block2.ID(), pageById[block2.ID()].Id)
	require.Equal(t, details2, pageById[block2.ID()].Details)
	require.Equal(t, snap2.State().Map(), pageById[block2.ID()].State.State)
	require.Equal(t, "test", pageById[block2.ID()].Snippet)
}
