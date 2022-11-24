package editor

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	dataview "github.com/anytypeio/go-anytype-middleware/core/block/editor/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/stext"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

var ErrAlreadyHasDataviewBlock = fmt.Errorf("already has the dataview block")

func NewSet() *Set {
	sb := &Set{
		SmartBlock: smartblock.New(),
	}

	sb.Basic = basic.NewBasic(sb)
	sb.IHistory = basic.NewHistory(sb)
	sb.Dataview = dataview.NewDataview(sb)
	sb.Text = stext.NewText(sb)
	return sb
}

type Set struct {
	smartblock.SmartBlock
	basic.Basic
	basic.IHistory
	dataview.Dataview
	stext.Text
}

func (p *Set) Init(ctx *smartblock.InitContext) (err error) {
	err = p.SmartBlock.Init(ctx)
	if err != nil {
		return err
	}

	var featuredRelations []string
	if ctx.State != nil {
		featuredRelations = pbtypes.GetStringList(ctx.State.Details(), bundle.RelationKeyFeaturedRelations.String())
	}
	// Add missing required featured relations
	featuredRelations = slice.Union(featuredRelations, []string{bundle.RelationKeyDescription.String(), bundle.RelationKeyType.String(), bundle.RelationKeySetOf.String()})

	templates := []template.StateTransformer{
		template.WithDataviewRelationMigrationRelation(template.DataviewBlockId, bundle.TypeKeyBookmark.URL(), bundle.RelationKeyUrl, bundle.RelationKeySource),
		template.WithObjectTypesAndLayout([]string{bundle.TypeKeySet.URL()}, model.ObjectType_set),
		template.WithForcedDetail(bundle.RelationKeyLayout, pbtypes.Float64(float64(model.ObjectType_set))),
		template.WithForcedDetail(bundle.RelationKeyFeaturedRelations, pbtypes.StringList(featuredRelations)),
		template.WithRelations([]bundle.RelationKey{bundle.RelationKeySetOf}),
		template.WithDescription,
		template.WithFeaturedRelations,
		template.WithBlockEditRestricted(p.Id()),
		template.WithCreatorRemovedFromFeaturedRelations,
	}
	if dvBlock := p.Pick(template.DataviewBlockId); dvBlock != nil {
		setOf := dvBlock.Model().GetDataview().GetSource()
		if len(setOf) == 0 {
			log.With("thread", p.Id()).With("sbType", p.SmartBlock.Type().String()).Errorf("dataview has an empty source")
		} else {
			templates = append(templates, template.WithForcedDetail(bundle.RelationKeySetOf, pbtypes.StringList(setOf)))
		}
		// add missing done relation
		templates = append(templates, template.WithDataviewRequiredRelation(template.DataviewBlockId, bundle.RelationKeyDone))
	}
	templates = append(templates, template.WithTitle)
	return smartblock.ObjectApplyTemplate(p, ctx.State, templates...)
}

func GetDefaultViewRelations(rels []*model.Relation) []*model.BlockContentDataviewRelation {
	var viewRels = make([]*model.BlockContentDataviewRelation, 0, len(rels))
	for _, rel := range rels {
		if rel.Hidden && rel.Key != bundle.RelationKeyName.String() {
			continue
		}
		var visible bool
		if rel.Key == bundle.RelationKeyName.String() {
			visible = true
		}
		viewRels = append(viewRels, &model.BlockContentDataviewRelation{Key: rel.Key, IsVisible: visible})
	}
	return viewRels
}
