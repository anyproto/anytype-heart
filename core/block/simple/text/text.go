package text

import (
	"fmt"
	"sort"

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
	t.content.Marks = marks
	sort.Sort(sortedMarks(t.content.Marks.Marks))
	return
}
