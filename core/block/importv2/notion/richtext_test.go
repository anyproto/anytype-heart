package notion

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const knownHex = "c7e313a11e14834ea5c701d1426f9999"
const knownId = "c7e313a1-1e14-834e-a5c7-01d1426f9999"

func newLinkFixture() *Converter {
	c := New(nil, nil, stubFactory{}, "")
	c.entityById[knownId] = Entity{Id: knownId}
	return c
}

func TestEntityIdFromUrl(t *testing.T) {
	c := newLinkFixture()

	t.Run("recognized shapes resolve to the imported entity", func(t *testing.T) {
		// The 2025-09-03 API emits app.notion.com mention hrefs and RELATIVE
		// /p/ text-run links; legacy notion.so still occurs in stored URLs.
		for _, rawUrl := range []string{
			"https://www.notion.so/Some-Page-" + knownHex,
			"https://app.notion.com/p/" + knownHex,
			"/p/" + knownHex,
			"/p/" + knownHex + "?pvs=4",
		} {
			id, ok := c.entityIdFromUrl(rawUrl)
			assert.True(t, ok, "url %q must resolve", rawUrl)
			assert.Equal(t, knownId, id)
		}
	})

	t.Run("unknown targets and foreign hosts do not resolve", func(t *testing.T) {
		for _, rawUrl := range []string{
			"/p/ffffffffffffffffffffffffffffffff",
			"https://app.notion.com/p/ffffffffffffffffffffffffffffffff",
			"https://example.com/" + knownHex,
			"https://example.com/page",
		} {
			_, ok := c.entityIdFromUrl(rawUrl)
			assert.False(t, ok, "url %q must not resolve", rawUrl)
		}
	})
}

func TestLinkTarget(t *testing.T) {
	c := newLinkFixture()
	runWithLink := func(href string) richText {
		return richText{PlainText: "x", Text: &struct {
			Content string `json:"content"`
			Link    *struct {
				Url string `json:"url"`
			} `json:"link"`
		}{Link: &struct {
			Url string `json:"url"`
		}{Url: href}}}
	}

	t.Run("intra-workspace link becomes a mention", func(t *testing.T) {
		mark := c.linkTarget(runWithLink("/p/" + knownHex))
		require.NotNil(t, mark)
		assert.Equal(t, model.BlockContentTextMark_Mention, mark.Type)
		assert.Equal(t, knownId, mark.Param)
	})

	t.Run("unresolved relative link is absolutized", func(t *testing.T) {
		mark := c.linkTarget(runWithLink("/p/ffffffffffffffffffffffffffffffff"))
		require.NotNil(t, mark)
		assert.Equal(t, model.BlockContentTextMark_Link, mark.Type)
		assert.Equal(t, "https://app.notion.com/p/ffffffffffffffffffffffffffffffff", mark.Param)
	})

	t.Run("foreign link stays a web link", func(t *testing.T) {
		mark := c.linkTarget(runWithLink("https://example.com/doc"))
		require.NotNil(t, mark)
		assert.Equal(t, model.BlockContentTextMark_Link, mark.Type)
		assert.Equal(t, "https://example.com/doc", mark.Param)
	})
}

func TestRenderRichTextInlinesEquations(t *testing.T) {
	c := newLinkFixture()
	runs := []richText{
		{PlainText: "energy: ", Type: "text"},
		{Type: "equation", PlainText: "E=mc^2", Equation: &struct {
			Expression string `json:"expression"`
		}{Expression: "E=mc^2"}},
		{PlainText: " done", Type: "text", Annotations: &annotations{Bold: true}},
	}

	rendered := c.renderRichText(runs)

	assert.Equal(t, "energy: E=mc^2 done", rendered.text,
		"equation expressions stay in the flow where no latex sibling can exist")
	require.Len(t, rendered.marks, 1)
	assert.Equal(t, model.BlockContentTextMark_Bold, rendered.marks[0].Type)
	assert.Equal(t, &model.Range{From: 14, To: 19}, rendered.marks[0].Range,
		"mark offsets must account for the inlined expression")
}

func TestMentionCarriesNoDuplicateLinkMark(t *testing.T) {
	c := newLinkFixture()
	pieces := c.renderRichTextPieces([]richText{{
		PlainText: "Beta",
		Type:      "mention",
		Href:      "https://app.notion.com/p/" + knownHex,
		Mention:   &mention{Type: "page", Page: &struct {
			Id string `json:"id"`
		}{Id: knownId}},
	}})

	require.Len(t, pieces, 1)
	require.Len(t, pieces[0].marks, 1, "exactly one mark: the mention itself")
	assert.Equal(t, model.BlockContentTextMark_Mention, pieces[0].marks[0].Type)
	assert.Equal(t, knownId, pieces[0].marks[0].Param)
}
