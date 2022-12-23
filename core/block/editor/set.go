package editor

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	dataview "github.com/anytypeio/go-anytype-middleware/core/block/editor/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/stext"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/restriction"
	relation2 "github.com/anytypeio/go-anytype-middleware/core/relation"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

var ErrAlreadyHasDataviewBlock = fmt.Errorf("already has the dataview block")

type Set struct {
	smartblock.SmartBlock
	basic.CommonOperations
	basic.IHistory
	dataview.Dataview
	stext.Text
}

func NewSet(
	anytype core.Service,
	objectStore objectstore.ObjectStore,
	relationService relation2.Service,
) *Set {
	sb := smartblock.New()
	return &Set{
		SmartBlock:       sb,
		CommonOperations: basic.NewBasic(sb),
		IHistory:         basic.NewHistory(sb),
		Dataview: dataview.NewDataview(
			sb,
			anytype,
			objectStore,
			relationService,
		),
		Text: stext.NewText(
			sb,
			objectStore,
		),
	}
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
	featuredRelations = slice.Remove(featuredRelations, bundle.RelationKeyCreator.String())

	templates := []template.StateTransformer{
		template.WithDataviewRelationMigrationRelation(template.DataviewBlockId, bundle.TypeKeyBookmark.URL(), bundle.RelationKeyUrl, bundle.RelationKeySource),
		template.WithObjectTypesAndLayout([]string{bundle.TypeKeySet.URL()}, model.ObjectType_set),
		template.WithRelations([]bundle.RelationKey{bundle.RelationKeySetOf}),
		template.WithDescription,
		template.WithDetail(bundle.RelationKeyFeaturedRelations, pbtypes.StringList(featuredRelations)),
		template.WithFeaturedRelations,
		template.WithBlockEditRestricted(p.Id()),
	}
	if dvBlock := p.Pick(template.DataviewBlockId); dvBlock != nil {
		setOf := dvBlock.Model().GetDataview().GetSource()
		if len(setOf) == 0 {
			log.With("thread", p.Id()).With("sbType", p.SmartBlock.Type().String()).Errorf("dataview has an empty source")
		} else {
			// add missing restrictions for dataview block in case we have set of internalType
			templates = append(templates, template.WithForcedDetail(bundle.RelationKeySetOf, pbtypes.StringList(setOf)))
			var hasInternalType bool
			for _, t := range bundle.InternalTypes {
				if setOf[0] == t.URL() {
					hasInternalType = true
					break
				}
			}

			if hasInternalType {
				rest := p.Restrictions()
				dv := rest.Dataview.Copy()
				var exists bool
				for _, r := range dv {
					if r.BlockId == template.DataviewBlockId {
						r.Restrictions = append(r.Restrictions, model.Restrictions_DVCreateObject)
						exists = true
						break
					}
				}
				if !exists {
					dv = append(dv, model.RestrictionsDataviewRestrictions{BlockId: template.DataviewBlockId, Restrictions: []model.RestrictionsDataviewRestriction{model.Restrictions_DVCreateObject}})
				}
				p.SetRestrictions(restriction.Restrictions{Object: rest.Object, Dataview: dv})
			}
		}
		// add missing done relation
		templates = append(templates, template.WithDataviewRequiredRelation(template.DataviewBlockId, bundle.RelationKeyDone))
		templates = append(templates, template.WithDataviewAddIDsToFilters(template.DataviewBlockId))
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
