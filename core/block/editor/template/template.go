package template

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	simpleDataview "github.com/anytypeio/go-anytype-middleware/core/block/simple/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/link"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/gogo/protobuf/types"
)

const (
	HeaderLayoutId      = "header"
	TitleBlockId        = "title"
	DescriptionBlockId  = "description"
	DataviewBlockId     = "dataview"
	FeaturedRelationsId = "featuredRelations"
)

var log = logging.Logger("anytype-state-template")

type StateTransformer func(s *state.State)

var WithEmpty = StateTransformer(func(s *state.State) {
	if s.Exists(s.RootId()) {
		return
	}

	s.Add(simple.New(&model.Block{
		Id: s.RootId(),
		Content: &model.BlockContentOfSmartblock{
			Smartblock: &model.BlockContentSmartblock{},
		},
	}))

})

var WithObjectTypes = func(otypes []string) StateTransformer {
	return func(s *state.State) {
		if len(s.ObjectTypes()) == 0 {
			s.SetObjectTypes(otypes)
		}
	}
}

var WithObjectTypeLayoutMigration = func() StateTransformer {
	return func(s *state.State) {
		layout := pbtypes.GetFloat64(s.Details(), bundle.RelationKeyLayout.String())

		if layout == float64(relation.ObjectType_objectType) {
			return
		}

		s.SetDetailAndBundledRelation(bundle.RelationKeyRecommendedLayout, pbtypes.Float64(layout))
		s.SetDetailAndBundledRelation(bundle.RelationKeyLayout, pbtypes.Float64(float64(relation.ObjectType_objectType)))
	}
}

var WithObjectTypeRecommendedRelationsMigration = func(relations []*relation.Relation) StateTransformer {
	return func(s *state.State) {
		var keys []string
		if len(pbtypes.GetStringList(s.Details(), bundle.RelationKeyRecommendedRelations.String())) > 0 {
			return
		}

		for _, rel := range relations {
			keys = append(keys, rel.Key)
			var found bool
			for _, exRel := range s.ExtraRelations() {
				if exRel.Key == rel.Key {
					found = true
					break
				}
			}
			if !found {
				s.AddRelation(rel)
			}
		}

		s.SetDetailAndBundledRelation(bundle.RelationKeyRecommendedRelations, pbtypes.StringList(keys))
	}
}

var WithObjectTypesAndLayout = func(otypes []string) StateTransformer {
	return func(s *state.State) {
		if len(s.ObjectTypes()) == 0 {
			s.SetObjectTypes(otypes)
		}

		d := s.Details()
		if d == nil || d.Fields == nil || d.Fields[bundle.RelationKeyLayout.String()] == nil {
			for _, ot := range otypes {
				t, err := bundle.GetTypeByUrl(ot)
				if err != nil {
					continue
				}
				s.SetDetailAndBundledRelation(bundle.RelationKeyLayout, pbtypes.Float64(float64(t.Layout)))
				s.SetExtraRelation(bundle.MustGetRelation(bundle.RelationKeyLayout))
			}
		}
	}
}

var WithLayout = func(layout relation.ObjectTypeLayout) StateTransformer {
	return WithDetail(bundle.RelationKeyLayout, pbtypes.Float64(float64(layout)))
}

var WithDetailName = func(name string) StateTransformer {
	return WithDetail(bundle.RelationKeyName, pbtypes.String(name))
}

var WithDetail = func(key bundle.RelationKey, value *types.Value) StateTransformer {
	return func(s *state.State) {
		if s.Details() == nil || s.Details().Fields == nil || s.Details().Fields[key.String()] == nil {
			s.SetDetail(key.String(), value)
		}

		if rel := pbtypes.GetRelation(s.ExtraRelations(), key.String()); rel == nil {
			s.SetExtraRelation(bundle.MustGetRelation(key))
		}
	}
}

var WithForcedDetail = func(key bundle.RelationKey, value *types.Value) StateTransformer {
	return func(s *state.State) {
		if s.Details() == nil || s.Details().Fields == nil || s.Details().Fields[key.String()] == nil || !s.Details().Fields[key.String()].Equal(value) {
			s.SetDetail(key.String(), value)
		}

		if rel := pbtypes.GetRelation(s.ExtraRelations(), key.String()); rel == nil {
			s.SetExtraRelation(bundle.MustGetRelation(key))
		}
	}
}

var WithDetailIconEmoji = func(iconEmoji string) StateTransformer {
	return WithDetail(bundle.RelationKeyIconEmoji, pbtypes.String(iconEmoji))

}

var WithHeader = StateTransformer(func(s *state.State) {
	WithEmpty(s)
	if s.Exists(HeaderLayoutId) {
		parent := s.PickParentOf(HeaderLayoutId)

		// case when Header is not the first block of the root
		if parent == nil || parent.Model().Id != s.RootId() || slice.FindPos(parent.Model().ChildrenIds, HeaderLayoutId) != 0 {
			s.Unlink(HeaderLayoutId)
			root := s.Get(s.RootId())
			root.Model().ChildrenIds = append([]string{HeaderLayoutId}, root.Model().ChildrenIds...)
		}
		return
	}

	s.Add(simple.New(&model.Block{
		Id: HeaderLayoutId,
		Restrictions: &model.BlockRestrictions{
			Edit:   true,
			Remove: true,
			Drag:   true,
			DropOn: true,
		},
		Content: &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_Header,
			},
		},
	}))

	// todo: rewrite when we add insert position Block_Inner_Leading
	root := s.Get(s.RootId())
	root.Model().ChildrenIds = append([]string{HeaderLayoutId}, root.Model().ChildrenIds...)
})

var WithTitle = StateTransformer(func(s *state.State) {
	WithHeader(s)

	if s.Exists(TitleBlockId) {
		return
	}

	s.Add(simple.New(&model.Block{
		Id: TitleBlockId,
		Restrictions: &model.BlockRestrictions{
			Remove: true,
			Drag:   true,
			DropOn: true,
		},
		Content: &model.BlockContentOfText{Text: &model.BlockContentText{Style: model.BlockContentText_Title}},
		Fields: &types.Struct{
			Fields: map[string]*types.Value{
				text.DetailsKeyFieldName: pbtypes.String("name"),
			},
		},
	}))

	if err := s.InsertTo(HeaderLayoutId, model.Block_Inner, TitleBlockId); err != nil {
		log.Errorf("template WithTitle failed to insert: %w", err)
	}
})

var WithDescription = StateTransformer(func(s *state.State) {
	WithHeader(s)

	if s.Exists(DescriptionBlockId) {
		return
	}

	s.Add(simple.New(&model.Block{
		Id: DescriptionBlockId,
		Restrictions: &model.BlockRestrictions{
			Remove: true,
			Drag:   true,
			DropOn: true,
		},
		Content: &model.BlockContentOfText{Text: &model.BlockContentText{Style: model.BlockContentText_Description}},
		Fields: &types.Struct{
			Fields: map[string]*types.Value{
				text.DetailsKeyFieldName: pbtypes.String("description"),
			},
		},
	}))

	if err := s.InsertTo(HeaderLayoutId, model.Block_Inner, DescriptionBlockId); err != nil {
		log.Errorf("template WithDescription failed to insert: %w", err)
	}
})

var WithFeaturedRelations = StateTransformer(func(s *state.State) {
	WithHeader(s)

	if s.Exists(FeaturedRelationsId) {
		return
	}

	s.Add(simple.New(&model.Block{
		Id: FeaturedRelationsId,
		Restrictions: &model.BlockRestrictions{
			Remove: true,
			Drag:   true,
			DropOn: true,
			Edit:   true,
		},
		Content: &model.BlockContentOfFeaturedRelations{FeaturedRelations: &model.BlockContentFeaturedRelations{}},
	}))

	if err := s.InsertTo(HeaderLayoutId, model.Block_Inner, FeaturedRelationsId); err != nil {
		log.Errorf("template FeaturedRelations failed to insert: %w", err)
	}
})

var WithAllBlocksEditsRestricted = StateTransformer(func(s *state.State) {
	s.Iterate(func(b simple.Block) (isContinue bool) {
		b.Model().Restrictions = &model.BlockRestrictions{
			Read:   false,
			Edit:   true,
			Remove: true,
			Drag:   true,
			DropOn: true,
		}
		return true
	})
})

var WithRootBlocks = func(blocks []*model.Block) StateTransformer {
	return func(s *state.State) {
		WithEmpty(s)

		for _, block := range blocks {
			if block.Id == "" {
				panic("WithRootBlocks arg must contains exact ids for blocks")
			}
			s.Add(simple.New(block))
			err := s.InsertTo(s.RootId(), model.Block_Inner, block.Id)
			if err != nil {
				log.Errorf("template WithDataview failed to insert: %w", err)
			}
		}
	}
}

var WithDataview = func(dataview model.BlockContentOfDataview, forceViews bool) StateTransformer {
	return func(s *state.State) {
		// remove old dataview
		var blockNeedToUpdate bool
		s.Iterate(func(b simple.Block) (isContinue bool) {
			if dvBlock, ok := b.(simpleDataview.Block); !ok {
				return true
			} else {
				if len(dvBlock.Model().GetDataview().Relations) == 0 ||
					dvBlock.Model().GetDataview().Source != dataview.Dataview.Source ||
					len(dvBlock.Model().GetDataview().Views) == 0 ||
					forceViews && len(dvBlock.Model().GetDataview().Relations) != len(dataview.Dataview.Relations) ||
					forceViews && !pbtypes.DataviewViewsEqualSorted(dvBlock.Model().GetDataview().Views, dataview.Dataview.Views) {

					log.With("thread", s.RootId()).With("name", pbtypes.GetString(s.Details(), "name")).Warnf("dataview needs to be migrated: %v, %v, %v, %v",
						len(dvBlock.Model().GetDataview().Relations) == 0,
						dvBlock.Model().GetDataview().Source != dataview.Dataview.Source,
						len(dvBlock.Model().GetDataview().Views) == 0,
						forceViews && len(dvBlock.Model().GetDataview().Views[0].Filters) != len(dataview.Dataview.Views[0].Filters) ||
							forceViews && len(dvBlock.Model().GetDataview().Relations) != len(dataview.Dataview.Relations))
					blockNeedToUpdate = true
					return false
				}
			}
			return true
		})

		if blockNeedToUpdate || !s.Exists(DataviewBlockId) {
			s.Set(simple.New(&model.Block{Content: &dataview, Id: DataviewBlockId}))
			if !s.IsParentOf(s.RootId(), DataviewBlockId) {
				err := s.InsertTo(s.RootId(), model.Block_Inner, DataviewBlockId)
				if err != nil {
					log.Errorf("template WithDataview failed to insert: %w", err)
				}
			}
		}

	}
}

var WithRootLink = func(targetBlockId string, style model.BlockContentLinkStyle) StateTransformer {
	return func(s *state.State) {
		var exists bool
		s.Iterate(func(b simple.Block) (isContinue bool) {
			if b, ok := b.(*link.Link); !ok {
				return true
			} else {
				if b.Model().GetLink().TargetBlockId == targetBlockId {
					exists = true
					return false
				}

				return true
			}
		})

		if exists {
			return
		}

		linkBlock := simple.New(&model.Block{
			Content: &model.BlockContentOfLink{
				Link: &model.BlockContentLink{
					TargetBlockId: targetBlockId,
					Style:         style,
				},
			},
		})

		s.Add(linkBlock)
		if err := s.InsertTo(s.RootId(), model.Block_Inner, linkBlock.Model().Id); err != nil {
			log.Errorf("can't insert link in template: %w", err)
		}

		return
	}
}

func InitTemplate(s *state.State, templates ...StateTransformer) (err error) {
	for _, template := range templates {
		template(s)
	}

	return
}

func ApplyTemplate(sb smartblock.SmartBlock, s *state.State, templates ...StateTransformer) (err error) {
	if s == nil {
		s = sb.NewState()
	}
	if err = InitTemplate(s, templates...); err != nil {
		return
	}
	return sb.Apply(s, smartblock.NoHistory, smartblock.NoEvent, smartblock.NoRestrictions, smartblock.SkipIfNoChanges)
}
