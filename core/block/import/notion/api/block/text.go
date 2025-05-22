package block

import (
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/import/notion/api"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	textUtil "github.com/anyproto/anytype-heart/util/text"
)

const mermaidLang = "mermaid"

type ParagraphBlock struct {
	Block
	Paragraph TextObjectWithChildren `json:"paragraph"`
}

func (p *ParagraphBlock) GetBlocks(req *api.NotionImportContext, pageID string) *MapResponse {
	childResp := &MapResponse{}
	if p.HasChildren {
		mapper := ChildrenMapper(&p.Paragraph)
		childResp = mapper.MapChildren(req, pageID)
	}
	childID := getChildID(childResp)
	resp := p.Paragraph.GetTextBlocks(model.BlockContentText_Paragraph, childID, req)
	resp.Merge(childResp)
	return resp
}

func (p *ParagraphBlock) HasChild() bool {
	return p.HasChildren
}

func (p *ParagraphBlock) SetChildren(children []interface{}) {
	p.Paragraph.Children = children
}

func (p *ParagraphBlock) GetID() string {
	return p.ID
}

type Heading1Block struct {
	Block
	Heading1 HeadingObject `json:"heading_1"`
}

func (h *Heading1Block) SetChildren(children []interface{}) {
	h.Heading1.Children = children
}

func (h *Heading1Block) HasChild() bool {
	return h.Heading1.IsToggleable
}

func (h *Heading1Block) GetID() string {
	return h.ID
}

func (h *Heading1Block) GetBlocks(req *api.NotionImportContext, pageID string) *MapResponse {
	resp := h.Heading1.GetTextBlocks(model.BlockContentText_Header1, nil, req)
	if h.Heading1.IsToggleable {
		mapper := ChildrenMapper(&h.Heading1)
		childResp := mapper.MapChildren(req, pageID)
		resp.Merge(childResp)
	}
	return resp
}

type Heading2Block struct {
	Block
	Heading2 HeadingObject `json:"heading_2"`
}

func (h *Heading2Block) SetChildren(children []interface{}) {
	h.Heading2.Children = children
}

func (h *Heading2Block) HasChild() bool {
	return h.Heading2.IsToggleable
}

func (h *Heading2Block) GetID() string {
	return h.ID
}

func (h *Heading2Block) GetBlocks(req *api.NotionImportContext, pageID string) *MapResponse {
	resp := h.Heading2.GetTextBlocks(model.BlockContentText_Header2, nil, req)
	if h.Heading2.IsToggleable {
		mapper := ChildrenMapper(&h.Heading2)
		childResp := mapper.MapChildren(req, pageID)
		resp.Merge(childResp)
	}
	return resp
}

type Heading3Block struct {
	Block
	Heading3 HeadingObject `json:"heading_3"`
}

func (h *Heading3Block) GetBlocks(req *api.NotionImportContext, pageID string) *MapResponse {
	resp := h.Heading3.GetTextBlocks(model.BlockContentText_Header3, nil, req)
	if h.Heading3.IsToggleable {
		mapper := ChildrenMapper(&h.Heading3)
		childResp := mapper.MapChildren(req, pageID)
		resp.Merge(childResp)
	}
	return resp
}

func (h *Heading3Block) SetChildren(children []interface{}) {
	h.Heading3.Children = children
}

func (h *Heading3Block) HasChild() bool {
	return h.Heading3.IsToggleable
}

func (h *Heading3Block) GetID() string {
	return h.ID
}

type HeadingObject struct {
	TextObjectWithChildren
	IsToggleable bool `json:"is_toggleable"`
}

type CalloutBlock struct {
	Block
	Callout CalloutObject `json:"callout"`
}

type CalloutObject struct {
	TextObjectWithChildren
	Icon *api.Icon `json:"icon" oneOf:"EmojiIcon,FileIcon,NamedIcon"`
}

func (c *CalloutBlock) GetBlocks(req *api.NotionImportContext, pageID string) *MapResponse {
	childResp := &MapResponse{}
	if c.HasChild() {
		mapper := ChildrenMapper(&c.Callout)
		childResp = mapper.MapChildren(req, pageID)
	}
	childIDs := getChildID(childResp)
	calloutResp := c.Callout.GetTextBlocks(model.BlockContentText_Callout, childIDs, req)
	extendedBlocks := make([]*model.Block, 0, len(calloutResp.Blocks))
	for _, cb := range calloutResp.Blocks {
		if text := c.makeTextContent(cb); text != nil {
			cb.Content = text
		}
		extendedBlocks = append(extendedBlocks, cb)
	}
	calloutResp.Blocks = extendedBlocks
	calloutResp.Merge(childResp)
	return calloutResp
}

func (c *CalloutBlock) HasChild() bool {
	return c.HasChildren
}

func (c *CalloutBlock) SetChildren(children []interface{}) {
	c.Callout.Children = children
}

func (c *CalloutBlock) GetID() string {
	return c.ID
}

func (c *CalloutBlock) makeTextContent(cb *model.Block) *model.BlockContentOfText {
	text, ok := cb.Content.(*model.BlockContentOfText)
	if !ok {
		return nil
	}
	if c.Callout.Icon != nil {
		if c.Callout.Icon.Emoji != nil {
			text.Text.IconEmoji = *c.Callout.Icon.Emoji
		}
		if c.Callout.Icon.Type == api.External && c.Callout.Icon.External != nil {
			text.Text.IconImage = c.Callout.Icon.External.URL
		}
		if c.Callout.Icon.Type == api.File && c.Callout.Icon.File != nil {
			text.Text.IconImage = c.Callout.Icon.File.URL
		}
	}
	return text
}

type QuoteBlock struct {
	Block
	Quote TextObjectWithChildren `json:"quote"`
}

func (q *QuoteBlock) GetBlocks(req *api.NotionImportContext, pageID string) *MapResponse {
	childResp := &MapResponse{}
	if q.HasChildren {
		mapper := ChildrenMapper(&q.Quote)
		childResp = mapper.MapChildren(req, pageID)
	}
	childID := getChildID(childResp)
	resp := q.Quote.GetTextBlocks(model.BlockContentText_Quote, childID, req)
	resp.Merge(childResp)
	return resp
}

func (q *QuoteBlock) HasChild() bool {
	return q.HasChildren
}

func (q *QuoteBlock) SetChildren(children []interface{}) {
	q.Quote.Children = children
}

func (q *QuoteBlock) GetID() string {
	return q.ID
}

type NumberedListBlock struct {
	Block
	NumberedList TextObjectWithChildren `json:"numbered_list_item"`
}

func (n *NumberedListBlock) GetBlocks(req *api.NotionImportContext, pageID string) *MapResponse {
	childResp := &MapResponse{}
	if n.HasChildren {
		mapper := ChildrenMapper(&n.NumberedList)
		childResp = mapper.MapChildren(req, pageID)
	}
	childID := getChildID(childResp)
	resp := n.NumberedList.GetTextBlocks(model.BlockContentText_Numbered, childID, req)
	resp.Merge(childResp)
	return resp
}

func (n *NumberedListBlock) HasChild() bool {
	return n.HasChildren
}

func (n *NumberedListBlock) SetChildren(children []interface{}) {
	n.NumberedList.Children = children
}

func (n *NumberedListBlock) GetID() string {
	return n.ID
}

type ToDoBlock struct {
	Block
	ToDo ToDoObject `json:"to_do"`
}

func (t *ToDoBlock) GetBlocks(req *api.NotionImportContext, pageID string) *MapResponse {
	childResp := &MapResponse{}
	if t.HasChildren {
		mapper := ChildrenMapper(&t.ToDo)
		childResp = mapper.MapChildren(req, pageID)
	}
	resp := t.ToDo.GetTextBlocks(model.BlockContentText_Checkbox, childResp.BlockIDs, req)
	t.setChecked(resp)
	resp.Merge(childResp)
	return resp
}

func (t *ToDoBlock) setChecked(resp *MapResponse) {
	for _, block := range resp.Blocks {
		if c, ok := block.Content.(*model.BlockContentOfText); ok {
			if c.Text != nil {
				c.Text.Checked = t.ToDo.Checked
			}
		}
	}
}

func (t *ToDoBlock) HasChild() bool {
	return t.HasChildren
}

func (t *ToDoBlock) SetChildren(children []interface{}) {
	t.ToDo.Children = children
}

func (t *ToDoBlock) GetID() string {
	return t.ID
}

type ToDoObject struct {
	TextObjectWithChildren
	Checked bool `json:"checked"`
}

type BulletedListBlock struct {
	Block
	BulletedList TextObjectWithChildren `json:"bulleted_list_item"`
}

func (b *BulletedListBlock) GetBlocks(req *api.NotionImportContext, pageID string) *MapResponse {
	childResp := &MapResponse{}
	if b.HasChildren {
		mapper := ChildrenMapper(&b.BulletedList)
		childResp = mapper.MapChildren(req, pageID)
	}
	childID := getChildID(childResp)
	resp := b.BulletedList.GetTextBlocks(model.BlockContentText_Marked, childID, req)
	resp.Merge(childResp)
	return resp
}

func (b *BulletedListBlock) HasChild() bool {
	return b.HasChildren
}

func (b *BulletedListBlock) SetChildren(children []interface{}) {
	b.BulletedList.Children = children
}

func (b *BulletedListBlock) GetID() string {
	return b.ID
}

type ToggleBlock struct {
	Block
	Toggle TextObjectWithChildren `json:"toggle"`
}

func (t *ToggleBlock) GetBlocks(req *api.NotionImportContext, pageID string) *MapResponse {
	childResp := &MapResponse{}
	if t.HasChildren {
		mapper := ChildrenMapper(&t.Toggle)
		childResp = mapper.MapChildren(req, pageID)
	}

	childID := getChildID(childResp)
	resp := t.Toggle.GetTextBlocks(model.BlockContentText_Toggle, childID, req)
	resp.Merge(childResp)
	return resp
}

func getChildID(childResp *MapResponse) []string {
	childIDs := make(map[string]struct{})
	for _, block := range childResp.Blocks {
		for _, id := range block.ChildrenIds {
			childIDs[id] = struct{}{}
		}
	}
	var notChildID []string

	for _, id := range childResp.BlockIDs {
		if _, ok := childIDs[id]; !ok {
			notChildID = append(notChildID, id)
		}
	}
	return notChildID
}

func (t *ToggleBlock) HasChild() bool {
	return t.HasChildren
}

func (t *ToggleBlock) SetChildren(children []interface{}) {
	t.Toggle.Children = children
}

func (t *ToggleBlock) GetID() string {
	return t.ID
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

func (c *CodeBlock) GetBlocks(*api.NotionImportContext, string) *MapResponse {
	var marks []*model.BlockContentTextMark
	var code string
	for _, rt := range c.Code.RichText {
		from := textUtil.UTF16RuneCountString(code)
		code += rt.PlainText
		to := textUtil.UTF16RuneCountString(code)
		marks = append(marks, rt.BuildMarkdownFromAnnotations(int32(from), int32(to))...)
	}
	id := bson.NewObjectId().Hex()
	if c.Code.Language == mermaidLang {
		return c.handleMermaidBlock(id, code)
	}
	bl := &model.Block{
		Id: id,
		Fields: &types.Struct{
			Fields: map[string]*types.Value{"lang": pbtypes.String(c.Code.Language)},
		},
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
	return &MapResponse{
		Blocks:   []*model.Block{bl},
		BlockIDs: []string{id},
	}
}

func (c *CodeBlock) handleMermaidBlock(id string, code string) *MapResponse {
	bl := &model.Block{
		Id:          id,
		ChildrenIds: []string{},
		Content: &model.BlockContentOfLatex{
			Latex: &model.BlockContentLatex{
				Text:      code,
				Processor: model.BlockContentLatex_Mermaid,
			},
		},
	}
	return &MapResponse{
		Blocks:   []*model.Block{bl},
		BlockIDs: []string{id},
	}
}

type EquationBlock struct {
	Block
	Equation api.EquationObject `json:"equation"`
}

func (e *EquationBlock) GetBlocks(*api.NotionImportContext, string) *MapResponse {
	bl := e.Equation.HandleEquation()
	return &MapResponse{
		Blocks:   []*model.Block{bl},
		BlockIDs: []string{bl.Id},
	}
}
