package core

import (
	"fmt"
	"sync"
	"testing"

	"github.com/anytypeio/go-anytype-library/core/smartblock"
	"github.com/anytypeio/go-anytype-library/database"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/structs"
	"github.com/anytypeio/go-anytype-library/vclock"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/require"
)

var doOnce sync.Once
var s Service

func getRunningServiceB(t *testing.B) Service {
	doOnce.Do(func() {
		s = createAccount(t)
		err := s.Start()
		require.NoError(t, err)
	})
	return s
}

func getRunningService(t *testing.T) Service {
	doOnce.Do(func() {
		s = createAccount(t)
		err := s.Start()
		require.NoError(t, err)

		err = s.InitPredefinedBlocks(false)
		require.NoError(t, err)
	})
	return s
}

func TestAnytype_IsStarted(t *testing.T) {
	s := getRunningService(t)
	require.True(t, s.IsStarted())
}

func TestAnytype_DeviceKeyEquals(t *testing.T) {
	s := getRunningService(t)
	require.Equal(t, s.(*Anytype).t.Host().ID().String(), s.(*Anytype).opts.Device.Address())
}

func TestAnytype_GetDatabaseByID(t *testing.T) {
	s := getRunningService(t)
	require.NotNil(t, s)

	err := s.InitPredefinedBlocks(false)
	require.NoError(t, err)

	block1, err := s.CreateBlock(smartblock.SmartBlockTypePage)
	require.NoError(t, err)

	state1 := vclock.New()
	details1 := &types.Struct{Fields: map[string]*types.Value{"name": structs.String("block1_name")}}
	_, err = block1.PushSnapshot(
		state1,
		&SmartBlockMeta{Details: details1},
		[]*model.Block{
			{
				Id:      "test_id1",
				Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "Kademlia is a distributed hash table for decentralized peer-to-peer computer networks designed by Petar Maymounkov and David Mazières in 2002.[1][2] It specifies the structure of the network and the exchange of information through node lookups. Kademlia nodes communicate among themselves using UDP. A virtual or overlay network is formed by the participant nodes. Each node is identified by a number or node ID. The node ID serves not only as identification, but the Kademlia algorithm uses the node ID to locate values (usually file hashes or keywords). In fact, the node ID provides a direct map to file hashes and that node stores information on where to obtain the file or resource."}},
			},
		},
	)

	db, err := s.DatabaseByID("pages")
	require.NoError(t, err)
	require.Equal(t, "https://anytype.io/schemas/page", db.Schema())

	results, err := db.Query(database.Query{Limit: 10, Sorts: []*model.BlockContentDataviewSort{{RelationId: "name"}}})
	require.NoError(t, err)
	require.Len(t, results, 1)

	require.Equal(t, details1.Fields["name"].GetStringValue(), results[0].Details.Fields["name"].GetStringValue())
	require.Equal(t, block1.ID(), results[0].Details.Fields["id"].GetStringValue())

}

func TestAnytype_PredefinedBlocks(t *testing.T) {
	s := getRunningService(t)
	require.NotNil(t, s)

	err := s.InitPredefinedBlocks(false)
	require.NoError(t, err)

	fmt.Printf("profile: %s\n", s.PredefinedBlocks().Profile)
	fmt.Printf("home: %s\n", s.PredefinedBlocks().Home)

	require.Len(t, s.PredefinedBlocks().Home, 57)
	require.Len(t, s.PredefinedBlocks().Profile, 57)
	require.Len(t, s.PredefinedBlocks().Archive, 57)

	tid, err := ProfileThreadIDFromAccountAddress(s.Account())
	require.NoError(t, err)

	require.Equal(t, s.PredefinedBlocks().Profile, tid.String())
}

func TestAnytype_CreateBlock(t *testing.T) {
	s := getRunningService(t)
	block, err := s.CreateBlock(smartblock.SmartBlockTypePage)
	require.NoError(t, err)
	require.Equal(t, block.Type(), smartblock.SmartBlockTypePage)
	require.Len(t, block.ID(), 57)
}
