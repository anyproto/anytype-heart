package block

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/import/notion/api"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
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

	bl := to.GetTextBlocks(model.BlockContentText_Paragraph, nil, &api.NotionImportContext{})
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

	bl := to.GetTextBlocks(model.BlockContentText_Paragraph, nil, &api.NotionImportContext{})
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

	bl := to.GetTextBlocks(model.BlockContentText_Paragraph, nil, &api.NotionImportContext{
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

	bl := to.GetTextBlocks(model.BlockContentText_Paragraph, nil, &api.NotionImportContext{
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

	bl := to.GetTextBlocks(model.BlockContentText_Paragraph, nil, &api.NotionImportContext{
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

	bl := to.GetTextBlocks(model.BlockContentText_Paragraph, nil, &api.NotionImportContext{
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

	bl := to.GetTextBlocks(model.BlockContentText_Paragraph, nil, &api.NotionImportContext{})
	assert.Len(t, bl.Blocks, 1)
	assert.Equal(t, bl.Blocks[0].GetText().Style, model.BlockContentText_Paragraph)
	assert.Len(t, bl.Blocks[0].GetText().Marks.Marks, 1)
	assert.Equal(t, bl.Blocks[0].GetText().Marks.Marks[0].Type, model.BlockContentTextMark_Mention)
	assert.Equal(t, bl.Blocks[0].GetText().Marks.Marks[0].Param, "_date_2022-11-14-00-00-00")
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

	bl := to.GetTextBlocks(model.BlockContentText_Paragraph, nil, &api.NotionImportContext{})
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

	bl := to.GetTextBlocks(model.BlockContentText_Paragraph, nil, &api.NotionImportContext{})
	assert.Len(t, bl.Blocks, 1)
	assert.NotNil(t, bl.Blocks[0].GetLatex())
	assert.Equal(t, bl.Blocks[0].GetLatex().Text, "Equation")
}

func Test_GetCodeBlocksSuccess(t *testing.T) {
	t.Run("create text block based on provided CodeBlock from notion", func(t *testing.T) {
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
		bl := co.GetBlocks(&api.NotionImportContext{}, "")
		assert.NotNil(t, bl)
		assert.Len(t, bl.Blocks, 1)
		assert.Equal(t, bl.Blocks[0].GetText().Text, "Code")
	})
	t.Run("create embed block, if language is mermaid", func(t *testing.T) {
		co := &CodeBlock{
			Code: CodeObject{
				RichText: []api.RichText{
					{
						Type:      api.Text,
						PlainText: "Code",
					},
				},
				Language: "mermaid",
			},
		}
		bl := co.GetBlocks(&api.NotionImportContext{}, "")
		assert.NotNil(t, bl)
		assert.Len(t, bl.Blocks, 1)
		assert.NotNil(t, bl.Blocks[0].GetLatex())
		assert.Equal(t, bl.Blocks[0].GetLatex().Text, "Code")
	})
}

func Test_GetTextBlocks(t *testing.T) {
	t.Run("page not in integration - write link in text block", func(t *testing.T) {
		to := &TextObject{
			RichText: []api.RichText{
				{
					Type:    api.Mention,
					Mention: &api.MentionObject{Type: api.Page, Page: &api.PageMention{ID: "not exist"}},
					Href:    "href",
				},
			},
		}

		bl := to.GetTextBlocks(model.BlockContentText_Paragraph, nil, &api.NotionImportContext{})
		assert.Len(t, bl.Blocks, 1)
		assert.Equal(t, bl.Blocks[0].GetText().Style, model.BlockContentText_Paragraph)
		assert.Equal(t, bl.Blocks[0].GetText().Text, "href")
		assert.NotNil(t, bl.Blocks[0].GetText().Marks)
		assert.Len(t, bl.Blocks[0].GetText().Marks.Marks, 1)
		assert.Equal(t, bl.Blocks[0].GetText().Marks.Marks[0].Type, model.BlockContentTextMark_Link)
	})
	t.Run("db not in integration - write link in text block", func(t *testing.T) {
		to := &TextObject{
			RichText: []api.RichText{
				{
					Type:    api.Mention,
					Mention: &api.MentionObject{Type: api.Database, Database: &api.DatabaseMention{ID: "not exist"}},
					Href:    "href",
				},
			},
		}

		bl := to.GetTextBlocks(model.BlockContentText_Paragraph, nil, &api.NotionImportContext{})
		assert.Len(t, bl.Blocks, 1)
		assert.Equal(t, bl.Blocks[0].GetText().Style, model.BlockContentText_Paragraph)
		assert.Equal(t, bl.Blocks[0].GetText().Text, "href")
		assert.NotNil(t, bl.Blocks[0].GetText().Marks)
		assert.Len(t, bl.Blocks[0].GetText().Marks.Marks, 1)
		assert.Equal(t, bl.Blocks[0].GetText().Marks.Marks[0].Type, model.BlockContentTextMark_Link)
	})
}
