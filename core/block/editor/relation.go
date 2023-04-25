package editor

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/file"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/restriction"
	relation2 "github.com/anytypeio/go-anytype-middleware/core/relation"
	"github.com/anytypeio/go-anytype-middleware/core/relation/relationutils"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/space/typeprovider"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

type Relation struct {
	*SubObject
}

func NewRelation(
	objectStore objectstore.ObjectStore,
	fileBlockService file.BlockService,
	anytype core.Service,
	relationService relation2.Service,
	tempDirProvider core.TempDirProvider,
	sbtProvider typeprovider.SmartBlockTypeProvider,
	layoutConverter converter.LayoutConverter,
) *Relation {
	return &Relation{
		SubObject: NewSubObject(
			objectStore,
			fileBlockService,
			anytype,
			relationService,
			tempDirProvider,
			sbtProvider,
			layoutConverter,
		),
	}
}

func (r *Relation) Init(ctx *smartblock.InitContext) error {
	if err := r.SubObject.Init(ctx); err != nil {
		return err
	}

	st := ctx.State

	var system bool
	for _, rel := range bundle.SystemRelations {
		if addr.RelationKeyToIdPrefix+rel.String() == r.RootId() {
			system = true
			break
		}
	}
	if system {
		rest := r.Restrictions()
		obj := append(rest.Object.Copy(), []model.RestrictionsObjectRestriction{model.Restrictions_Delete, model.Restrictions_Relations, model.Restrictions_Details}...)
		r.SetRestrictions(restriction.Restrictions{Object: obj, Dataview: rest.Dataview})
	}

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

	return smartblock.ObjectApplyTemplate(r, st,
		template.WithAllBlocksEditsRestricted,
		template.WithForcedDetail(bundle.RelationKeyLayout, pbtypes.Int64(int64(model.ObjectType_relation))),
		template.WithForcedDetail(bundle.RelationKeyIsReadonly, pbtypes.Bool(false)),
		template.WithForcedDetail(bundle.RelationKeyType, pbtypes.String(bundle.TypeKeyRelation.URL())),
		template.WithAddedFeaturedRelation(bundle.RelationKeySourceObject),
		template.MigrateRelationValue(bundle.RelationKeySource, bundle.RelationKeySourceObject),
		template.WithTitle,
		template.WithDescription,
		fixTypes,
		maxCountForStatus,
		template.WithDefaultFeaturedRelations,
		template.WithDataview(dataview, false))
}
