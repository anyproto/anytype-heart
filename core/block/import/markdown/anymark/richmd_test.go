package anymark

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Rich markdown round-trip tests: anything the md exporters emit
// (core/block/export/writer.go, converter/md) must parse back into the
// same block structure. anytype:// object links and ```mermaid fences are
// the exporter's serialization of Mention marks and Latex blocks.

const (
	testObjectID = "bafyreihoiapx62rvdgmcuhy5ba22aqvf6k2syeaghiomajrya4zfmlvcme"
	testSpaceID  = "bafyreidr2epoyudmzxf2i4y6s2usjjxk42txchui5tx35wmwbct7yrdtgy.2oypzszch86r"
)

func TestAnytypeObjectLinkBecomesMentionMark(t *testing.T) {
	t.Run("mention with spaceId", func(t *testing.T) {
		md := fmt.Sprintf("See [测试引用](anytype://object?objectId=%s&spaceId=%s) here.", testObjectID, testSpaceID)
		blocks, _, err := MarkdownToBlocks([]byte(md), "", []string{})
		require.NoError(t, err)
		require.Len(t, blocks, 1)

		txt := blocks[0].GetText()
		require.NotNil(t, txt)
		assert.Equal(t, "See 测试引用 here.", txt.Text)

		require.Len(t, txt.GetMarks().GetMarks(), 1)
		m := txt.GetMarks().GetMarks()[0]
		assert.Equal(t, model.BlockContentTextMark_Mention, m.Type)
		assert.Equal(t, testObjectID, m.Param)
		assert.Equal(t, int32(4), m.Range.From)
		assert.Equal(t, int32(8), m.Range.To)
	})

	t.Run("mention without spaceId", func(t *testing.T) {
		md := fmt.Sprintf("[ref](anytype://object?objectId=%s)", testObjectID)
		blocks, _, err := MarkdownToBlocks([]byte(md), "", []string{})
		require.NoError(t, err)
		require.Len(t, blocks, 1)

		txt := blocks[0].GetText()
		require.NotNil(t, txt)
		require.Len(t, txt.GetMarks().GetMarks(), 1)
		m := txt.GetMarks().GetMarks()[0]
		assert.Equal(t, model.BlockContentTextMark_Mention, m.Type)
		assert.Equal(t, testObjectID, m.Param)
	})

	t.Run("plain https link stays a link mark", func(t *testing.T) {
		md := "[site](https://example.com/page)"
		blocks, _, err := MarkdownToBlocks([]byte(md), "", []string{})
		require.NoError(t, err)
		require.Len(t, blocks, 1)

		txt := blocks[0].GetText()
		require.NotNil(t, txt)
		require.Len(t, txt.GetMarks().GetMarks(), 1)
		m := txt.GetMarks().GetMarks()[0]
		assert.Equal(t, model.BlockContentTextMark_Link, m.Type)
		assert.Equal(t, "https://example.com/page", m.Param)
	})

	t.Run("anytype scheme without objectId stays a link mark", func(t *testing.T) {
		md := "[home](anytype://home)"
		blocks, _, err := MarkdownToBlocks([]byte(md), "", []string{})
		require.NoError(t, err)
		require.Len(t, blocks, 1)

		txt := blocks[0].GetText()
		require.NotNil(t, txt)
		require.Len(t, txt.GetMarks().GetMarks(), 1)
		m := txt.GetMarks().GetMarks()[0]
		assert.Equal(t, model.BlockContentTextMark_Link, m.Type)
		assert.Equal(t, "anytype://home", m.Param)
	})
}

func TestMermaidFenceBecomesLatexBlock(t *testing.T) {
	md := "```mermaid\nflowchart TD\n    A --> B\n```"
	blocks, _, err := MarkdownToBlocks([]byte(md), "", []string{})
	require.NoError(t, err)
	require.Len(t, blocks, 1)

	lx := blocks[0].GetLatex()
	require.NotNil(t, lx, "```mermaid fence must become a Latex block, got %T", blocks[0].Content)
	assert.Equal(t, model.BlockContentLatex_Mermaid, lx.Processor)
	assert.Contains(t, lx.Text, "flowchart TD")
	assert.Contains(t, lx.Text, "A --> B")
}

func TestRegularCodeFenceStaysCodeBlock(t *testing.T) {
	md := "```go\nfmt.Println(1)\n```"
	blocks, _, err := MarkdownToBlocks([]byte(md), "", []string{})
	require.NoError(t, err)
	require.Len(t, blocks, 1)

	txt := blocks[0].GetText()
	require.NotNil(t, txt)
	assert.Equal(t, model.BlockContentText_Code, txt.Style)
	require.NotNil(t, blocks[0].Fields)
	assert.Equal(t, "go", blocks[0].Fields.Fields["lang"].GetStringValue())
}
