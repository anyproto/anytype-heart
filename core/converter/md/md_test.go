package md

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestMD_Convert(t *testing.T) {
	newState := func(bs ...*model.Block) *state.State {
		var sbs []simple.Block
		var ids []string
		for _, b := range bs {
			sb := simple.New(b)
			ids = append(ids, sb.Model().Id)
			sbs = append(sbs, sb)
		}
		blocks := map[string]simple.Block{
			"root": simple.New(&model.Block{Id: "root", ChildrenIds: ids}),
		}
		for _, sb := range sbs {
			blocks[sb.Model().Id] = sb
		}
		return state.NewDoc("root", blocks).(*state.State)
	}

	t.Run("markup render", func(t *testing.T) {
		s := newState(
			&model.Block{
				Content: &model.BlockContentOfText{
					Text: &model.BlockContentText{
						Text:  "Header 1",
						Style: model.BlockContentText_Header1,
					},
				},
			},
			&model.Block{
				Content: &model.BlockContentOfText{
					Text: &model.BlockContentText{
						Text:  "Header 2",
						Style: model.BlockContentText_Header2,
					},
				},
			},
			&model.Block{
				Content: &model.BlockContentOfDiv{
					Div: &model.BlockContentDiv{
						Style: model.BlockContentDiv_Dots,
					},
				},
			},
			&model.Block{
				Content: &model.BlockContentOfText{
					Text: &model.BlockContentText{
						Text:  "Header 3",
						Style: model.BlockContentText_Header3,
					},
				},
			},
			&model.Block{
				Content: &model.BlockContentOfText{
					Text: &model.BlockContentText{
						Text: "Usual text",
					},
				},
			},
			&model.Block{
				Content: &model.BlockContentOfText{
					Text: &model.BlockContentText{
						Text:  "Header 4",
						Style: model.BlockContentText_Header4,
					},
				},
			},
		)
		c := NewMDConverter(nil, s, nil)
		res := c.Convert(0)
		exp := "# Header 1   \n## Header 2   \n --- \n### Header 3   \nUsual text   \n#### Header 4   \n"
		assert.Equal(t, exp, string(res))
	})

	t.Run("test header rendering", func(t *testing.T) {
		s := newState(&model.Block{
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: "some text with marks @mention",
					Marks: &model.BlockContentTextMarks{
						Marks: []*model.BlockContentTextMark{
							{
								Range: &model.Range{0, 4},
								Type:  model.BlockContentTextMark_Bold,
							},
							{
								Range: &model.Range{0, 4},
								Type:  model.BlockContentTextMark_Italic,
							},
							{
								Range: &model.Range{0, 4},
								Type:  model.BlockContentTextMark_Link,
								Param: "http://golang.org",
							},
							{
								Range: &model.Range{5, 6},
								Type:  model.BlockContentTextMark_Link,
								Param: "http://golang.org",
							},
							{
								Range: &model.Range{6, 7},
								Type:  model.BlockContentTextMark_Link,
								Param: "http://golang.org",
							},
							{
								Range: &model.Range{10, 16},
								Type:  model.BlockContentTextMark_Bold,
							},
							{
								Range: &model.Range{12, 18},
								Type:  model.BlockContentTextMark_Strikethrough,
							},
						},
					},
				},
			},
		})
		c := NewMDConverter(nil, s, nil)
		res := c.Convert(0)
		exp := "***[some](http://golang.org)*** [t](http://golang.org) [e](http://golang.org)xt **wi~~th m~~**~~ar~~ks @mention   \n"
		assert.Equal(t, exp, string(res))
	})
}
