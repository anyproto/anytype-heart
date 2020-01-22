package link

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
)

func newListener(ctrl simple.Ctrl, linkId, targetId string) *metaListener {
	return &metaListener{
		linkId:   linkId,
		targetId: targetId,
		ctrl:     ctrl,
		done:     make(chan struct{}),
	}
}

type metaListener struct {
	linkId   string
	targetId string
	ctrl     simple.Ctrl
	cancel   func()
	done     chan struct{}
}

func (l *metaListener) listen() {
	defer func() {
		close(l.done)
		fmt.Println("middle: link: unsubscribe for:", l.targetId)
	}()
	ch, err := l.subscribe()
	if err != nil {
		fmt.Println("middle: link: can't subscribe to smart block:", err)
		return
	}
	for meta := range ch {
		l.updateMeta(meta)
	}
}

func (l *metaListener) subscribe() (ch chan core.BlockVersionMeta, err error) {
	ch = make(chan core.BlockVersionMeta)
	block, err := l.ctrl.Anytype().GetBlock(l.targetId)
	if err != nil {
		err = fmt.Errorf("GetBlock error: %v", err)
		return
	}
	verId, err := block.GetCurrentVersionId()
	if err != nil {
		err = fmt.Errorf("GetCurrentVersionId error: %v", err)
		return
	}
	l.cancel, err = block.SubscribeMetaOfNewVersionsOfBlock(verId, true, ch)
	return
}

func (l *metaListener) updateMeta(meta core.BlockVersionMeta) {
	err := l.ctrl.UpdateBlock(l.linkId, func(b simple.Block) error {
		if link, ok := b.(*Link); ok {
			link.content.Fields = meta.ExternalFields()
			return nil
		}
		return fmt.Errorf("unexpected block type; want Link; have: %T", b)
	})
	if err != nil {
		fmt.Println("middle: can't update link block:", err)
	}
}

func (l *metaListener) close() {
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
