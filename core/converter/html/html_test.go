package html

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestHTML_Convert(t *testing.T) {

	t.Run("empty selection", func(t *testing.T) {
		s := state.NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{}),
		}).(*state.State)
		assert.Empty(t, NewHTMLConverter(s, nil).Convert())
	})

	t.Run("markup", func(t *testing.T) {
		// given
		doc := givenMarkup()

		// when
		html := convertHtml(doc)

		// then
		expected := givenTrimmedString(markupExpectation)

		assert.Equal(t, expected, givenTrimmedString(html))
	})

	t.Run("lists", func(t *testing.T) {
		// given
		doc := givenLists()

		// when
		html := convertHtml(doc)

		// then
		expected := givenTrimmedString(listExpectation)

		assert.Equal(t, expected, givenTrimmedString(html))
	})

	t.Run("lists in lists", func(t *testing.T) {
		// given
		doc := givenListsInLists()

		// when
		html := convertHtml(doc)

		// then
		expected := givenTrimmedString(listInListExpectation)

		assert.Equal(t, expected, givenTrimmedString(html))
	})

	t.Run("intersection of marks", func(t *testing.T) {
		// given
		doc := givenIntersectMarks()

		// when
		html := convertHtml(doc)

		// then
		expected := givenTrimmedString(intersectMarksExpectation)

		assert.Equal(t, expected, givenTrimmedString(html))
	})

	t.Run("columns", func(t *testing.T) {
		// given
		doc := givenColumns()

		// when
		html := convertHtml(doc)

		// then
		expected := givenTrimmedString(columnsExpectation)

		assert.Equal(t, expected, givenTrimmedString(html))
	})
}

func convertHtml(s *state.State) string {
	return NewHTMLConverter(s, nil).Convert()
}

func givenMarkup() *state.State {
	return state.NewDoc("root", map[string]simple.Block{
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
							{
								Range: &model.Range{From: 3, To: 4},
								Type:  model.BlockContentTextMark_Underscored,
							},
						},
					},
				},
			},
		}),
	}).(*state.State)
}

func givenLists() *state.State {
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
	return s
}

func givenListsInLists() *state.State {
	s := state.NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{ChildrenIds: []string{"1", "2"}}),
	}).(*state.State)
	s.Add(simple.New(&model.Block{
		Id: "1",
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  "1",
				Style: model.BlockContentText_Numbered,
			},
		},
		ChildrenIds: []string{"1.1", "1.2"},
	}))
	s.Add(simple.New(&model.Block{
		Id: "2",
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  "2",
				Style: model.BlockContentText_Numbered,
			},
		},
		ChildrenIds: []string{"2.1", "2.2"},
	}))
	s.Add(simple.New(&model.Block{
		Id: "1.1",
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  "1.1",
				Style: model.BlockContentText_Marked,
			},
		},
	}))
	s.Add(simple.New(&model.Block{
		Id: "1.2",
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  "1.2",
				Style: model.BlockContentText_Marked,
			},
		},
	}))
	s.Add(simple.New(&model.Block{
		Id: "2.1",
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  "2.1",
				Style: model.BlockContentText_Marked,
			},
		},
		ChildrenIds: []string{"2.1.1"},
	}))
	s.Add(simple.New(&model.Block{
		Id: "2.2",
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  "2.2",
				Style: model.BlockContentText_Marked,
			},
		},
	}))
	s.Add(simple.New(&model.Block{
		Id: "2.1.1",
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  "2.1.1",
				Style: model.BlockContentText_Numbered,
			},
		},
		ChildrenIds: []string{"2.1.1.1"},
	}))
	s.Add(simple.New(&model.Block{
		Id: "2.1.1.1",
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  "2.1.1.1",
				Style: model.BlockContentText_Marked,
			},
		},
	}))
	return s
}

func givenIntersectMarks() *state.State {
	s := state.NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{ChildrenIds: []string{"1"}}),
	}).(*state.State)
	s.Add(simple.New(&model.Block{
		Id: "1",
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  "abcdef",
				Style: model.BlockContentText_Marked,
				Marks: &model.BlockContentTextMarks{
					Marks: []*model.BlockContentTextMark{
						{
							Range: &model.Range{
								From: 4,
								To:   5,
							},
							Type: model.BlockContentTextMark_Italic,
						},
						{
							Range: &model.Range{
								From: 2,
								To:   6,
							},
							Type: model.BlockContentTextMark_Bold,
						},
						{
							Range: &model.Range{
								From: 1,
								To:   3,
							},
							Type: model.BlockContentTextMark_Italic,
						},
					},
				},
			},
		},
	}))
	return s
}

func givenColumns() *state.State {
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
	_ = s.InsertTo("1", model.Block_Right, "2")
	return s
}

func givenTrimmedString(s string) string {
	s = strings.ReplaceAll(s, wrapCopyStart, "")
	s = strings.ReplaceAll(s, wrapCopyEnd, "")
	res := regexp.MustCompile(`[\t\r\n\\]+`).ReplaceAllString(s, "")
	return res
}

const markupExpectation = `
<div style="font-size: 15px; line-height: 24px; letter-spacing: -0.08px; font-weight: 400; word-wrap: break-word;" class="paragraph">
	<b>0
		<i>1</i>
	</b>
	<a href="http://test.test">2</a>
	<u>
		<span style="color:#aca996">3</span>
	</u>456789
</div>
`

const listExpectation = `
<ol style=\"font-size:15px;\">
	<li>1</li>
	<li>2</li>
	<li>3
		<ul style=\"font-size:15px;\">
			<li>3.1</li>
			<li>3.2</li>
		</ul>
	</li>
</ol>
<hr class=\"dots\">
<ol style=\"font-size:15px;\">
	<li>5</li>
	<li>6</li>
</ol>
`

const listInListExpectation = `
<ol style=\"font-size:15px;\">
	<li>1
		<ul style=\"font-size:15px;\">
			<li>1.1</li>
			<li>1.2</li>
		</ul>
	</li>
	<li>2
		<ul style=\"font-size:15px;\">
			<li>2.1
				<ol style=\"font-size:15px;\">
					<li>2.1.1
						<ul style=\"font-size:15px;\">
							<li>2.1.1.1</li>
						</ul>
					</li>
				</ol>
			</li>
			<li>2.2</li>
		</ul>
	</li>
</ol>`

const intersectMarksExpectation = `
<ul style="font-size:15px;">
	<li>a
		<i>b
			<b>c</b>
		</i>
		<b>d
			<i>e</i>f
		</b>
	</li>
</ul>
`

const columnsExpectation = `
<div class="row" style="display: flex">
	<div class="column" >
		<div style="font-size: 15px; line-height: 24px; letter-spacing: -0.08px; font-weight: 400; word-wrap: break-word;" class="paragraph">1</div></div><div class="column" ><div style="font-size: 15px; line-height: 24px; letter-spacing: -0.08px; font-weight: 400; word-wrap: break-word;" class="paragraph">
			2
		</div>
	</div>
</div>
`
