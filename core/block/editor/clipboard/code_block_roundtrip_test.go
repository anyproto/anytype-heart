package clipboard

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/tests/blockbuilder"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

// GO-7515: copying a code block and pasting it back through the HTML slot used
// to lose every line break and the Code style, because the exported markup
// nested the tags as <code><pre> instead of <pre><code>.
//
// This exercises the real Copy and the real Paste, not just the converter: only
// HtmlSlot is populated on the paste request, so Paste's slot priority (File ->
// Any -> Html -> Text) is forced down the HTML branch, and the result goes
// through Paste's own post-processing.
func TestClipboard_CodeBlockCopyPasteRoundTripThroughHtmlSlot(t *testing.T) {
	const code = "def f():\n    if x:\n        return 1\n    return 2"

	// given: a document holding a real multi-line, indented code block
	source := smarttest.New("source")
	source.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
		blockbuilder.ID("root"),
		blockbuilder.Children(
			blockbuilder.Text(
				code,
				blockbuilder.ID("code1"),
				blockbuilder.TextStyle(model.BlockContentText_Code),
			),
		)))

	// when: the real Copy produces the clipboard slots
	sourceCb := newFixture(t, source)
	_, htmlSlot, _, err := sourceCb.Copy(nil, pb.RpcBlockCopyRequest{
		Blocks: []*model.Block{source.Pick("code1").Model()},
	})
	require.NoError(t, err)
	require.NotEmpty(t, htmlSlot)

	// and: it is pasted into an empty document with ONLY the HTML slot set, so
	// the HTML branch of Paste is the one that runs
	target := createPage(t, createBlocks([]string{}, []string{}, emptyMarks))
	targetCb := newFixture(t, target)
	_, _, _, _, err = targetCb.Paste(nil, &pb.RpcBlockPasteRequest{
		HtmlSlot:          htmlSlot,
		SelectedTextRange: &model.Range{},
	}, "")
	require.NoError(t, err)

	// then: exactly one code block came back, with its line breaks and its
	// indentation intact and no inline code-span mark
	var pasted []*model.Block
	for _, id := range target.Pick(target.RootId()).Model().ChildrenIds {
		b := target.Pick(id)
		require.NotNil(t, b)
		if txt := b.Model().GetText(); txt != nil && txt.Text != "" {
			pasted = append(pasted, b.Model())
		}
	}
	require.Len(t, pasted, 1, "expected a single pasted block, got:\n%+v", pasted)

	got := pasted[0].GetText()
	assert.Equal(t, model.BlockContentText_Code, got.Style,
		"pasted block must be a code block, got:\n%+v", pasted)
	assert.Equal(t, code, strings.TrimSuffix(got.Text, "\n"),
		"line breaks and indentation must survive the copy/paste round trip")
	assert.Empty(t, got.GetMarks().GetMarks(),
		"a code block must not come back as a paragraph carrying an inline code-span mark, got:\n%+v", pasted)
}
