package md

import (
	"testing"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/stretchr/testify/assert"
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
		res := c.Convert()
		exp := "***[some](http://golang.org)*** [t](http://golang.org) [e](http://golang.org)xt **wi~~th m~~**~~ar~~ks @mention   \n"
		assert.Equal(t, exp, string(res))
	})
}
