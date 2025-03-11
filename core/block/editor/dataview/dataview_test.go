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
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
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
				bundle.RelationKeySpaceId:     domain.String(spcId),
				bundle.RelationKeyId:          domain.String("rel-id"),
				bundle.RelationKeyRelationKey: domain.String("id"),
			},
			{
				bundle.RelationKeySpaceId:     domain.String(spcId),
				bundle.RelationKeyId:          domain.String("rel-name"),
				bundle.RelationKeyRelationKey: domain.String("name"),
			},
		})

		// when
		err := fx.SetSource(nil, "dv", source)

		// then
		assert.NoError(t, err)
		setOf := fx.sb.CombinedDetails().GetStringList(bundle.RelationKeySetOf)
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
		err := fx.sb.SetDetails(nil, []domain.Detail{{
			Key:   bundle.RelationKeySetOf,
			Value: domain.StringList([]string{"ot-bookmark"}),
		}}, false)
		require.NoError(t, err)

		// when
		err = fx.SetSource(nil, "dv", nil)

		// then
		assert.NoError(t, err)
		setOf := fx.sb.CombinedDetails().GetStringList(bundle.RelationKeySetOf)
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
		err := fx.sb.SetDetails(nil, []domain.Detail{{
			Key:   bundle.RelationKeySetOf,
			Value: domain.StringList([]string{"rel-name", "rel-id"}),
		}, {
			Key:   bundle.RelationKeyInternalFlags,
			Value: domain.Int64List([]int64{int64(model.InternalFlag_editorDeleteEmpty)}),
		}}, false)
		require.NoError(t, err)

		fx.store.AddObjects(t, []objectstore.TestObject{map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:                           domain.String(bundle.TypeKeyPage.URL()),
			bundle.RelationKeySpaceId:                      domain.String(spcId),
			bundle.RelationKeyUniqueKey:                    domain.String(bundle.TypeKeyPage.URL()),
			bundle.RelationKeyType:                         domain.String(bundle.TypeKeyObjectType.URL()),
			bundle.RelationKeyRecommendedRelations:         domain.StringList([]string{bundle.RelationKeyAssignee.URL(), bundle.RelationKeyDone.URL()}),
			bundle.RelationKeyRecommendedFeaturedRelations: domain.StringList([]string{bundle.RelationKeyType.URL(), bundle.RelationKeyBacklinks.URL(), bundle.RelationKeyDone.URL()}),
			bundle.RelationKeyRecommendedFileRelations:     domain.StringList([]string{bundle.RelationKeyFileExt.URL()}),
			bundle.RelationKeyRecommendedHiddenRelations:   domain.StringList([]string{bundle.RelationKeyTag.URL()}),
		}, generateTestRelationObject(bundle.RelationKeyAssignee, model.RelationFormat_object),
			generateTestRelationObject(bundle.RelationKeyDone, model.RelationFormat_checkbox),
			generateTestRelationObject(bundle.RelationKeyType, model.RelationFormat_object),
			generateTestRelationObject(bundle.RelationKeyBacklinks, model.RelationFormat_object),
			generateTestRelationObject(bundle.RelationKeyFileExt, model.RelationFormat_shorttext),
			generateTestRelationObject(bundle.RelationKeyTag, model.RelationFormat_tag),
		})

		// when
		err = fx.SetSourceInSet(nil, []string{bundle.TypeKeyPage.URL()})

		// then
		assert.NoError(t, err)
		setOf := fx.sb.NewState().Details().GetStringList(bundle.RelationKeySetOf)
		require.Len(t, setOf, 1)
		assert.Equal(t, "ot-page", setOf[0])

		b := fx.sb.Pick(template.DataviewBlockId)
		require.NotNil(t, b)
		dv := b.Model().GetDataview()
		require.NotNil(t, dv)
		require.Len(t, dv.Views, 2)
		assert.Len(t, dv.RelationLinks, 12) // 7 default + 6 recommended - 1 common (backlinks)
		assert.Empty(t, dv.Views[0].DefaultTemplateId)
		assert.Empty(t, dv.Views[0].DefaultObjectTypeId)
		assert.Len(t, dv.Views[0].Relations, 12)
		assert.Empty(t, dv.Views[1].DefaultTemplateId)
		assert.Empty(t, dv.Views[1].DefaultObjectTypeId)
		assert.Len(t, dv.Views[1].Relations, 12)

		assert.Empty(t, fx.sb.NewState().Details().GetInt64List(bundle.RelationKeyInternalFlags))
	})

	// TODO: GO-4189 Add more tests when more logic on SetSourceToSet will be added
}

func generateTestRelationObject(key domain.RelationKey, format model.RelationFormat) objectstore.TestObject {
	return objectstore.TestObject{
		bundle.RelationKeyId:             domain.String(key.URL()),
		bundle.RelationKeyRelationKey:    domain.String(key.String()),
		bundle.RelationKeyType:           domain.String(bundle.TypeKeyRelation.URL()),
		bundle.RelationKeyRelationFormat: domain.Int64(format),
	}
}
