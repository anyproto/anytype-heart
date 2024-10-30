package dataview

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/dataview"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

const (
	objId = "root"
	spcId = "spaceId"
)

type fixture struct {
	store *spaceindex.StoreFixture
	sb    *smarttest.SmartTest

	*sdataview
}

func newFixture(t *testing.T) *fixture {
	store := spaceindex.NewStoreFixture(t)
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
		err := fx.store.SetActiveViews(objId, blocksToView)
		require.NoError(t, err)
		info := getInfo()

		// when
		err = fx.injectActiveViews(info)
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
		info := getInfo()

		// when
		err := fx.injectActiveViews(info)

		// then
		assert.NoError(t, err)
	})
}

func TestDataview_SetSource(t *testing.T) {
	t.Run("set source to dataview block", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.sb.AddBlock(simple.New(&model.Block{Id: objId, ChildrenIds: []string{"dv"}}))
		fx.sb.AddBlock(simple.New(&model.Block{Id: "dv", Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{}}}))
		source := []string{"rel-name", "rel-id"}

		fx.store.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeySpaceId:     pbtypes.String(spcId),
				bundle.RelationKeyId:          pbtypes.String("rel-id"),
				bundle.RelationKeyRelationKey: pbtypes.String("id"),
			},
			{
				bundle.RelationKeySpaceId:     pbtypes.String(spcId),
				bundle.RelationKeyId:          pbtypes.String("rel-name"),
				bundle.RelationKeyRelationKey: pbtypes.String("name"),
			},
		})

		// when
		err := fx.SetSource(nil, "dv", source)

		// then
		assert.NoError(t, err)
		setOf := pbtypes.GetStringList(fx.sb.LocalDetails(), bundle.RelationKeySetOf.String())
		require.Len(t, setOf, 2)
		assert.True(t, slice.UnsortedEqual(setOf, source))

		block := fx.sb.Pick("dv")
		assert.NotNil(t, block)
		_, ok := block.(dataview.Block)
		require.True(t, ok)
	})

	t.Run("unset source from inline dataview block", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.sb.AddBlock(simple.New(&model.Block{Id: objId, ChildrenIds: []string{"dv"}}))
		fx.sb.AddBlock(simple.New(&model.Block{Id: "dv", Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
			Source: []string{"ot-bookmark"},
		}}}))
		err := fx.sb.SetDetails(nil, []*model.Detail{{
			Key:   bundle.RelationKeySetOf.String(),
			Value: pbtypes.StringList([]string{"ot-bookmark"}),
		}}, false)
		require.NoError(t, err)

		// when
		err = fx.SetSource(nil, "dv", nil)

		// then
		assert.NoError(t, err)
		setOf := pbtypes.GetStringList(fx.sb.LocalDetails(), bundle.RelationKeySetOf.String())
		assert.Len(t, setOf, 0)

		block := fx.sb.Pick("dv")
		assert.Nil(t, block)
	})
}

func TestDataview_SetSourceInSet(t *testing.T) {
	t.Run("no error", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.sb.AddBlock(simple.New(&model.Block{Id: objId, ChildrenIds: []string{template.DataviewBlockId}}))
		fx.sb.AddBlock(simple.New(&model.Block{Id: template.DataviewBlockId, Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{Views: []*model.BlockContentDataviewView{
			{DefaultObjectTypeId: "ot-note", DefaultTemplateId: "NoTe"},
			{DefaultObjectTypeId: "ot-task", DefaultTemplateId: "tAsK"},
		}}}}))
		err := fx.sb.SetDetails(nil, []*model.Detail{{
			Key:   bundle.RelationKeySetOf.String(),
			Value: pbtypes.StringList([]string{"rel-name", "rel-id"}),
		}, {
			Key:   bundle.RelationKeyInternalFlags.String(),
			Value: pbtypes.IntList(int(model.InternalFlag_editorDeleteEmpty)),
		}}, false)
		require.NoError(t, err)

		// when
		err = fx.SetSourceInSet(nil, []string{"ot-page"})

		// then
		assert.NoError(t, err)
		setOf := pbtypes.GetStringList(fx.sb.NewState().Details(), bundle.RelationKeySetOf.String())
		require.Len(t, setOf, 1)
		assert.Equal(t, "ot-page", setOf[0])

		b := fx.sb.Pick(template.DataviewBlockId)
		require.NotNil(t, b)
		dv := b.Model().GetDataview()
		require.NotNil(t, dv)
		require.Len(t, dv.Views, 2)
		assert.Empty(t, dv.Views[0].DefaultTemplateId)
		assert.Empty(t, dv.Views[0].DefaultObjectTypeId)
		assert.Empty(t, dv.Views[1].DefaultTemplateId)
		assert.Empty(t, dv.Views[1].DefaultObjectTypeId)

		assert.Empty(t, pbtypes.GetIntList(fx.sb.NewState().Details(), bundle.RelationKeyInternalFlags.String()))
	})

	// TODO: GO-4189 Add more tests when more logic on SetSourceToSet will be added
}
