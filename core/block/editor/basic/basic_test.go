package basic

import (
	"errors"
	"math/rand"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/table"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/core/files/fileobject/mock_fileobject"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"

	_ "github.com/anyproto/anytype-heart/core/block/simple/base"
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
		b := NewBasic(sb, nil, converter.NewLayoutConverter(), nil)
		st := sb.NewState()
		id, err := b.CreateBlock(st, pb.RpcBlockCreateRequest{
			Block: &model.Block{Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "ll"}}},
		})
		require.NoError(t, sb.Apply(st))
		require.NoError(t, err)
		require.NotEmpty(t, id)
		assert.Len(t, sb.Results.Applies, 1)
	})
	t.Run("title", func(t *testing.T) {
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test"}))
		require.NoError(t, smartblock.ObjectApplyTemplate(sb, sb.NewState(), template.WithTitle))
		b := NewBasic(sb, nil, converter.NewLayoutConverter(), nil)
		s := sb.NewState()
		id, err := b.CreateBlock(s, pb.RpcBlockCreateRequest{
			TargetId: template.TitleBlockId,
			Position: model.Block_Top,
			Block:    &model.Block{Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "ll"}}},
		})
		require.NoError(t, err)
		require.NotEmpty(t, id)

		assert.Equal(t, []string{template.HeaderLayoutId, id}, s.Pick(s.RootId()).Model().ChildrenIds)
	})
	t.Run("restricted", func(t *testing.T) {
		sb := smarttest.New("test")
		sb.TestRestrictions = restriction.Restrictions{
			Object: restriction.ObjectRestrictions{
				model.Restrictions_Blocks: {},
			},
		}
		sb.AddBlock(simple.New(&model.Block{Id: "test"}))
		require.NoError(t, smartblock.ObjectApplyTemplate(sb, sb.NewState(), template.WithTitle))
		b := NewBasic(sb, nil, converter.NewLayoutConverter(), nil)
		_, err := b.CreateBlock(sb.NewState(), pb.RpcBlockCreateRequest{})
		assert.ErrorIs(t, err, restriction.ErrRestricted)
	})
}

func TestBasic_Duplicate(t *testing.T) {
	t.Run("dup blocks to same state", func(t *testing.T) {
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"2"}})).
			AddBlock(simple.New(&model.Block{Id: "2", ChildrenIds: []string{"3"}})).
			AddBlock(simple.New(&model.Block{Id: "3"}))

		st := sb.NewState()
		newIds, err := NewBasic(sb, nil, converter.NewLayoutConverter(), nil).Duplicate(st, st, "", 0, []string{"2"})
		require.NoError(t, err)

		err = sb.Apply(st)
		require.NoError(t, err)

		require.Len(t, newIds, 1)
		s := sb.NewState()
		assert.Len(t, s.Pick(newIds[0]).Model().ChildrenIds, 1)
		assert.Len(t, sb.Blocks(), 5)
	})

	for _, tc := range []struct {
		name     string
		fos      func() fileobject.Service
		spaceIds []string
		targets  []string
	}{
		{
			name: "dup file block - same space",
			fos: func() fileobject.Service {
				return nil
			},
			spaceIds: []string{"space1", "space1"},
			targets:  []string{"file1_space1", "file2_space1"},
		},
		{
			name: "dup file block - other space",
			fos: func() fileobject.Service {
				fos := mock_fileobject.NewMockService(t)
				fos.EXPECT().GetFileIdFromObject("file1_space1").Return(domain.FullFileId{SpaceId: "space1", FileId: "file1"}, nil)
				fos.EXPECT().CreateFromImport(domain.FullFileId{SpaceId: "space2", FileId: "file1"}, mock.Anything).Return("file1_space2", nil)
				fos.EXPECT().GetFileIdFromObject("file2_space1").Return(domain.FullFileId{SpaceId: "space1", FileId: "file2"}, nil)
				fos.EXPECT().CreateFromImport(domain.FullFileId{SpaceId: "space2", FileId: "file2"}, mock.Anything).Return("file2_space2", nil)
				return fos
			},
			spaceIds: []string{"space1", "space2"},
			targets:  []string{"file1_space2", "file2_space2"},
		},
		{
			name: "dup file block - no target change if failed to retrieve file id",
			fos: func() fileobject.Service {
				fos := mock_fileobject.NewMockService(t)
				fos.EXPECT().GetFileIdFromObject(mock.Anything).Return(domain.FullFileId{}, errors.New("no such file")).Times(2)
				return fos
			},
			spaceIds: []string{"space1", "space2"},
			targets:  []string{"file1_space1", "file2_space1"},
		},
		{
			name: "dup file block - no target change if failed to create file object",
			fos: func() fileobject.Service {
				fos := mock_fileobject.NewMockService(t)
				fos.EXPECT().GetFileIdFromObject("file1_space1").Return(domain.FullFileId{SpaceId: "space1", FileId: "file1"}, nil)
				fos.EXPECT().GetFileIdFromObject("file2_space1").Return(domain.FullFileId{SpaceId: "space1", FileId: "file2"}, nil)
				fos.EXPECT().CreateFromImport(mock.Anything, mock.Anything).Return("", errors.New("creation failure"))
				return fos
			},
			spaceIds: []string{"space1", "space2"},
			targets:  []string{"file1_space1", "file2_space1"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			source := smarttest.New("source").
				AddBlock(simple.New(&model.Block{Id: "source", ChildrenIds: []string{"1", "f1"}})).
				AddBlock(simple.New(&model.Block{Id: "1", ChildrenIds: []string{"f2"}})).
				AddBlock(simple.New(&model.Block{Id: "f1", Content: &model.BlockContentOfFile{File: &model.BlockContentFile{TargetObjectId: "file1_space1"}}})).
				AddBlock(simple.New(&model.Block{Id: "f2", Content: &model.BlockContentOfFile{File: &model.BlockContentFile{TargetObjectId: "file2_space1"}}}))
			ss := source.NewState()
			ss.SetDetail(bundle.RelationKeySpaceId, domain.String(tc.spaceIds[0]))

			target := smarttest.New("target").
				AddBlock(simple.New(&model.Block{Id: "target"}))
			ts := target.NewState()
			ts.SetDetail(bundle.RelationKeySpaceId, domain.String(tc.spaceIds[1]))

			// when
			newIds, err := NewBasic(source, nil, nil, tc.fos()).Duplicate(ss, ts, "target", model.Block_Inner, []string{"1", "f1"})
			require.NoError(t, err)
			require.NoError(t, target.Apply(ts))

			// then
			assert.Len(t, newIds, 2)

			ts = target.NewState()
			root := ts.Pick("target")
			assert.Equal(t, newIds, root.Model().ChildrenIds)
			block1 := ts.Pick(newIds[0])
			require.NotNil(t, block1)
			blockChildren := block1.Model().ChildrenIds
			assert.Len(t, blockChildren, 1)

			for fbID, targetID := range map[string]string{newIds[1]: tc.targets[0], blockChildren[0]: tc.targets[1]} {
				fb := ts.Pick(fbID)
				assert.NotNil(t, fb)
				f, ok := fb.Model().Content.(*model.BlockContentOfFile)
				assert.True(t, ok)
				assert.Equal(t, targetID, f.File.TargetObjectId)
			}
		})
	}

}

func TestBasic_Unlink(t *testing.T) {
	t.Run("base case", func(t *testing.T) {
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"2"}})).
			AddBlock(simple.New(&model.Block{Id: "2", ChildrenIds: []string{"3"}})).
			AddBlock(simple.New(&model.Block{Id: "3"}))

		b := NewBasic(sb, nil, converter.NewLayoutConverter(), nil)

		err := b.Unlink(nil, "2")
		require.NoError(t, err)
		assert.Nil(t, sb.NewState().Pick("2"))

		assert.Error(t, b.Unlink(nil, "2"))
	})
	t.Run("unlink parent and its child", func(t *testing.T) {
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"2"}})).
			AddBlock(simple.New(&model.Block{Id: "2", ChildrenIds: []string{"3"}})).
			AddBlock(simple.New(&model.Block{Id: "3"}))

		b := NewBasic(sb, nil, converter.NewLayoutConverter(), nil)

		err := b.Unlink(nil, "2", "3")
		require.NoError(t, err)
		assert.False(t, sb.NewState().Exists("2"))
		assert.False(t, sb.NewState().Exists("3"))
	})
}

func TestBasic_Move(t *testing.T) {
	t.Run("generic", func(t *testing.T) {
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"2", "4"}})).
			AddBlock(simple.New(&model.Block{Id: "2", ChildrenIds: []string{"3"}})).
			AddBlock(simple.New(&model.Block{Id: "3"})).
			AddBlock(simple.New(&model.Block{Id: "4"}))

		b := NewBasic(sb, nil, converter.NewLayoutConverter(), nil)
		st := sb.NewState()

		err := b.Move(st, st, "4", model.Block_Inner, []string{"3"})
		require.NoError(t, err)
		require.NoError(t, sb.Apply(st))
		assert.Len(t, sb.NewState().Pick("2").Model().ChildrenIds, 0)
		assert.Len(t, sb.NewState().Pick("4").Model().ChildrenIds, 1)

	})
	t.Run("header", func(t *testing.T) {
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test"}))
		require.NoError(t, smartblock.ObjectApplyTemplate(sb, sb.NewState(), template.WithTitle))
		b := NewBasic(sb, nil, converter.NewLayoutConverter(), nil)
		s := sb.NewState()
		id1, err := b.CreateBlock(s, pb.RpcBlockCreateRequest{
			TargetId: template.HeaderLayoutId,
			Position: model.Block_Bottom,
			Block:    &model.Block{Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "ll"}}},
		})
		require.NoError(t, err)
		require.NotEmpty(t, id1)

		id0, err := b.CreateBlock(s, pb.RpcBlockCreateRequest{
			TargetId: template.HeaderLayoutId,
			Position: model.Block_Bottom,
			Block:    &model.Block{Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "ll"}}},
		})
		require.NoError(t, err)
		require.NotEmpty(t, id0)

		st := sb.NewState()
		err = b.Move(st, st, template.TitleBlockId, model.Block_Top, []string{id0})
		require.NoError(t, err)
		require.NoError(t, sb.Apply(st))
		assert.Equal(t, []string{template.HeaderLayoutId, id0, id1}, s.Pick(s.RootId()).Model().ChildrenIds)
	})
	for _, relation := range []string{
		template.TitleBlockId,
		template.HeaderLayoutId,
		template.DescriptionBlockId,
		template.FeaturedRelationsId,
	} {
		t.Run("do not move block - when a required relation is ("+relation+")", func(t *testing.T) {
			// given
			testDoc := smarttest.New("test")
			testDoc.
				AddBlock(
					simple.New(
						&model.Block{
							Id:          "root",
							ChildrenIds: []string{relation},
						},
					),
				).
				AddBlock(
					simple.New(
						&model.Block{
							Id: "target",
						},
					),
				)
			basic := NewBasic(testDoc, nil, converter.NewLayoutConverter(), nil)
			state := testDoc.NewState()

			// when
			err := basic.Move(state, state, "target", model.Block_Bottom, []string{relation})

			// then
			require.Error(t, err)
		})
	}
	t.Run("replace empty", func(t *testing.T) {
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2"}})).
			AddBlock(newTextBlock("1", "", nil)).
			AddBlock(newTextBlock("2", "one", nil))

		b := NewBasic(sb, nil, converter.NewLayoutConverter(), nil)
		st := sb.NewState()
		err := b.Move(st, st, "1", model.Block_InnerFirst, []string{"2"})
		require.NoError(t, err)
		require.NoError(t, sb.Apply(st))
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

		b := NewBasic(sb, nil, converter.NewLayoutConverter(), nil)
		st := sb.NewState()
		err := b.Move(st, st, "1", model.Block_InnerFirst, []string{"2"})
		require.NoError(t, err)
		require.NoError(t, sb.Apply(st))
		assert.Equal(t, sb.NewState().Pick("2").Model().BackgroundColor, "first_block_background_color")
		assert.Equal(t, sb.NewState().Pick("2").Model().GetText().Color, "second_block_text_color")
	})
	t.Run("do not replace empty on top insert", func(t *testing.T) {
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2"}})).
			AddBlock(newTextBlock("1", "", nil)).
			AddBlock(newTextBlock("2", "one", nil))

		b := NewBasic(sb, nil, converter.NewLayoutConverter(), nil)
		st := sb.NewState()
		err := b.Move(st, nil, "1", model.Block_Top, []string{"2"})
		require.NoError(t, err)
		require.NoError(t, sb.Apply(st))
		assert.Len(t, sb.NewState().Pick("test").Model().ChildrenIds, 2)
	})
}

func TestBasic_MoveTableBlocks(t *testing.T) {
	getSB := func() *smarttest.SmartTest {
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"upper", "table", "block"}})).
			AddBlock(simple.New(&model.Block{Id: "table", ChildrenIds: []string{"columns", "rows"}, Content: &model.BlockContentOfTable{Table: &model.BlockContentTable{}}})).
			AddBlock(simple.New(&model.Block{Id: "columns", ChildrenIds: []string{"column"}, Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_TableColumns}}})).
			AddBlock(simple.New(&model.Block{Id: "column", ChildrenIds: []string{}, Content: &model.BlockContentOfTableColumn{TableColumn: &model.BlockContentTableColumn{}}})).
			AddBlock(simple.New(&model.Block{Id: "rows", ChildrenIds: []string{"row", "row2"}, Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_TableRows}}})).
			AddBlock(simple.New(&model.Block{Id: "row", ChildrenIds: []string{"column-row"}, Content: &model.BlockContentOfTableRow{TableRow: &model.BlockContentTableRow{IsHeader: false}}})).
			AddBlock(simple.New(&model.Block{Id: "row2", ChildrenIds: []string{}, Content: &model.BlockContentOfTableRow{TableRow: &model.BlockContentTableRow{IsHeader: false}}})).
			AddBlock(simple.New(&model.Block{Id: "column-row", ChildrenIds: []string{}})).
			AddBlock(simple.New(&model.Block{Id: "block", ChildrenIds: []string{}})).
			AddBlock(simple.New(&model.Block{Id: "upper", ChildrenIds: []string{}}))
		return sb
	}

	for _, block := range []string{"columns", "rows", "column", "row", "column-row"} {
		t.Run("moving non-root table block '"+block+"' leads to error", func(t *testing.T) {
			// given
			sb := getSB()
			b := NewBasic(sb, nil, converter.NewLayoutConverter(), nil)
			st := sb.NewState()

			// when
			err := b.Move(st, st, "block", model.Block_Bottom, []string{block})

			// then
			assert.Error(t, err)
			assert.True(t, errors.Is(err, table.ErrCannotMoveTableBlocks))
		})
	}

	t.Run("no error on moving root table block", func(t *testing.T) {
		// given
		sb := getSB()
		b := NewBasic(sb, nil, converter.NewLayoutConverter(), nil)
		st := sb.NewState()

		// when
		err := b.Move(st, st, "block", model.Block_Bottom, []string{"table"})

		// then
		assert.NoError(t, err)
		assert.Equal(t, []string{"upper", "block", "table"}, st.Pick("test").Model().ChildrenIds)
	})

	t.Run("no error on moving one row between another", func(t *testing.T) {
		// given
		sb := getSB()
		b := NewBasic(sb, nil, converter.NewLayoutConverter(), nil)
		st := sb.NewState()

		// when
		err := b.Move(st, st, "row2", model.Block_Bottom, []string{"row"})

		// then
		assert.NoError(t, err)
		assert.Equal(t, []string{"row2", "row"}, st.Pick("rows").Model().ChildrenIds)
	})

	t.Run("moving rows with incorrect position leads to error", func(t *testing.T) {
		// given
		sb := getSB()
		b := NewBasic(sb, nil, converter.NewLayoutConverter(), nil)
		st := sb.NewState()

		// when
		err := b.Move(st, st, "row2", model.Block_Left, []string{"row"})

		// then
		assert.Error(t, err)
	})

	t.Run("moving rows and some other blocks between another leads to error", func(t *testing.T) {
		// given
		sb := getSB()
		b := NewBasic(sb, nil, converter.NewLayoutConverter(), nil)
		st := sb.NewState()

		// when
		err := b.Move(st, st, "row2", model.Block_Top, []string{"row", "rows"})

		// then
		assert.Error(t, err)
	})

	t.Run("moving the row between itself leads to error", func(t *testing.T) {
		// given
		sb := getSB()
		b := NewBasic(sb, nil, converter.NewLayoutConverter(), nil)
		st := sb.NewState()

		// when
		err := b.Move(st, st, "row2", model.Block_Bottom, []string{"row2"})

		// then
		assert.Error(t, err)
	})

	t.Run("moving table block from invalid table leads to error", func(t *testing.T) {
		// given
		sb := getSB()
		b := NewBasic(sb, nil, converter.NewLayoutConverter(), nil)
		st := sb.NewState()
		st.Unlink("columns")

		// when
		err := b.Move(st, st, "block", model.Block_Bottom, []string{"column-row"})

		// then
		assert.Error(t, err)
		assert.True(t, errors.Is(err, table.ErrCannotMoveTableBlocks))
	})

	for _, block := range []string{"columns", "rows", "column", "row", "column-row"} {
		t.Run("moving a block to '"+block+"' block leads to moving it under the table", func(t *testing.T) {
			// given
			sb := getSB()
			b := NewBasic(sb, nil, converter.NewLayoutConverter(), nil)
			st := sb.NewState()

			// when
			err := b.Move(st, st, block, model.BlockPosition(rand.Intn(len(model.BlockPosition_name))), []string{"upper"})

			// then
			assert.NoError(t, err)
			assert.Equal(t, []string{"table", "upper", "block"}, st.Pick("test").Model().ChildrenIds)
		})
	}

	t.Run("moving a block to the invalid table leads to moving it under the table", func(t *testing.T) {
		// given
		sb := getSB()
		b := NewBasic(sb, nil, converter.NewLayoutConverter(), nil)
		st := sb.NewState()
		st.Unlink("columns")

		// when
		err := b.Move(st, st, "rows", model.BlockPosition(rand.Intn(6)), []string{"upper"})

		// then
		assert.NoError(t, err)
		assert.Equal(t, []string{"table", "upper", "block"}, st.Pick("test").Model().ChildrenIds)
	})
}

func TestBasic_MoveToAnotherObject(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		sb1 := smarttest.New("test1")
		sb1.AddBlock(simple.New(&model.Block{Id: "test1", ChildrenIds: []string{"2", "4"}})).
			AddBlock(newTextBlock("2", "t2", []string{"3"})).
			AddBlock(newTextBlock("3", "t3", nil)).
			AddBlock(newTextBlock("4", "", nil))

		sb2 := smarttest.New("test2")
		sb2.AddBlock(simple.New(&model.Block{Id: "test2", ChildrenIds: []string{}}))

		b := NewBasic(sb1, nil, converter.NewLayoutConverter(), nil)

		srcState := sb1.NewState()
		destState := sb2.NewState()

		srcId := "2"
		wantBlocks := append([]simple.Block{srcState.Pick(srcId)}, srcState.Descendants(srcId)...)
		err := b.Move(srcState, destState, "test2", model.Block_Inner, []string{srcId})
		require.NoError(t, err)

		require.NoError(t, sb1.Apply(srcState))
		require.NoError(t, sb2.Apply(destState))

		// Block is removed from source object
		assert.Equal(t, []string{"4"}, sb1.NewState().Pick("test1").Model().ChildrenIds)
		assert.Nil(t, sb1.NewState().Pick(srcId))

		// Block is added to dest object
		gotState := sb2.NewState()
		gotId := gotState.Pick(gotState.RootId()).Model().ChildrenIds[0]
		gotBlocks := append([]simple.Block{gotState.Pick(gotId)}, gotState.Descendants(gotId)...)

		for i := range wantBlocks {
			wb, gb := wantBlocks[i].Model(), gotBlocks[i].Model()
			// ids are reassigned
			assert.NotEqual(t, wb.Id, gb.Id)
			assert.Equal(t, wb.Content, gb.Content)
		}
	})
}

func TestBasic_Replace(t *testing.T) {
	sb := smarttest.New("test")
	sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"2"}})).
		AddBlock(simple.New(&model.Block{Id: "2"}))
	b := NewBasic(sb, nil, converter.NewLayoutConverter(), nil)
	newId, err := b.Replace(nil, "2", &model.Block{Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "l"}}})
	require.NoError(t, err)
	require.NotEmpty(t, newId)
}

func TestBasic_SetFields(t *testing.T) {
	sb := smarttest.New("test")
	sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"2"}})).
		AddBlock(simple.New(&model.Block{Id: "2"}))
	b := NewBasic(sb, nil, converter.NewLayoutConverter(), nil)

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
	b := NewBasic(sb, nil, converter.NewLayoutConverter(), nil)

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
	b := NewBasic(sb, nil, converter.NewLayoutConverter(), nil)

	err := b.SetDivStyle(nil, model.BlockContentDiv_Dots, "2")
	require.NoError(t, err)
	r := sb.NewState()
	assert.Equal(t, model.BlockContentDiv_Dots, r.Pick("2").Model().GetDiv().Style)
}

func TestBasic_SetRelationKey(t *testing.T) {
	fillSb := func(sb *smarttest.SmartTest) {
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2"}})).
			AddBlock(simple.New(&model.Block{Id: "1"})).
			AddBlock(simple.New(&model.Block{Id: "2", Content: &model.BlockContentOfRelation{
				Relation: &model.BlockContentRelation{},
			}}))
		sb.AddRelationLinks(nil, "testRelKey")
	}
	t.Run("correct", func(t *testing.T) {
		sb := smarttest.New("test")
		fillSb(sb)
		b := NewBasic(sb, nil, converter.NewLayoutConverter(), nil)
		err := b.SetRelationKey(nil, pb.RpcBlockRelationSetKeyRequest{
			BlockId: "2",
			Key:     "testRelKey",
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
		assert.Equal(t, "testRelKey", setRelationEvent.GetKey().Value)
		assert.Equal(t, "testRelKey", sb.NewState().Pick("2").Model().GetRelation().Key)
	})
	t.Run("not relation block", func(t *testing.T) {
		sb := smarttest.New("test")
		fillSb(sb)
		b := NewBasic(sb, nil, converter.NewLayoutConverter(), nil)
		require.Error(t, b.SetRelationKey(nil, pb.RpcBlockRelationSetKeyRequest{
			BlockId: "1",
			Key:     "key",
		}))
	})
	t.Run("relation not found", func(t *testing.T) {
		sb := smarttest.New("test")
		fillSb(sb)
		b := NewBasic(sb, nil, converter.NewLayoutConverter(), nil)
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
	s.AddBundledRelationLinks(bundle.RelationKeyName)
	s.AddBundledRelationLinks(bundle.RelationKeyDescription)
	require.NoError(t, sb.Apply(s))

	b := NewBasic(sb, nil, converter.NewLayoutConverter(), nil)
	newRel := []string{bundle.RelationKeyDescription.String(), bundle.RelationKeyName.String()}
	require.NoError(t, b.FeaturedRelationAdd(nil, newRel...))

	res := sb.NewState()
	assert.Equal(t, newRel, res.Details().GetStringList(bundle.RelationKeyFeaturedRelations))
	assert.NotNil(t, res.Pick(template.DescriptionBlockId))
}

func TestBasic_FeaturedRelationRemove(t *testing.T) {
	sb := smarttest.New("test")
	s := sb.NewState()
	s.SetDetail(bundle.RelationKeyFeaturedRelations, domain.StringList([]string{bundle.RelationKeyDescription.String(), bundle.RelationKeyName.String()}))
	template.WithDescription(s)
	require.NoError(t, sb.Apply(s))

	b := NewBasic(sb, nil, converter.NewLayoutConverter(), nil)
	require.NoError(t, b.FeaturedRelationRemove(nil, bundle.RelationKeyDescription.String()))

	res := sb.NewState()
	assert.Equal(t, []string{bundle.RelationKeyName.String()}, res.Details().GetStringList(bundle.RelationKeyFeaturedRelations))
	assert.Nil(t, res.PickParentOf(template.DescriptionBlockId))
}

func TestBasic_ReplaceLink(t *testing.T) {
	var newId, oldId = "newId", "oldId"

	sb := smarttest.New("test")
	s := sb.NewState()
	s.SetDetail("link", domain.String(oldId))
	s.AddRelationLinks(&model.RelationLink{Key: "link", Format: model.RelationFormat_object})
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

	b := NewBasic(sb, nil, converter.NewLayoutConverter(), nil)
	require.NoError(t, b.ReplaceLink(oldId, newId))

	res := sb.NewState()
	assert.Equal(t, newId, res.Details().GetString("link"))
	assert.Equal(t, newId, res.Pick(newBlocks[0].Model().Id).Model().GetLink().TargetBlockId)
	assert.Equal(t, newId, res.Pick(newBlocks[1].Model().Id).Model().GetText().GetMarks().Marks[0].Param)
}
