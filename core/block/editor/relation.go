package editor

import (
	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/block/editor/file"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/getblock"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/relation"
	"github.com/anyproto/anytype-heart/core/relation/relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/typeprovider"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type Relation struct {
	*SubObject
}

func NewRelation(
	sb smartblock.SmartBlock,
	objectStore objectstore.ObjectStore,
	fileBlockService file.BlockService,
	anytype core.Service,
	relationService relation.Service,
	tempDirProvider core.TempDirProvider,
	sbtProvider typeprovider.SmartBlockTypeProvider,
	layoutConverter converter.LayoutConverter,
	fileService files.Service,
	picker getblock.Picker,
) *Relation {
	return &Relation{
		SubObject: NewSubObject(
			sb,
			objectStore,
			fileBlockService,
			anytype,
			relationService,
			tempDirProvider,
			sbtProvider,
			layoutConverter,
			fileService,
			picker,
		),
	}
}

func (r *Relation) Init(ctx *smartblock.InitContext) error {
	if err := r.SubObject.Init(ctx); err != nil {
		return err
	}

	return nil
}

func (r *Relation) InitState(st *state.State) {
	// temp fix for our internal accounts with inconsistent types (should be removed later)
	// todo: remove after release
	fixTypes := func(s *state.State) {
		if list := pbtypes.GetStringList(s.Details(), bundle.RelationKeyRelationFormatObjectTypes.String()); list != nil {
			list, _ = relationutils.MigrateObjectTypeIds(list)
			s.SetDetail(bundle.RelationKeyRelationFormatObjectTypes.String(), pbtypes.StringList(list))
		}
	}

	maxCountForStatus := func(s *state.State) {
		if f := pbtypes.GetFloat64(s.Details(), bundle.RelationKeyRelationFormat.String()); int32(f) == int32(model.RelationFormat_status) {
			if maxCount := pbtypes.GetFloat64(s.Details(), bundle.RelationKeyRelationMaxCount.String()); maxCount == 0 {
				s.SetDetail(bundle.RelationKeyRelationMaxCount.String(), pbtypes.Int64(1))
			}
		}
	}

	relKey := pbtypes.GetString(st.Details(), bundle.RelationKeyRelationKey.String())
	dataview := model.BlockContentOfDataview{
		Dataview: &model.BlockContentDataview{
			Source: []string{st.RootId()},
			Views: []*model.BlockContentDataviewView{
				{
					Id:   "1",
					Type: model.BlockContentDataviewView_Table,
					Name: "All",
					Sorts: []*model.BlockContentDataviewSort{
						{
							RelationKey: relKey,
							Type:        model.BlockContentDataviewSort_Asc,
						},
					},
					Relations: []*model.BlockContentDataviewRelation{{
						Key:       bundle.RelationKeyName.String(),
						IsVisible: true,
					},
						{
							Key:       relKey,
							IsVisible: true,
						},
					},
					Filters: nil,
				},
			},
		},
	}

	template.InitTemplate(st,
		template.WithAllBlocksEditsRestricted,
		template.WithForcedDetail(bundle.RelationKeyLayout, pbtypes.Int64(int64(model.ObjectType_relation))),
		template.WithForcedDetail(bundle.RelationKeyIsReadonly, pbtypes.Bool(false)),
		template.WithForcedDetail(bundle.RelationKeyType, pbtypes.String(bundle.TypeKeyRelation.URL())),
		template.WithAddedFeaturedRelation(bundle.RelationKeySourceObject),
		template.WithTitle,
		template.WithDescription,
		fixTypes,
		maxCountForStatus,
		template.WithDefaultFeaturedRelations,
		template.WithDataview(dataview, false))
}
