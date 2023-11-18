package anymark

import (
	"reflect"
	"testing"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestConvertBlocks(t *testing.T) {
	source := []byte("## Hello world!\n Olol*ol*olo \n\n 123123")

	blocks, _, err := MarkdownToBlocks(source, "", nil)
	if err != nil {
		t.Error(err.Error())
	}

	assert.NotEmpty(t, blocks)
	assert.NoError(t, err)
}

func TestPreprocessBlocksEmpty(t *testing.T) {
	blocks := preprocessBlocks([]*model.Block{})
	assert.Empty(t, blocks)
}

func TestPreprocessBlocksOneCodeBlock(t *testing.T) {
	bl := &model.Block{
		Id: bson.NewObjectId().Hex(),
		Fields: &types.Struct{Fields: map[string]*types.Value{
			"lang": pbtypes.String("Java"),
		}},
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  "code",
				Style: model.BlockContentText_Code,
			},
		},
	}
	blocks := preprocessBlocks([]*model.Block{bl})
	assert.Len(t, blocks, 1)
	assert.Equal(t, blocks[0].Id, bl.Id)
}

func TestPreprocessBlocksTwoDifferentCodeBlocks(t *testing.T) {
	bl := &model.Block{
		Id: bson.NewObjectId().Hex(),
		Fields: &types.Struct{Fields: map[string]*types.Value{
			"lang": pbtypes.String("java"),
		}},
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  "code",
				Style: model.BlockContentText_Code,
			},
		},
	}
	bl2 := &model.Block{
		Id: bson.NewObjectId().Hex(),
		Fields: &types.Struct{Fields: map[string]*types.Value{
			"lang": pbtypes.String("go"),
		}},
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  "code",
				Style: model.BlockContentText_Code,
			},
		},
	}
	blocks := preprocessBlocks([]*model.Block{bl, bl2})
	assert.Len(t, blocks, 2)
	assert.Equal(t, blocks[0].Id, bl.Id)
	assert.Equal(t, blocks[1].Id, bl2.Id)
	assert.True(t, reflect.DeepEqual(blocks[0].Fields, bl.Fields))
	assert.True(t, reflect.DeepEqual(blocks[1].Fields, bl2.Fields))
}

func TestPreprocessBlocksThreeCodeBlock(t *testing.T) {
	bl := &model.Block{
		Id: bson.NewObjectId().Hex(),
		Fields: &types.Struct{Fields: map[string]*types.Value{
			"lang": pbtypes.String("java"),
		}},
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  "code",
				Style: model.BlockContentText_Code,
			},
		},
	}
	bl2 := &model.Block{
		Id: bson.NewObjectId().Hex(),
		Fields: &types.Struct{Fields: map[string]*types.Value{
			"lang": pbtypes.String("go"),
		}},
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  "code",
				Style: model.BlockContentText_Code,
			},
		},
	}
	bl3 := &model.Block{
		Id: bson.NewObjectId().Hex(),
		Fields: &types.Struct{Fields: map[string]*types.Value{
			"lang": pbtypes.String("go"),
		}},
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  "code1",
				Style: model.BlockContentText_Code,
			},
		},
	}
	blocks := preprocessBlocks([]*model.Block{bl, bl2, bl3})
	assert.Len(t, blocks, 2)

	assert.Equal(t, blocks[0].Id, bl.Id)
	assert.Equal(t, blocks[1].Id, bl2.Id) // second block is a part of first block now, because they have the same language
	assert.Equal(t, blocks[0].Fields.Fields["lang"], pbtypes.String("java"))
	assert.Equal(t, blocks[1].Fields.Fields["lang"], pbtypes.String("go"))

	assert.Equal(t, blocks[0].GetText().GetText(), bl.GetText().GetText())
	assert.Equal(t, blocks[1].GetText().GetText(), bl2.GetText().GetText()+"\n"+bl3.GetText().GetText())
}

func TestCloseTextBlock(t *testing.T) {
	t.Run("1 checkbox block is a child of another checkbox block", func(t *testing.T) {
		// given
		renderer := newBlocksRenderer("", nil)
		renderer.openedTextBlocks = append(renderer.openedTextBlocks,
			[]*textBlock{
				{
					Block: model.Block{
						Id: "id1",
						Content: &model.BlockContentOfText{
							Text: &model.BlockContentText{
								Style: model.BlockContentText_Marked,
							},
						},
					},
					textBuffer: "[] Level 1",
				},
				{
					Block: model.Block{
						Id: "id2",
						Content: &model.BlockContentOfText{
							Text: &model.BlockContentText{
								Style: model.BlockContentText_Marked,
							},
						},
					},
					textBuffer: "[] Level 2",
				},
			}...)

		// when
		renderer.CloseTextBlock(model.BlockContentText_Marked) // handle first text block

		// then
		assert.Len(t, renderer.blocks, 1)

		assert.NotNil(t, renderer.blocks[0].GetText())
		assert.Equal(t, model.BlockContentText_Checkbox, renderer.blocks[0].GetText().Style)
		assert.Equal(t, "Level 2", renderer.blocks[0].GetText().Text)
		assert.Len(t, renderer.blocks[0].ChildrenIds, 0)

		// when
		renderer.CloseTextBlock(model.BlockContentText_Marked) // handle second text block

		// then
		assert.Len(t, renderer.blocks, 2)

		assert.NotNil(t, renderer.blocks[1].GetText())
		assert.Equal(t, model.BlockContentText_Checkbox, renderer.blocks[1].GetText().Style)
		assert.Equal(t, "Level 1", renderer.blocks[1].GetText().Text)
		assert.Len(t, renderer.blocks[1].ChildrenIds, 1)
		assert.Equal(t, "id2", renderer.blocks[1].ChildrenIds[0])
	})
	t.Run("1 checkbox block is a child of another block", func(t *testing.T) {
		// given
		renderer := newBlocksRenderer("", nil)
		renderer.openedTextBlocks = append(renderer.openedTextBlocks,
			[]*textBlock{
				{
					Block: model.Block{
						Id: "id1",
						Content: &model.BlockContentOfText{
							Text: &model.BlockContentText{
								Style: model.BlockContentText_Numbered,
							},
						},
					},
					textBuffer: "1. Level 1",
				},
				{
					Block: model.Block{
						Id: "id2",
						Content: &model.BlockContentOfText{
							Text: &model.BlockContentText{
								Style: model.BlockContentText_Marked,
							},
						},
					},
					textBuffer: "[] Level 2",
				},
			}...)

		// when
		renderer.CloseTextBlock(model.BlockContentText_Marked)

		// then
		assert.Len(t, renderer.blocks, 1)

		assert.NotNil(t, renderer.blocks[0].GetText())
		assert.Equal(t, model.BlockContentText_Checkbox, renderer.blocks[0].GetText().Style)
		assert.Equal(t, "Level 2", renderer.blocks[0].GetText().Text)
		assert.Len(t, renderer.blocks[0].ChildrenIds, 0)

		// when
		renderer.CloseTextBlock(model.BlockContentText_Marked)

		// then
		assert.Len(t, renderer.blocks, 2)

		assert.NotNil(t, renderer.blocks[1].GetText())
		assert.Equal(t, model.BlockContentText_Numbered, renderer.blocks[1].GetText().Style)
		assert.Equal(t, "1. Level 1", renderer.blocks[1].GetText().Text)
		assert.Len(t, renderer.blocks[1].ChildrenIds, 1)
		assert.Equal(t, "id2", renderer.blocks[1].ChildrenIds[0])
	})
}
