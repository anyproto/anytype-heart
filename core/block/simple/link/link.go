package link

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

func init() {
	simple.RegisterCreator(NewLink)
}

func NewLink(m *model.Block) simple.Block {
	if link := m.GetLink(); link != nil {
		return &Link{
			Base:    base.NewBase(m).(*base.Base),
			content: link,
		}
	}
	return nil
}

type Block interface {
	simple.Block
	FillSmartIds(ids []string) []string
	HasSmartIds() bool
	ApplyEvent(e *pb.EventBlockSetLink) error
}

type Link struct {
	*base.Base
	content *model.BlockContentLink
}

func (l *Link) Copy() simple.Block {
	copy := pbtypes.CopyBlock(l.Model())
	return &Link{
		Base:    base.NewBase(copy).(*base.Base),
		content: copy.GetLink(),
	}
}

func (l *Link) Diff(b simple.Block) (msgs []simple.EventMessage, err error) {
	link, ok := b.(*Link)
	if !ok {
		return nil, fmt.Errorf("can't make diff with different block type")
	}
	if msgs, err = l.Base.Diff(link); err != nil {
		return
	}
	changes := &pb.EventBlockSetLink{
		Id: link.Id,
	}
	hasChanges := false

	if l.content.Style != link.content.Style {
		hasChanges = true
		changes.Style = &pb.EventBlockSetLinkStyle{Value: link.content.Style}
	}
	if l.content.TargetBlockId != link.content.TargetBlockId {
		hasChanges = true
		changes.TargetBlockId = &pb.EventBlockSetLinkTargetBlockId{Value: link.content.TargetBlockId}
	}

	if hasChanges {
		msgs = append(msgs, simple.EventMessage{Msg: &pb.EventMessage{Value: &pb.EventMessageValueOfBlockSetLink{BlockSetLink: changes}}})
	}
	return
}

func (l *Link) FillSmartIds(ids []string) []string {
	if l.content.TargetBlockId != "" {
		ids = append(ids, l.content.TargetBlockId)
	}
	return ids
}

func (l *Link) HasSmartIds() bool {
	return l.content.TargetBlockId != ""
}

func (l *Link) ApplyEvent(e *pb.EventBlockSetLink) error {
	if e.Style != nil {
		l.content.Style = e.Style.GetValue()
	}
	if e.TargetBlockId != nil {
		l.content.TargetBlockId = e.TargetBlockId.GetValue()
	}
	return nil
}
