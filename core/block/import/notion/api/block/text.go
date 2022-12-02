package block

import (
	"strings"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	textUtil "github.com/anytypeio/go-anytype-middleware/util/text"
)

type ParagraphBlock struct {
	Block
	Paragraph TextObjectWithChildren `json:"paragraph"`
}

type Heading1Block struct {
	Block
	Heading1 HeadingObject `json:"heading_1"`
}

type Heading2Block struct {
	Block
	Heading2 HeadingObject `json:"heading_2"`
}

type Heading3Block struct {
	Block
	Heading3 HeadingObject `json:"heading_3"`
}

type HeadingObject struct {
	TextObject
	IsToggleable bool `json:"is_toggleable"`
}

type CalloutBlock struct {
	Block
	Callout CalloutObject `json:"callout"`
}

type CalloutObject struct {
	TextObjectWithChildren
	Icon *api.Icon `json:"icon"`
}

type QuoteBlock struct {
	Block
	Quote TextObjectWithChildren `json:"quote"`
}

type NumberedListBlock struct {
	Block
	NumberedList TextObjectWithChildren `json:"bulleted_list_item"`
}

type ToDoBlock struct {
	Block
	ToDo ToDoObject `json:"to_do"`
}

type ToDoObject struct {
	TextObjectWithChildren
	Checked bool `json:"checked"`
}

type BulletedListBlock struct {
	Block
	BulletedList TextObjectWithChildren `json:"bulleted_list_item"`
}

type ToggleBlock struct {
	Block
	Toggle TextObjectWithChildren `json:"toggle"`
}

type TextObjectWithChildren struct {
	TextObject
	Children []interface{} `json:"children"`
}

type TextObject struct {
	RichText []api.RichText `json:"rich_text"`
	Color    string         `json:"color"`
}

func (c *CalloutObject) GetCalloutBlocks(childIds []string) ([]*model.Block, []string) {
	calloutBlocks, blockIDs := c.GetTextBlocks(model.BlockContentText_Callout, childIds, nil, nil, nil, nil)
	for _, cb := range calloutBlocks {
		text, ok := cb.Content.(*model.BlockContentOfText)
		if ok {
			if c.Icon != nil {
				if c.Icon.Emoji != nil {
					text.Text.IconEmoji = *c.Icon.Emoji
				}
				if c.Icon.Type == api.External && c.Icon.External != nil {
					text.Text.IconImage = c.Icon.External.URL
				}
				if c.Icon.Type == api.File && c.Icon.File != nil {
					text.Text.IconImage = c.Icon.File.URL
				}
			}
			cb.Content = text
		}
	}
	return calloutBlocks, blockIDs
}

func (t *TextObject) GetTextBlocks(style model.BlockContentTextStyle, childIds []string, notionPageIdsToAnytype, notionDatabaseIdsToAnytype, pageNameToID, databaseNameToID map[string]string) ([]*model.Block, []string) {
	marks := []*model.BlockContentTextMark{}
	id := bson.NewObjectId().Hex()
	allBlocks := make([]*model.Block, 0)
	allIds := make([]string, 0)
	var text strings.Builder
	for _, rt := range t.RichText {
		if rt.Type == api.Text {
			marks = append(marks, t.handleTextType(rt, &text, notionPageIdsToAnytype, notionDatabaseIdsToAnytype)...)
		}
		if rt.Type == api.Mention {
			marks = append(marks, t.handleMentionType(rt, &text, notionPageIdsToAnytype, notionDatabaseIdsToAnytype, pageNameToID, databaseNameToID)...)
		}
		if rt.Type == api.Equation {
			eqBlock := rt.Equation.HandleEquation()
			allBlocks = append(allBlocks, eqBlock)
			allIds = append(allIds, eqBlock.Id)
		}
	}
	var backgroundColor string
	if strings.HasSuffix(t.Color, api.NotionBackgroundColorSuffix) {
		backgroundColor = api.NotionColorToAnytype[t.Color]
	}

	if len(t.RichText) == 1 && t.RichText[0].Type == api.Equation {
		return allBlocks, allIds
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
				Color:   api.NotionColorToAnytype[t.Color],
			},
		},
	})
	for _, b := range allBlocks {
		allIds = append(allIds, b.Id)
	}
	return allBlocks, allIds
}

func (t *TextObject) handleTextType(rt api.RichText, text *strings.Builder, notionPageIdsToAnytype, notionDatabaseIdsToAnytype map[string]string) []*model.BlockContentTextMark {
	marks := []*model.BlockContentTextMark{}
	from := textUtil.UTF16RuneCountString(text.String())
	if rt.Text != nil && rt.Text.Link != nil && rt.Text.Link.Url != "" {
		text.WriteString(rt.Text.Content)
	} else {
		text.WriteString(rt.PlainText)
	}
	to := textUtil.UTF16RuneCountString(text.String())
	if rt.Text != nil && rt.Text.Link != nil && rt.Text.Link.Url != "" {
		url := strings.Trim(rt.Text.Link.Url, "/")
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

func (t *TextObject) handleMentionType(rt api.RichText, text *strings.Builder, notionPageIdsToAnytype, notionDatabaseIdsToAnytype, pageNameToID, databaseNameToID map[string]string) []*model.BlockContentTextMark {
	if rt.Mention.Type == api.UserMention {
		return t.handleUserMention(rt, text)
	}
	if rt.Mention.Type == api.Database {
		return t.handleDatabaseMention(rt, text, notionDatabaseIdsToAnytype, databaseNameToID)
	}
	if rt.Mention.Type == api.Page {
		return t.handlePageMention(rt, text, notionPageIdsToAnytype, pageNameToID)
	}
	if rt.Mention.Type == api.Date {
		return t.handleDateMention(rt, text)
	}
	if rt.Mention.Type == api.LinkPreview {
		return t.handleLinkPreviewMention(rt, text)
	}
	return nil
}

func (t *TextObject) handleUserMention(rt api.RichText, text *strings.Builder) []*model.BlockContentTextMark {
	from := textUtil.UTF16RuneCountString(text.String())
	text.WriteString(rt.Mention.User.Name)
	to := textUtil.UTF16RuneCountString(text.String())
	return rt.BuildMarkdownFromAnnotations(int32(from), int32(to))
}

func (t *TextObject) handleDatabaseMention(rt api.RichText, text *strings.Builder, notionDatabaseIdsToAnytype, databaseNameToID map[string]string) []*model.BlockContentTextMark {
	if notionDatabaseIdsToAnytype == nil {
		return nil
	}
	from := textUtil.UTF16RuneCountString(text.String())
	text.WriteString(databaseNameToID[rt.Mention.Database.ID])
	to := textUtil.UTF16RuneCountString(text.String())
	marks := rt.BuildMarkdownFromAnnotations(int32(from), int32(to))
	marks = append(marks, &model.BlockContentTextMark{
		Range: &model.Range{
			From: int32(from),
			To:   int32(to),
		},
		Type:  model.BlockContentTextMark_Mention,
		Param: notionDatabaseIdsToAnytype[rt.Mention.Database.ID],
	})
	return marks
}

func (t *TextObject) handlePageMention(rt api.RichText, text *strings.Builder, notionPageIdsToAnytype, pageNameToID map[string]string) []*model.BlockContentTextMark {
	if notionPageIdsToAnytype == nil {
		return nil
	}
	from := textUtil.UTF16RuneCountString(text.String())
	text.WriteString(pageNameToID[rt.Mention.Page.ID])
	to := textUtil.UTF16RuneCountString(text.String())
	marks := rt.BuildMarkdownFromAnnotations(int32(from), int32(to))
	marks = append(marks, &model.BlockContentTextMark{
		Range: &model.Range{
			From: int32(from),
			To:   int32(to),
		},
		Type:  model.BlockContentTextMark_Mention,
		Param: notionPageIdsToAnytype[rt.Mention.Page.ID],
	})
	return marks
}

func (t *TextObject) handleDateMention(rt api.RichText, text *strings.Builder) []*model.BlockContentTextMark {
	var textDate string
	if rt.Mention.Date.Start != "" {
		textDate = rt.Mention.Date.Start
	}
	if rt.Mention.Date.End != "" {
		textDate += " " + rt.Mention.Date.End
	}
	from := textUtil.UTF16RuneCountString(text.String())
	text.WriteString(textDate)
	to := textUtil.UTF16RuneCountString(text.String())
	marks := rt.BuildMarkdownFromAnnotations(int32(from), int32(to))
	return marks
}

func (t *TextObject) handleLinkPreviewMention(rt api.RichText, text *strings.Builder) []*model.BlockContentTextMark {
	from := textUtil.UTF16RuneCountString(text.String())
	text.WriteString(rt.Mention.LinkPreview.Url)
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

type CodeBlock struct {
	Block
	Code CodeObject `json:"code"`
}

type CodeObject struct {
	RichText []api.RichText `json:"rich_text"`
	Caption  []api.RichText `json:"caption"`
	Language string         `json:"language"`
}

func (c *CodeObject) GetCodeBlock() *model.Block {
	id := bson.NewObjectId().Hex()
	bl := &model.Block{
		Id: id,
		Fields: &types.Struct{
			Fields: map[string]*types.Value{"lang": pbtypes.String(c.Language)},
		},
	}
	marks := []*model.BlockContentTextMark{}
	var code string
	for _, rt := range c.RichText {
		from := textUtil.UTF16RuneCountString(code)
		code += rt.PlainText
		to := textUtil.UTF16RuneCountString(code)
		marks = append(marks, rt.BuildMarkdownFromAnnotations(int32(from), int32(to))...)
	}
	bl.Content = &model.BlockContentOfText{
		Text: &model.BlockContentText{
			Text:  code,
			Style: model.BlockContentText_Code,
			Marks: &model.BlockContentTextMarks{
				Marks: marks,
			},
		},
	}
	return bl
}

type EquationBlock struct {
	Block
	Equation api.EquationObject `json:"equation"`
}
