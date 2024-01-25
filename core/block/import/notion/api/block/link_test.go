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
	importContext.PageTree.ParentPageToChildIDs = map[string][]string{"parentID": {"id"}}
	bl := c.GetLinkToObjectBlock(importContext, "parentID", "")
	assert.NotNil(t, bl)
	content, ok := bl.Content.(*model.BlockContentOfLink)
	assert.True(t, ok)
	assert.Equal(t, content.Link.TargetBlockId, "anytypeId")
	assert.Equal(t, content.Link.IconSize, model.BlockContentLink_SizeSmall)
}

func Test_GetLinkToObjectBlockTwoPagesWithSameName(t *testing.T) {
	c := &ChildPage{Title: "title"}
	importContext := api.NewNotionImportContext()
	importContext.PageNameToID = map[string]string{"id": "title", "id1": "title"}
	importContext.NotionPageIdsToAnytype = map[string]string{"id": "anytypeId", "id1": "anytypeId1"}
	importContext.PageTree.ParentPageToChildIDs = map[string][]string{"parentID": {"id"}}
	bl := c.GetLinkToObjectBlock(importContext, "parentID", "")
	assert.NotNil(t, bl)
	content, ok := bl.Content.(*model.BlockContentOfLink)
	assert.True(t, ok)
	assert.Equal(t, content.Link.TargetBlockId, "anytypeId")
	assert.Equal(t, content.Link.IconSize, model.BlockContentLink_SizeSmall)
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
	importContext.PageTree.ParentPageToChildIDs = map[string][]string{"parentID": {"id"}}
	bl := c.GetBlock(importContext, "parentID", "")
	assert.NotNil(t, bl)
	content, ok := bl.Content.(*model.BlockContentOfLink)
	assert.True(t, ok)
	assert.Equal(t, content.Link.TargetBlockId, "anytypeId")
	assert.Equal(t, content.Link.IconSize, model.BlockContentLink_SizeSmall)
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
	importContext.PageTree.ParentPageToChildIDs = map[string][]string{"parentID": {"id", "id1"}}
	bl := c.GetLinkToObjectBlock(importContext, "parentID", "")
	assert.NotNil(t, bl)
	content, ok := bl.Content.(*model.BlockContentOfLink)
	assert.True(t, ok)
	assert.Equal(t, content.Link.TargetBlockId, "anytypeId")
	assert.Equal(t, content.Link.IconSize, model.BlockContentLink_SizeSmall)

	bl = c.GetLinkToObjectBlock(importContext, "parentID", "")
	assert.NotNil(t, bl)
	content, ok = bl.Content.(*model.BlockContentOfLink)
	assert.True(t, ok)
	assert.Equal(t, content.Link.TargetBlockId, "anytypeId1")
	assert.Equal(t, content.Link.IconSize, model.BlockContentLink_SizeSmall)
}

func Test_GetLinkToObjectBlockPageWithTwoChildPagesWithSameNameNotFail(t *testing.T) {
	// because the object has an unique title
	c := &ChildPage{Title: "uniqueTitle"}
	importContext := api.NewNotionImportContext()
	importContext.PageNameToID = map[string]string{"id": "uniqueTitle"}
	importContext.NotionPageIdsToAnytype = map[string]string{"id": "anytypeId"}
	importContext.PageTree.ParentPageToChildIDs = map[string][]string{"parentID": {"id", "id1"}}
	bl := c.GetLinkToObjectBlock(importContext, "parentID", "")
	assert.NotNil(t, bl)
	content, ok := bl.Content.(*model.BlockContentOfLink)
	assert.True(t, ok)
	assert.Equal(t, content.Link.TargetBlockId, "anytypeId")
	assert.Equal(t, content.Link.IconSize, model.BlockContentLink_SizeSmall)

	bl = c.GetLinkToObjectBlock(importContext, "parentID", "")
	assert.NotNil(t, bl)
	assert.Equal(t, content.Link.TargetBlockId, "anytypeId")
	assert.Equal(t, content.Link.IconSize, model.BlockContentLink_SizeSmall)
}

func Test_GetLinkToObjectBlockPageWithTwoChildPagesWithSameNameFail(t *testing.T) {
	// because there is more than 1 object with the same title "title"
	c := &ChildPage{Title: "notUniqueTitle"}
	importContext := api.NewNotionImportContext()
	importContext.PageNameToID = map[string]string{"id": "notUniqueTitle", "id2": "notUniqueTitle"}
	importContext.NotionPageIdsToAnytype = map[string]string{"id": "anytypeId", "id2": "anytypeId2"}
	importContext.PageTree.ParentPageToChildIDs = map[string][]string{"parentID": {"id", "id1"}}
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
	importContext.PageTree.ParentPageToChildIDs = map[string][]string{"blockID": {"id"}}
	bl := c.GetLinkToObjectBlock(importContext, "parentID", "blockID")
	assert.NotNil(t, bl)
	content, ok := bl.Content.(*model.BlockContentOfLink)
	assert.True(t, ok)
	assert.Equal(t, content.Link.TargetBlockId, "anytypeId")
	assert.Equal(t, content.Link.IconSize, model.BlockContentLink_SizeSmall)
}

func Test_GetLinkToObjectBlockTwoPageHaveBlockParent(t *testing.T) {
	c := &ChildPage{Title: "title"}
	importContext := api.NewNotionImportContext()
	importContext.PageNameToID = map[string]string{"id": "title", "id1": "title"}
	importContext.NotionPageIdsToAnytype = map[string]string{"id": "anytypeId", "id1": "anytypeId1"}
	importContext.PageTree.ParentPageToChildIDs = map[string][]string{"blockID": {"id", "id1"}}
	bl := c.GetLinkToObjectBlock(importContext, "parentID", "blockID")
	assert.NotNil(t, bl)
	content, ok := bl.Content.(*model.BlockContentOfLink)
	assert.True(t, ok)
	assert.Equal(t, content.Link.TargetBlockId, "anytypeId")
	assert.Equal(t, content.Link.IconSize, model.BlockContentLink_SizeSmall)

	bl = c.GetLinkToObjectBlock(importContext, "parentID", "blockID")
	assert.NotNil(t, bl)
	content, ok = bl.Content.(*model.BlockContentOfLink)
	assert.True(t, ok)
	assert.Equal(t, content.Link.TargetBlockId, "anytypeId1")
	assert.Equal(t, content.Link.IconSize, model.BlockContentLink_SizeSmall)
}

func Test_EmbedBlockGetBlocks(t *testing.T) {
	t.Run("random url - we create link block", func(t *testing.T) {
		vo := &EmbedBlock{
			Embed: LinkToWeb{
				URL: "https://example.com/1",
			},
		}

		bl := vo.GetBlocks(nil, "")
		assert.NotNil(t, bl)
		assert.Len(t, bl.Blocks, 1)
		assert.NotNil(t, bl.Blocks[0].GetText())
	})
	t.Run("miro url - we create embed block", func(t *testing.T) {
		vo := &EmbedBlock{
			Embed: LinkToWeb{
				URL: "https://miro.com/app/board/a=/",
			},
		}

		bl := vo.GetBlocks(nil, "")
		assert.NotNil(t, bl)
		assert.Len(t, bl.Blocks, 1)
		assert.NotNil(t, bl.Blocks[0].GetLatex())
	})
	t.Run("soundcloud url - we create embed block", func(t *testing.T) {
		vo := &EmbedBlock{
			Embed: LinkToWeb{
				URL: "https://soundcloud.com/1",
			},
		}

		bl := vo.GetBlocks(nil, "")
		assert.NotNil(t, bl)
		assert.Len(t, bl.Blocks, 1)
		assert.NotNil(t, bl.Blocks[0].GetLatex())
	})
	t.Run("google maps url google.com/maps - we create embed block", func(t *testing.T) {
		vo := &EmbedBlock{
			Embed: LinkToWeb{
				URL: "https://www.google.com/maps",
			},
		}

		bl := vo.GetBlocks(nil, "")
		assert.NotNil(t, bl)
		assert.Len(t, bl.Blocks, 1)
		assert.NotNil(t, bl.Blocks[0].GetLatex())
	})
	t.Run("google maps url google.com/maps/Place - we create embed block", func(t *testing.T) {
		vo := &EmbedBlock{
			Embed: LinkToWeb{
				URL: "https://www.google.com/maps/place/Berliner+Fernsehturm/@52.5225829,13.4098161,16.79z",
			},
		}

		bl := vo.GetBlocks(nil, "")
		assert.NotNil(t, bl)
		assert.Len(t, bl.Blocks, 1)
		assert.NotNil(t, bl.Blocks[0].GetLatex())
	})
	t.Run("google maps url google.com/maps/coordinates - we create embed block", func(t *testing.T) {
		vo := &EmbedBlock{
			Embed: LinkToWeb{
				URL: "https://www.google.com/maps/@52.5225829,13.4098161,16.79z?entry=ttu",
			},
		}

		bl := vo.GetBlocks(nil, "")
		assert.NotNil(t, bl)
		assert.Len(t, bl.Blocks, 1)
		assert.NotNil(t, bl.Blocks[0].GetLatex())
	})

	t.Run("github gist url like github.gist.com/user/gist - we create embed block", func(t *testing.T) {
		vo := &EmbedBlock{
			Embed: LinkToWeb{
				URL: "https://gist.github.com/username/123456789abcdef",
			},
		}

		bl := vo.GetBlocks(nil, "")
		assert.NotNil(t, bl)
		assert.Len(t, bl.Blocks, 1)
		assert.NotNil(t, bl.Blocks[0].GetLatex())
		assert.Equal(t, model.BlockContentLatex_GithubGist, bl.Blocks[0].GetLatex().GetProcessor())
	})
	t.Run("github gist url like github.gist.com - not create embed block", func(t *testing.T) {
		vo := &EmbedBlock{
			Embed: LinkToWeb{
				URL: "https://gist.github.com/",
			},
		}

		bl := vo.GetBlocks(nil, "")
		assert.NotNil(t, bl)
		assert.Len(t, bl.Blocks, 1)
		assert.Nil(t, bl.Blocks[0].GetLatex())
	})
	t.Run("github gist url like github.gist.com/user - not create embed block", func(t *testing.T) {
		vo := &EmbedBlock{
			Embed: LinkToWeb{
				URL: "https://gist.github.com/",
			},
		}

		bl := vo.GetBlocks(nil, "")
		assert.NotNil(t, bl)
		assert.Len(t, bl.Blocks, 1)
		assert.Nil(t, bl.Blocks[0].GetLatex())
	})
	t.Run("codepen url like codepen.io - not create embed block", func(t *testing.T) {
		vo := &EmbedBlock{
			Embed: LinkToWeb{
				URL: "https://codepen.io/",
			},
		}
		bl := vo.GetBlocks(nil, "")
		assert.NotNil(t, bl)
		assert.Len(t, bl.Blocks, 1)
		assert.Nil(t, bl.Blocks[0].GetLatex())
	})
	t.Run("codepen url like codepen.io/user/pen/id - we create embed block", func(t *testing.T) {
		vo := &EmbedBlock{
			Embed: LinkToWeb{
				URL: "https://codepen.io/user/pen/id",
			},
		}
		bl := vo.GetBlocks(nil, "")
		assert.NotNil(t, bl)
		assert.Len(t, bl.Blocks, 1)
		assert.NotNil(t, bl.Blocks[0].GetLatex())
		assert.Equal(t, model.BlockContentLatex_Codepen, bl.Blocks[0].GetLatex().GetProcessor())
	})
	t.Run("codepen url like codepen.io/user/details/id - we create embed block", func(t *testing.T) {
		vo := &EmbedBlock{
			Embed: LinkToWeb{
				URL: "https://codepen.io/user/details/id",
			},
		}
		bl := vo.GetBlocks(nil, "")
		assert.NotNil(t, bl)
		assert.Len(t, bl.Blocks, 1)
		assert.NotNil(t, bl.Blocks[0].GetLatex())
		assert.Equal(t, model.BlockContentLatex_Codepen, bl.Blocks[0].GetLatex().GetProcessor())
	})
	t.Run("codepen url like codepen.io/user/details/id/edit - we create embed block", func(t *testing.T) {
		vo := &EmbedBlock{
			Embed: LinkToWeb{
				URL: "https://codepen.io/user/details/id/edit",
			},
		}
		bl := vo.GetBlocks(nil, "")
		assert.NotNil(t, bl)
		assert.Len(t, bl.Blocks, 1)
		assert.NotNil(t, bl.Blocks[0].GetLatex())
		assert.Equal(t, model.BlockContentLatex_Codepen, bl.Blocks[0].GetLatex().GetProcessor())
	})
}
