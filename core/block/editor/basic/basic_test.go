package basic

import (
	"testing"

	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock/smarttest"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/restriction"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTextBlock(id, contentText string, childrenIds []string) simple.Block {
	return text.NewText(&model.Block{
		Id: id,
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text: contentText,
			},
		},
		ChildrenIds: childrenIds,
	})
}

func TestBasic_Create(t *testing.T) {
	t.Run("generic", func(t *testing.T) {
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test"}))
		b := NewBasic(sb)
		id, err := b.Create(nil, "", pb.RpcBlockCreateRequest{
			Block: &model.Block{Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "ll"}}},
		})
		require.NoError(t, err)
		require.NotEmpty(t, id)
		assert.Len(t, sb.Results.Applies, 1)
	})
	t.Run("title", func(t *testing.T) {
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test"}))
		require.NoError(t, smartblock.ApplyTemplate(sb, sb.NewState(), template.WithTitle))
		b := NewBasic(sb)
		id, err := b.Create(nil, "", pb.RpcBlockCreateRequest{
			TargetId: template.TitleBlockId,
			Position: model.Block_Top,
			Block:    &model.Block{Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "ll"}}},
		})
		require.NoError(t, err)
		require.NotEmpty(t, id)
		s := sb.NewState()
		assert.Equal(t, []string{template.HeaderLayoutId, id}, s.Pick(s.RootId()).Model().ChildrenIds)
	})
	t.Run("restricted", func(t *testing.T) {
		sb := smarttest.New("test")
		sb.TestRestrictions = restriction.Restrictions{
			Object: restriction.ObjectRestrictions{
				model.Restrictions_Blocks,
			},
		}
		sb.AddBlock(simple.New(&model.Block{Id: "test"}))
		require.NoError(t, smartblock.ApplyTemplate(sb, sb.NewState(), template.WithTitle))
		b := NewBasic(sb)
		_, err := b.Create(nil, "", pb.RpcBlockCreateRequest{})
		assert.Equal(t, restriction.ErrRestricted, err)
	})
}

func TestBasic_Duplicate(t *testing.T) {
	sb := smarttest.New("test")
	sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"2"}})).
		AddBlock(simple.New(&model.Block{Id: "2", ChildrenIds: []string{"3"}})).
		AddBlock(simple.New(&model.Block{Id: "3"}))

	b := NewBasic(sb)
	newIds, err := b.Duplicate(nil, pb.RpcBlockListDuplicateRequest{
		BlockIds: []string{"2"},
	})
	require.NoError(t, err)
	require.Len(t, newIds, 1)
	s := sb.NewState()
	assert.Len(t, s.Pick(newIds[0]).Model().ChildrenIds, 1)
	assert.Len(t, sb.Blocks(), 5)
}

func TestBasic_Unlink(t *testing.T) {
	sb := smarttest.New("test")
	sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"2"}})).
		AddBlock(simple.New(&model.Block{Id: "2", ChildrenIds: []string{"3"}})).
		AddBlock(simple.New(&model.Block{Id: "3"}))

	b := NewBasic(sb)

	err := b.Unlink(nil, "2")
	require.NoError(t, err)
	assert.Nil(t, sb.NewState().Pick("2"))

	assert.Equal(t, smartblock.ErrSimpleBlockNotFound, b.Unlink(nil, "2"))
}

func TestBasic_Move(t *testing.T) {
	t.Run("generic", func(t *testing.T) {
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"2", "4"}})).
			AddBlock(simple.New(&model.Block{Id: "2", ChildrenIds: []string{"3"}})).
			AddBlock(simple.New(&model.Block{Id: "3"})).
			AddBlock(simple.New(&model.Block{Id: "4"}))

		b := NewBasic(sb)

		err := b.Move(nil, pb.RpcBlockListMoveRequest{
			BlockIds:     []string{"3"},
			DropTargetId: "4",
			Position:     model.Block_Inner,
		})
		require.NoError(t, err)
		assert.Len(t, sb.NewState().Pick("2").Model().ChildrenIds, 0)
		assert.Len(t, sb.NewState().Pick("4").Model().ChildrenIds, 1)

	})
	t.Run("header", func(t *testing.T) {
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test"}))
		require.NoError(t, smartblock.ApplyTemplate(sb, sb.NewState(), template.WithTitle))
		b := NewBasic(sb)
		id1, err := b.Create(nil, "", pb.RpcBlockCreateRequest{
			TargetId: template.HeaderLayoutId,
			Position: model.Block_Bottom,
			Block:    &model.Block{Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "ll"}}},
		})
		require.NoError(t, err)
		require.NotEmpty(t, id1)
		id0, err := b.Create(nil, "", pb.RpcBlockCreateRequest{
			TargetId: template.HeaderLayoutId,
			Position: model.Block_Bottom,
			Block:    &model.Block{Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "ll"}}},
		})
		require.NoError(t, err)
		require.NotEmpty(t, id0)

		err = b.Move(nil, pb.RpcBlockListMoveRequest{
			BlockIds:     []string{id0},
			DropTargetId: template.TitleBlockId,
			Position:     model.Block_Top,
		})
		require.NoError(t, err)
		s := sb.NewState()
		assert.Equal(t, []string{template.HeaderLayoutId, id0, id1}, s.Pick(s.RootId()).Model().ChildrenIds)
	})
	t.Run("replace empty", func(t *testing.T) {
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2"}})).
			AddBlock(newTextBlock("1", "", nil)).
			AddBlock(newTextBlock("2", "one", nil))

		b := NewBasic(sb)

		err := b.Move(nil, pb.RpcBlockListMoveRequest{
			BlockIds:     []string{"2"},
			DropTargetId: "1",
			Position:     model.Block_InnerFirst,
		})
		require.NoError(t, err)
		assert.Len(t, sb.NewState().Pick("test").Model().ChildrenIds, 1)
	})
	t.Run("replace background and color", func(t *testing.T) {
		sb := smarttest.New("test")

		firstBlock := newTextBlock("1", "", nil)
		firstBlock.Model().BackgroundColor = "first_block_background_color"

		secondBlock := newTextBlock("2", "two", nil)
		secondBlock.Model().GetText().Color = "second_block_text_color"

		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2"}})).
			AddBlock(firstBlock).
			AddBlock(secondBlock)

		b := NewBasic(sb)

		err := b.Move(nil, pb.RpcBlockListMoveRequest{
			BlockIds:     []string{"2"},
			DropTargetId: "1",
			Position:     model.Block_InnerFirst,
		})
		require.NoError(t, err)
		assert.Equal(t, sb.NewState().Pick("2").Model().BackgroundColor, "first_block_background_color")
		assert.Equal(t, sb.NewState().Pick("2").Model().GetText().Color, "second_block_text_color")
	})
}

func TestBasic_Replace(t *testing.T) {
	sb := smarttest.New("test")
	sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"2"}})).
		AddBlock(simple.New(&model.Block{Id: "2"}))
	b := NewBasic(sb)
	newId, err := b.Replace(nil, "2", &model.Block{Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "l"}}})
	require.NoError(t, err)
	require.NotEmpty(t, newId)
}

func TestBasic_SetFields(t *testing.T) {
	sb := smarttest.New("test")
	sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"2"}})).
		AddBlock(simple.New(&model.Block{Id: "2"}))
	b := NewBasic(sb)

	fields := &types.Struct{
		Fields: map[string]*types.Value{
			"x": pbtypes.String("x"),
		},
	}
	err := b.SetFields(nil, &pb.RpcBlockListSetFieldsRequestBlockField{
		BlockId: "2",
		Fields:  fields,
	})
	require.NoError(t, err)
	assert.Equal(t, fields, sb.NewState().Pick("2").Model().Fields)
}

func TestBasic_Update(t *testing.T) {
	sb := smarttest.New("test")
	sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"2"}})).
		AddBlock(simple.New(&model.Block{Id: "2"}))
	b := NewBasic(sb)

	err := b.Update(nil, func(b simple.Block) error {
		b.Model().BackgroundColor = "test"
		return nil
	}, "2")
	require.NoError(t, err)
	assert.Equal(t, "test", sb.NewState().Pick("2").Model().BackgroundColor)
}

func TestBasic_SetDivStyle(t *testing.T) {
	sb := smarttest.New("test")
	sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"2"}})).
		AddBlock(simple.New(&model.Block{Id: "2", Content: &model.BlockContentOfDiv{Div: &model.BlockContentDiv{}}}))
	b := NewBasic(sb)

	err := b.SetDivStyle(nil, model.BlockContentDiv_Dots, "2")
	require.NoError(t, err)
	r := sb.NewState()
	assert.Equal(t, model.BlockContentDiv_Dots, r.Pick("2").Model().GetDiv().Style)
}

func TestBasic_PasteBlocks(t *testing.T) {
	sb := smarttest.New("test")
	sb.AddBlock(simple.New(&model.Block{Id: "test"}))
	b := NewBasic(sb)
	err := b.PasteBlocks([]simple.Block{
		simple.New(&model.Block{Id: "1", ChildrenIds: []string{"1.1"}}),
		simple.New(&model.Block{Id: "1.1", ChildrenIds: []string{"1.1.1"}}),
		simple.New(&model.Block{Id: "1.1.1"}),
		simple.New(&model.Block{Id: "2", ChildrenIds: []string{"2.1"}}),
		simple.New(&model.Block{Id: "2.1"}),
	})
	require.NoError(t, err)
	s := sb.NewState()
	require.Len(t, s.Blocks(), 6)
	assert.Len(t, s.Pick(s.RootId()).Model().ChildrenIds, 2)
}

func TestBasic_SetRelationKey(t *testing.T) {
	fillSb := func(sb *smarttest.SmartTest) {
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2"}})).
			AddBlock(simple.New(&model.Block{Id: "1"})).
			AddBlock(simple.New(&model.Block{Id: "2", Content: &model.BlockContentOfRelation{
				Relation: &model.BlockContentRelation{},
			}}))
		sb.AddExtraRelations(nil, []*model.Relation{
			{Key: "key"},
		})
	}
	t.Run("correct", func(t *testing.T) {
		sb := smarttest.New("test")
		fillSb(sb)
		b := NewBasic(sb)
		err := b.SetRelationKey(nil, pb.RpcBlockRelationSetKeyRequest{
			BlockId: "2",
			Key:     "key",
		})
		require.NoError(t, err)
		var setRelationEvent *pb.EventBlockSetRelation
		for _, ev := range sb.Results.Events {
			for _, em := range ev {
				if m := em.Msg.GetBlockSetRelation(); m != nil {
					setRelationEvent = m
					break
				}
			}
		}
		require.NotNil(t, setRelationEvent)
		assert.Equal(t, "key", setRelationEvent.GetKey().Value)
		assert.Equal(t, "key", sb.NewState().Pick("2").Model().GetRelation().Key)
	})
	t.Run("not relation block", func(t *testing.T) {
		sb := smarttest.New("test")
		fillSb(sb)
		b := NewBasic(sb)
		require.Error(t, b.SetRelationKey(nil, pb.RpcBlockRelationSetKeyRequest{
			BlockId: "1",
			Key:     "key",
		}))
	})
	t.Run("relation not found", func(t *testing.T) {
		sb := smarttest.New("test")
		fillSb(sb)
		b := NewBasic(sb)
		require.Error(t, b.SetRelationKey(nil, pb.RpcBlockRelationSetKeyRequest{
			BlockId: "2",
			Key:     "not exists",
		}))
	})
}

func TestBasic_FeaturedRelationAdd(t *testing.T) {
	sb := smarttest.New("test")
	s := sb.NewState()
	template.WithTitle(s)
	s.AddRelation(bundle.MustGetRelation(bundle.RelationKeyName))
	s.AddRelation(bundle.MustGetRelation(bundle.RelationKeyDescription))
	require.NoError(t, sb.Apply(s))

	b := NewBasic(sb)
	newRel := []string{bundle.RelationKeyDescription.String(), bundle.RelationKeyName.String()}
	require.NoError(t, b.FeaturedRelationAdd(nil, newRel...))

	res := sb.NewState()
	assert.Equal(t, newRel, pbtypes.GetStringList(res.Details(), bundle.RelationKeyFeaturedRelations.String()))
	assert.NotNil(t, res.Pick(template.DescriptionBlockId))
}

func TestBasic_FeaturedRelationRemove(t *testing.T) {
	sb := smarttest.New("test")
	s := sb.NewState()
	s.SetDetail(bundle.RelationKeyFeaturedRelations.String(), pbtypes.StringList([]string{bundle.RelationKeyDescription.String(), bundle.RelationKeyName.String()}))
	template.WithDescription(s)
	require.NoError(t, sb.Apply(s))

	b := NewBasic(sb)
	require.NoError(t, b.FeaturedRelationRemove(nil, bundle.RelationKeyDescription.String()))

	res := sb.NewState()
	assert.Equal(t, []string{bundle.RelationKeyName.String()}, pbtypes.GetStringList(res.Details(), bundle.RelationKeyFeaturedRelations.String()))
	assert.Nil(t, res.PickParentOf(template.DescriptionBlockId))
}

func TestBasic_ReplaceLink(t *testing.T) {
	var newId, oldId = "newId", "oldId"

	sb := smarttest.New("test")
	s := sb.NewState()
	s.SetDetail("link", pbtypes.String(oldId))
	s.AddRelation(&model.Relation{Key: "link", Format: model.RelationFormat_object})
	template.WithDescription(s)
	newBlocks := []simple.Block{
		simple.New(&model.Block{Content: &model.BlockContentOfLink{
			Link: &model.BlockContentLink{
				TargetBlockId: oldId,
			},
		}}),
		simple.New(&model.Block{Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text: "123",
				Marks: &model.BlockContentTextMarks{
					Marks: []*model.BlockContentTextMark{&model.BlockContentTextMark{Type: model.BlockContentTextMark_Mention, Param: oldId}},
				},
			},
		}}),
	}
	for _, nb := range newBlocks {
		s.Add(nb)
		require.NoError(t, s.InsertTo(s.RootId(), model.Block_Inner, nb.Model().Id))
	}
	require.NoError(t, sb.Apply(s))

	b := NewBasic(sb)
	require.NoError(t, b.ReplaceLink(oldId, newId))

	res := sb.NewState()
	assert.Equal(t, pbtypes.GetString(res.Details(), "link"), newId)
	assert.Equal(t, res.Pick(newBlocks[0].Model().Id).Model().GetLink().TargetBlockId, newId)
	assert.Equal(t, res.Pick(newBlocks[1].Model().Id).Model().GetText().GetMarks().Marks[0].Param, newId)
}
