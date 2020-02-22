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
	GetText() (text string)
	SetStyle(style model.BlockContentTextStyle)
	SetChecked(v bool)
	SetTextColor(color string)
	Split(pos int32) (simple.Block, error)
	RangeSplit(from int32, to int32) ([]simple.Block, string, error)
	RangeTextPaste(from int32, to int32, newText string, newMarks []*model.BlockContentTextMark) error
	Merge(b simple.Block) error
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
		if cot.Text.Marks == nil {
			cot.Text.Marks = &model.BlockContentTextMarks{}
		}
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
	if t.content.Color != text.content.Color {
		hasChanges = true
		changes.Color = &pb.EventBlockSetTextColor{Value: text.content.Color}
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

func (t *Text) SetTextColor(color string) {
	t.content.Color = color
}

func (t *Text) SetText(text string, marks *model.BlockContentTextMarks) (err error) {
	t.content.Text = text
	if marks == nil {
		marks = &model.BlockContentTextMarks{}
	}
	t.content.Marks = marks
	t.normalizeMarks()
	return
}

func (t *Text) GetText() (text string) {
	return t.content.Text
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
					From: 0, // Sure? @enkogu
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

// TODO: should be 100% tested @enkogu
func (t *Text) RangeSplit(from int32, to int32) ([]simple.Block, string, error) {
	if from < 0 || int(from) > utf8.RuneCountInString(t.content.Text) {
		return nil, "", ErrOutOfRange
	}
	if to < 0 || int(to) > utf8.RuneCountInString(t.content.Text) {
		return nil, "", ErrOutOfRange
	}
	if from > to {
		return nil, "", ErrOutOfRange // Maybe different error? @enkogu
	}
	var newBlocks []simple.Block
	// special cases
	if from == 0 && to == 0 {
		return newBlocks, t.content.Text, nil
	}

	runes := []rune(t.content.Text)
	t.content.Text = string(runes[:from])
	if t.content.Marks == nil {
		t.content.Marks = &model.BlockContentTextMarks{}
	}

	newMarks := &model.BlockContentTextMarks{}
	oldMarks := &model.BlockContentTextMarks{}
	r := model.Range{From:from, To:to}
	oldMarks.Marks, newMarks.Marks = t.splitMarks(t.content.Marks.Marks, &r, 0)
	// TODO:

	t.content.Marks = oldMarks
	newBlock := NewText(&model.Block{
		Content: &model.BlockContentOfText{Text: &model.BlockContentText{
			Text:    string(runes[to:]),
			Style:   t.content.Style,
			Marks:   newMarks,
			Checked: t.content.Checked,
		}},
	})

	// if oldBlock is empty and newBlock is non empty -> replace content
	if len(string(runes[:from])) == 0 {
		t.content.Text = string(runes[to:])

		// if newBlock is empty -> don't push it
	} else if len(string(runes[to:])) > 0 {
		newBlocks = append(newBlocks, newBlock)
	}
	return newBlocks, t.content.Text, nil
}

 func Abs (x int32) int32 {
	if x < 0 { return -x }
	return x
}

func (t *Text) splitMarks (marks []*model.BlockContentTextMark, r *model.Range, textLength int32) (topMarks []*model.BlockContentTextMark, botMarks []*model.BlockContentTextMark) {
	for i := 0; i < len(marks); i++ {
		m := marks[i]

		// <b>lorem</b> lorem (**********)  :--->   <b>lorem</b> lorem __PASTE__
		if (m.Range.From < r.From) && (m.Range.To <= r.From) {
			topMarks = append(topMarks, &model.BlockContentTextMark{
				Range: &model.Range{
					From: m.Range.From,
					To:   m.Range.To,
				},
				Type:  m.Type,
				Param: m.Param,
			})
		} else

		// <b>lorem lorem(******</b>******)  :--->   <b>lorem lorem</b> __PASTE__
		if (m.Range.From < r.From) && (m.Range.To > r.From) && (m.Range.To < r.To) {
			topMarks = append(topMarks, &model.BlockContentTextMark{
				Range: &model.Range{
					From: m.Range.From,
					To:   r.From,
				},
				Type:  m.Type,
				Param: m.Param,
			})

		} else

		// (**<b>******</b>******)  :--->     __PASTE__
		if (m.Range.From >= r.From) && (m.Range.To <= r.To) {
			continue
		} else

		// <b>lorem (*********) lorem</b>  :--->   <b>lorem</b> __PASTE__ <b>lorem</b>
		if (m.Range.From < r.From) && (m.Range.To > r.To) {
			topMarks = append(topMarks, &model.BlockContentTextMark{
				Range: &model.Range{
					From: m.Range.From,
					To:   r.From,
				},
				Type:  m.Type,
				Param: m.Param,
			})

			botMarks = append(botMarks, &model.BlockContentTextMark{
				Range: &model.Range{
					From: r.From + textLength,
					To:   m.Range.To - (r.To - r.From) + textLength,
				},
				Type:  m.Type,
				Param: m.Param,
			})

		} else
		// (*********) <b>lorem lorem</b>  :--->   __PASTE__ <b>lorem lorem</b>
		if (m.Range.From >= r.To) {
			botMarks = append(botMarks, &model.BlockContentTextMark{
				Range: &model.Range{
					From: m.Range.From  - (r.To - r.From) + textLength,
					To:   m.Range.To - (r.To - r.From) + textLength,
				},
				Type:  m.Type,
				Param: m.Param,
			})
		}
	}
	return topMarks, botMarks
}


func (t *Text) RangeTextPaste(from int32, to int32, newText string, newMarks []*model.BlockContentTextMark) error {
	if from < 0 || int(from) > utf8.RuneCountInString(t.content.Text) {
		return ErrOutOfRange
	}
	if to < 0 || int(to) > utf8.RuneCountInString(t.content.Text) {
		return ErrOutOfRange
	}
	if from > to {
		return ErrOutOfRange // Maybe different error? @enkogu
	}

	var combinedMarks []*model.BlockContentTextMark

	addLen := int32(utf8.RuneCountInString(newText))

	for _, mark := range newMarks {
		mark.Range.From = mark.Range.From + from
		mark.Range.To = mark.Range.To + from

		combinedMarks = append(combinedMarks, mark)
	}

	leftMarks := &model.BlockContentTextMarks{}
	rightMarks := &model.BlockContentTextMarks{}
	r := model.Range{From:from, To:to}
	leftMarks.Marks, rightMarks.Marks = t.splitMarks(t.content.Marks.Marks, &r, addLen)
//....
	for _, mark := range leftMarks.Marks {
		combinedMarks = append(combinedMarks, mark)
	}

	for _, mark := range rightMarks.Marks {
		combinedMarks = append(combinedMarks, mark)
	}

	runes := []rune(t.content.Text)
	t.content.Text = string(runes[:from]) + newText + string(runes[to:])
	t.content.Marks = &model.BlockContentTextMarks{ Marks: combinedMarks }
	return nil
}


func (t *Text) Merge(b simple.Block) error {
	text, ok := b.(*Text)
	if ! ok {
		return fmt.Errorf("unexpected block type for merge: %T", b)
	}
	curLen := int32(utf8.RuneCountInString(t.content.Text))
	t.content.Text += text.content.Text
	for _, m := range text.content.Marks.Marks {
		t.content.Marks.Marks = append(t.content.Marks.Marks, &model.BlockContentTextMark{
			Range: &model.Range{
				From: m.Range.From + curLen,
				To:   m.Range.To + curLen,
			},
			Type:  m.Type,
			Param: m.Param,
		})
	}
	t.normalizeMarks()
	return nil
}

func (t *Text) normalizeMarks() {
	sort.Sort(sortedMarks(t.content.Marks.Marks))
	for i := 0; i < len(t.content.Marks.Marks); i++ {
		if i+1 == len(t.content.Marks.Marks) {
			break
		}
		m := t.content.Marks.Marks[i]
		sm := t.content.Marks.Marks[i+1]
		if m.Type == sm.Type && m.Param == sm.Param && m.Range.To >= sm.Range.From {
			m.Range.To = sm.Range.To
			t.content.Marks.Marks[i+1] = nil
			t.content.Marks.Marks = append(t.content.Marks.Marks[:i+1], t.content.Marks.Marks[i+2:]...)
			i = -1
		}
	}
}
