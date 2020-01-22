package link

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/mohae/deepcopy"
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
	simple.BlockInit
	link()
}

type Link struct {
	*base.Base
	content  *model.BlockContentLink
	listener *metaListener
}

func (l *Link) Init(ctrl simple.Ctrl) {
	l.listener = newListener(ctrl, l.Model().Id, l.content.TargetBlockId)
	go l.listener.listen()
}

func (l *Link) Copy() simple.Block {
	copy := deepcopy.Copy(l.Model()).(*model.Block)
	return &Link{
		Base:    base.NewBase(copy).(*base.Base),
		content: copy.GetLink(),
	}
}

func (l *Link) Diff(b simple.Block) (msgs []*pb.EventMessage, err error) {
	link, ok := b.(*Link)
	if ! ok {
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
	if !l.content.Fields.Equal(link.content.Fields) {
		hasChanges = true
		changes.Fields = &pb.EventBlockSetLinkFields{Value: link.content.Fields}
	}
	if l.content.TargetBlockId != link.content.TargetBlockId {
		hasChanges = true
		changes.TargetBlockId = &pb.EventBlockSetLinkTargetBlockId{Value: link.content.TargetBlockId}
	}

	if hasChanges {
		msgs = append(msgs, &pb.EventMessage{Value: &pb.EventMessageValueOfBlockSetLink{BlockSetLink: changes}})
	}
	return
}

func (l *Link) link() {}

func (l *Link) Close() {
	if l.listener != nil {
		l.listener.close()
	}
}
