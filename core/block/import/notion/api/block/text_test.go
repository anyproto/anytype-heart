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
				Type:      api.Text,
				PlainText: "test",
			},
			{
				Type:      api.Text,
				PlainText: "test2",
			},
		},
		Color: api.RedBackGround,
	}

	bl := to.GetTextBlocks(model.BlockContentText_Paragraph, nil, &MapRequest{})
	assert.Len(t, bl.Blocks, 1)
	assert.Equal(t, bl.Blocks[0].GetText().Style, model.BlockContentText_Paragraph)
	assert.Equal(t, bl.Blocks[0].BackgroundColor, api.AnytypeRed)
	assert.Equal(t, bl.Blocks[0].GetText().Text, "testtest2")
}

func Test_GetTextBlocksTextUserMention(t *testing.T) {
	to := &TextObject{
		RichText: []api.RichText{
			{
				Type: api.Mention,
				Mention: &api.MentionObject{
					Type: api.UserMention,
					User: &api.User{
						ID:   "id",
						Name: "Nastya",
					},
				},
				PlainText: "Nastya",
			},
		},
	}

	bl := to.GetTextBlocks(model.BlockContentText_Paragraph, nil, &MapRequest{})
	assert.Len(t, bl.Blocks, 1)
	assert.Equal(t, bl.Blocks[0].GetText().Style, model.BlockContentText_Paragraph)
	assert.Len(t, bl.Blocks[0].GetText().Marks.Marks, 0)
	assert.Equal(t, bl.Blocks[0].GetText().Text, "Nastya")
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

	bl := to.GetTextBlocks(model.BlockContentText_Paragraph, nil, &MapRequest{
		NotionPageIdsToAnytype: map[string]string{"notionID": "anytypeID"},
		PageNameToID:           map[string]string{"notionID": "Page"},
	})
	assert.Len(t, bl.Blocks, 1)
	assert.Equal(t, bl.Blocks[0].GetText().Style, model.BlockContentText_Paragraph)
	assert.Len(t, bl.Blocks[0].GetText().Marks.Marks, 1)
	assert.Equal(t, bl.Blocks[0].GetText().Marks.Marks[0].Type, model.BlockContentTextMark_Mention)
	assert.Equal(t, bl.Blocks[0].GetText().Text, "Page")
}

func Test_GetTextBlocksTextPageMentionNotFound(t *testing.T) {
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

	bl := to.GetTextBlocks(model.BlockContentText_Paragraph, nil, &MapRequest{
		NotionPageIdsToAnytype: map[string]string{},
		PageNameToID:           map[string]string{},
	})
	assert.Len(t, bl.Blocks, 1)
	assert.Equal(t, bl.Blocks[0].GetText().Style, model.BlockContentText_Paragraph)
	assert.Equal(t, bl.Blocks[0].GetText().Text, notExistingObjectMessage)
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

	bl := to.GetTextBlocks(model.BlockContentText_Paragraph, nil, &MapRequest{
		NotionDatabaseIdsToAnytype: map[string]string{},
		DatabaseNameToID:           map[string]string{},
	})
	assert.Len(t, bl.Blocks, 1)
	assert.Equal(t, bl.Blocks[0].GetText().Style, model.BlockContentText_Paragraph)
	assert.Equal(t, bl.Blocks[0].GetText().Text, notExistingObjectMessage)
}

func Test_GetTextBlocksDatabaseMentionWithoutSource(t *testing.T) {
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
				PlainText: "Database name",
			},
		},
	}

	bl := to.GetTextBlocks(model.BlockContentText_Paragraph, nil, &MapRequest{
		NotionDatabaseIdsToAnytype: map[string]string{},
		DatabaseNameToID:           map[string]string{},
	})
	assert.Len(t, bl.Blocks, 1)
	assert.Equal(t, bl.Blocks[0].GetText().Style, model.BlockContentText_Paragraph)
	assert.Equal(t, bl.Blocks[0].GetText().Text, "Database name")
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

	bl := to.GetTextBlocks(model.BlockContentText_Paragraph, nil, &MapRequest{})
	assert.Len(t, bl.Blocks, 1)
	assert.Equal(t, bl.Blocks[0].GetText().Style, model.BlockContentText_Paragraph)
	assert.Len(t, bl.Blocks[0].GetText().Marks.Marks, 1)
	assert.Equal(t, bl.Blocks[0].GetText().Marks.Marks[0].Type, model.BlockContentTextMark_Mention)
	assert.Equal(t, bl.Blocks[0].GetText().Marks.Marks[0].Param, "_date_2022-11-14")
}

func Test_GetTextBlocksLinkPreview(t *testing.T) {
	to := &TextObject{
		RichText: []api.RichText{
			{
				Type: api.Mention,
				Mention: &api.MentionObject{
					Type: api.LinkPreview,
					LinkPreview: &api.Link{
						URL: "ref",
					},
				},
			},
		},
	}

	bl := to.GetTextBlocks(model.BlockContentText_Paragraph, nil, &MapRequest{})
	assert.Len(t, bl.Blocks, 1)
	assert.Equal(t, bl.Blocks[0].GetText().Style, model.BlockContentText_Paragraph)
	assert.NotNil(t, bl.Blocks[0].GetText().Marks)
	assert.Len(t, bl.Blocks[0].GetText().Marks.Marks, 1)
	assert.Equal(t, bl.Blocks[0].GetText().Marks.Marks[0].Type, model.BlockContentTextMark_Link)
	assert.Equal(t, bl.Blocks[0].GetText().Text, "ref")
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

	bl := to.GetTextBlocks(model.BlockContentText_Paragraph, nil, &MapRequest{})
	assert.Len(t, bl.Blocks, 1)
	assert.NotNil(t, bl.Blocks[0].GetLatex())
	assert.Equal(t, bl.Blocks[0].GetLatex().Text, "Equation")
}

func Test_GetCodeBlocksSuccess(t *testing.T) {
	co := &CodeBlock{
		Code: CodeObject{
			RichText: []api.RichText{
				{
					Type:      api.Text,
					PlainText: "Code",
				},
			},
			Language: "Go",
		},
	}
	bl := co.GetBlocks(&MapRequest{})
	assert.NotNil(t, bl)
	assert.Len(t, bl.Blocks, 1)
	assert.Equal(t, bl.Blocks[0].GetText().Text, "Code")
}
