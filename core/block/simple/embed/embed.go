package embed

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/base"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func init() {
	simple.RegisterCreator(NewLatex)
}

func NewLatex(m *model.Block) simple.Block {
	if embed := m.GetLatex(); embed != nil {
		return &Latex{
			Base:    base.NewBase(m).(*base.Base),
			content: embed,
		}
	}
	return nil
}

type Block interface {
	simple.Block
	SetText(text string)
	ApplyEvent(e *pb.EventBlockSetLatex) error
}

var _ Block = (*Latex)(nil)

type Latex struct {
	*base.Base
	content *model.BlockContentLatex
}

func (l *Latex) Copy() simple.Block {
	copy := pbtypes.CopyBlock(l.Model())
	return &Latex{
		Base:    base.NewBase(copy).(*base.Base),
		content: copy.GetLatex(),
	}
}

// Validate TODO: add validation rules
func (l *Latex) Validate() error {
	return nil
}

func (l *Latex) Diff(spaceId string, b simple.Block) (msgs []simple.EventMessage, err error) {
	embed, ok := b.(*Latex)
	if !ok {
		return nil, fmt.Errorf("can't make diff with different block type")
	}
	if msgs, err = l.Base.Diff(spaceId, embed); err != nil {
		return
	}
	changes := &pb.EventBlockSetLatex{
		Id: embed.Id,
	}
	hasChanges := false

	if l.content.Text != embed.content.Text {
		hasChanges = true
		changes.Text = &pb.EventBlockSetLatexText{Value: embed.content.Text}
	}

	if l.content.Processor != embed.content.Processor {
		hasChanges = true
		changes.Processor = &pb.EventBlockSetLatexProcessor{Value: embed.content.Processor}
	}

	if hasChanges {
		msgs = append(msgs, simple.EventMessage{Msg: event.NewMessage(spaceId, &pb.EventMessageValueOfBlockSetLatex{BlockSetLatex: changes})})
	}
	return
}

func (r *Latex) SetText(text string) {
	r.content.Text = text
}

func (r *Latex) SetProcessor(processor model.BlockContentLatexProcessor) {
	r.content.Processor = processor
}

func (l *Latex) ApplyEvent(e *pb.EventBlockSetLatex) error {
	if e.Text != nil {
		l.content.Text = e.Text.GetValue()
	}
	if e.Processor != nil {
		l.content.Processor = e.Processor.GetValue()
	}
	return nil
}

func (l *Latex) IsEmpty() bool {
	return l.content.Text == ""
}
