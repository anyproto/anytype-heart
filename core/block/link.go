package block

import (
	"fmt"
	"sync"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/link"
	"github.com/gogo/protobuf/types"
)

type metaInfo struct {
	targetId string
	meta     core.BlockVersionMeta
}

type linkBlockUpdater interface {
	UpdateBlock(linkIds []string, hist bool, apply func(b simple.Block) error) error
}

type linkSubscriptionsOp int

const (
	linkSubscriptionsOpCreate = iota
	linkSubscriptionsOpChange
	linkSubscriptionsOpDelete
)

type linkBlockAction struct {
	op      linkSubscriptionsOp
	link    link.Block
	updater linkBlockUpdater
}

func newLinkSubscriptions(a anytype.Anytype) *linkSubscriptions {
	ls := &linkSubscriptions{
		anytype:   a,
		links:     make(map[string]map[linkBlockUpdater][]string),
		listeners: make(map[string]*linkListener),
		metaCache: make(map[string]*linkData),
		actionCh:  make(chan linkBlockAction, 100),
		metaCh:    make(chan metaInfo, 10),
		closeCh:   make(chan struct{}),
	}
	go ls.loop()
	return ls
}

type linkData struct {
	fields     *types.Struct
	isArchived bool
}

type linkSubscriptions struct {
	anytype anytype.Anytype
	// targetId -> smartBlock -> []linkId
	links     map[string]map[linkBlockUpdater][]string
	listeners map[string]*linkListener
	actionCh  chan linkBlockAction
	metaCh    chan metaInfo
	closeCh   chan struct{}
	closeOnce sync.Once

	metaCache map[string]*linkData
	m         sync.Mutex
}

func (ls *linkSubscriptions) onCreate(u linkBlockUpdater, b simple.Block) {
	if link, ok := b.(link.Block); ok {
		linkContent := link.Model().GetLink()
		if data := ls.getMetaCache(linkContent.TargetBlockId); data != nil {
			linkContent.Fields = data.fields
			linkContent.IsArchived = data.isArchived
		}
		ls.actionCh <- linkBlockAction{
			op:      linkSubscriptionsOpCreate,
			link:    link,
			updater: u,
		}
	}
}

func (ls *linkSubscriptions) onChange(u linkBlockUpdater, b simple.Block) {
	if link, ok := b.(link.Block); ok {
		ls.actionCh <- linkBlockAction{
			op:      linkSubscriptionsOpChange,
			link:    link,
			updater: u,
		}
	}
}

func (ls *linkSubscriptions) onDelete(u linkBlockUpdater, b simple.Block) {
	if link, ok := b.(link.Block); ok {
		ls.actionCh <- linkBlockAction{
			op:      linkSubscriptionsOpDelete,
			link:    link,
			updater: u,
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
				ls.create(action.updater, action.link)
			case linkSubscriptionsOpChange:
				ls.change(action.updater, action.link)
			case linkSubscriptionsOpDelete:
				ls.delete(action.updater, action.link)
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

func (ls *linkSubscriptions) create(u linkBlockUpdater, l link.Block) {
	linkId := l.Model().Id
	targetId := l.Model().GetLink().TargetBlockId
	fmt.Println("add link to subscriber", linkId)
	ls.startListener(targetId)
	updaters := ls.links[targetId]
	if updaters == nil {
		updaters = make(map[linkBlockUpdater][]string)
		ls.links[targetId] = updaters
	}
	linkIds := ls.links[targetId][u]
	linkIds = append(linkIds, linkId)
	ls.links[targetId][u] = linkIds
}

func (ls *linkSubscriptions) change(u linkBlockUpdater, l link.Block) {
	linkId := l.Model().Id
	targetId := l.Model().GetLink().TargetBlockId
	updaters := ls.links[targetId]
	if updaters == nil {
		updaters = make(map[linkBlockUpdater][]string)
		ls.links[targetId] = updaters
	}
	linkIds := ls.links[targetId][u]
	if pos := findPosInSlice(linkIds, linkId); pos != -1 {
		// target id not changed - do nothing
		return
	}
	// find and remove old link
	for targetId, upds := range ls.links {
		if lIds, ok := upds[u]; ok {
			if pos := findPosInSlice(lIds, linkId); pos != -1 {
				lIds = removeFromSlice(lIds, linkId)
				ls.links[targetId][u] = lIds
			}
		}
	}
	ls.create(u, l)
	ls.closeUnused()
}

func (ls *linkSubscriptions) delete(u linkBlockUpdater, l link.Block) {
	linkId := l.Model().Id
	targetId := l.Model().GetLink().TargetBlockId
	updaters := ls.links[targetId]
	if updaters == nil {
		updaters = make(map[linkBlockUpdater][]string)
		ls.links[targetId] = updaters
	}
	linkIds := ls.links[targetId][u]
	ls.links[targetId][u] = removeFromSlice(linkIds, linkId)
	ls.closeUnused()
}

func (ls *linkSubscriptions) closeUnused() {
	for targetId, upds := range ls.links {
		var count int
		for _, lIds := range upds {
			count += len(lIds)
		}
		if count == 0 {
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
	if _, ok := ls.listeners[targetId]; ok {
		fmt.Println("stop listener for:", targetId)
		ls.listeners[targetId].close()
		delete(ls.listeners, targetId)
	}
}

func (ls *linkSubscriptions) setMeta(info metaInfo) {
	fmt.Println("middle: update link meta for", info.targetId)
	isArchived := false
	if data := info.meta.Model(); data != nil {
		isArchived = data.IsArchived
	}
	ls.setMetaCache(info.targetId, &linkData{fields: info.meta.ExternalFields(), isArchived: isArchived})
	updaters := ls.links[info.targetId]
	if updaters == nil {
		return
	}
	for u, linkIds := range updaters {
		err := u.UpdateBlock(linkIds, false, func(b simple.Block) error {
			if l, ok := b.(link.Block); ok {
				l.SetMeta(info.meta)
			}
			return nil
		})
		if err != nil {
			fmt.Println("middle: can't set updated meta to block:", err)
		}
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

func (ls *linkSubscriptions) getMetaCache(id string) *linkData {
	ls.m.Lock()
	defer ls.m.Unlock()
	return ls.metaCache[id]
}

func (ls *linkSubscriptions) setMetaCache(id string, data *linkData) {
	ls.m.Lock()
	defer ls.m.Unlock()
	ls.metaCache[id] = data
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
	block, err := ll.ls.anytype.GetBlock(ll.targetId)
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
