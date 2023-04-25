package editor

import (
	"fmt"

	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/clipboard"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/file"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/stext"
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

type SubObject struct {
	forcedObjectType bundle.TypeKey

	smartblock.SmartBlock

	basic.AllOperations
	basic.IHistory
	stext.Text
	clipboard.Clipboard
	dataview.Dataview
}

func NewSubObject(
	objectStore objectstore.ObjectStore,
	fileBlockService file.BlockService,
	anytype core.Service,
	relationService relation2.Service,
	forcedObjectType bundle.TypeKey,
	tempDirProvider core.TempDirProvider,
	sbtProvider typeprovider.SmartBlockTypeProvider,
) *SubObject {
	sb := smartblock.New()
	return &SubObject{
		SmartBlock:    sb,
		AllOperations: basic.NewBasic(sb),
		IHistory:      basic.NewHistory(sb),
		Text: stext.NewText(
			sb,
			objectStore,
		),
		Clipboard: clipboard.NewClipboard(
			sb,
			file.NewFile(
				sb,
				fileBlockService,
				anytype,
				tempDirProvider,
			),
			anytype,
			tempDirProvider,
		),
		Dataview: dataview.NewDataview(
			sb,
			anytype,
			objectStore,
			relationService,
			sbtProvider,
		),
		forcedObjectType: forcedObjectType,
	}
}

func (o *SubObject) Init(ctx *smartblock.InitContext) (err error) {
	objectType := o.forcedObjectType
	if objectType == "" {
		if len(ctx.ObjectTypeUrls) == 1 {
			objectType, err = bundle.TypeKeyFromUrl(ctx.ObjectTypeUrls[0])
			if err != nil {
				return err
			}
		} else {
			return fmt.Errorf("missing object type for subobject")
		}
	}

	if err = o.SmartBlock.Init(ctx); err != nil {
		return
	}

	switch objectType {
	case bundle.TypeKeyRelation:
		return o.initRelation(ctx.State)
	case bundle.TypeKeyObjectType:
		panic("not implemented") // should never happen because objectType case proceed by ObjectType implementation
	case bundle.TypeKeyRelationOption:
		return o.initRelationOption(ctx.State)
	default:
		return fmt.Errorf("unknown subobject type %s", objectType)
	}

}

func (o *SubObject) SetStruct(st *types.Struct) error {
	o.Lock()
	defer o.Unlock()
	s := o.NewState()
	s.SetDetails(st)
	return o.Apply(s)
}

func (o *SubObject) initRelation(st *state.State) error {
	var system bool
	for _, rel := range bundle.SystemRelations {
		if addr.RelationKeyToIdPrefix+rel.String() == o.RootId() {
			system = true
			break
		}
	}
	if system {
		rest := o.Restrictions()
		obj := append(rest.Object.Copy(), []model.RestrictionsObjectRestriction{model.Restrictions_Delete, model.Restrictions_Relations, model.Restrictions_Details}...)
		o.SetRestrictions(restriction.Restrictions{Object: obj, Dataview: rest.Dataview})
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

	return smartblock.ObjectApplyTemplate(o, st,
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

func (o *SubObject) initRelationOption(st *state.State) error {
	relKey := pbtypes.GetString(st.Details(), bundle.RelationKeyRelationKey.String())
	dataview := model.BlockContentOfDataview{
		Dataview: &model.BlockContentDataview{
			Source: []string{relKey},
			Views: []*model.BlockContentDataviewView{
				{
					Id:   "1",
					Type: model.BlockContentDataviewView_Table,
					Name: "All",
					Sorts: []*model.BlockContentDataviewSort{
						{
							RelationKey: "name",
							Type:        model.BlockContentDataviewSort_Asc,
						},
					},
					Relations: []*model.BlockContentDataviewRelation{},
					Filters: []*model.BlockContentDataviewFilter{{
						RelationKey: relKey,
						Condition:   model.BlockContentDataviewFilter_In,
						Value:       pbtypes.String(st.RootId()),
					}},
				},
			},
		},
	}

	return smartblock.ObjectApplyTemplate(o, st,
		template.WithAllBlocksEditsRestricted,
		template.WithForcedDetail(bundle.RelationKeyLayout, pbtypes.Int64(int64(model.ObjectType_relationOption))),
		template.WithForcedDetail(bundle.RelationKeyIsReadonly, pbtypes.Bool(false)),
		template.WithForcedDetail(bundle.RelationKeyType, pbtypes.String(bundle.TypeKeyRelationOption.URL())),
		template.WithTitle,
		template.WithDefaultFeaturedRelations,
		template.WithDataview(dataview, false))
}
