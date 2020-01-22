package block

import (
	"fmt"
	"sync"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/link"
)

type metaInfo struct {
	targetId string
	meta     core.BlockVersionMeta
}

type linkSubscriptionsOp int

const (
	linkSubscriptionsOpCreate = iota
	linkSubscriptionsOpChange
	linkSubscriptionsOpDelete
)

type linkBlockAction struct {
	op   linkSubscriptionsOp
	link link.Block
}

func newLinkSubscriptions(sb smartBlock) *linkSubscriptions {
	ls := &linkSubscriptions{
		sb:        sb,
		links:     make(map[string][]string),
		listeners: make(map[string]*linkListener),
		actionCh:  make(chan linkBlockAction, 100),
		metaCh:    make(chan metaInfo),
		closeCh:   make(chan struct{}),
	}
	go ls.loop()
	return ls
}

type linkSubscriptions struct {
	sb smartBlock
	// targetId -> []linkId
	links     map[string][]string
	listeners map[string]*linkListener
	actionCh  chan linkBlockAction
	metaCh    chan metaInfo
	closeCh   chan struct{}
	closeOnce sync.Once
}

func (ls *linkSubscriptions) onCreate(b simple.Block) {
	if link, ok := b.(link.Block); ok {
		ls.actionCh <- linkBlockAction{
			op:   linkSubscriptionsOpCreate,
			link: link,
		}
	}
}

func (ls *linkSubscriptions) onChange(b simple.Block) {
	if link, ok := b.(link.Block); ok {
		ls.actionCh <- linkBlockAction{
			op:   linkSubscriptionsOpChange,
			link: link,
		}
	}
}

func (ls *linkSubscriptions) onDelete(b simple.Block) {
	if link, ok := b.(link.Block); ok {
		ls.actionCh <- linkBlockAction{
			op:   linkSubscriptionsOpDelete,
			link: link,
		}
	}
}

func (ls *linkSubscriptions) onMeta(info metaInfo) {
	ls.metaCh <- info
}

func (ls *linkSubscriptions) loop() {
	defer close(ls.closeCh)
	for {
		select {
		case action := <-ls.actionCh:
			switch action.op {
			case linkSubscriptionsOpCreate:
				ls.create(action.link)
			case linkSubscriptionsOpChange:
				ls.change(action.link)
			case linkSubscriptionsOpDelete:
				ls.delete(action.link)
			}
		case info := <-ls.metaCh:
			ls.setMeta(info)
		case <-ls.closeCh:
			for _, ll := range ls.listeners {
				ll.close()
			}
			return
		}
	}
}

func (ls *linkSubscriptions) create(l link.Block) {
	linkId := l.Model().Id
	targetId := l.Model().GetLink().TargetBlockId
	fmt.Println("add link to subscriber", linkId)
	ls.startListener(targetId)
	linkIds := ls.links[targetId]
	linkIds = append(linkIds, linkId)
	ls.links[targetId] = linkIds
}

func (ls *linkSubscriptions) change(l link.Block) {
	linkId := l.Model().Id
	targetId := l.Model().GetLink().TargetBlockId
	linkIds := ls.links[targetId]
	if pos := findPosInSlice(linkIds, linkId); pos != -1 {
		// target id not changed - do nothing
		return
	}
	// find and remove old link
	for targetId, lIds := range ls.links {
		if pos := findPosInSlice(linkIds, linkId); pos != -1 {
			lIds = removeFromSlice(lIds, linkId)
			ls.links[targetId] = lIds
		}
	}
	ls.create(l)
	ls.closeUnused()
}

func (ls *linkSubscriptions) delete(l link.Block) {
	linkId := l.Model().Id
	targetId := l.Model().GetLink().TargetBlockId
	linkIds := ls.links[targetId]
	ls.links[targetId] = removeFromSlice(linkIds, linkId)
	ls.closeUnused()
}

func (ls *linkSubscriptions) closeUnused() {
	for targetId, lIds := range ls.links {
		if len(lIds) == 0 {
			ls.stopListener(targetId)
			delete(ls.links, targetId)
		}
	}
}

func (ls *linkSubscriptions) startListener(targetId string) {
	if _, ok := ls.listeners[targetId]; ok {
		return
	}
	fmt.Println("start listener for:", targetId)
	listener, err := newLinkListener(targetId, ls)
	if err != nil {
		fmt.Println("middle: can't create link listener:", err)
		return
	}
	ls.listeners[targetId] = listener
}

func (ls *linkSubscriptions) stopListener(targetId string) {
	fmt.Println("stop listener for:", targetId)
	ls.listeners[targetId].close()
	delete(ls.listeners, targetId)
}

func (ls *linkSubscriptions) setMeta(info metaInfo) {
	fmt.Println("middle: update link meta for", info.targetId)
	linkIds := ls.links[info.targetId]
	if len(linkIds) == 0 {
		return
	}
	err := ls.sb.UpdateBlock(linkIds, func(b simple.Block) error {
		if l, ok := b.(link.Block); ok {
			l.SetMeta(info.meta)
		}
		return nil
	})
	if err != nil {
		fmt.Println("middle: can't set updated meta to block:", err)
	}
}

func (ls *linkSubscriptions) close() {
	ls.closeOnce.Do(func() {
		select {
		case <-ls.closeCh:
			return
		default:
			ls.closeCh <- struct{}{}
			<-ls.closeCh
		}
	})
}

func newLinkListener(targetId string, ls *linkSubscriptions) (ll *linkListener, err error) {
	ll = &linkListener{
		ls:       ls,
		targetId: targetId,
		ch:       make(chan core.BlockVersionMeta),
		done:     make(chan struct{}),
	}
	if err = ll.subscribe(); err != nil {
		return
	}
	go ll.listen()
	return
}

type linkListener struct {
	ls       *linkSubscriptions
	targetId string
	ch       chan core.BlockVersionMeta
	cancel   func()
	done     chan struct{}
}

func (ll *linkListener) listen() {
	for meta := range ll.ch {
		select {
		case <-ll.done:
		default:
			ll.ls.onMeta(metaInfo{meta: meta, targetId: ll.targetId})
		}
	}
}

func (ll *linkListener) subscribe() (err error) {
	block, err := ll.ls.sb.Anytype().GetBlock(ll.targetId)
	if err != nil {
		err = fmt.Errorf("linkListener anytype.GetBlock(%s) error: %v", ll.targetId, err)
		return
	}
	verId, err := block.GetCurrentVersionId()
	if err != nil {
		err = fmt.Errorf("linkListener block.GetCurrentVersionId() error: %v", err)
		return
	}
	ll.cancel, err = block.SubscribeMetaOfNewVersionsOfBlock(verId, true, ll.ch)
	if err != nil {
		err = fmt.Errorf("linkListener block.SubscribeMetaOfNewVersionsOfBlock(%s) error: %v", verId, err)
		return
	}
	return
}

func (ll *linkListener) close() {
	select {
	case <-ll.done:
	default:
		close(ll.done)
		ll.cancel()
	}
}
