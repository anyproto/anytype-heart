package editor

import (
	"strings"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	dataview2 "github.com/anyproto/anytype-heart/core/block/editor/dataview"
	"github.com/anyproto/anytype-heart/core/block/editor/file"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/getblock"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/relation"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/typeprovider"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

type ObjectType struct {
	relationService relation.Service
	*SubObject
}

func NewObjectType(
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
	eventSender event.Sender,
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
			picker,
			eventSender,
		),
	}
}

// nolint:funlen
func (t *ObjectType) Init(ctx *smartblock.InitContext) (err error) {
	if err = t.SmartBlock.Init(ctx); err != nil {
		return
	}

	return nil
}

func (ot *ObjectType) InitState(st *state.State) {
	id := st.RootId()

	dataview := model.BlockContentOfDataview{
		Dataview: &model.BlockContentDataview{
			Source: []string{id},
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
	if strings.HasPrefix(id, addr.BundledObjectTypeURLPrefix) {
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
							Value:       pbtypes.String(id),
						}},
				},
			},
		},
	}
	var recommendedRelationsKeys []string
	for _, relId := range pbtypes.GetStringList(st.Details(), bundle.RelationKeyRecommendedRelations.String()) {
		relKey, err := pbtypes.RelationIdToKey(relId)
		if err != nil {
			log.Errorf("recommendedRelations of %s has incorrect id: %s", id, relId)
			continue
		}
		if slice.FindPos(recommendedRelationsKeys, relKey) == -1 {
			recommendedRelationsKeys = append(recommendedRelationsKeys, relKey)
		}
	}

	recommendedLayout := pbtypes.GetInt64(st.Details(), bundle.RelationKeyRecommendedLayout.String())
	recommendedLayoutObj := bundle.MustGetLayout(model.ObjectTypeLayout(recommendedLayout))
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

	var recommendedRelationIDs []string
	rels, err := ot.relationService.FetchKeys(recommendedRelationsKeys...)
	if err != nil {
		log.Errorf("failed to fetch relation keys: %s", err.Error())
	}
	for _, relKey := range recommendedRelationsKeys {
		r := rels.GetByKey(relKey)
		if r == nil {
			log.Debugf("ot relation missing relation: %s", relKey)
			continue
		}
		recommendedRelationIDs = append(recommendedRelationIDs, r.Id)

		// add recommended relation to the dataview
		dataview.Dataview.RelationLinks = append(dataview.Dataview.RelationLinks, r.RelationLink())
		if r.Hidden {
			continue
		}
		dataview.Dataview.Views[0].Relations = append(dataview.Dataview.Views[0].Relations, &model.BlockContentDataviewRelation{
			Key:       r.Key,
			IsVisible: true,
		})
	}

	defaultValue := &types.Struct{Fields: map[string]*types.Value{bundle.RelationKeyTargetObjectType.String(): pbtypes.String(id)}}

	var objectType string
	if isBundled {
		objectType = bundle.TypeKeyObjectType.BundledURL()
	} else {
		objectType = bundle.TypeKeyObjectType.URL()
	}
	template.InitTemplate(st,
		template.WithForcedObjectTypes([]string{objectType}),
		template.WithDetail(bundle.RelationKeyRecommendedLayout, pbtypes.Int64(recommendedLayout)),
		template.WithForcedDetail(bundle.RelationKeyLayout, pbtypes.Float64(float64(model.ObjectType_objectType))),
		template.WithEmpty,
		template.WithTitle,
		template.WithDefaultFeaturedRelations,
		template.WithDescription,
		template.WithFeaturedRelations,
		template.WithDataviewID("templates", templatesDataview, true),
		template.WithDataview(dataview, true),
		template.WithForcedDetail(bundle.RelationKeyRecommendedRelations, pbtypes.StringList(recommendedRelationIDs)),
		template.WithCondition(!isBundled, template.WithAddedFeaturedRelation(bundle.RelationKeySourceObject)),
		template.WithObjectTypeLayoutMigration(),
		template.WithRequiredRelations(),
		template.WithBlockField("templates", dataview2.DefaultDetailsFieldName, pbtypes.Struct(defaultValue)),
	)
}

func (t *ObjectType) SetStruct(st *types.Struct) error {
	t.Lock()
	defer t.Unlock()
	s := t.NewState()
	s.SetDetails(st)
	return t.Apply(s)
}
