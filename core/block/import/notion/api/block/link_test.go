package block

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/import/notion/api"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func Test_GetBookmarkBlock(t *testing.T) {
	bo := &BookmarkObject{
		URL:     "",
		Caption: []*api.RichText{},
	}
	bb, _ := bo.GetBookmarkBlock()
	assert.NotNil(t, bb)
	assert.Equal(t, bb.GetBookmark().GetUrl(), bo.URL)
	assert.Equal(t, bb.GetBookmark().GetTitle(), "")

	bo = &BookmarkObject{
		URL:     "http://example.com",
		Caption: []*api.RichText{},
	}
	bb, _ = bo.GetBookmarkBlock()
	assert.NotNil(t, bb)
	assert.Equal(t, bb.GetBookmark().GetUrl(), bo.URL)
	assert.Equal(t, bb.GetBookmark().GetTitle(), "")

	bo = &BookmarkObject{
		URL: "",
		Caption: []*api.RichText{{
			Type:      api.Text,
			PlainText: "Text",
		}},
	}
	bb, _ = bo.GetBookmarkBlock()
	assert.NotNil(t, bb)
	assert.Equal(t, bb.GetBookmark().GetUrl(), bo.URL)
	assert.Equal(t, bb.GetBookmark().GetTitle(), "Text")
}

func Test_GetLinkToObjectBlockSuccess(t *testing.T) {
	c := &ChildPage{Title: "title"}
	importContext := NewNotionImportContext()
	importContext.PageNameToID = map[string]string{"id": "title"}
	importContext.NotionPageIdsToAnytype = map[string]string{"id": "anytypeId"}
	importContext.ParentPageToChildIDs = map[string][]string{"parentID": {"id"}}
	bl := c.GetLinkToObjectBlock(importContext, "parentID")
	assert.NotNil(t, bl)
	content, ok := bl.Content.(*model.BlockContentOfLink)
	assert.True(t, ok)
	assert.Equal(t, content.Link.TargetBlockId, "anytypeId")
}

func Test_GetLinkToObjectBlockTwoPagesWithSameName(t *testing.T) {
	c := &ChildPage{Title: "title"}
	importContext := NewNotionImportContext()
	importContext.PageNameToID = map[string]string{"id": "title", "id1": "title"}
	importContext.NotionPageIdsToAnytype = map[string]string{"id": "anytypeId", "id1": "anytypeId1"}
	importContext.ParentPageToChildIDs = map[string][]string{"parentID": {"id"}}
	bl := c.GetLinkToObjectBlock(importContext, "parentID")
	assert.NotNil(t, bl)
	content, ok := bl.Content.(*model.BlockContentOfLink)
	assert.True(t, ok)
	assert.Equal(t, content.Link.TargetBlockId, "anytypeId")
}

func Test_GetLinkToObjectBlockFail(t *testing.T) {
	c := &ChildPage{Title: "title"}
	bl := c.GetLinkToObjectBlock(NewNotionImportContext(), "id")
	assert.NotNil(t, bl)
	content, ok := bl.Content.(*model.BlockContentOfText)
	assert.True(t, ok)
	assert.Equal(t, content.Text.Text, notFoundPageMessage)

	importContext := NewNotionImportContext()
	importContext.PageNameToID = map[string]string{"id": "title"}
	bl = c.GetLinkToObjectBlock(importContext, "pageID")
	assert.NotNil(t, bl)
	content, ok = bl.Content.(*model.BlockContentOfText)
	assert.True(t, ok)
	assert.Equal(t, content.Text.Text, notFoundPageMessage)
}

func Test_GetLinkToObjectBlockInlineCollection(t *testing.T) {
	c := &ChildDatabase{Title: "title"}
	importContext := NewNotionImportContext()
	importContext.DatabaseNameToID = map[string]string{"id": "title"}
	importContext.NotionDatabaseIdsToAnytype = map[string]string{"id": "anytypeId"}
	importContext.ParentPageToChildIDs = map[string][]string{"parentID": {"id"}}
	bl := c.GetDataviewBlock(importContext, "parentID")
	assert.NotNil(t, bl)
	content, ok := bl.Content.(*model.BlockContentOfDataview)
	assert.True(t, ok)
	assert.Equal(t, content.Dataview.TargetObjectId, "anytypeId")
}

func Test_GetLinkToObjectBlockInlineCollectionEmpty(t *testing.T) {
	c := &ChildDatabase{Title: "title"}
	bl := c.GetDataviewBlock(NewNotionImportContext(), "")
	assert.NotNil(t, bl)
	content, ok := bl.Content.(*model.BlockContentOfDataview)
	assert.True(t, ok)
	assert.Equal(t, content.Dataview.TargetObjectId, "")
}

func Test_GetLinkToObjectBlockPageWithTwoChildPagesWithSameName(t *testing.T) {
	c := &ChildPage{Title: "title"}
	importContext := NewNotionImportContext()
	importContext.PageNameToID = map[string]string{"id": "title", "id1": "title"}
	importContext.NotionPageIdsToAnytype = map[string]string{"id": "anytypeId", "id1": "anytypeId1"}
	importContext.ParentPageToChildIDs = map[string][]string{"parentID": {"id", "id1"}}
	bl := c.GetLinkToObjectBlock(importContext, "parentID")
	assert.NotNil(t, bl)
	content, ok := bl.Content.(*model.BlockContentOfLink)
	assert.True(t, ok)
	assert.Equal(t, content.Link.TargetBlockId, "anytypeId")

	bl = c.GetLinkToObjectBlock(importContext, "parentID")
	assert.NotNil(t, bl)
	content, ok = bl.Content.(*model.BlockContentOfLink)
	assert.True(t, ok)
	assert.Equal(t, content.Link.TargetBlockId, "anytypeId1")
}

func Test_GetLinkToObjectBlockPageWithTwoChildPagesWithSameNameFail(t *testing.T) {
	c := &ChildPage{Title: "title"}
	importContext := NewNotionImportContext()
	importContext.PageNameToID = map[string]string{"id": "title"}
	importContext.NotionPageIdsToAnytype = map[string]string{"id": "anytypeId"}
	importContext.ParentPageToChildIDs = map[string][]string{"parentID": {"id", "id1"}}
	bl := c.GetLinkToObjectBlock(importContext, "parentID")
	assert.NotNil(t, bl)
	content, ok := bl.Content.(*model.BlockContentOfLink)
	assert.True(t, ok)
	assert.Equal(t, content.Link.TargetBlockId, "anytypeId")

	bl = c.GetLinkToObjectBlock(importContext, "parentID")
	assert.NotNil(t, bl)
	textContent, ok := bl.Content.(*model.BlockContentOfText)
	assert.True(t, ok)
	assert.Equal(t, notFoundPageMessage, textContent.Text.Text)
}
