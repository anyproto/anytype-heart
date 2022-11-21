package block

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

func Test_GetTextBlocksTextSuccess(t *testing.T) {
	to := &TextObject{
		RichText: []api.RichText{
			{
				Type: api.Text,
				PlainText: "test",
			},
			{
				Type: api.Text,
				PlainText: "test2",
			},
		},
		Color:    api.RedBackGround,
	}

	bl, _ := to.GetTextBlocks(model.BlockContentText_Paragraph, nil, nil, nil, nil, nil)
	assert.Len(t, bl, 1)
	assert.Equal(t, bl[0].GetText().Style, model.BlockContentText_Paragraph)
	assert.Equal(t, bl[0].BackgroundColor, api.AnytypeRed)
	assert.Equal(t, bl[0].GetText().Text, "testtest2")
}

func Test_GetTextBlocksTextUserMention(t *testing.T) {
	to := &TextObject{
		RichText: []api.RichText{
			{
				Type: api.Mention,
				Mention: &api.MentionObject{
					Type: api.UserMention,
					User: &api.User{
						ID:        "id",
						Name:      "Nastya",
					},
				},
			},
		},
	}

	bl, _ := to.GetTextBlocks(model.BlockContentText_Paragraph, nil, nil, nil, nil, nil)
	assert.Len(t, bl, 1)
	assert.Equal(t, bl[0].GetText().Style, model.BlockContentText_Paragraph)
	assert.Len(t, bl[0].GetText().Marks.Marks, 0)
	assert.Equal(t, bl[0].GetText().Text, "Nastya")
}

func Test_GetTextBlocksTextPageMention(t *testing.T) {
	to := &TextObject{
		RichText: []api.RichText{
			{
				Type: api.Mention,
				Mention: &api.MentionObject{
					Type: api.Page,
					Page: &api.PageMention{
						ID: "notionID",
					},
				},
			},
		},
	}

	bl, _ := to.GetTextBlocks(model.BlockContentText_Paragraph, nil, map[string]string{"notionID": "anytypeID"}, nil, map[string]string{"notionID": "Page"}, nil)
	assert.Len(t, bl, 1)
	assert.Equal(t, bl[0].GetText().Style, model.BlockContentText_Paragraph)
	assert.Len(t, bl[0].GetText().Marks.Marks, 1)
	assert.Equal(t, bl[0].GetText().Marks.Marks[0].Type, model.BlockContentTextMark_Mention)
	assert.Equal(t, bl[0].GetText().Text, "Page")
}

func Test_GetTextBlocksDatabaseMention(t *testing.T) {
	to := &TextObject{
		RichText: []api.RichText{
			{
				Type: api.Mention,
				Mention: &api.MentionObject{
					Type: api.Database,
					Database: &api.DatabaseMention{
						ID: "notionID",
					},
				},
			},
		},
	}

	bl, _ := to.GetTextBlocks(model.BlockContentText_Paragraph, nil, nil, map[string]string{"notionID": "anytypeID"}, nil, map[string]string{"notionID": "Database"})
	assert.Len(t, bl, 1)
	assert.Equal(t, bl[0].GetText().Style, model.BlockContentText_Paragraph)
	assert.Len(t, bl[0].GetText().Marks.Marks, 1)
	assert.Equal(t, bl[0].GetText().Marks.Marks[0].Type, model.BlockContentTextMark_Mention)
	assert.Equal(t, bl[0].GetText().Text, "Database")
}

func Test_GetTextBlocksDateMention(t *testing.T) {
	to := &TextObject{
		RichText: []api.RichText{
			{
				Type: api.Mention,
				Mention: &api.MentionObject{
					Type: api.Date,
					Date: &api.DateObject{
						Start: "2022-11-14",
					},
				},
			},
		},
	}

	bl, _ := to.GetTextBlocks(model.BlockContentText_Paragraph, nil, nil, nil, nil, nil)
	assert.Len(t, bl, 1)
	assert.Equal(t, bl[0].GetText().Style, model.BlockContentText_Paragraph)
	assert.Len(t, bl[0].GetText().Marks.Marks, 0)
	assert.Equal(t, bl[0].GetText().Text, "2022-11-14")
}

func Test_GetTextBlocksLinkPreview(t *testing.T) {
	to := &TextObject{
		RichText: []api.RichText{
			{
				Type: api.Mention,
				Mention: &api.MentionObject{
					Type: api.LinkPreview,
					LinkPreview: &api.Link{
						Url: "ref",
					},
				},
			},
		},
	}

	bl, _ := to.GetTextBlocks(model.BlockContentText_Paragraph, nil, nil, nil, nil, nil)
	assert.Len(t, bl, 1)
	assert.Equal(t, bl[0].GetText().Style, model.BlockContentText_Paragraph)
	assert.NotNil(t, bl[0].GetText().Marks)
	assert.Len(t, bl[0].GetText().Marks.Marks, 1)
	assert.Equal(t, bl[0].GetText().Marks.Marks[0].Type, model.BlockContentTextMark_Link)
	assert.Equal(t, bl[0].GetText().Text, "ref")
}

func Test_GetTextBlocksEquation(t *testing.T) {
	to := &TextObject{
		RichText: []api.RichText{
			{
				Type: api.Equation,
				Equation: &api.EquationObject{
					Expression: "Equation",
				},
			},
		},
	}

	bl, _ := to.GetTextBlocks(model.BlockContentText_Paragraph, nil, nil, nil, nil, nil)
	assert.Len(t, bl, 1)
	assert.NotNil(t, bl[0].GetLatex())
	assert.Equal(t, bl[0].GetLatex().Text, "Equation")
}

func Test_GetCodeBlocksSuccess(t *testing.T) {
	co := &CodeBlock{ 
		Code: CodeObject{
			RichText: []api.RichText{
				{
					Type: api.Text,
					PlainText: "Code",
				},
			},
			Language: "Go",
		},
	}
	bl := co.Code.GetCodeBlock()
	assert.NotNil(t, bl)
	assert.Equal(t, bl.GetText().Text, "Code")
}