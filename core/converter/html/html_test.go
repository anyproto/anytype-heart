package html

import (
	"strings"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTML_Convert(t *testing.T) {
	t.Run("empty selection", func(t *testing.T) {
		s := state.NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{}),
		}).(*state.State)
		assert.Empty(t, NewHTMLConverter(nil, s).Convert())
	})
	t.Run("markup", func(t *testing.T) {
		s := state.NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{ChildrenIds: []string{"1"}}),
			"1": simple.New(&model.Block{
				Id: "1",
				Content: &model.BlockContentOfText{
					Text: &model.BlockContentText{
						Text: "0123456789",
						Marks: &model.BlockContentTextMarks{
							Marks: []*model.BlockContentTextMark{
								{
									Range: &model.Range{To: 2},
									Type:  model.BlockContentTextMark_Bold,
								},
								{
									Range: &model.Range{From: 1, To: 2},
									Type:  model.BlockContentTextMark_Italic,
								},
								{
									Range: &model.Range{From: 2, To: 3},
									Type:  model.BlockContentTextMark_Link,
									Param: "http://test.test",
								},
								{
									Range: &model.Range{From: 3, To: 4},
									Type:  model.BlockContentTextMark_TextColor,
									Param: "grey",
								},
							},
						},
					},
				},
			}),
		}).(*state.State)
		res := NewHTMLConverter(nil, s).Convert()
		res = strings.ReplaceAll(res, wrapCopyStart, "")
		res = strings.ReplaceAll(res, wrapCopyEnd, "")
		exp := `<div style="font-size: 15px; line-height: 24px; letter-spacing: -0.08px; font-weight: 400; word-wrap: break-word;" class="paragraph" style="font-size: 15px; line-height: 24px; letter-spacing: -0.08px; font-weight: 400; word-wrap: break-word;"><b>0<i>1</b></i><a href="http://test.test">2</a><span style="color:#aca996">3</span>456789</div>`
		assert.Equal(t, exp, res)
	})
	t.Run("lists", func(t *testing.T) {
		s := state.NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{ChildrenIds: []string{"1", "2", "3", "4", "5", "6"}}),
		}).(*state.State)
		s.Add(simple.New(&model.Block{
			Id: "1",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "1",
					Style: model.BlockContentText_Numbered,
				},
			},
		}))
		s.Add(simple.New(&model.Block{
			Id: "2",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "2",
					Style: model.BlockContentText_Numbered,
				},
			},
		}))
		s.Add(simple.New(&model.Block{
			Id: "3",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "3",
					Style: model.BlockContentText_Numbered,
				},
			},
			ChildrenIds: []string{"3.1", "3.2"},
		}))
		s.Add(simple.New(&model.Block{
			Id: "4",
			Content: &model.BlockContentOfDiv{
				Div: &model.BlockContentDiv{
					Style: model.BlockContentDiv_Dots,
				},
			},
		}))
		s.Add(simple.New(&model.Block{
			Id: "5",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "5",
					Style: model.BlockContentText_Numbered,
				},
			},
		}))
		s.Add(simple.New(&model.Block{
			Id: "6",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "6",
					Style: model.BlockContentText_Numbered,
				},
			},
		}))
		s.Add(simple.New(&model.Block{
			Id: "3.1",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "3.1",
					Style: model.BlockContentText_Marked,
				},
			},
		}))
		s.Add(simple.New(&model.Block{
			Id: "3.2",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "3.2",
					Style: model.BlockContentText_Marked,
				},
			},
		}))
		res := NewHTMLConverter(nil, s).Convert()
		res = strings.ReplaceAll(res, wrapCopyStart, "")
		res = strings.ReplaceAll(res, wrapCopyEnd, "")
		exp := `<ol style="font-size:15px;"><li>1</li><li>2</li><li>3<ul style="font-size:15px;"><li>3.1</li><li>3.2</li></li></ol><hr class="dots"><ol style="font-size:15px;"><li>5</li><li>6</li>`
		assert.Equal(t, exp, res)
	})
	t.Run("columns", func(t *testing.T) {
		s := state.NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{Id: "root", ChildrenIds: []string{"1"}}),
		}).(*state.State)
		s.Set(simple.New(&model.Block{
			Id: "1",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: "1",
				},
			},
		}))
		s.Set(simple.New(&model.Block{
			Id: "2",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: "2",
				},
			},
		}))
		require.NoError(t, s.InsertTo("1", model.Block_Right, "2"))
		res := NewHTMLConverter(nil, s).Convert()
		res = strings.ReplaceAll(res, wrapCopyStart, "")
		res = strings.ReplaceAll(res, wrapCopyEnd, "")
		exp := `<div class="row" style="display: flex"><div class="column" ><div style="font-size: 15px; line-height: 24px; letter-spacing: -0.08px; font-weight: 400; word-wrap: break-word;" class="paragraph" style="font-size: 15px; line-height: 24px; letter-spacing: -0.08px; font-weight: 400; word-wrap: break-word;">1</div></div><div class="column" ><div style="font-size: 15px; line-height: 24px; letter-spacing: -0.08px; font-weight: 400; word-wrap: break-word;" class="paragraph" style="font-size: 15px; line-height: 24px; letter-spacing: -0.08px; font-weight: 400; word-wrap: break-word;">2</div></div></div>`
		assert.Equal(t, exp, res)
	})
}
