package link

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
)

func newListener(ctrl simple.Ctrl, linkId, targetId string) *listener {
	return &listener{
		linkId:   linkId,
		targetId: targetId,
		ctrl:     ctrl,
		done:     make(chan struct{}),
	}
}

type listener struct {
	linkId   string
	targetId string
	ctrl     simple.Ctrl
	cancel   func()
	done     chan struct{}
}

func (l *listener) listen() {
	defer close(l.done)
	ch, err := l.subscribe()
	if err != nil {
		fmt.Println("middle: link: can't subscribe to smart block:", err)
	}
	for meta := range ch {
		l.updateMeta(meta)
	}
}

func (l *listener) subscribe() (ch chan core.BlockVersionMeta, err error) {
	ch = make(chan core.BlockVersionMeta)
	block, err := l.ctrl.Anytype().GetBlock(l.targetId)
	if err != nil {
		return
	}
	vers, err := block.GetVersions("", 1, true)
	if err != nil {
		return
	}
	if len(vers) == 0 {
		err = fmt.Errorf("GetVersions returns empty version list")
		return
	}
	l.cancel, err = block.SubscribeMetaOfNewVersionsOfBlock(vers[0].Model().Id, true, ch)
	return
}

func (l *listener) updateMeta(meta core.BlockVersionMeta) {
	err := l.ctrl.UpdateBlock(l.linkId, func(b simple.Block) error {
		if link, ok := b.(*Link); ok {
			link.content.Fields = meta.ExternalFields()
		}
		return fmt.Errorf("unexpected block type; want Link; have: %T", b)
	})
	if err != nil {
		fmt.Println("middle: can't update link block:", err)
	}
}

func (l *listener) close() {
	select {
	case <-l.done:
		return
	default:
	}
	if l.cancel != nil {
		l.cancel()
		<-l.done
	}
}
