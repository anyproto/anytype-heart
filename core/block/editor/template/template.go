package template

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	simpleDataview "github.com/anytypeio/go-anytype-middleware/core/block/simple/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/link"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/gogo/protobuf/types"
	"github.com/google/martian/log"
)

const (
	HeaderLayoutId  = "header"
	TitleBlockId    = "title"
	DataviewBlockId = "dataview"
)

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

		s.SetDetail(bundle.RelationKeyRecommendedLayout.String(), pbtypes.Float64(layout))
		s.SetDetail(bundle.RelationKeyLayout.String(), pbtypes.Float64(float64(relation.ObjectType_objectType)))
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
				s.SetDetail(bundle.RelationKeyLayout.String(), pbtypes.Float64(float64(t.Layout)))
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
		s.Iterate(func(b simple.Block) (isContinue bool) {
			if dvBlock, ok := b.(simpleDataview.Block); !ok {
				return true
			} else {
				if dvBlock.Model().GetDataview().Source == "pages" ||
					len(dvBlock.Model().GetDataview().Relations) == 0 ||
					dvBlock.Model().GetDataview().Source != dataview.Dataview.Source ||
					len(dvBlock.Model().GetDataview().Views) == 0 ||
					forceViews && len(dvBlock.Model().GetDataview().Views[0].Filters) != len(dataview.Dataview.Views[0].Filters) ||
					forceViews && len(dvBlock.Model().GetDataview().Relations) != len(dataview.Dataview.Relations) {
					// remove old pages set
					s.Unlink(b.Model().Id)
					return false
				}
			}
			return true
		})

		// todo: move to the begin of func
		if s.Exists(DataviewBlockId) {
			return
		}

		s.Add(simple.New(&model.Block{Content: &dataview, Id: DataviewBlockId}))
		err := s.InsertTo(s.RootId(), model.Block_Inner, DataviewBlockId)
		if err != nil {
			log.Errorf("template WithDataview failed to insert: %w", err)
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
