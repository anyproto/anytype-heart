package block

import (
	"strings"

	"github.com/globalsign/mgo/bson"

	"github.com/anytypeio/go-anytype-middleware/core/block/collection"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	textUtil "github.com/anytypeio/go-anytype-middleware/util/text"
)

const notFoundPageMessage = "Can't access object in Notion, please provide access in API"

type EmbedBlock struct {
	Block
	Embed LinkToWeb `json:"embed"`
}

func (b *EmbedBlock) GetBlocks(req *MapRequest) *MapResponse {
	return b.Embed.GetBlocks(req)
}

type LinkToWeb struct {
	URL string `json:"url"`
}

type LinkPreviewBlock struct {
	Block
	LinkPreview LinkToWeb `json:"link_preview"`
}

func (b *LinkPreviewBlock) GetBlocks(req *MapRequest) *MapResponse {
	return b.LinkPreview.GetBlocks(req)
}

func (b *LinkToWeb) GetBlocks(*MapRequest) *MapResponse {
	id := bson.NewObjectId().Hex()

	to := textUtil.UTF16RuneCountString(b.URL)

	bl := &model.Block{
		Id:          id,
		ChildrenIds: nil,
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text: b.URL,
				Marks: &model.BlockContentTextMarks{
					Marks: []*model.BlockContentTextMark{
						{
							Range: &model.Range{
								From: int32(0),
								To:   int32(to),
							},
							Type:  model.BlockContentTextMark_Link,
							Param: b.URL,
						},
					},
				},
			},
		},
	}
	return &MapResponse{
		Blocks:   []*model.Block{bl},
		BlockIDs: []string{id},
	}
}

type ChildPageBlock struct {
	Block
	ChildPage ChildPage `json:"child_page"`
}

type ChildPage struct {
	Title string `json:"title"`
}

func (b *ChildPageBlock) GetBlocks(req *MapRequest) *MapResponse {
	bl := b.ChildPage.GetLinkToObjectBlock(req.NotionPageIdsToAnytype, req.PageNameToID)
	return &MapResponse{
		Blocks:   []*model.Block{bl},
		BlockIDs: []string{bl.Id},
	}
}

func (p ChildPage) GetLinkToObjectBlock(notionIdsToAnytype, idToName map[string]string) *model.Block {
	var (
		targetBlockID string
		ok            bool
	)
	for id, name := range idToName {
		if strings.EqualFold(name, p.Title) {
			targetBlockID, ok = notionIdsToAnytype[id]
			break
		}
	}

	id := bson.NewObjectId().Hex()
	if !ok {
		return &model.Block{
			Id:          id,
			ChildrenIds: nil,
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: notFoundPageMessage,
					Marks: &model.BlockContentTextMarks{
						Marks: []*model.BlockContentTextMark{},
					},
				},
			},
		}
	}

	return &model.Block{
		Id:          id,
		ChildrenIds: nil,
		Content: &model.BlockContentOfLink{
			Link: &model.BlockContentLink{
				TargetBlockId: targetBlockID,
			},
		}}
}

type ChildDatabaseBlock struct {
	Block
	ChildDatabase ChildDatabase `json:"child_database"`
}

func (b *ChildDatabaseBlock) GetBlocks(req *MapRequest) *MapResponse {
	bl := b.ChildDatabase.GetDataviewBlock(req.NotionDatabaseIdsToAnytype, req.DatabaseNameToID)
	return &MapResponse{
		Blocks:   []*model.Block{bl},
		BlockIDs: []string{bl.Id},
	}
}

type ChildDatabase struct {
	Title string `json:"title"`
}

func (c *ChildDatabase) GetDataviewBlock(notionIdsToAnytype, idToName map[string]string) *model.Block {
	var (
		targetBlockID string
	)
	for id, name := range idToName {
		if strings.EqualFold(name, c.Title) {
			if len(notionIdsToAnytype) > 0 {
				targetBlockID = notionIdsToAnytype[id]
			}
			break
		}
	}

	id := bson.NewObjectId().Hex()

	block := collection.MakeDataviewContent()

	block.Dataview.TargetObjectId = targetBlockID

	return &model.Block{
		Id:          id,
		ChildrenIds: nil,
		Content:     block,
	}
}

type LinkToPageBlock struct {
	Block
	LinkToPage api.Parent `json:"link_to_page"`
}

func (l *LinkToPageBlock) GetBlocks(req *MapRequest) *MapResponse {
	var anytypeID string
	if l.LinkToPage.PageID != "" {
		anytypeID = req.NotionPageIdsToAnytype[l.LinkToPage.PageID]
	}
	if l.LinkToPage.DatabaseID != "" {
		anytypeID = req.NotionDatabaseIdsToAnytype[l.LinkToPage.DatabaseID]
	}

	id := bson.NewObjectId().Hex()
	bl := &model.Block{
		Id:          id,
		ChildrenIds: nil,
		Content: &model.BlockContentOfLink{
			Link: &model.BlockContentLink{
				TargetBlockId: anytypeID,
			},
		}}
	// notion page has link to page/database which isn't added to notion integration,
	// so we don't create anytype page for it
	if anytypeID == "" {
		bl = &model.Block{
			Id:          id,
			ChildrenIds: nil,
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: notFoundPageMessage,
					Marks: &model.BlockContentTextMarks{
						Marks: []*model.BlockContentTextMark{},
					},
				},
			},
		}
	}
	return &MapResponse{
		Blocks:   []*model.Block{bl},
		BlockIDs: []string{id},
	}
}

type BookmarkBlock struct {
	Block
	Bookmark BookmarkObject `json:"bookmark"`
}

func (b *BookmarkBlock) GetBlocks(*MapRequest) *MapResponse {
	bl, id := b.Bookmark.GetBookmarkBlock()
	return &MapResponse{
		Blocks:   []*model.Block{bl},
		BlockIDs: []string{id},
	}
}

type BookmarkObject struct {
	URL     string          `json:"url"`
	Caption []*api.RichText `json:"caption"`
}

func (b BookmarkObject) GetBookmarkBlock() (*model.Block, string) {
	id := bson.NewObjectId().Hex()
	title := api.RichTextToDescription(b.Caption)

	return &model.Block{
		Id:          id,
		ChildrenIds: []string{},
		Content: &model.BlockContentOfBookmark{
			Bookmark: &model.BlockContentBookmark{
				Url:   b.URL,
				Title: title,
			},
		}}, id
}
