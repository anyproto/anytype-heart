package collection

import (
	"github.com/anyproto/anytype-heart/core/block/backlinks"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/util/internalflag"
	"github.com/anyproto/anytype-heart/util/slice"
)

type component struct {
	smartblock.SmartBlock

	backlinksUpdater backlinks.UpdateWatcher
	subscriptions    map[string]chan []string
}

type Collection interface {
	AddToCollection(ctx session.Context, req *pb.RpcObjectCollectionAddRequest) error
	RemoveFromCollection(ctx session.Context, req *pb.RpcObjectCollectionRemoveRequest) error
	ReorderCollection(ctx session.Context, req *pb.RpcObjectCollectionSortRequest) error
	SubscribeForCollection(subscriptionId string) ([]string, <-chan []string, error)
	UnsubscribeFromCollection(subscriptionId string)
}

func New(sb smartblock.SmartBlock, backlinksUpdater backlinks.UpdateWatcher) Collection {
	return &component{
		SmartBlock:       sb,
		backlinksUpdater: backlinksUpdater,
		subscriptions:    map[string]chan []string{},
	}
}

func (c *component) AddToCollection(ctx session.Context, req *pb.RpcObjectCollectionAddRequest) error {
	var toAdd []string
	err := c.updateCollection(ctx, func(col []string) []string {
		toAdd = slice.Difference(req.ObjectIds, col)
		pos := slice.FindPos(col, req.AfterId)
		if pos >= 0 {
			col = slice.Insert(col, pos+1, toAdd...)
		} else {
			col = append(toAdd, col...)
		}
		return col
	})
	if err != nil {
		return err
	}

	// we update backlinks of objects added to collection synchronously to avoid object rerender after backlinks accumulation interval
	if len(toAdd) != 0 {
		c.backlinksUpdater.FlushUpdates()
	}

	return nil
}

func (c *component) RemoveFromCollection(ctx session.Context, req *pb.RpcObjectCollectionRemoveRequest) error {
	return c.updateCollection(ctx, func(col []string) []string {
		col = slice.Filter(col, func(id string) bool {
			return slice.FindPos(req.ObjectIds, id) == -1
		})
		return col
	})
}

func (c *component) ReorderCollection(ctx session.Context, req *pb.RpcObjectCollectionSortRequest) error {
	return c.updateCollection(ctx, func(col []string) []string {
		exist := map[string]struct{}{}
		for _, id := range col {
			exist[id] = struct{}{}
		}
		col = col[:0]
		for _, id := range req.ObjectIds {
			// Reorder only existing objects
			if _, ok := exist[id]; ok {
				col = append(col, id)
			}
		}
		return col
	})
}

func (c *component) SubscribeForCollection(subscriptionId string) ([]string, <-chan []string, error) {
	var initialObjectIDs []string

	st := c.NewState()
	c.collectionAddHookOnce()

	initialObjectIDs = st.GetStoreSlice(template.CollectionStoreKey)

	ch, ok := c.subscriptions[subscriptionId]
	if !ok {
		ch = make(chan []string)
		c.subscriptions[subscriptionId] = ch
	}

	return initialObjectIDs, ch, nil
}

func (c *component) UnsubscribeFromCollection(subscriptionId string) {
	ch, ok := c.subscriptions[subscriptionId]
	if ok {
		close(ch)
		delete(c.subscriptions, subscriptionId)
	}
}

type Subscription struct {
	objectsCh chan []string
	closeCh   chan struct{}
}

func (s *Subscription) Chan() <-chan []string {
	return s.objectsCh
}

func (s *Subscription) Close() {
	close(s.closeCh)
}

func (c *component) updateCollection(ctx session.Context, modifier func(src []string) []string) error {
	st := c.NewStateCtx(ctx)
	c.collectionAddHookOnce()
	lst := st.GetStoreSlice(template.CollectionStoreKey)
	lst = modifier(lst)
	st.UpdateStoreSlice(template.CollectionStoreKey, lst)
	// TODO why we're adding empty list of flags?
	flags := internalflag.Set{}
	flags.AddToState(st)
	return c.Apply(st, smartblock.KeepInternalFlags)
}

func (c *component) collectionAddHookOnce() {
	c.AddHookOnce("collection", func(info smartblock.ApplyInfo) (err error) {
		for _, ch := range info.Changes {
			if upd := ch.GetStoreSliceUpdate(); upd != nil && upd.Key == template.CollectionStoreKey {
				c.broadcast(info.State.GetStoreSlice(template.CollectionStoreKey))
				return nil
			}
		}
		return nil
	}, smartblock.HookAfterApply)
}

func (c *component) broadcast(objectIDs []string) {
	for _, ch := range c.subscriptions {
		ch <- objectIDs
	}
}
