package editor

import (
	"strings"

	"github.com/gogo/protobuf/types"
	"golang.org/x/exp/slices"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/converter"
	dataview2 "github.com/anytypeio/go-anytype-middleware/core/block/editor/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/file"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/restriction"
	"github.com/anytypeio/go-anytype-middleware/core/files"
	relation2 "github.com/anytypeio/go-anytype-middleware/core/relation"
	"github.com/anytypeio/go-anytype-middleware/core/relation/relationutils"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/space/typeprovider"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

type ObjectType struct {
	relationService relation2.Service
	*SubObject
}

func NewObjectType(
	sb smartblock.SmartBlock,
	objectStore objectstore.ObjectStore,
	fileBlockService file.BlockService,
	anytype core.Service,
	relationService relation2.Service,
	tempDirProvider core.TempDirProvider,
	sbtProvider typeprovider.SmartBlockTypeProvider,
	layoutConverter converter.LayoutConverter,
	fileService files.IService,
) *ObjectType {
	return &ObjectType{
		relationService: relationService,
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
		),
	}
}

// nolint:funlen
func (t *ObjectType) Init(ctx *smartblock.InitContext) (err error) {
	if err = t.SmartBlock.Init(ctx); err != nil {
		return
	}

	dataview := model.BlockContentOfDataview{
		Dataview: &model.BlockContentDataview{
			Source: []string{t.Id()},
			Views: []*model.BlockContentDataviewView{
				{
					Id:   "_view1_1",
					Type: model.BlockContentDataviewView_Table,
					Name: "All",
					Sorts: []*model.BlockContentDataviewSort{
						{
							RelationKey: "name",
							Type:        model.BlockContentDataviewSort_Asc,
						},
					},
					Relations: []*model.BlockContentDataviewRelation{},
					Filters:   nil,
				},
			},
		},
	}
	var templatesSource string
	var isBundled bool
	if strings.HasPrefix(t.Id(), addr.BundledObjectTypeURLPrefix) {
		isBundled = true
	}

	if isBundled {
		templatesSource = bundle.TypeKeyTemplate.BundledURL()
	} else {
		templatesSource = bundle.TypeKeyTemplate.URL()
	}

	templatesDataview := model.BlockContentOfDataview{
		Dataview: &model.BlockContentDataview{
			Source: []string{templatesSource},
			Views: []*model.BlockContentDataviewView{
				{
					Id:   "_view2_1",
					Type: model.BlockContentDataviewView_Table,
					Name: "All",
					Sorts: []*model.BlockContentDataviewSort{
						{
							RelationKey: "name",
							Type:        model.BlockContentDataviewSort_Asc,
						},
					},
					Relations: []*model.BlockContentDataviewRelation{},
					Filters: []*model.BlockContentDataviewFilter{
						{
							Operator:    model.BlockContentDataviewFilter_And,
							RelationKey: bundle.RelationKeyTargetObjectType.String(),
							Condition:   model.BlockContentDataviewFilter_Equal,
							Value:       pbtypes.String(t.RootId()),
						}},
				},
			},
		},
	}
	var recommendedRelationsKeys []string
	for _, relId := range pbtypes.GetStringList(ctx.State.Details(), bundle.RelationKeyRecommendedRelations.String()) {
		relKey, err := pbtypes.RelationIdToKey(relId)
		if err != nil {
			log.Errorf("recommendedRelations of %s has incorrect id: %s", t.Id(), relId)
			continue
		}
		if slice.FindPos(recommendedRelationsKeys, relKey) == -1 {
			recommendedRelationsKeys = append(recommendedRelationsKeys, relKey)
		}
	}

	// todo: remove this
	/*
		for _, rel := range bundle.RequiredInternalRelations {
			if slice.FindPos(recommendedRelationsKeys, rel.String()) == -1 {
				recommendedRelationsKeys = append(recommendedRelationsKeys, rel.String())
			}
		}*/

	recommendedLayout := pbtypes.GetString(t.Details(), bundle.RelationKeyRecommendedLayout.String())
	if recommendedLayout == "" {
		recommendedLayout = model.ObjectType_basic.String()
	} else if _, ok := model.ObjectTypeLayout_value[recommendedLayout]; !ok {
		recommendedLayout = model.ObjectType_basic.String()
	}

	recommendedLayoutObj := bundle.MustGetLayout(model.ObjectTypeLayout(model.ObjectTypeLayout_value[recommendedLayout]))
	for _, rel := range recommendedLayoutObj.RequiredRelations {
		if slice.FindPos(recommendedRelationsKeys, rel.Key) == -1 {
			recommendedRelationsKeys = append(recommendedRelationsKeys, rel.Key)
		}
	}

	// filter out internal relations from the recommended
	recommendedRelationsKeys = slice.Filter(recommendedRelationsKeys, func(relKey string) bool {
		for _, k := range bundle.RequiredInternalRelations {
			if k.String() == relKey {
				return false
			}
		}
		return true
	})

	var relIds []string
	var r *relationutils.Relation
	for _, rel := range recommendedRelationsKeys {
		if isBundled {
			relIds = append(relIds, addr.BundledRelationURLPrefix+rel)
		} else {
			relIds = append(relIds, addr.RelationKeyToIdPrefix+rel)
		}

		if r2, _ := bundle.GetRelation(bundle.RelationKey(rel)); r2 != nil {
			if r2.Hidden {
				continue
			}
			r = &relationutils.Relation{Relation: r2}
		} else {
			// nolint:errcheck
			r, _ = t.relationService.FetchKey(rel)
			if r == nil {
				continue
			}
		}
		// add recommended relation to the dataview
		dataview.Dataview.RelationLinks = append(dataview.Dataview.RelationLinks, r.RelationLink())
		dataview.Dataview.Views[0].Relations = append(dataview.Dataview.Views[0].Relations, &model.BlockContentDataviewRelation{
			Key:       r.Key,
			IsVisible: true,
		})
	}

	defaultValue := &types.Struct{Fields: map[string]*types.Value{bundle.RelationKeyTargetObjectType.String(): pbtypes.String(t.RootId())}}

	if !isBundled {
		var system bool
		for _, o := range bundle.SystemTypes {
			if o.URL() == t.RootId() {
				system = true
				break
			}
		}

		var internal bool
		for _, o := range bundle.InternalTypes {
			if o.URL() == t.RootId() {
				internal = true
				break
			}
		}

		if system {
			rest := t.Restrictions()
			obj := append(rest.Object.Copy(), []model.RestrictionsObjectRestriction{model.Restrictions_Details, model.Restrictions_Delete}...)
			dv := rest.Dataview.Copy()
			if internal {
				// internal mean not possible to create the object using the standard ObjectCreate flow
				dv = append(dv, model.RestrictionsDataviewRestrictions{BlockId: template.DataviewBlockId, Restrictions: []model.RestrictionsDataviewRestriction{model.Restrictions_DVCreateObject}})
			}
			t.SetRestrictions(restriction.Restrictions{Object: obj, Dataview: dv})

		}
	}

	fixMissingSmartblockTypes := func(s *state.State) {
		if isBundled {
			return
		}

		// we have a bug in internal release that was not adding smartblocktype to newly created custom types
		currTypes := pbtypes.GetIntList(s.Details(), bundle.RelationKeySmartblockTypes.String())
		sourceObject := pbtypes.GetString(s.Details(), bundle.RelationKeySourceObject.String())
		var (
			err     error
			sbTypes []int
		)
		if sourceObject != "" {
			sbTypes, err = state.ListSmartblockTypes(sourceObject)
			if err != nil {
				log.Errorf("failed to list smartblock types for %s: %v", sourceObject, err)
			}
		} else {
			sbTypes = []int{int(model.SmartBlockType_Page)}
		}

		if !slices.Equal(currTypes, sbTypes) {
			s.SetDetailAndBundledRelation(bundle.RelationKeySmartblockTypes, pbtypes.IntList(sbTypes...))
		}
	}

	var objectType string
	if isBundled {
		objectType = bundle.TypeKeyObjectType.BundledURL()
	} else {
		objectType = bundle.TypeKeyObjectType.URL()
	}
	return smartblock.ObjectApplyTemplate(t, ctx.State,
		template.WithForcedObjectTypes([]string{objectType}),
		template.WithForcedDetail(bundle.RelationKeyLayout, pbtypes.Float64(float64(model.ObjectType_objectType))),
		template.WithEmpty,
		template.WithTitle,
		template.WithDefaultFeaturedRelations,
		template.WithDescription,
		template.WithFeaturedRelations,
		template.WithDataviewID("templates", templatesDataview, true),
		template.WithDataview(dataview, true),
		template.WithForcedDetail(bundle.RelationKeyRecommendedRelations, pbtypes.StringList(relIds)),
		template.MigrateRelationValue(bundle.RelationKeySource, bundle.RelationKeySourceObject),
		template.WithChildrenSorter(t.RootId(), func(blockIds []string) {
			i := slice.FindPos(blockIds, "templates")
			j := slice.FindPos(blockIds, template.DataviewBlockId)
			// templates dataview must come before the type dataview
			if i > j {
				blockIds[i], blockIds[j] = blockIds[j], blockIds[i]
			}
		}),
		template.WithCondition(!isBundled, template.WithAddedFeaturedRelation(bundle.RelationKeySourceObject)),
		template.WithObjectTypeLayoutMigration(),
		template.WithRequiredRelations(),
		template.WithBlockField("templates", dataview2.DefaultDetailsFieldName, pbtypes.Struct(defaultValue)),
		fixMissingSmartblockTypes,
	)
}

func (t *ObjectType) SetStruct(st *types.Struct) error {
	t.Lock()
	defer t.Unlock()
	s := t.NewState()
	s.SetDetails(st)
	return t.Apply(s)
}
