package block

import (
	"strings"
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
	importContext := api.NewNotionImportContext()
	importContext.PageNameToID = map[string]string{"id": "title"}
	importContext.NotionPageIdsToAnytype = map[string]string{"id": "anytypeId"}
	importContext.ParentPageToChildIDs = map[string][]string{"parentID": {"id"}}
	bl := c.GetLinkToObjectBlock(importContext, "parentID", "")
	assert.NotNil(t, bl)
	content, ok := bl.Content.(*model.BlockContentOfLink)
	assert.True(t, ok)
	assert.Equal(t, content.Link.TargetBlockId, "anytypeId")
}

func Test_GetLinkToObjectBlockTwoPagesWithSameName(t *testing.T) {
	c := &ChildPage{Title: "title"}
	importContext := api.NewNotionImportContext()
	importContext.PageNameToID = map[string]string{"id": "title", "id1": "title"}
	importContext.NotionPageIdsToAnytype = map[string]string{"id": "anytypeId", "id1": "anytypeId1"}
	importContext.ParentPageToChildIDs = map[string][]string{"parentID": {"id"}}
	bl := c.GetLinkToObjectBlock(importContext, "parentID", "")
	assert.NotNil(t, bl)
	content, ok := bl.Content.(*model.BlockContentOfLink)
	assert.True(t, ok)
	assert.Equal(t, content.Link.TargetBlockId, "anytypeId")
}

func Test_GetLinkToObjectBlockFail(t *testing.T) {
	c := &ChildPage{Title: "onetitle"}
	bl := c.GetLinkToObjectBlock(api.NewNotionImportContext(), "id", "")
	assert.NotNil(t, bl)
	content, ok := bl.Content.(*model.BlockContentOfText)
	assert.True(t, ok)
	assert.True(t, strings.HasPrefix(content.Text.Text, pageNotFoundMessage))

	importContext := api.NewNotionImportContext()
	importContext.PageNameToID = map[string]string{"id": "anothertitle"}
	bl = c.GetLinkToObjectBlock(importContext, "pageID", "")
	assert.NotNil(t, bl)
	content, ok = bl.Content.(*model.BlockContentOfText)
	assert.True(t, ok)
	assert.True(t, strings.HasPrefix(content.Text.Text, pageNotFoundMessage))
}

func Test_GetLinkToObjectBlockLinkToDatabase(t *testing.T) {
	c := &ChildDatabase{Title: "title"}
	importContext := api.NewNotionImportContext()
	importContext.DatabaseNameToID = map[string]string{"id": "title"}
	importContext.NotionDatabaseIdsToAnytype = map[string]string{"id": "anytypeId"}
	importContext.ParentPageToChildIDs = map[string][]string{"parentID": {"id"}}
	bl := c.GetBlock(importContext, "parentID", "")
	assert.NotNil(t, bl)
	content, ok := bl.Content.(*model.BlockContentOfLink)
	assert.True(t, ok)
	assert.Equal(t, content.Link.TargetBlockId, "anytypeId")
}

func Test_GetLinkToObjectBlockInlineCollection(t *testing.T) {
	c := &ChildDatabase{Title: "title"}
	importContext := api.NewNotionImportContext()
	bl := c.GetBlock(importContext, "parentID", "")
	assert.NotNil(t, bl)
	content, ok := bl.Content.(*model.BlockContentOfDataview)
	assert.True(t, ok)
	assert.Equal(t, content.Dataview.TargetObjectId, "")
}

func Test_GetLinkToObjectBlockInlineCollectionEmpty(t *testing.T) {
	c := &ChildDatabase{Title: "title"}
	bl := c.GetBlock(api.NewNotionImportContext(), "", "")
	assert.NotNil(t, bl)
	content, ok := bl.Content.(*model.BlockContentOfDataview)
	assert.True(t, ok)
	assert.Equal(t, content.Dataview.TargetObjectId, "")
}

func Test_GetLinkToObjectBlockPageWithTwoChildPagesWithSameName(t *testing.T) {
	c := &ChildPage{Title: "title"}
	importContext := api.NewNotionImportContext()
	importContext.PageNameToID = map[string]string{"id": "title", "id1": "title"}
	importContext.NotionPageIdsToAnytype = map[string]string{"id": "anytypeId", "id1": "anytypeId1"}
	importContext.ParentPageToChildIDs = map[string][]string{"parentID": {"id", "id1"}}
	bl := c.GetLinkToObjectBlock(importContext, "parentID", "")
	assert.NotNil(t, bl)
	content, ok := bl.Content.(*model.BlockContentOfLink)
	assert.True(t, ok)
	assert.Equal(t, content.Link.TargetBlockId, "anytypeId")

	bl = c.GetLinkToObjectBlock(importContext, "parentID", "")
	assert.NotNil(t, bl)
	content, ok = bl.Content.(*model.BlockContentOfLink)
	assert.True(t, ok)
	assert.Equal(t, content.Link.TargetBlockId, "anytypeId1")
}

func Test_GetLinkToObjectBlockPageWithTwoChildPagesWithSameNameNotFail(t *testing.T) {
	// because the object has an unique title
	c := &ChildPage{Title: "uniqueTitle"}
	importContext := api.NewNotionImportContext()
	importContext.PageNameToID = map[string]string{"id": "uniqueTitle"}
	importContext.NotionPageIdsToAnytype = map[string]string{"id": "anytypeId"}
	importContext.ParentPageToChildIDs = map[string][]string{"parentID": {"id", "id1"}}
	bl := c.GetLinkToObjectBlock(importContext, "parentID", "")
	assert.NotNil(t, bl)
	content, ok := bl.Content.(*model.BlockContentOfLink)
	assert.True(t, ok)
	assert.Equal(t, content.Link.TargetBlockId, "anytypeId")

	bl = c.GetLinkToObjectBlock(importContext, "parentID", "")
	assert.NotNil(t, bl)
	assert.Equal(t, content.Link.TargetBlockId, "anytypeId")
}

func Test_GetLinkToObjectBlockPageWithTwoChildPagesWithSameNameFail(t *testing.T) {
	// because there is more than 1 object with the same title "title"
	c := &ChildPage{Title: "notUniqueTitle"}
	importContext := api.NewNotionImportContext()
	importContext.PageNameToID = map[string]string{"id": "notUniqueTitle", "id2": "notUniqueTitle"}
	importContext.NotionPageIdsToAnytype = map[string]string{"id": "anytypeId", "id2": "anytypeId2"}
	importContext.ParentPageToChildIDs = map[string][]string{"parentID": {"id", "id1"}}
	bl := c.GetLinkToObjectBlock(importContext, "parentID", "")
	assert.NotNil(t, bl)
	content, ok := bl.Content.(*model.BlockContentOfLink)
	assert.True(t, ok)
	assert.Equal(t, content.Link.TargetBlockId, "anytypeId")

	bl = c.GetLinkToObjectBlock(importContext, "parentID", "")
	assert.NotNil(t, bl)
	textContent, ok := bl.Content.(*model.BlockContentOfText)
	assert.True(t, ok)
	assert.True(t, strings.HasPrefix(textContent.Text.Text, ambiguousPageMessage))
}

func Test_GetLinkToObjectBlockPageHasBlockParent(t *testing.T) {
	c := &ChildPage{Title: "title"}
	importContext := api.NewNotionImportContext()
	importContext.PageNameToID = map[string]string{"id": "title"}
	importContext.NotionPageIdsToAnytype = map[string]string{"id": "anytypeId"}
	importContext.ParentPageToChildIDs = map[string][]string{"blockID": {"id"}}
	bl := c.GetLinkToObjectBlock(importContext, "parentID", "blockID")
	assert.NotNil(t, bl)
	content, ok := bl.Content.(*model.BlockContentOfLink)
	assert.True(t, ok)
	assert.Equal(t, content.Link.TargetBlockId, "anytypeId")
}

func Test_GetLinkToObjectBlockTwoPageHaveBlockParent(t *testing.T) {
	c := &ChildPage{Title: "title"}
	importContext := api.NewNotionImportContext()
	importContext.PageNameToID = map[string]string{"id": "title", "id1": "title"}
	importContext.NotionPageIdsToAnytype = map[string]string{"id": "anytypeId", "id1": "anytypeId1"}
	importContext.ParentPageToChildIDs = map[string][]string{"blockID": {"id", "id1"}}
	bl := c.GetLinkToObjectBlock(importContext, "parentID", "blockID")
	assert.NotNil(t, bl)
	content, ok := bl.Content.(*model.BlockContentOfLink)
	assert.True(t, ok)
	assert.Equal(t, content.Link.TargetBlockId, "anytypeId")

	bl = c.GetLinkToObjectBlock(importContext, "parentID", "blockID")
	assert.NotNil(t, bl)
	content, ok = bl.Content.(*model.BlockContentOfLink)
	assert.True(t, ok)
	assert.Equal(t, content.Link.TargetBlockId, "anytypeId1")
}
