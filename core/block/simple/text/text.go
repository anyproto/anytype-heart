package text

import (
	"fmt"
	"sort"
	"unicode/utf8"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/mohae/deepcopy"
)

var (
	ErrOutOfRange = fmt.Errorf("out of range")
)

func init() {
	simple.RegisterCreator(func(m *model.Block) simple.Block {
		if _, err := toTextContent(m.Content); err != nil {
			return nil
		}
		return NewText(m)
	})
}

func NewText(block *model.Block) simple.Block {
	tc := mustTextContent(block.Content)
	t := &Text{Base: base.NewBase(block).(*base.Base), content: tc}
	return t
}

type Block interface {
	simple.Block
	SetText(text string, marks *model.BlockContentTextMarks) (err error)
	SetStyle(style model.BlockContentTextStyle)
	SetChecked(v bool)
	Split(pos int32) (simple.Block, error)
}

type Text struct {
	*base.Base
	content *model.BlockContentText
}

func mustTextContent(content model.IsBlockContent) *model.BlockContentText {
	res, err := toTextContent(content)
	if err != nil {
		panic(err)
	}
	return res
}

func toTextContent(content model.IsBlockContent) (textContent *model.BlockContentText, err error) {
	if cot, ok := content.(*model.BlockContentOfText); ok {
		return cot.Text, nil
	}
	return nil, fmt.Errorf("unexpected content type: %T; want text", content)
}

func (t *Text) Copy() simple.Block {
	return NewText(deepcopy.Copy(t.Model()).(*model.Block))
}

func (t *Text) Diff(b simple.Block) (msgs []*pb.EventMessage, err error) {
	text, ok := b.(*Text)
	if ! ok {
		return nil, fmt.Errorf("can't make diff with different block type")
	}
	if msgs, err = t.Base.Diff(text); err != nil {
		return
	}
	changes := &pb.EventBlockSetText{
		Id: text.Id,
	}
	hasChanges := false

	if t.content.Text != text.content.Text {
		hasChanges = true
		changes.Text = &pb.EventBlockSetTextText{Value: text.content.Text}
	}
	if t.content.Style != text.content.Style {
		hasChanges = true
		changes.Style = &pb.EventBlockSetTextStyle{Value: text.content.Style}
	}
	if t.content.Checked != text.content.Checked {
		hasChanges = true
		changes.Checked = &pb.EventBlockSetTextChecked{Value: text.content.Checked}
	}
	if !marksEq(t.content.Marks, text.content.Marks) {
		hasChanges = true
		changes.Marks = &pb.EventBlockSetTextMarks{Value: text.content.Marks}
	}
	if hasChanges {
		msgs = append(msgs, &pb.EventMessage{Value: &pb.EventMessageValueOfBlockSetText{BlockSetText: changes}})
	}
	return
}

func (t *Text) SetStyle(style model.BlockContentTextStyle) {
	t.content.Style = style
}

func (t *Text) SetChecked(v bool) {
	t.content.Checked = v
}

func (t *Text) SetText(text string, marks *model.BlockContentTextMarks) (err error) {
	t.content.Text = text
	if marks == nil {
		marks = &model.BlockContentTextMarks{}
	}
	t.content.Marks = marks
	sort.Sort(sortedMarks(t.content.Marks.Marks))
	return
}

func (t *Text) Split(pos int32) (simple.Block, error) {
	if pos < 0 || int(pos) >= utf8.RuneCountInString(t.content.Text) {
		return nil, ErrOutOfRange
	}
	runes := []rune(t.content.Text)
	t.content.Text = string(runes[:pos])
	if t.content.Marks == nil {
		t.content.Marks = &model.BlockContentTextMarks{}
	}
	newMarks := &model.BlockContentTextMarks{}
	oldMarks := &model.BlockContentTextMarks{}
	for _, mark := range t.content.Marks.Marks {
		if mark.Range.From >= pos {
			mark.Range.From -= pos
			mark.Range.To -= pos
			newMarks.Marks = append(newMarks.Marks, mark)
		} else if mark.Range.To <= pos {
			oldMarks.Marks = append(oldMarks.Marks, mark)
		} else {
			newMark := &model.BlockContentTextMark{
				Range: &model.Range{
					From: 0,
					To:   mark.Range.To - pos,
				},
				Type:  mark.Type,
				Param: mark.Param,
			}
			newMarks.Marks = append(newMarks.Marks, newMark)
			mark.Range.To = pos
			oldMarks.Marks = append(oldMarks.Marks, mark)
		}
	}
	t.content.Marks = oldMarks
	newBlock := NewText(&model.Block{
		Content: &model.BlockContentOfText{Text: &model.BlockContentText{
			Text:    string(runes[pos:]),
			Style:   t.content.Style,
			Marks:   newMarks,
			Checked: t.content.Checked,
		}},
	})
	return newBlock, nil
}
