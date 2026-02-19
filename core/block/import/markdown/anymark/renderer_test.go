package anymark

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestWikiLinks(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		expected []expectedBlock
	}{
		{
			name:     "Simple wiki link",
			markdown: "This is a [[wiki link]] in text.",
			expected: []expectedBlock{
				{
					Type: model.BlockContentText_Paragraph,
					Text: "This is a wiki link in text.",
					Marks: []*model.BlockContentTextMark{
						{
							Type:  model.BlockContentTextMark_Link,
							Param: "wiki link.md",
							Range: &model.Range{From: 10, To: 19},
						},
					},
				},
			},
		},
		{
			name:     "Wiki link with pipe",
			markdown: "This is a [[page name|display text]] in text.",
			expected: []expectedBlock{
				{
					Type: model.BlockContentText_Paragraph,
					Text: "This is a display text in text.",
					Marks: []*model.BlockContentTextMark{
						{
							Type:  model.BlockContentTextMark_Link,
							Param: "page name.md",
							Range: &model.Range{From: 10, To: 22},
						},
					},
				},
			},
		},
		{
			name:     "Embed wiki link as text",
			markdown: "This is an embedded ![[document]] link.",
			expected: []expectedBlock{
				{
					Type: model.BlockContentText_Paragraph,
					Text: "This is an embedded document link.",
					Marks: []*model.BlockContentTextMark{
						{
							Type:  model.BlockContentTextMark_Link,
							Param: "document.md",
							Range: &model.Range{From: 20, To: 28},
						},
					},
				},
			},
		},
		{
			name:     "Embed wiki link with image",
			markdown: "![[image.png]]",
			expected: []expectedBlock{
				{
					IsImage: true,
					Param:   "image.png",
				},
			},
		},
		{
			name:     "Text before embed image",
			markdown: "Here is text\n\n![[image.png]]",
			expected: []expectedBlock{
				{
					Type: model.BlockContentText_Paragraph,
					Text: "Here is text",
				},
				{
					IsImage: true,
					Param:   "image.png",
				},
			},
		},
		{
			name:     "Multiple wiki links",
			markdown: "Link to [[page1]] and [[page2]] and embed ![[page3]].",
			expected: []expectedBlock{
				{
					Type: model.BlockContentText_Paragraph,
					Text: "Link to page1 and page2 and embed page3.",
					Marks: []*model.BlockContentTextMark{
						{
							Type:  model.BlockContentTextMark_Link,
							Param: "page1.md",
							Range: &model.Range{From: 8, To: 13},
						},
						{
							Type:  model.BlockContentTextMark_Link,
							Param: "page2.md",
							Range: &model.Range{From: 18, To: 23},
						},
						{
							Type:  model.BlockContentTextMark_Link,
							Param: "page3.md",
							Range: &model.Range{From: 34, To: 39},
						},
					},
				},
			},
		},
		{
			name:     "Wiki link with spaces",
			markdown: "Link to [[My Document Name]] here.",
			expected: []expectedBlock{
				{
					Type: model.BlockContentText_Paragraph,
					Text: "Link to My Document Name here.",
					Marks: []*model.BlockContentTextMark{
						{
							Type:  model.BlockContentTextMark_Link,
							Param: "My Document Name.md",
							Range: &model.Range{From: 8, To: 24},
						},
					},
				},
			},
		},
		{
			name:     "Incomplete wiki link",
			markdown: "This [[is not complete",
			expected: []expectedBlock{
				{
					Type: model.BlockContentText_Paragraph,
					Text: "This [[is not complete",
				},
			},
		},
		{
			name:     "Mixed regular and wiki links",
			markdown: "Regular [link](http://example.com) and [[wiki link]].",
			expected: []expectedBlock{
				{
					Type: model.BlockContentText_Paragraph,
					Text: "Regular link and wiki link.",
					Marks: []*model.BlockContentTextMark{
						{
							Type:  model.BlockContentTextMark_Link,
							Param: "http://example.com",
							Range: &model.Range{From: 8, To: 12},
						},
						{
							Type:  model.BlockContentTextMark_Link,
							Param: "wiki link.md",
							Range: &model.Range{From: 17, To: 26},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks, _, err := MarkdownToBlocks([]byte(tt.markdown), "", nil)
			require.NoError(t, err)

			// Filter out document blocks
			var textBlocks []*model.Block
			for _, block := range blocks {
				if block.GetText() != nil || block.GetFile() != nil {
					textBlocks = append(textBlocks, block)
				}
			}

			require.Equal(t, len(tt.expected), len(textBlocks), "Number of blocks mismatch")

			for i, expectedBlock := range tt.expected {
				block := textBlocks[i]

				if expectedBlock.IsImage {
					// Check image block
					require.NotNil(t, block.GetFile())
					assert.Equal(t, expectedBlock.Param, block.GetFile().GetName())
					assert.Equal(t, model.BlockContentFile_Image, block.GetFile().GetType())
				} else {
					// Check text block
					require.NotNil(t, block.GetText())
					assert.Equal(t, expectedBlock.Text, block.GetText().GetText())
					assert.Equal(t, expectedBlock.Type, block.GetText().GetStyle())

					// Check marks
					if len(expectedBlock.Marks) > 0 {
						marks := block.GetText().GetMarks()
						require.NotNil(t, marks)
						assert.Equal(t, len(expectedBlock.Marks), len(marks.GetMarks()))
						for j, expectedMark := range expectedBlock.Marks {
							actualMark := marks.GetMarks()[j]
							assert.Equal(t, expectedMark.Type, actualMark.Type)
							assert.Equal(t, expectedMark.Param, actualMark.Param)
							assert.Equal(t, expectedMark.Range.From, actualMark.Range.From)
							assert.Equal(t, expectedMark.Range.To, actualMark.Range.To)
						}
					}
				}
			}
		})
	}
}

type expectedBlock struct {
	Type    model.BlockContentTextStyle
	Text    string
	Param   string
	IsImage bool
	Marks   []*model.BlockContentTextMark
}
