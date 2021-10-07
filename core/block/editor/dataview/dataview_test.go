package dataview

import (
	"fmt"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock/smarttest"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_calculateEntriesDiff(t *testing.T) {
	a := []database.Record{
		{Details: &types.Struct{
			Fields: map[string]*types.Value{
				"id":   pbtypes.String("id1"),
				"name": pbtypes.String("name1"),
			},
		}},
		{Details: &types.Struct{
			Fields: map[string]*types.Value{
				"id":   pbtypes.String("id2"),
				"name": pbtypes.String("name2"),
			},
		}},
		{Details: &types.Struct{
			Fields: map[string]*types.Value{
				"id":   pbtypes.String("id3"),
				"name": pbtypes.String("name3"),
			},
		}},
	}

	b := []database.Record{
		{Details: &types.Struct{
			Fields: map[string]*types.Value{
				"id":   pbtypes.String("id1"),
				"name": pbtypes.String("name1_change"),
			},
		}},
		{Details: &types.Struct{
			Fields: map[string]*types.Value{
				"id":   pbtypes.String("id2"),
				"name": pbtypes.String("name2"),
			},
		}},
		{Details: &types.Struct{
			Fields: map[string]*types.Value{
				"id":   pbtypes.String("id4"),
				"name": pbtypes.String("name4"),
			},
		}},
	}

	updated, removed, inserted := calculateEntriesDiff(a, b)

	require.Len(t, updated, 1)
	require.Len(t, removed, 1)
	require.Len(t, inserted, 1)

	require.Equal(t, b[0].Details, updated[0])
	require.Equal(t, "id3", removed[0])
	require.Equal(t, 2, inserted[0].position)
	require.Equal(t, []*types.Struct{b[2].Details}, inserted[0].entries)
}

func TestDataviewCollectionImpl_SetViewPosition(t *testing.T) {
	newTestDv := func() (Dataview, *smarttest.SmartTest) {
		sb := smarttest.New("root")
		sbs := sb.Doc.(*state.State)
		sbs.Add(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"dv"}}))
		sbs.Add(simple.New(&model.Block{Id: "dv", Content: &model.BlockContentOfDataview{
			Dataview: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{
					{Id: "1"},
					{Id: "2"},
					{Id: "3"},
				},
			},
		}}))
		return NewDataview(sb), sb
	}
	assertViewPositions := func(viewId string, pos uint32, exp []string) {
		dv, sb := newTestDv()
		ctx := state.NewContext(nil)
		err := dv.SetViewPosition(ctx, "dv", viewId, pos)
		require.NoError(t, err)
		views := sb.Doc.Pick("dv").Model().GetDataview().Views
		var viewIds []string
		for _, v := range views {
			viewIds = append(viewIds, v.Id)
		}
		assert.Equal(t, exp, viewIds, fmt.Sprintf("viewId: %s; pos: %d", viewId, pos))
	}

	assertViewPositions("2", 0, []string{"2", "1", "3"})
	assertViewPositions("2", 2, []string{"1", "3", "2"})
	assertViewPositions("1", 0, []string{"1", "2", "3"})
	assertViewPositions("1", 42, []string{"2", "3", "1"})
}
