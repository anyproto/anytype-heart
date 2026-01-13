package block

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"regexp"

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

var (
	miroRegexp       = regexp.MustCompile(`https?:\/\/(?:www\.)?miro\.com\/app\/board\/[a-zA-Z0-9_=-]+\/?`)
	googleMapsRegexp = regexp.MustCompile(`https?:\/\/(?:www\.)?google\.com\/maps(?:\/[^\/\n\s]+)?(?:\/@(-?\d+\.\d+),(-?\d+\.\d+),\d+z?)?(?:\/[^\/\n\s]+)?`)
	githubGistRegexp = regexp.MustCompile(`https:\/\/gist\.github\.com\/[a-zA-Z0-9_-]+\/([a-fA-F0-9]+)`)
	codepenRegexp    = regexp.MustCompile(`https:\/\/codepen\.io\/[a-zA-Z0-9_-]+\/(?:pen\/([a-zA-Z0-9_-]+)|details\/([a-zA-Z0-9_-]+)(?:\/[a-zA-Z0-9_-]+)?)\/?`)
)

type EmbedBlock struct {
	Block
	Embed LinkToWeb `json:"embed"`
}

func (b *EmbedBlock) GetBlocks(req *api.NotionImportContext, _ string) *MapResponse {
	if b.isEmbedBlock() {
		return b.provideEmbedBlock()
	}
	return b.Embed.GetBlocks(req, "")
}

func (b *EmbedBlock) provideEmbedBlock() *MapResponse {
	processor := b.getProcessor()
	id := bson.NewObjectId().Hex()
	bl := &model.Block{
		Id:          id,
		ChildrenIds: []string{},
		Content: &model.BlockContentOfLatex{
			Latex: &model.BlockContentLatex{
				Text:      b.Embed.URL,
				Processor: processor,
			},
		},
	}
	return &MapResponse{
		Blocks:   []*model.Block{bl},
		BlockIDs: []string{id},
	}
}

func (b *EmbedBlock) getProcessor() model.BlockContentLatexProcessor {
	var processor model.BlockContentLatexProcessor
	if googleMapsRegexp.MatchString(b.Embed.URL) {
		processor = model.BlockContentLatex_GoogleMaps
	}
	if miroRegexp.MatchString(b.Embed.URL) {
		processor = model.BlockContentLatex_Miro
	}
	if soundCloudRegexp.MatchString(b.Embed.URL) {
		processor = model.BlockContentLatex_Soundcloud
	}
	if githubGistRegexp.MatchString(b.Embed.URL) {
		processor = model.BlockContentLatex_GithubGist
	}
	if codepenRegexp.MatchString(b.Embed.URL) {
		processor = model.BlockContentLatex_Codepen
	}
	return processor
}

func (b *EmbedBlock) isEmbedBlock() bool {
	return miroRegexp.MatchString(b.Embed.URL) || googleMapsRegexp.MatchString(b.Embed.URL) || soundCloudRegexp.MatchString(b.Embed.URL) ||
		codepenRegexp.MatchString(b.Embed.URL) || githubGistRegexp.MatchString(b.Embed.URL)
}

type LinkToWeb struct {
	URL string `json:"url"`
}

type LinkPreviewBlock struct {
	Block
	LinkPreview LinkToWeb `json:"link_preview"`
}

func (b *LinkPreviewBlock) GetBlocks(req *api.NotionImportContext, _ string) *MapResponse {
	return b.LinkPreview.GetBlocks(req, "")
}

func (b *LinkToWeb) GetBlocks(*api.NotionImportContext, string) *MapResponse {
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

func (b *ChildPageBlock) GetBlocks(req *api.NotionImportContext, pageID string) *MapResponse {
	bl := b.ChildPage.GetLinkToObjectBlock(req, pageID, b.Parent.BlockID)
	return &MapResponse{
		Blocks:   []*model.Block{bl},
		BlockIDs: []string{bl.Id},
	}
}

func (p ChildPage) GetLinkToObjectBlock(importContext *api.NotionImportContext, pageID, parentBlockID string) *model.Block {
	targetBlockID, err := getTargetBlock(importContext, importContext.PageNameToID, importContext.NotionPageIdsToAnytype, pageID, p.Title, parentBlockID)

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
				IconSize:      model.BlockContentLink_SizeSmall,
			},
		}}
}

type ChildDatabaseBlock struct {
	Block
	ChildDatabase ChildDatabase `json:"child_database"`
}

func (b *ChildDatabaseBlock) GetBlocks(req *api.NotionImportContext, pageID string) *MapResponse {
	bl := b.ChildDatabase.GetBlock(req, pageID, b.Parent.BlockID)
	return &MapResponse{
		Blocks:   []*model.Block{bl},
		BlockIDs: []string{bl.Id},
	}
}

type ChildDatabase struct {
	Title string `json:"title"`
}

func (c *ChildDatabase) GetBlock(importContext *api.NotionImportContext, pageID, parentBlockID string) *model.Block {
	targetBlockID, err := getTargetBlock(importContext,
		importContext.DatabaseNameToID,
		importContext.NotionDatabaseIdsToAnytype,
		pageID, c.Title, parentBlockID)

	id := bson.NewObjectId().Hex()
	if err != nil || targetBlockID == "" {
		block := template.MakeDataviewContent(true, nil, nil, nil)
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
				IconSize:      model.BlockContentLink_SizeSmall,
			},
		},
	}
}

type LinkToPageBlock struct {
	Block
	LinkToPage api.Parent `json:"link_to_page"`
}

func (l *LinkToPageBlock) GetBlocks(req *api.NotionImportContext, _ string) *MapResponse {
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
				IconSize:      model.BlockContentLink_SizeSmall,
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

func (b *BookmarkBlock) GetBlocks(*api.NotionImportContext, string) *MapResponse {
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
func getTargetBlock(importContext *api.NotionImportContext, pageIDToName, notionIDsToAnytype map[string]string, pageID, title, parentBlockID string) (string, error) {
	var (
		targetBlockID string
		ok            bool
	)

	findByPageAndTitle := func(pageID, title string) string {
		if childIDs, exist := importContext.PageTree.Get(pageID); exist {
			for childIdx, childID := range childIDs {
				if pageName, pageExist := pageIDToName[childID]; pageExist && pageName == title {
					if targetBlockID, ok = notionIDsToAnytype[childID]; !ok {
						return ""
					}

					childIDs = slices.Delete(childIDs, childIdx, childIdx+1)
					break
				}
			}
			importContext.PageTree.Set(pageID, childIDs)
		}

		return targetBlockID
	}

	// first, try to find page in parent blocks
	if parentBlockID != "" {
		targetBlockID = findByPageAndTitle(parentBlockID, title)
		if targetBlockID != "" {
			importContext.BlockToPage.Set(parentBlockID, pageID)
			return targetBlockID, nil
		}
	}

	// try to find it in the list of child of the current page
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
		if name == title {
			idsWithGivenName = append(idsWithGivenName, id)
		}
	}

	var err error
	if len(idsWithGivenName) == 1 {
		id := idsWithGivenName[0]
		if targetBlockID, ok = notionIDsToAnytype[id]; ok {
			return targetBlockID, nil
		} else {
			err = fmt.Errorf("%s '%s'", objectNotFoundMessage, hashText(title))
			log.With("notionID", hashText(id)).With("title", hashText(title)).Errorf("getTargetBlock: anytype id not found")
		}
	} else if len(idsWithGivenName) > 1 {
		err = fmt.Errorf("%s '%s'", ambiguousPageMessage, hashText(title))
		log.With("title", hashText(title)).With("options", len(idsWithGivenName)).Warnf("getTargetBlock: ambligious page title")
	} else {
		err = fmt.Errorf("%s '%s'", pageNotFoundMessage, hashText(title))
		log.With("title", hashText(title)).Errorf("getTargetBlock: target not found")
	}

	return targetBlockID, err
}

func hashText(text string) string {
	hash := md5.Sum([]byte(text))
	return hex.EncodeToString(hash[:])
}
