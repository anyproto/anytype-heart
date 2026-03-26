package vectorsearch

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const maxChunkChars = 2000

type TextChunk struct {
	ID          string
	SpaceID     string
	ObjectID    string
	Position    int
	Title       string // header text that starts this chunk
	ObjectTitle string // main object name
	Text        string // concatenated paragraph text under this header
}

type SemanticTask struct {
	ObjectID    string
	SpaceID     string
	ObjectTitle string
	Blocks      []*model.Block
}

func ChunkBlocks(task SemanticTask) []TextChunk {
	blockMap := make(map[string]*model.Block, len(task.Blocks))
	for _, b := range task.Blocks {
		blockMap[b.Id] = b
	}

	var chunks []TextChunk
	var current *TextChunk

	ensureCurrent := func() {
		if current == nil {
			current = &TextChunk{
				SpaceID:     task.SpaceID,
				ObjectID:    task.ObjectID,
				ObjectTitle: task.ObjectTitle,
			}
		}
	}

	// Walk blocks depth-first via ChildrenIds, replicating state.Iterate order.
	// Blocks() returns BFS order but we reconstruct depth-first from the tree structure.
	var walk func(id string)
	walk = func(id string) {
		b, ok := blockMap[id]
		if !ok {
			return
		}

		if tb := b.GetText(); tb != nil {
			text := strings.TrimSpace(tb.Text)
			if text != "" {
				if isHeaderStyle(tb.Style) {
					// Flush current chunk if it has content
					if current != nil && (current.Title != "" || current.Text != "") {
						chunks = append(chunks, *current)
					}
					current = &TextChunk{
						SpaceID:     task.SpaceID,
						ObjectID:    task.ObjectID,
						ObjectTitle: task.ObjectTitle,
						Title:       text,
					}
				} else {
					ensureCurrent()
					if current.Text != "" {
						current.Text += "\n"
					}
					current.Text += text
				}
			}
		}

		for _, childID := range b.ChildrenIds {
			walk(childID)
		}
	}

	// Find root block (first block in the slice, which is the root in Blocks() output)
	if len(task.Blocks) > 0 {
		walk(task.Blocks[0].Id)
	}

	// Flush last chunk
	if current != nil && (current.Title != "" || current.Text != "") {
		chunks = append(chunks, *current)
	}

	// Split oversized chunks into <=maxChunkChars pieces
	var split []TextChunk
	for _, c := range chunks {
		if len(c.Text) <= maxChunkChars {
			split = append(split, c)
			continue
		}
		text := c.Text
		first := true
		for len(text) > 0 {
			end := maxChunkChars
			if end > len(text) {
				end = len(text)
			}
			piece := TextChunk{
				SpaceID:     c.SpaceID,
				ObjectID:    c.ObjectID,
				ObjectTitle: c.ObjectTitle,
				Text:        text[:end],
			}
			if first {
				piece.Title = c.Title
				first = false
			} else {
				piece.Title = c.Title + " (cont.)"
			}
			split = append(split, piece)
			text = text[end:]
		}
	}

	// Assign positions and IDs
	for idx := range split {
		split[idx].Position = idx
		split[idx].ID = deterministicUUID(fmt.Sprintf("%s/vs/%d", task.ObjectID, idx))
	}

	return split
}

// deterministicUUID generates a UUID v5-like string from an arbitrary input.
// Qdrant requires point IDs to be UUIDs or unsigned integers.
func deterministicUUID(input string) string {
	h := sha256.Sum256([]byte(input))
	// Format as UUID v4 layout (xxxxxxxx-xxxx-4xxx-8xxx-xxxxxxxxxxxx)
	h[6] = (h[6] & 0x0f) | 0x40 // version 4
	h[8] = (h[8] & 0x3f) | 0x80 // variant
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		h[0:4], h[4:6], h[6:8], h[8:10], h[10:16])
}

func isHeaderStyle(style model.BlockContentTextStyle) bool {
	switch style {
	case model.BlockContentText_Header1,
		model.BlockContentText_Header2,
		model.BlockContentText_Header3,
		model.BlockContentText_Header4,
		model.BlockContentText_ToggleHeader1,
		model.BlockContentText_ToggleHeader2,
		model.BlockContentText_ToggleHeader3:
		return true
	}
	return false
}
