package editor

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/clipboard"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/stext"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/restriction"
	"github.com/anytypeio/go-anytype-middleware/core/relation/relationutils"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
	"strings"
)

func NewSubObject() *SubObject {
	sb := smartblock.New()
	return &SubObject{
		SmartBlock: sb,
		Basic:      basic.NewBasic(sb),
		IHistory:   basic.NewHistory(sb),
		Text:       stext.NewText(sb),
		Clipboard:  clipboard.NewClipboard(sb, nil),
		Dataview:   dataview.NewDataview(sb),
	}
}

type SubObject struct {
	smartblock.SmartBlock
	basic.Basic
	basic.IHistory
	stext.Text
	clipboard.Clipboard
	dataview.Dataview
}

func (o *SubObject) Init(ctx *smartblock.InitContext) (err error) {
	if err = o.SmartBlock.Init(ctx); err != nil {
		return
	}
	ot := pbtypes.GetString(ctx.State.CombinedDetails(), bundle.RelationKeyType.String())

	if strings.HasPrefix(ot, addr.BundledObjectTypeURLPrefix) {
		ot = addr.ObjectTypeKeyToIdPrefix + strings.TrimPrefix(ot, addr.BundledObjectTypeURLPrefix)
	}

	if strings.HasPrefix(ot, addr.BundledRelationURLPrefix) {
		ot = addr.RelationKeyToIdPrefix + strings.TrimPrefix(ot, addr.BundledRelationURLPrefix)
	}

	switch ot {
	case addr.ObjectTypeKeyToIdPrefix + bundle.TypeKeyRelation.String():
		return o.initRelation(ctx.State)
	case addr.ObjectTypeKeyToIdPrefix + bundle.TypeKeyObjectType.String():
		panic("not implemented") // should never happen because objectType case proceed by ObjectType implementation
	case addr.ObjectTypeKeyToIdPrefix + bundle.TypeKeyRelationOption.String():
		return o.initRelationOption(ctx.State)
	default:
		return fmt.Errorf("unknown subobject type %s", ot)
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
		template.WithAddedFeaturedRelation(bundle.RelationKeySourceObject),
		template.MigrateRelationValue(bundle.RelationKeySource, bundle.RelationKeySourceObject),
		template.WithTitle,
		template.WithDescription,
		fixTypes,
		template.WithDefaultFeaturedRelations,
		template.WithDataview(dataview, false))
}

func (o *SubObject) initRelationOption(st *state.State) error {
	// temp fix for our internal accounts with inconsistent types (should be removed later)
	// todo: remove after release
	fixTypes := func(s *state.State) {
		if list := pbtypes.GetStringList(s.Details(), bundle.RelationKeyRelationFormatObjectTypes.String()); list != nil {
			list, _ = relationutils.MigrateObjectTypeIds(list)
			s.SetDetail(bundle.RelationKeyRelationFormatObjectTypes.String(), pbtypes.StringList(list))
		}
	}

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
		template.WithTitle,
		fixTypes,
		template.WithDefaultFeaturedRelations,
		template.WithDataview(dataview, false))
}
