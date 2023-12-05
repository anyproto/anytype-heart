package embed

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/base"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func init() {
	simple.RegisterCreator(NewEmbed)
}

func NewEmbed(m *model.Block) simple.Block {
	if embed := m.GetLatex(); embed != nil {
		return &Embed{
			Base:    base.NewBase(m).(*base.Base),
			content: embed,
		}
	}
	return nil
}

type Block interface {
	simple.Block
	SetText(text string)
	ApplyEvent(e *pb.EventBlockSetEmbed) error
}

var _ Block = (*Embed)(nil)

type Embed struct {
	*base.Base
	content *model.BlockContentEmbed
}

func (l *Embed) Copy() simple.Block {
	copy := pbtypes.CopyBlock(l.Model())
	return &Embed{
		Base:    base.NewBase(copy).(*base.Base),
		content: copy.GetLatex(),
	}
}

// Validate TODO: add validation rules
func (l *Embed) Validate() error {
	return nil
}

func (l *Embed) Diff(b simple.Block) (msgs []simple.EventMessage, err error) {
	embed, ok := b.(*Embed)
	if !ok {
		return nil, fmt.Errorf("can't make diff with different block type")
	}
	if msgs, err = l.Base.Diff(embed); err != nil {
		return
	}
	changes := &pb.EventBlockSetEmbed{
		Id: embed.Id,
	}
	hasChanges := false

	if l.content.Text != embed.content.Text {
		hasChanges = true
		changes.Text = &pb.EventBlockSetEmbedText{Value: embed.content.Text}
	}

	if l.content.Processor != embed.content.Processor {
		hasChanges = true
		changes.Processor = &pb.EventBlockSetEmbedProcessor{Value: embed.content.Processor}
	}

	if hasChanges {
		msgs = append(msgs, simple.EventMessage{Msg: &pb.EventMessage{Value: &pb.EventMessageValueOfBlockSetEmbed{BlockSetEmbed: changes}}})
	}
	return
}

func (r *Embed) SetText(text string) {
	r.content.Text = text
}

func (r *Embed) SetProcessor(processor model.BlockContentEmbedProcessor) {
	r.content.Processor = processor
}

func (l *Embed) ApplyEvent(e *pb.EventBlockSetEmbed) error {
	if e.Text != nil {
		l.content.Text = e.Text.GetValue()
	}
	if e.Processor != nil {
		l.content.Processor = e.Processor.GetValue()
	}
	return nil
}
