package dataview

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

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

		return NewDataview(sb, nil), sb
	}
	assertViewPositions := func(viewId string, pos uint32, exp []string) {
		dv, sb := newTestDv()
		ctx := session.NewContext()
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
