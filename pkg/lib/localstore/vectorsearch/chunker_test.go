package vectorsearch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestChunkBlocks(t *testing.T) {
	t.Run("no blocks produces no chunks", func(t *testing.T) {
		task := SemanticTask{
			ObjectID:    "obj1",
			SpaceID:     "space1",
			ObjectTitle: "My Object",
		}
		chunks := ChunkBlocks(task)
		assert.Empty(t, chunks)
	})

	t.Run("paragraphs only produce single chunk", func(t *testing.T) {
		task := SemanticTask{
			ObjectID:    "obj1",
			SpaceID:     "space1",
			ObjectTitle: "My Object",
			Blocks: []*model.Block{
				{Id: "root", ChildrenIds: []string{"p1", "p2"}},
				{Id: "p1", Content: &model.BlockContentOfText{Text: &model.BlockContentText{
					Text: "First paragraph", Style: model.BlockContentText_Paragraph,
				}}},
				{Id: "p2", Content: &model.BlockContentOfText{Text: &model.BlockContentText{
					Text: "Second paragraph", Style: model.BlockContentText_Paragraph,
				}}},
			},
		}

		chunks := ChunkBlocks(task)
		require.Len(t, chunks, 1)
		assert.Equal(t, "", chunks[0].Title)
		assert.Equal(t, "First paragraph\nSecond paragraph", chunks[0].Text)
		assert.Equal(t, "My Object", chunks[0].ObjectTitle)
		assert.Equal(t, 0, chunks[0].Position)
		assert.Len(t, chunks[0].ID, 36, "ID should be UUID format")
		assert.Contains(t, chunks[0].ID, "-", "ID should contain dashes (UUID format)")
	})

	t.Run("headers split into separate chunks", func(t *testing.T) {
		task := SemanticTask{
			ObjectID:    "obj1",
			SpaceID:     "space1",
			ObjectTitle: "My Object",
			Blocks: []*model.Block{
				{Id: "root", ChildrenIds: []string{"h1", "p1", "h2", "p2"}},
				{Id: "h1", Content: &model.BlockContentOfText{Text: &model.BlockContentText{
					Text: "Introduction", Style: model.BlockContentText_Header1,
				}}},
				{Id: "p1", Content: &model.BlockContentOfText{Text: &model.BlockContentText{
					Text: "Some intro text", Style: model.BlockContentText_Paragraph,
				}}},
				{Id: "h2", Content: &model.BlockContentOfText{Text: &model.BlockContentText{
					Text: "Details", Style: model.BlockContentText_Header2,
				}}},
				{Id: "p2", Content: &model.BlockContentOfText{Text: &model.BlockContentText{
					Text: "Some detail text", Style: model.BlockContentText_Paragraph,
				}}},
			},
		}

		chunks := ChunkBlocks(task)
		require.Len(t, chunks, 2)

		assert.Equal(t, "Introduction", chunks[0].Title)
		assert.Equal(t, "Some intro text", chunks[0].Text)
		assert.Equal(t, 0, chunks[0].Position)

		assert.Equal(t, "Details", chunks[1].Title)
		assert.Equal(t, "Some detail text", chunks[1].Text)
		assert.Equal(t, 1, chunks[1].Position)
	})

	t.Run("preamble before first header becomes chunk 0", func(t *testing.T) {
		task := SemanticTask{
			ObjectID:    "obj1",
			SpaceID:     "space1",
			ObjectTitle: "My Object",
			Blocks: []*model.Block{
				{Id: "root", ChildrenIds: []string{"p0", "h1", "p1"}},
				{Id: "p0", Content: &model.BlockContentOfText{Text: &model.BlockContentText{
					Text: "Preamble text", Style: model.BlockContentText_Paragraph,
				}}},
				{Id: "h1", Content: &model.BlockContentOfText{Text: &model.BlockContentText{
					Text: "Section One", Style: model.BlockContentText_Header1,
				}}},
				{Id: "p1", Content: &model.BlockContentOfText{Text: &model.BlockContentText{
					Text: "Section content", Style: model.BlockContentText_Paragraph,
				}}},
			},
		}

		chunks := ChunkBlocks(task)
		require.Len(t, chunks, 2)
		assert.Equal(t, "", chunks[0].Title)
		assert.Equal(t, "Preamble text", chunks[0].Text)
		assert.Equal(t, "Section One", chunks[1].Title)
		assert.Equal(t, "Section content", chunks[1].Text)
	})

	t.Run("empty text blocks are skipped", func(t *testing.T) {
		task := SemanticTask{
			ObjectID:    "obj1",
			SpaceID:     "space1",
			ObjectTitle: "My Object",
			Blocks: []*model.Block{
				{Id: "root", ChildrenIds: []string{"p1", "empty", "p2"}},
				{Id: "p1", Content: &model.BlockContentOfText{Text: &model.BlockContentText{
					Text: "Content", Style: model.BlockContentText_Paragraph,
				}}},
				{Id: "empty", Content: &model.BlockContentOfText{Text: &model.BlockContentText{
					Text: "   ", Style: model.BlockContentText_Paragraph,
				}}},
				{Id: "p2", Content: &model.BlockContentOfText{Text: &model.BlockContentText{
					Text: "More content", Style: model.BlockContentText_Paragraph,
				}}},
			},
		}

		chunks := ChunkBlocks(task)
		require.Len(t, chunks, 1)
		assert.Equal(t, "Content\nMore content", chunks[0].Text)
	})

	t.Run("toggle headers are treated as headers", func(t *testing.T) {
		task := SemanticTask{
			ObjectID:    "obj1",
			SpaceID:     "space1",
			ObjectTitle: "My Object",
			Blocks: []*model.Block{
				{Id: "root", ChildrenIds: []string{"th1", "p1"}},
				{Id: "th1", Content: &model.BlockContentOfText{Text: &model.BlockContentText{
					Text: "Toggle Section", Style: model.BlockContentText_ToggleHeader1,
				}}},
				{Id: "p1", Content: &model.BlockContentOfText{Text: &model.BlockContentText{
					Text: "Toggle content", Style: model.BlockContentText_Paragraph,
				}}},
			},
		}

		chunks := ChunkBlocks(task)
		require.Len(t, chunks, 1)
		assert.Equal(t, "Toggle Section", chunks[0].Title)
		assert.Equal(t, "Toggle content", chunks[0].Text)
	})

	t.Run("nested children are walked depth-first", func(t *testing.T) {
		task := SemanticTask{
			ObjectID:    "obj1",
			SpaceID:     "space1",
			ObjectTitle: "My Object",
			Blocks: []*model.Block{
				{Id: "root", ChildrenIds: []string{"h1"}},
				{Id: "h1", ChildrenIds: []string{"p1", "p2"}, Content: &model.BlockContentOfText{Text: &model.BlockContentText{
					Text: "Parent Header", Style: model.BlockContentText_Header1,
				}}},
				{Id: "p1", Content: &model.BlockContentOfText{Text: &model.BlockContentText{
					Text: "Child 1", Style: model.BlockContentText_Paragraph,
				}}},
				{Id: "p2", Content: &model.BlockContentOfText{Text: &model.BlockContentText{
					Text: "Child 2", Style: model.BlockContentText_Paragraph,
				}}},
			},
		}

		chunks := ChunkBlocks(task)
		require.Len(t, chunks, 1)
		assert.Equal(t, "Parent Header", chunks[0].Title)
		assert.Equal(t, "Child 1\nChild 2", chunks[0].Text)
	})

	t.Run("consecutive headers without text", func(t *testing.T) {
		task := SemanticTask{
			ObjectID:    "obj1",
			SpaceID:     "space1",
			ObjectTitle: "My Object",
			Blocks: []*model.Block{
				{Id: "root", ChildrenIds: []string{"h1", "h2", "p1"}},
				{Id: "h1", Content: &model.BlockContentOfText{Text: &model.BlockContentText{
					Text: "First Header", Style: model.BlockContentText_Header1,
				}}},
				{Id: "h2", Content: &model.BlockContentOfText{Text: &model.BlockContentText{
					Text: "Second Header", Style: model.BlockContentText_Header2,
				}}},
				{Id: "p1", Content: &model.BlockContentOfText{Text: &model.BlockContentText{
					Text: "Some text", Style: model.BlockContentText_Paragraph,
				}}},
			},
		}

		chunks := ChunkBlocks(task)
		require.Len(t, chunks, 2)
		assert.Equal(t, "First Header", chunks[0].Title)
		assert.Equal(t, "", chunks[0].Text)
		assert.Equal(t, "Second Header", chunks[1].Title)
		assert.Equal(t, "Some text", chunks[1].Text)
	})
}

func TestIsHeaderStyle(t *testing.T) {
	headers := []model.BlockContentTextStyle{
		model.BlockContentText_Header1,
		model.BlockContentText_Header2,
		model.BlockContentText_Header3,
		model.BlockContentText_Header4,
		model.BlockContentText_ToggleHeader1,
		model.BlockContentText_ToggleHeader2,
		model.BlockContentText_ToggleHeader3,
	}
	for _, s := range headers {
		assert.True(t, isHeaderStyle(s), "expected %v to be header", s)
	}

	nonHeaders := []model.BlockContentTextStyle{
		model.BlockContentText_Paragraph,
		model.BlockContentText_Quote,
		model.BlockContentText_Code,
		model.BlockContentText_Title,
		model.BlockContentText_Checkbox,
		model.BlockContentText_Marked,
		model.BlockContentText_Numbered,
		model.BlockContentText_Toggle,
		model.BlockContentText_Callout,
	}
	for _, s := range nonHeaders {
		assert.False(t, isHeaderStyle(s), "expected %v to NOT be header", s)
	}
}
