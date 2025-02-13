package converter

import (
	"fmt"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider/mock_typeprovider"
)

const (
	root    = "root"
	spaceId = "space"
)

func TestLayoutConverter_Convert(t *testing.T) {
	store := objectstore.NewStoreFixture(t)
	store.AddObjects(t, spaceId, []spaceindex.TestObject{{
		bundle.RelationKeyId:        domain.String(bundle.TypeKeyTask.URL()),
		bundle.RelationKeySpaceId:   domain.String(spaceId),
		bundle.RelationKeyUniqueKey: domain.String(bundle.TypeKeyTask.URL()),
	}, {
		bundle.RelationKeyId:              domain.String(bundle.TypeKeySet.URL()),
		bundle.RelationKeySpaceId:         domain.String(spaceId),
		bundle.RelationKeyDefaultTypeId:   domain.String(bundle.TypeKeySet.URL()),
		bundle.RelationKeyDefaultViewType: domain.Int64(int64(model.BlockContentDataviewView_Gallery)),
	}})

	for _, from := range []model.ObjectTypeLayout{
		model.ObjectType_basic,
		model.ObjectType_note,
		model.ObjectType_todo,
		model.ObjectType_collection,
		model.ObjectType_tag,
	} {
		t.Run(fmt.Sprintf("convert from %s to set", from.String()), func(t *testing.T) {
			// given
			st := state.NewDoc(root, map[string]simple.Block{
				root: simple.New(&model.Block{Id: root, ChildrenIds: []string{}}),
			}).NewState()
			st.SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeySpaceId: domain.String(spaceId),
				bundle.RelationKeySetOf:   domain.StringList([]string{bundle.TypeKeyTask.URL()}),
				bundle.RelationKeyType:    domain.String(bundle.TypeKeySet.URL()),
			}))

			lc := layoutConverter{objectStore: store}

			// when
			err := lc.Convert(st, from, model.ObjectType_set, true)

			// then
			assert.NoError(t, err)
			dvb := st.Get(template.DataviewBlockId)
			assert.NotNil(t, dvb)
			dv := dvb.Model().GetDataview()
			require.NotNil(t, dv)
			assert.NotEmpty(t, dv.RelationLinks)
			assert.Len(t, dv.Views, 1)
			assert.Equal(t, bundle.TypeKeySet.URL(), dv.Views[0].DefaultObjectTypeId)
			assert.Equal(t, model.BlockContentDataviewView_Gallery, dv.Views[0].Type)
		})
	}
	t.Run("convert set to collection", func(t *testing.T) {
		// given
		st := state.NewDoc(root, map[string]simple.Block{
			root: simple.New(&model.Block{Id: root, ChildrenIds: []string{template.DataviewBlockId}}),
			template.DataviewBlockId: simple.New(&model.Block{Id: template.DataviewBlockId, ChildrenIds: []string{}, Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{
					{
						Id: "view1",
						Relations: []*model.BlockContentDataviewRelation{
							{
								Key: bundle.RelationKeyName.String(),
							},
							{
								Key: bundle.RelationKeyType.String(),
							},
						},
					},
					{
						Id: "view2",
						Relations: []*model.BlockContentDataviewRelation{
							{
								Key: bundle.RelationKeyName.String(),
							},
						},
					},
				},
				TargetObjectId: "id",
			}}}),
		}).NewState()
		st.SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeySpaceId: domain.String(spaceId),
			bundle.RelationKeySetOf:   domain.StringList([]string{bundle.TypeKeyTask.URL()}),
		}))

		provider := mock_typeprovider.NewMockSmartBlockTypeProvider(t)
		provider.EXPECT().PartitionIDsByType(spaceId, []string{bundle.TypeKeyTask.URL()}).Return(map[smartblock.SmartBlockType][]string{}, nil)
		lc := layoutConverter{objectStore: store, sbtProvider: provider}

		// when
		err := lc.Convert(st, model.ObjectType_set, model.ObjectType_collection, true)

		// then
		assert.NoError(t, err)
		dvb := st.Get(template.DataviewBlockId)
		assert.NotNil(t, dvb)
		dv := dvb.Model().GetDataview()
		require.NotNil(t, dv)
		assert.Len(t, dv.Views, 2)

		for _, view := range dv.Views {
			for _, relation := range template.DefaultCollectionRelations() {
				assert.True(t, lo.ContainsBy(view.Relations, func(item *model.BlockContentDataviewRelation) bool {
					return item.Key == relation.String()
				}))
			}
		}
	})
}

func TestInsertGroupRelationKey(t *testing.T) {
	relationLinksOfAllFormats := []*model.RelationLink{
		{Key: bundle.RelationKeyName.String(), Format: model.RelationFormat_longtext},
		{Key: bundle.RelationKeySnippet.String(), Format: model.RelationFormat_shorttext},
		{Key: bundle.RelationKeyRestrictions.String(), Format: model.RelationFormat_number},
		{Key: bundle.RelationKeyStatus.String(), Format: model.RelationFormat_status},
		{Key: bundle.RelationKeyTag.String(), Format: model.RelationFormat_tag},
		{Key: bundle.RelationKeyCreatedDate.String(), Format: model.RelationFormat_date},
		{Key: bundle.RelationKeyPicture.String(), Format: model.RelationFormat_file},
		{Key: bundle.RelationKeyIsHidden.String(), Format: model.RelationFormat_checkbox},
		{Key: bundle.RelationKeyUrl.String(), Format: model.RelationFormat_url},
		{Key: bundle.RelationKeyEmail.String(), Format: model.RelationFormat_email},
		{Key: bundle.RelationKeyPhone.String(), Format: model.RelationFormat_phone},
		{Key: bundle.RelationKeyIconEmoji.String(), Format: model.RelationFormat_emoji},
		{Key: bundle.RelationKeyLinks.String(), Format: model.RelationFormat_object},
		{Key: "?relations?", Format: model.RelationFormat_relations},
	}
	for _, tc := range []struct {
		name        string
		viewType    model.BlockContentDataviewViewType
		relLinks    []*model.RelationLink
		expectedKey domain.RelationKey
	}{
		{
			"kanban receives first status relation", model.BlockContentDataviewView_Kanban,
			[]*model.RelationLink{
				{Key: bundle.RelationKeyName.String(), Format: model.RelationFormat_longtext},
				{Key: bundle.RelationKeyCreatedDate.String(), Format: model.RelationFormat_date},
				{Key: bundle.RelationKeyRestrictions.String(), Format: model.RelationFormat_number},
				{Key: bundle.RelationKeyLinks.String(), Format: model.RelationFormat_object},
				{Key: bundle.RelationKeyStatus.String(), Format: model.RelationFormat_status},
				{Key: bundle.RelationKeyTag.String(), Format: model.RelationFormat_tag},
			},
			bundle.RelationKeyStatus,
		},
		{
			"kanban receives first checkbox relation", model.BlockContentDataviewView_Kanban,
			[]*model.RelationLink{
				{Key: bundle.RelationKeyName.String(), Format: model.RelationFormat_longtext},
				{Key: "不可譯的", Format: model.RelationFormat_checkbox},
				{Key: bundle.RelationKeyStatus.String(), Format: model.RelationFormat_status},
			},
			domain.RelationKey("不可譯的"),
		},
		{
			"no suitable relation for kanban", model.BlockContentDataviewView_Kanban,
			[]*model.RelationLink{
				{Key: bundle.RelationKeyName.String(), Format: model.RelationFormat_longtext},
				{Key: bundle.RelationKeyCreatedDate.String(), Format: model.RelationFormat_date},
				{Key: bundle.RelationKeyRestrictions.String(), Format: model.RelationFormat_number},
			},
			domain.RelationKey(""),
		},
		{
			"calendar receives first unhidden date relation", model.BlockContentDataviewView_Calendar,
			[]*model.RelationLink{
				{Key: bundle.RelationKeyName.String(), Format: model.RelationFormat_longtext},
				{Key: bundle.RelationKeyLastUsedDate.String(), Format: model.RelationFormat_date},
				{Key: bundle.RelationKeyCreatedDate.String(), Format: model.RelationFormat_date},
				{Key: bundle.RelationKeyLastModifiedDate.String(), Format: model.RelationFormat_date},
				{Key: bundle.RelationKeyRestrictions.String(), Format: model.RelationFormat_number},
			},
			bundle.RelationKeyCreatedDate,
		},
		{"table view", model.BlockContentDataviewView_Table, relationLinksOfAllFormats, domain.RelationKey("")},
		{"list view", model.BlockContentDataviewView_List, relationLinksOfAllFormats, domain.RelationKey("")},
		{"gallery view", model.BlockContentDataviewView_Gallery, relationLinksOfAllFormats, domain.RelationKey("")},
		{"graph view", model.BlockContentDataviewView_Graph, relationLinksOfAllFormats, domain.RelationKey("")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			block := &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{{
					Type: tc.viewType,
				}},
				RelationLinks: tc.relLinks,
			}}

			// when
			insertGroupRelationKey(block, tc.viewType)

			// then
			assert.Equal(t, tc.expectedKey, domain.RelationKey(block.Dataview.Views[0].GroupRelationKey))
		})
	}
}
