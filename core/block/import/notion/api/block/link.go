package block

import (
	"strings"

	"github.com/globalsign/mgo/bson"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	textUtil "github.com/anytypeio/go-anytype-middleware/util/text"
)

const notFoundPageMessage = "Can't access object in Notion, please provide access in API"

type EmbedBlock struct {
	Block
	Embed LinkToWeb `json:"embed"`
}

type LinkToWeb struct {
	URL string `json:"url"`
}

type LinkPreviewBlock struct {
	Block
	LinkPreview LinkToWeb `json:"link_preview"`
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
	ChildPage Child `json:"child_page"`
}

func (b *ChildPageBlock) GetBlocks(req *MapRequest) *MapResponse {
	bl, id := b.ChildPage.GetLinkToObjectBlock(req.NotionPageIdsToAnytype, req.PageNameToID)
	return &MapResponse{
		Blocks:   []*model.Block{bl},
		BlockIDs: []string{id},
	}
}

type ChildDatabaseBlock struct {
	Block
	ChildDatabase Child `json:"child_database"`
}

func (b *ChildDatabaseBlock) GetBlocks(req *MapRequest) *MapResponse {
	bl, id := b.ChildDatabase.GetLinkToObjectBlock(req.NotionDatabaseIdsToAnytype, req.DatabaseNameToID)
	return &MapResponse{
		Blocks:   []*model.Block{bl},
		BlockIDs: []string{id},
	}
}

type Child struct {
	Title string `json:"title"`
}

func (c *Child) GetLinkToObjectBlock(notionIdsToAnytype, idToName map[string]string) (*model.Block, string) {
	var (
		targetBlockID string
		ok            bool
	)
	for id, name := range idToName {
		if strings.EqualFold(name, c.Title) {
			if len(notionIdsToAnytype) > 0 {
				targetBlockID, ok = notionIdsToAnytype[id]
			}
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
		}, id
	}

	return &model.Block{
		Id:          id,
		ChildrenIds: nil,
		Content: &model.BlockContentOfLink{
			Link: &model.BlockContentLink{
				TargetBlockId: targetBlockID,
			},
		}}, id
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
	return &MapResponse{
		Blocks:   []*model.Block{bl},
		BlockIDs: []string{id},
	}
}

type BookmarkBlock struct {
	Block
	Bookmark BookmarkObject `json:"bookmark"`
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
