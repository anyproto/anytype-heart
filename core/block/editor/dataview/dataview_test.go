package dataview

import (
	"errors"
	"fmt"
	"testing"

	anystore "github.com/anyproto/any-store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/dataview"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/mock_objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const objId = "root"

type fixture struct {
	store *mock_objectstore.MockObjectStore
	sb    *smarttest.SmartTest

	*sdataview
}

func newFixture(t *testing.T) *fixture {
	store := mock_objectstore.NewMockObjectStore(t)
	sb := smarttest.New(objId)

	dv := NewDataview(sb, store).(*sdataview)

	return &fixture{
		store:     store,
		sb:        sb,
		sdataview: dv,
	}
}

func TestDataviewCollectionImpl_SetViewPosition(t *testing.T) {
	newTestDv := func() (Dataview, *smarttest.SmartTest) {
		fx := newFixture(t)
		sbs := fx.sb.Doc.(*state.State)
		sbs.Add(simple.New(&model.Block{Id: objId, ChildrenIds: []string{"dv"}}))
		sbs.Add(simple.New(&model.Block{Id: "dv", Content: &model.BlockContentOfDataview{
			Dataview: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{
					{Id: "1"},
					{Id: "2"},
					{Id: "3"},
				},
			},
		}}))
		fx.store.EXPECT().GetActiveViews(mock.Anything).Return(nil, nil).Maybe()
		return fx.sdataview, fx.sb
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

func TestInjectActiveView(t *testing.T) {
	dv1 := "dataview1"
	dv2 := "dataview2"
	dv3 := "dataview3"

	getInfo := func() smartblock.ApplyInfo {
		st := state.NewDoc(objId, map[string]simple.Block{
			objId: simple.New(&model.Block{Id: objId, ChildrenIds: []string{dv1, dv2, dv3}}),
			dv1: dataview.NewDataview(&model.Block{
				Id:      dv1,
				Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{}},
			}),
			dv2: dataview.NewDataview(&model.Block{
				Id:      dv2,
				Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{}},
			}),
			dv3: dataview.NewDataview(&model.Block{
				Id:      dv3,
				Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{}},
			}),
		}).(*state.State)
		return smartblock.ApplyInfo{State: st}
	}

	t.Run("inject active views to dataview blocks", func(t *testing.T) {
		// given
		blocksToView := map[string]string{dv1: "view1", dv2: "view2"}
		fx := newFixture(t)
		fx.store.EXPECT().GetActiveViews(mock.Anything).RunAndReturn(func(id string) (map[string]string, error) {
			assert.Equal(t, objId, id)
			return blocksToView, nil
		})
		info := getInfo()

		// when
		err := fx.injectActiveViews(info)
		st := info.State

		// then
		assert.NoError(t, err)
		assert.Equal(t, blocksToView[dv1], st.Pick(dv1).Model().GetDataview().ActiveView)
		assert.Equal(t, blocksToView[dv2], st.Pick(dv2).Model().GetDataview().ActiveView)
		assert.Empty(t, st.Pick(dv3).Model().GetDataview().ActiveView)
	})

	t.Run("do nothing if active views are not found in DB", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.store.EXPECT().GetActiveViews(mock.Anything).RunAndReturn(func(id string) (map[string]string, error) {
			assert.Equal(t, objId, id)
			return nil, anystore.ErrDocNotFound
		})
		info := getInfo()

		// when
		err := fx.injectActiveViews(info)

		// then
		assert.NoError(t, err)
	})

	t.Run("fail on other DB error", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.store.EXPECT().GetActiveViews(mock.Anything).RunAndReturn(func(id string) (map[string]string, error) {
			assert.Equal(t, objId, id)
			return nil, errors.New("badger was stolen by UFO")
		})
		info := getInfo()

		// when
		err := fx.injectActiveViews(info)

		// then
		assert.Error(t, err)
	})
}
