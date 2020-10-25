package template

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	simpleDataview "github.com/anytypeio/go-anytype-middleware/core/block/simple/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/link"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
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
		if len(s.ObjectTypes()) > 0 {
			return
		}
		s.SetObjectTypes(otypes)
	}
}

var WithDetailName = func(name string) StateTransformer {
	return func(s *state.State) {
		if s.Details() != nil && s.Details().Fields != nil && s.Details().Fields["name"] != nil {
			return
		}

		s.SetDetail("name", pbtypes.String(name))
	}
}

var WithDetailIconEmoji = func(iconEmoji string) StateTransformer {
	return func(s *state.State) {
		if s.Details() != nil && s.Details().Fields != nil && s.Details().Fields["iconEmoji"] != nil {
			return
		}

		s.SetDetail("iconEmoji", pbtypes.String(iconEmoji))
	}
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

var WithDataview = func(dataview model.BlockContentOfDataview) StateTransformer {
	return func(s *state.State) {
		// remove old dataview
		s.Iterate(func(b simple.Block) (isContinue bool) {
			if dvBlock, ok := b.(simpleDataview.Block); !ok {
				return true
			} else {
				if dvBlock.Model().GetDataview().Source == "pages" {
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
	return sb.Apply(s, smartblock.NoHistory, smartblock.NoEvent, smartblock.NoRestrictions)
}
