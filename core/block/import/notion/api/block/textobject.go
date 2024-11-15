package block

import (
	"strings"

	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/core/block/import/notion/api"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/dateutil"
	textUtil "github.com/anyproto/anytype-heart/util/text"
)

const DateMentionTimeFormat = "2006-01-02"

const (
	// notExistingName is used in mention, when pages/database mentions is not existing in integration. We show such pages
	// in anytype as "Not found"
	notExistingName          = "Untitled"
	notExistingObjectMessage = "Can't access object from Notion"
)

type ChildrenMapper interface {
	MapChildren(req *api.NotionImportContext, pageID string) *MapResponse
}

type TextObject struct {
	RichText []api.RichText `json:"rich_text"`
	Color    string         `json:"color"`
}

func (t *TextObject) GetTextBlocks(style model.BlockContentTextStyle, childIds []string, req *api.NotionImportContext) *MapResponse {
	var marks []*model.BlockContentTextMark
	id := bson.NewObjectId().Hex()
	allBlocks := make([]*model.Block, 0)
	allIds := make([]string, 0)
	var (
		text strings.Builder
	)
	for _, rt := range t.RichText {
		if rt.Type == api.Text {
			marks = append(marks, t.handleTextType(rt, &text, req.NotionPageIdsToAnytype, req.NotionDatabaseIdsToAnytype)...)
		}
		if rt.Type == api.Mention {
			marks = append(marks, t.handleMentionType(rt, &text, req)...)
		}
		if rt.Type == api.Equation {
			eqBlock := rt.Equation.HandleEquation()
			allBlocks = append(allBlocks, eqBlock)
			allIds = append(allIds, eqBlock.Id)
		}
	}
	var backgroundColor, textColor string
	if strings.Contains(t.Color, api.NotionBackgroundColorSuffix) {
		backgroundColor = api.NotionColorToAnytype[t.Color]
	} else {
		textColor = api.NotionColorToAnytype[t.Color]
	}

	if t.isNotTextBlocks() {
		return &MapResponse{
			Blocks:   allBlocks,
			BlockIDs: allIds,
		}
	}
	allBlocks = append(allBlocks, &model.Block{
		Id:              id,
		ChildrenIds:     childIds,
		BackgroundColor: backgroundColor,
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:    text.String(),
				Style:   style,
				Marks:   &model.BlockContentTextMarks{Marks: marks},
				Checked: false,
				Color:   textColor,
			},
		},
	})
	for _, b := range allBlocks {
		allIds = append(allIds, b.Id)
	}
	return &MapResponse{
		Blocks:   allBlocks,
		BlockIDs: allIds,
	}
}

func (t *TextObject) handleTextType(rt api.RichText,
	text *strings.Builder,
	notionPageIdsToAnytype,
	notionDatabaseIdsToAnytype map[string]string) []*model.BlockContentTextMark {
	var marks []*model.BlockContentTextMark
	from := textUtil.UTF16RuneCountString(text.String())
	if rt.Text != nil && rt.Text.Link != nil && rt.Text.Link.URL != "" {
		text.WriteString(rt.Text.Content)
	} else {
		text.WriteString(rt.PlainText)
	}
	to := textUtil.UTF16RuneCountString(text.String())
	if rt.Text != nil && rt.Text.Link != nil && rt.Text.Link.URL != "" {
		url := strings.Trim(rt.Text.Link.URL, "/")
		if databaseID, ok := notionDatabaseIdsToAnytype[url]; ok {
			url = databaseID
		}
		if pageID, ok := notionPageIdsToAnytype[url]; ok {
			url = pageID
		}
		marks = append(marks, &model.BlockContentTextMark{
			Range: &model.Range{
				From: int32(from),
				To:   int32(to),
			},
			Type:  model.BlockContentTextMark_Link,
			Param: url,
		})
	}
	marks = append(marks, rt.BuildMarkdownFromAnnotations(int32(from), int32(to))...)
	return marks
}

func (t *TextObject) handleMentionType(rt api.RichText,
	text *strings.Builder,
	req *api.NotionImportContext) []*model.BlockContentTextMark {
	if rt.Mention.Type == api.UserMention {
		return t.handleUserMention(rt, text)
	}
	if rt.Mention.Type == api.Database {
		return t.handleDatabaseMention(rt, text, req.NotionDatabaseIdsToAnytype, req.DatabaseNameToID)
	}
	if rt.Mention.Type == api.Page {
		return t.handlePageMention(rt, text, req.NotionPageIdsToAnytype, req.PageNameToID)
	}
	if rt.Mention.Type == api.LinkPreview {
		return t.handleLinkPreviewMention(rt, text)
	}
	if rt.Mention.Type == api.Date {
		return t.handleDateMention(rt, text)
	}
	return nil
}

func (t *TextObject) handleUserMention(rt api.RichText, text *strings.Builder) []*model.BlockContentTextMark {
	from := textUtil.UTF16RuneCountString(text.String())
	text.WriteString(rt.PlainText)
	to := textUtil.UTF16RuneCountString(text.String())
	return rt.BuildMarkdownFromAnnotations(int32(from), int32(to))
}

func (t *TextObject) handleDatabaseMention(rt api.RichText,
	text *strings.Builder,
	notionDatabaseIdsToAnytype, databaseNameToID map[string]string) []*model.BlockContentTextMark {
	from := textUtil.UTF16RuneCountString(text.String())
	if p, ok := databaseNameToID[rt.Mention.Database.ID]; ok {
		text.WriteString(p)
	} else if rt.PlainText != "" && rt.PlainText != notExistingName {
		text.WriteString(rt.PlainText)
		to := textUtil.UTF16RuneCountString(text.String())
		return rt.BuildMarkdownFromAnnotations(int32(from), int32(to))
	} else if rt.Href != "" {
		return t.writeLink(rt, text, from)
	} else {
		text.WriteString(notExistingObjectMessage)
		to := textUtil.UTF16RuneCountString(text.String())
		return rt.BuildMarkdownFromAnnotations(int32(from), int32(to))
	}
	to := textUtil.UTF16RuneCountString(text.String())
	marks := rt.BuildMarkdownFromAnnotations(int32(from), int32(to))
	var dbID string
	if notionDatabaseIdsToAnytype != nil {
		dbID = notionDatabaseIdsToAnytype[rt.Mention.Database.ID]
	}
	marks = append(marks, &model.BlockContentTextMark{
		Range: &model.Range{
			From: int32(from),
			To:   int32(to),
		},
		Type:  model.BlockContentTextMark_Mention,
		Param: dbID,
	})
	return marks
}

func (t *TextObject) handlePageMention(rt api.RichText,
	text *strings.Builder,
	notionPageIdsToAnytype, pageNameToID map[string]string) []*model.BlockContentTextMark {
	from := textUtil.UTF16RuneCountString(text.String())
	if p, ok := pageNameToID[rt.Mention.Page.ID]; ok {
		text.WriteString(p)
	} else if rt.PlainText != "" && rt.PlainText != notExistingName {
		text.WriteString(rt.PlainText)
		to := textUtil.UTF16RuneCountString(text.String())
		return rt.BuildMarkdownFromAnnotations(int32(from), int32(to))
	} else if rt.Href != "" {
		return t.writeLink(rt, text, from)
	} else {
		text.WriteString(notExistingObjectMessage)
		to := textUtil.UTF16RuneCountString(text.String())
		return rt.BuildMarkdownFromAnnotations(int32(from), int32(to))
	}
	to := textUtil.UTF16RuneCountString(text.String())
	marks := rt.BuildMarkdownFromAnnotations(int32(from), int32(to))
	var pageID string
	if notionPageIdsToAnytype != nil {
		pageID = notionPageIdsToAnytype[rt.Mention.Page.ID]
	}
	marks = append(marks, &model.BlockContentTextMark{
		Range: &model.Range{
			From: int32(from),
			To:   int32(to),
		},
		Type:  model.BlockContentTextMark_Mention,
		Param: pageID,
	})
	return marks
}

func (t *TextObject) writeLink(rt api.RichText, text *strings.Builder, from int) []*model.BlockContentTextMark {
	text.WriteString(rt.Href)
	to := textUtil.UTF16RuneCountString(text.String())
	marks := rt.BuildMarkdownFromAnnotations(int32(from), int32(to))
	marks = append(marks, &model.BlockContentTextMark{
		Range: &model.Range{
			From: int32(from),
			To:   int32(to),
		},
		Type:  model.BlockContentTextMark_Link,
		Param: rt.Href,
	})
	return marks
}

func (t *TextObject) handleDateMention(rt api.RichText,
	text *strings.Builder) []*model.BlockContentTextMark {
	var textDate string
	if rt.Mention.Date.Start != "" {
		textDate = rt.Mention.Date.Start
	}
	if rt.Mention.Date.End != "" {
		textDate = rt.Mention.Date.End
	}
	date, err := dateutil.ParseDateId(textDate)
	if err != nil {
		return nil
	}
	from := textUtil.UTF16RuneCountString(text.String())
	to := textUtil.UTF16RuneCountString(text.String())
	return []*model.BlockContentTextMark{
		{
			Range: &model.Range{
				From: int32(from),
				To:   int32(to),
			},
			Type:  model.BlockContentTextMark_Mention,
			Param: dateutil.TimeToDateId(date, false),
		},
	}
}

func (t *TextObject) handleLinkPreviewMention(rt api.RichText, text *strings.Builder) []*model.BlockContentTextMark {
	from := textUtil.UTF16RuneCountString(text.String())
	text.WriteString(rt.Mention.LinkPreview.URL)
	to := textUtil.UTF16RuneCountString(text.String())
	marks := rt.BuildMarkdownFromAnnotations(int32(from), int32(to))
	marks = append(marks, &model.BlockContentTextMark{
		Range: &model.Range{
			From: int32(from),
			To:   int32(to),
		},
		Type: model.BlockContentTextMark_Link,
	})
	return marks
}

func (t *TextObject) isNotTextBlocks() bool {
	return len(t.RichText) == 1 && t.RichText[0].Type == api.Equation
}

type TextObjectWithChildren struct {
	TextObject
	Children []interface{} `json:"children"`
}

func (t *TextObjectWithChildren) MapChildren(req *api.NotionImportContext, pageID string) *MapResponse {
	resp := MapBlocks(req, t.Children, pageID)
	return resp
}
