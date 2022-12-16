package block

import (
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

func (p *ParagraphBlock) GetBlocks(req *MapRequest) *MapResponse {
	childResp := &MapResponse{}
	if p.HasChildren {
		mapper := ChildrenMapper(&p.Paragraph)
		childResp = mapper.MapChildren(req)
	}
	resp := p.Paragraph.GetTextBlocks(model.BlockContentText_Paragraph, childResp.BlockIDs, req)
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

func (h *Heading1Block) GetBlocks(req *MapRequest) *MapResponse {
	return h.Heading1.GetTextBlocks(model.BlockContentText_Header1, nil, req)
}

type Heading2Block struct {
	Block
	Heading2 HeadingObject `json:"heading_2"`
}

func (h *Heading2Block) GetBlocks(req *MapRequest) *MapResponse {
	return h.Heading2.GetTextBlocks(model.BlockContentText_Header2, nil, req)
}

type Heading3Block struct {
	Block
	Heading3 HeadingObject `json:"heading_3"`
}

func (p *Heading3Block) GetBlocks(req *MapRequest) *MapResponse {
	return p.Heading3.GetTextBlocks(model.BlockContentText_Header3, nil, req)
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

func (c *CalloutBlock) GetBlocks(req *MapRequest) *MapResponse {
	calloutResp := c.Callout.GetTextBlocks(model.BlockContentText_Callout, nil, req)
	extendedBlocks := make([]*model.Block, 0, len(calloutResp.Blocks))
	for _, cb := range calloutResp.Blocks {
		text, ok := cb.Content.(*model.BlockContentOfText)
		if !ok {
			extendedBlocks = append(extendedBlocks, cb)
			continue
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
		cb.Content = text
		extendedBlocks = append(extendedBlocks, cb)
	}
	calloutResp.Blocks = extendedBlocks
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

type QuoteBlock struct {
	Block
	Quote TextObjectWithChildren `json:"quote"`
}

func (q *QuoteBlock) GetBlocks(req *MapRequest) *MapResponse {
	childResp := &MapResponse{}
	if q.HasChildren {
		mapper := ChildrenMapper(&q.Quote)
		childResp = mapper.MapChildren(req)
	}
	resp := q.Quote.GetTextBlocks(model.BlockContentText_Quote, childResp.BlockIDs, req)
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

func (n *NumberedListBlock) GetBlocks(req *MapRequest) *MapResponse {
	childResp := &MapResponse{}
	if n.HasChildren {
		mapper := ChildrenMapper(&n.NumberedList)
		childResp = mapper.MapChildren(req)
	}
	resp := n.NumberedList.GetTextBlocks(model.BlockContentText_Numbered, childResp.BlockIDs, req)
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

func (t *ToDoBlock) GetBlocks(req *MapRequest) *MapResponse {
	childResp := &MapResponse{}
	if t.HasChildren {
		mapper := ChildrenMapper(&t.ToDo)
		childResp = mapper.MapChildren(req)
	}
	resp := t.ToDo.GetTextBlocks(model.BlockContentText_Checkbox, childResp.BlockIDs, req)
	resp.Merge(childResp)
	return resp
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

func (b *BulletedListBlock) GetBlocks(req *MapRequest) *MapResponse {
	childResp := &MapResponse{}
	if b.HasChildren {
		mapper := ChildrenMapper(&b.BulletedList)
		childResp = mapper.MapChildren(req)
	}
	resp := b.BulletedList.GetTextBlocks(model.BlockContentText_Marked, childResp.BlockIDs, req)
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

func (t *ToggleBlock) GetBlocks(req *MapRequest) *MapResponse {
	childResp := &MapResponse{}
	if t.HasChildren {
		mapper := ChildrenMapper(&t.Toggle)
		childResp = mapper.MapChildren(req)
	}
	resp := t.Toggle.GetTextBlocks(model.BlockContentText_Toggle, childResp.BlockIDs, req)
	resp.Merge(childResp)
	return resp
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

func (c *CodeBlock) GetBlocks(req *MapRequest) *MapResponse {
	id := bson.NewObjectId().Hex()
	bl := &model.Block{
		Id: id,
		Fields: &types.Struct{
			Fields: map[string]*types.Value{"lang": pbtypes.String(c.Code.Language)},
		},
	}
	marks := []*model.BlockContentTextMark{}
	var code string
	for _, rt := range c.Code.RichText {
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
	return &MapResponse{
		Blocks:   []*model.Block{bl},
		BlockIDs: []string{id},
	}
}

type EquationBlock struct {
	Block
	Equation api.EquationObject `json:"equation"`
}

func (e *EquationBlock) GetBlocks(req *MapRequest) *MapResponse {
	bl := e.Equation.HandleEquation()
	return &MapResponse{
		Blocks:   []*model.Block{bl},
		BlockIDs: []string{bl.Id},
	}
}
