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

func TestAnytypeObjectLinks(t *testing.T) {
	const objectID = "bafyreiepe46rglxszewcokukqhbbuu2quzby2hue4h6cue3lhdxil2lxii"
	const spaceID = "bafyreielnx2xgkomxvfdhg36kgemica7kdokt5r7d3rmdwnndaewvfesdi.7wp5no5ccvt2"

	tests := []struct {
		name     string
		markdown string
		expected []expectedBlock
	}{
		{
			name:     "Inline anytype object link becomes mention",
			markdown: "See [My Task](anytype://object?objectId=" + objectID + "&spaceId=" + spaceID + ") for details.",
			expected: []expectedBlock{
				{
					Type: model.BlockContentText_Paragraph,
					Text: "See My Task for details.",
					Marks: []*model.BlockContentTextMark{
						{
							Type:  model.BlockContentTextMark_Mention,
							Param: objectID,
							Range: &model.Range{From: 4, To: 11},
						},
					},
				},
			},
		},
		{
			name:     "Anytype link without spaceId",
			markdown: "Link to [Page](anytype://object?objectId=" + objectID + ") here.",
			expected: []expectedBlock{
				{
					Type: model.BlockContentText_Paragraph,
					Text: "Link to Page here.",
					Marks: []*model.BlockContentTextMark{
						{
							Type:  model.BlockContentTextMark_Mention,
							Param: objectID,
							Range: &model.Range{From: 8, To: 12},
						},
					},
				},
			},
		},
		{
			name:     "Regular HTTP link stays as Link mark",
			markdown: "Visit [Google](https://google.com) today.",
			expected: []expectedBlock{
				{
					Type: model.BlockContentText_Paragraph,
					Text: "Visit Google today.",
					Marks: []*model.BlockContentTextMark{
						{
							Type:  model.BlockContentTextMark_Link,
							Param: "https://google.com",
							Range: &model.Range{From: 6, To: 12},
						},
					},
				},
			},
		},
		{
			name:     "Mixed anytype and HTTP links",
			markdown: "See [task](anytype://object?objectId=" + objectID + ") and [docs](https://example.com).",
			expected: []expectedBlock{
				{
					Type: model.BlockContentText_Paragraph,
					Text: "See task and docs.",
					Marks: []*model.BlockContentTextMark{
						{
							Type:  model.BlockContentTextMark_Mention,
							Param: objectID,
							Range: &model.Range{From: 4, To: 8},
						},
						{
							Type:  model.BlockContentTextMark_Link,
							Param: "https://example.com",
							Range: &model.Range{From: 13, To: 17},
						},
					},
				},
			},
		},
		{
			name:     "Standalone anytype link on its own line",
			markdown: "[My Task](anytype://object?objectId=" + objectID + "&spaceId=" + spaceID + ")",
			expected: []expectedBlock{
				{
					Type: model.BlockContentText_Paragraph,
					Text: "My Task",
					Marks: []*model.BlockContentTextMark{
						{
							Type:  model.BlockContentTextMark_Mention,
							Param: objectID,
							Range: &model.Range{From: 0, To: 7},
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

			var textBlocks []*model.Block
			for _, block := range blocks {
				if block.GetText() != nil {
					textBlocks = append(textBlocks, block)
				}
			}

			require.Equal(t, len(tt.expected), len(textBlocks), "Number of blocks mismatch")

			for i, exp := range tt.expected {
				block := textBlocks[i]
				require.NotNil(t, block.GetText())
				assert.Equal(t, exp.Text, block.GetText().GetText())
				assert.Equal(t, exp.Type, block.GetText().GetStyle())

				if len(exp.Marks) > 0 {
					marks := block.GetText().GetMarks()
					require.NotNil(t, marks)
					require.Equal(t, len(exp.Marks), len(marks.GetMarks()))
					for j, expectedMark := range exp.Marks {
						actualMark := marks.GetMarks()[j]
						assert.Equal(t, expectedMark.Type, actualMark.Type, "mark %d type mismatch", j)
						assert.Equal(t, expectedMark.Param, actualMark.Param, "mark %d param mismatch", j)
						assert.Equal(t, expectedMark.Range.From, actualMark.Range.From, "mark %d range.from mismatch", j)
						assert.Equal(t, expectedMark.Range.To, actualMark.Range.To, "mark %d range.to mismatch", j)
					}
				}
			}
		})
	}
}

func TestExtractAnytypeObjectID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Valid anytype object URL",
			input:    "anytype://object?objectId=bafyrei123&spaceId=space456",
			expected: "bafyrei123",
		},
		{
			name:     "Valid anytype object URL without spaceId",
			input:    "anytype://object?objectId=bafyrei123",
			expected: "bafyrei123",
		},
		{
			name:     "HTTP URL returns empty",
			input:    "https://example.com",
			expected: "",
		},
		{
			name:     "Anytype URL with wrong host",
			input:    "anytype://file?fileId=abc",
			expected: "",
		},
		{
			name:     "Anytype object URL without objectId param",
			input:    "anytype://object?spaceId=space456",
			expected: "",
		},
		{
			name:     "Empty string returns empty",
			input:    "",
			expected: "",
		},
		{
			name:     "Local file path returns empty",
			input:    "my-document.md",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractAnytypeObjectID(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
