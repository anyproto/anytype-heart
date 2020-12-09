package core

import (
	"testing"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/structs"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/require"
)

func Test_Anytype_ObjectInfoWithLinks(t *testing.T) {
	s := getRunningService(t)
	block1, err := s.CreateBlock(smartblock.SmartBlockTypePage)
	require.NoError(t, err)

	blockID := "test_id1"

	blockContent1 := "Kademlia is a distributed hash table for decentralized peer-to-peer computer networks designed by" +
		" Petar Maymounkov and David Mazières in 2002.[1][2] It specifies the structure of the network and the exchange " +
		"of information through node lookups. Kademlia nodes communicate among themselves using UDP. A virtual or overlay" +
		" network is formed by the participant nodes. Each node is identified by a number or node ID. The node ID serves " +
		"not only as identification, but the Kademlia algorithm uses the node ID to locate values (usually file hashes " +
		"or keywords). In fact, the node ID provides a direct map to file hashes and that node stores information on " +
		"where to obtain the file or resource."
	details1 := &types.Struct{Fields: map[string]*types.Value{"name": structs.String("block1_name")}}
	blocks1 := []*model.Block{
		{
			Id:      blockID,
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: blockContent1}},
		},
	}
	err = block1.(*smartBlock).indexSnapshot(details1, nil, blocks1)
	require.NoError(t, err)

	block2, err := s.CreateBlock(smartblock.SmartBlockTypePage)
	require.NoError(t, err)

	details2 := &types.Struct{Fields: map[string]*types.Value{"name": structs.String("block2_name")}}
	blocks2 := []*model.Block{
		{
			Id:      blockID,
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "test"}},
		},
		{
			Id:      blockID,
			Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: block1.ID()}},
		},
	}

	err = block2.(*smartBlock).indexSnapshot(details2, nil, blocks2)
	require.NoError(t, err)

	info2, err := s.ObjectInfoWithLinks(block2.ID())
	require.NoError(t, err)
	require.NotNil(t, info2.Links)
	require.NotNil(t, info2.Links.Outbound)
	require.Len(t, info2.Links.Outbound, 1)

	require.Equal(t, block1.ID(), info2.Links.Outbound[0].Id)
	details1.Fields["id"] = pbtypes.String(block1.ID())

	require.True(t, info2.Links.Outbound[0].Details.Compare(details1) == 0)

	info1, err := s.ObjectInfoWithLinks(block1.ID())
	require.NoError(t, err)
	require.NotNil(t, info1.Links)
	require.Len(t, info1.Links.Inbound, 1)

	require.Equal(t, block2.ID(), info1.Links.Inbound[0].Id)
	details2.Fields["id"] = pbtypes.String(block2.ID())

	require.True(t, info1.Links.Inbound[0].Details.Compare(details2) == 0)
	require.Equal(t, getSnippet(blocks1), info1.Info.Snippet)

	// test change of existing page index
	blockContent2 := "newtext"
	details2Modified := &types.Struct{Fields: map[string]*types.Value{"name": structs.String("block2_name_modified")}}
	blocks2Modified := []*model.Block{
		{
			Id:      blockID,
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: blockContent2}},
		},
	}

	details2Modified.Fields["id"] = pbtypes.String(block2.ID())
	err = block2.(*smartBlock).indexSnapshot(details2Modified, nil, blocks2Modified)
	require.NoError(t, err)

	info2Modified, err := s.ObjectInfoWithLinks(block2.ID())
	require.NoError(t, err)

	info1Modified, err := s.ObjectInfoWithLinks(block1.ID())
	require.NoError(t, err)

	require.Len(t, info1Modified.Links.Inbound, 1)
	require.Len(t, info2Modified.Links.Outbound, 1)
	require.Equal(t, getSnippet(blocks2Modified), info2Modified.Info.Snippet)
	require.True(t, details2Modified.Compare(info2Modified.Info.Details) == 0)

	err = s.DeleteBlock(block1.ID())
	require.NoError(t, err)

	info1Modified, err = s.ObjectInfoWithLinks(block1.ID())
	require.Error(t, err)
	require.Nil(t, info1Modified)

	info2Modified, err = s.ObjectInfoWithLinks(block2.ID())
	require.NoError(t, err)
	require.Len(t, info2Modified.Links.Outbound, 0)
}

func Test_Anytype_PageList(t *testing.T) {
	s := getRunningService(t)
	block1, err := s.CreateBlock(smartblock.SmartBlockTypePage)
	require.NoError(t, err)

	blockID := "test_id1"

	details1 := &types.Struct{Fields: map[string]*types.Value{"name": structs.String("block1_name")}}
	blocks1 := []*model.Block{
		{
			Id:      blockID,
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "test"}},
		},
	}
	err = block1.(*smartBlock).indexSnapshot(details1, nil, blocks1)

	require.NoError(t, err)
	block2, err := s.CreateBlock(smartblock.SmartBlockTypePage)
	require.NoError(t, err)

	details2 := &types.Struct{Fields: map[string]*types.Value{"name": structs.String("block2_name")}}
	blocks2 := []*model.Block{
		{
			Id:      blockID,
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "test"}},
		},
		{
			Id:      blockID,
			Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: block1.ID()}},
		},
	}

	err = block2.(*smartBlock).indexSnapshot(details2, nil, blocks2)
	require.NoError(t, err)

	pages, err := s.ObjectList()
	require.NoError(t, err)

	var pageById = make(map[string]*model.ObjectInfo)
	for _, page := range pages {
		pageById[page.Id] = page
	}

	require.NotNil(t, pageById[block1.ID()])
	details1.Fields["id"] = pbtypes.String(block1.ID())
	details2.Fields["id"] = pbtypes.String(block2.ID())

	require.True(t, details1.Compare(pageById[block1.ID()].Details) == 0)
	require.Equal(t, "test", pageById[block1.ID()].Snippet)

	require.Equal(t, block2.ID(), pageById[block2.ID()].Id)
	require.Equal(t, details2, pageById[block2.ID()].Details)
	require.Equal(t, "test", pageById[block2.ID()].Snippet)
}
