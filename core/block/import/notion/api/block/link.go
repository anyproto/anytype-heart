package block

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/globalsign/mgo/bson"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	textUtil "github.com/anyproto/anytype-heart/util/text"
)

const (
	ambiguousPageMessage  = "ambiguous page"   // more than one page with the same title without direct parent
	pageNotFoundMessage   = "page not found"   // can't find notion page by title
	objectNotFoundMessage = "object not found" // can't find anytypeId for notion page
)

type EmbedBlock struct {
	Block
	Embed LinkToWeb `json:"embed"`
}

func (b *EmbedBlock) GetBlocks(req *NotionImportContext, _ string) *MapResponse {
	return b.Embed.GetBlocks(req, "")
}

type LinkToWeb struct {
	URL string `json:"url"`
}

type LinkPreviewBlock struct {
	Block
	LinkPreview LinkToWeb `json:"link_preview"`
}

func (b *LinkPreviewBlock) GetBlocks(req *NotionImportContext, _ string) *MapResponse {
	return b.LinkPreview.GetBlocks(req, "")
}

func (b *LinkToWeb) GetBlocks(*NotionImportContext, string) *MapResponse {
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

func (b *ChildPageBlock) GetBlocks(req *NotionImportContext, pageID string) *MapResponse {
	bl := b.ChildPage.GetLinkToObjectBlock(req, pageID)
	return &MapResponse{
		Blocks:   []*model.Block{bl},
		BlockIDs: []string{bl.Id},
	}
}

func (p ChildPage) GetLinkToObjectBlock(importContext *NotionImportContext, pageID string) *model.Block {
	targetBlockID, err := getTargetBlock(importContext.ParentPageToChildIDs, importContext.PageNameToID, importContext.NotionPageIdsToAnytype, pageID, p.Title)

	id := bson.NewObjectId().Hex()
	if err != nil {
		return &model.Block{
			Id:          id,
			ChildrenIds: nil,
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: err.Error(),
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

func (b *ChildDatabaseBlock) GetBlocks(req *NotionImportContext, pageID string) *MapResponse {
	bl := b.ChildDatabase.GetBlock(req, pageID)
	return &MapResponse{
		Blocks:   []*model.Block{bl},
		BlockIDs: []string{bl.Id},
	}
}

type ChildDatabase struct {
	Title string `json:"title"`
}

func (c *ChildDatabase) GetBlock(importContext *NotionImportContext, pageID string) *model.Block {
	targetBlockID, _ := getTargetBlock(importContext.ParentPageToChildIDs,
		importContext.DatabaseNameToID,
		importContext.NotionDatabaseIdsToAnytype,
		pageID, c.Title)

	id := bson.NewObjectId().Hex()
	if targetBlockID == "" {
		block := template.MakeCollectionDataviewContent()
		block.Dataview.TargetObjectId = targetBlockID
		return &model.Block{
			Id:          id,
			ChildrenIds: nil,
			Content:     block,
		}
	}
	return &model.Block{
		Id:          id,
		ChildrenIds: nil,
		Content: &model.BlockContentOfLink{
			Link: &model.BlockContentLink{
				TargetBlockId: targetBlockID,
			},
		},
	}
}

type LinkToPageBlock struct {
	Block
	LinkToPage api.Parent `json:"link_to_page"`
}

func (l *LinkToPageBlock) GetBlocks(req *NotionImportContext, _ string) *MapResponse {
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

func (b *BookmarkBlock) GetBlocks(*NotionImportContext, string) *MapResponse {
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

// getTargetBlock has the logic, that each page in Notion can have many child pages inside, which can also have the same name.
// But the Notion API sends us only the names of these pages. Therefore, as a result, we can get an approximate answers like
//
//	parentPage {
//	      childPage: “Title”,
//	      childPage: “Title”,
//	      childPage: "Title"
//	}
//
// And these pages are different. Therefore, we get children of given page and compare its name with those
// returned to us by the Notion API. As a result we get Anytype ID of child page and make it targetBlockID
// But we can end up with 3 links to the same page, and to avoid that,
// we remove the childID from the parentIDToChildrenID map. So it helps to not create links with the same targetBlockID.
func getTargetBlock(parentPageIDToChildIDs map[string][]string, pageIDToName, notionIDsToAnytype map[string]string, pageID, title string) (string, error) {
	var (
		targetBlockID string
		ok            bool
	)

	findByPageAndTitle := func(pageID, title string) string {
		if childIDs, exist := parentPageIDToChildIDs[pageID]; exist {
			for childIdx, childID := range childIDs {
				if pageName, pageExist := pageIDToName[childID]; pageExist && pageName == title {
					if targetBlockID, ok = notionIDsToAnytype[childID]; !ok {
						return ""
					}

					childIDs = slices.Delete(childIDs, childIdx, childIdx+1)
					break
				}
			}
			parentPageIDToChildIDs[pageID] = childIDs
		}

		return targetBlockID
	}
	// first, try to find it in the list of child of the current page
	targetBlockID = findByPageAndTitle(pageID, title)
	if targetBlockID != "" {
		return targetBlockID, nil
	}

	// then, try to find it in the list of pages without direct parent
	targetBlockID = findByPageAndTitle("", title)
	if targetBlockID != "" {
		return targetBlockID, nil
	}

	// fallback to just match by title
	var idsWithGivenName []string
	for id, name := range pageIDToName {
		if strings.EqualFold(name, title) {
			idsWithGivenName = append(idsWithGivenName, id)
		}
	}

	var err error
	if len(idsWithGivenName) == 1 {
		id := idsWithGivenName[0]
		if targetBlockID, ok = notionIDsToAnytype[id]; ok {
			return targetBlockID, nil
		} else {
			err = fmt.Errorf("%s '%s'", objectNotFoundMessage, title)
			logger.With("notionID", hashText(id)).With("title", hashText(title)).Errorf("getTargetBlock: anytype id not found")
		}
	} else if len(idsWithGivenName) > 1 {
		err = fmt.Errorf("%s '%s'", ambiguousPageMessage, title)
		logger.With("title", hashText(title)).With("options", len(idsWithGivenName)).Warnf("getTargetBlock: ambligious page title")
	} else {
		err = fmt.Errorf("%s '%s'", pageNotFoundMessage, title)
		logger.With("title", hashText(title)).Errorf("getTargetBlock: target not found")
	}

	return targetBlockID, err
}

func hashText(text string) string {
	hash := md5.Sum([]byte(text))
	return hex.EncodeToString(hash[:])
}
