package core

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/anytypeio/go-anytype-library/core/smartblock"
	"github.com/anytypeio/go-anytype-library/database"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/structs"
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
	require.True(t, s.(*Anytype).isStarted)
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

	block2, err := s.CreateBlock(smartblock.SmartBlockTypePage)
	require.NoError(t, err)

	details1 := &types.Struct{Fields: map[string]*types.Value{"name": structs.String("block1_name")}}
	blocks1 := []*model.Block{
		{
			Id:      "test_id1",
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "Kademlia is a distributed hash table for decentralized peer-to-peer computer networks designed by Petar Maymounkov and David Mazières in 2002.[1][2] It specifies the structure of the network and the exchange of information through node lookups. Kademlia nodes communicate among themselves using UDP. A virtual or overlay network is formed by the participant nodes. Each node is identified by a number or node ID. The node ID serves not only as identification, but the Kademlia algorithm uses the node ID to locate values (usually file hashes or keywords). In fact, the node ID provides a direct map to file hashes and that node stores information on where to obtain the file or resource."}},
		},
	}
	err = block1.(*smartBlock).indexSnapshot(details1, blocks1)
	require.NoError(t, err)

	details2 := &types.Struct{Fields: map[string]*types.Value{"name": structs.String("block2_name")}}
	blocks2 := []*model.Block{
		{
			Id:      "test_id2",
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "Kademlia is a distributed hash table for decentralized peer-to-peer computer networks designed by Petar Maymounkov and David Mazières in 2002.[1][2] It specifies the structure of the network and the exchange of information through node lookups. Kademlia nodes communicate among themselves using UDP. A virtual or overlay network is formed by the participant nodes. Each node is identified by a number or node ID. The node ID serves not only as identification, but the Kademlia algorithm uses the node ID to locate values (usually file hashes or keywords). In fact, the node ID provides a direct map to file hashes and that node stores information on where to obtain the file or resource."}},
		},
	}

	err = block2.(*smartBlock).indexSnapshot(details2, blocks2)
	require.NoError(t, err)

	db, err := s.DatabaseByID("pages")
	require.NoError(t, err)
	require.Equal(t, "https://anytype.io/schemas/page", db.Schema())

	results, total, err := db.Query(database.Query{Limit: 1, Sorts: []*model.BlockContentDataviewSort{{RelationId: "name"}}})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, 2, total)
	require.Equal(t, details1.Fields["name"].GetStringValue(), results[0].Details.Fields["name"].GetStringValue())
	require.Equal(t, block1.ID(), results[0].Details.Fields["id"].GetStringValue())

	results, total, err = db.Query(database.Query{Limit: 10, Filters: []*model.BlockContentDataviewFilter{{
		Operator:   model.BlockContentDataviewFilter_And,
		RelationId: "name",
		Condition:  model.BlockContentDataviewFilter_Like,
		Value:      structs.String("lock1"),
	}},

		Sorts: []*model.BlockContentDataviewSort{{RelationId: "name"}}})

	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, total, 1)

	n := time.Now()
	nowTruncatedToDay := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)

	s.PageStore().UpdateLastOpened(block1.ID(), time.Now())
	results, total, err = db.Query(database.Query{Limit: 10, Filters: []*model.BlockContentDataviewFilter{{
		Operator:   model.BlockContentDataviewFilter_And,
		RelationId: "lastOpened",
		Condition:  model.BlockContentDataviewFilter_Equal,
		Value:      structs.Float64(float64(nowTruncatedToDay.Unix())),
	}},
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, total, 1)

	s.PageStore().UpdateLastModified(block1.ID(), time.Now())
	s.PageStore().UpdateLastModified(block2.ID(), time.Now())

	results, total, err = db.Query(database.Query{Limit: 10, Filters: []*model.BlockContentDataviewFilter{{
		Operator:   model.BlockContentDataviewFilter_And,
		RelationId: "lastModified",
		Condition:  model.BlockContentDataviewFilter_Equal,
		Value:      structs.Float64(float64(nowTruncatedToDay.Unix())),
	}},
	})
	require.NoError(t, err)
	require.Len(t, results, 2)
	require.Equal(t, total, 2)

	nextDay := time.Date(n.Year(), n.Month(), n.Day()+1, 0, 0, 0, 0, time.UTC)

	results, total, err = db.Query(database.Query{Limit: 10, Filters: []*model.BlockContentDataviewFilter{{
		Operator:   model.BlockContentDataviewFilter_And,
		RelationId: "lastModified",
		Condition:  model.BlockContentDataviewFilter_Less,
		Value:      structs.Float64(float64(nextDay.Unix())),
	}},
	})
	require.NoError(t, err)
	require.Len(t, results, 2)
	require.Equal(t, total, 2)

	prevDay := time.Date(n.Year(), n.Month(), n.Day()-1, 0, 0, 0, 0, time.UTC)

	results, total, err = db.Query(database.Query{Limit: 10, Filters: []*model.BlockContentDataviewFilter{{
		Operator:   model.BlockContentDataviewFilter_And,
		RelationId: "lastModified",
		Condition:  model.BlockContentDataviewFilter_Greater,
		Value:      structs.Float64(float64(prevDay.Unix())),
	}},
	})
	require.NoError(t, err)
	require.Len(t, results, 2)
	require.Equal(t, total, 2)

	results, total, err = db.Query(database.Query{Limit: 10, Filters: []*model.BlockContentDataviewFilter{{
		Operator:   model.BlockContentDataviewFilter_And,
		RelationId: "lastModified",
		Condition:  model.BlockContentDataviewFilter_Greater,
		Value:      structs.Float64(float64(nextDay.Unix())),
	}},
	})
	require.NoError(t, err)
	require.Len(t, results, 0)
	require.Equal(t, total, 0)
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
