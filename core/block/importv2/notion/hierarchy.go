package notion

import (
	"context"
	"sort"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

// childLink is the block that links a child entity from the page it lives
// in, as mapChildEntity actually emitted it.
type childLink struct {
	pageId  string
	blockId string
	// holdsParentBlock: the child is block-parented and its parent block is
	// in this page's tree — the evidence that this page is where it lives.
	holdsParentBlock bool
}

// noteChildLink records a child_page/child_database link emitted for a
// child entity — every page's first one, since only some pages can be the
// child's parent (a synced-block duplicate links it too, from a page that
// does not hold its parent block). Runs on the converter goroutine (mapping
// happens at emit).
func (c *Converter) noteChildLink(mctx mapContext, childId, blockId string) {
	for _, link := range c.childLinks[childId] {
		if link.pageId == mctx.pageId {
			return
		}
	}
	link := childLink{pageId: mctx.pageId, blockId: blockId}
	if entity, ok := c.entityById[childId]; ok && entity.Parent.Type == "block_id" {
		_, link.holdsParentBlock = mctx.blockIds[entity.Parent.BlockId]
	}
	c.childLinks[childId] = append(c.childLinks[childId], link)
}

// setCreatedInContext records where Notion keeps the entity as its
// createdInContext pair: the parent object's source key (the resolver maps
// it to the final id, or drops the pair when the parent is not in the
// import) and the block there linking the entity.
//
// The ref is never inferred: object GC treats a non-empty ref as "owned
// through this block, offer for cleanup once nothing links it", so it is set
// only from a link the parent was actually emitted with (the parent must
// convert first — see parentFirst). Without that evidence the pair keeps an
// empty ref, which GC ignores: the parent page's blocks failed to fetch, a
// database's second data source (its block links the first), a parent
// recorded by an earlier incarnation of a resumed crawl.
func (c *Converter) setCreatedInContext(entity Entity, details *domain.Details) {
	parentKey, ref := c.createdInContext(entity)
	if parentKey == "" {
		return
	}
	details.SetString(bundle.RelationKeyCreatedInContext, parentKey)
	if ref != "" {
		details.SetString(bundle.RelationKeyCreatedInContextRef, ref)
	}
}

func (c *Converter) createdInContext(entity Entity) (parentKey, ref string) {
	links := c.childLinks[entity.Id]
	switch entity.Parent.Type {
	case "page_id":
		if !c.isImportedPage(entity.Parent.PageId) {
			return "", ""
		}
		for _, link := range links {
			if link.pageId == entity.Parent.PageId {
				return entity.Parent.PageId, link.blockId
			}
		}
		return entity.Parent.PageId, ""
	case "block_id":
		// The parent block's page is not in the entity's own parent; the
		// page that emitted the child's link while holding that block is.
		for _, link := range links {
			if link.holdsParentBlock && c.isImportedPage(link.pageId) {
				return link.pageId, link.blockId
			}
		}
	case "data_source_id":
		if c.isImportedCollection(entity.Parent.DataSourceId) {
			return entity.Parent.DataSourceId, ""
		}
	case "database_id":
		if id, ok := c.resolveDatabaseRef(entity.Parent.DatabaseId); ok && c.isImportedCollection(id) {
			return id, ""
		}
	}
	return "", ""
}

func (c *Converter) isImportedPage(id string) bool {
	entity, ok := c.entityById[id]
	return ok && !entity.isCollectionLike()
}

// isImportedCollection excludes databases whose planned type took their
// place: no collection object carries their source key.
func (c *Converter) isImportedCollection(id string) bool {
	entity, ok := c.entityById[id]
	return ok && entity.isCollectionLike() && !c.typeBackedContainers[id]
}

// parentFirst orders pages so a page converts after the page it lives in,
// which is what lets the child read the link its parent emitted. A page whose
// chain of page_id parents reaches a block-parented page converts after every
// page whose chain does not (the block's page is only known once some page's
// tree turns out to hold the block), and within each tier by depth along the
// chain. Stable, so search order breaks ties and stays deterministic.
func (c *Converter) parentFirst(pages []Entity) []Entity {
	type rank struct{ blockTier, depth int }
	ranks := map[string]rank{}
	var rankOf func(id string, seen int) rank
	rankOf = func(id string, seen int) rank {
		if r, ok := ranks[id]; ok {
			return r
		}
		var r rank
		entity, ok := c.entityById[id]
		switch {
		case !ok || seen >= maxBlockDepth:
		case entity.Parent.Type == "block_id":
			r.blockTier = 1
		case entity.Parent.Type == "page_id":
			if _, parentKnown := c.entityById[entity.Parent.PageId]; parentKnown {
				parent := rankOf(entity.Parent.PageId, seen+1)
				r = rank{blockTier: parent.blockTier, depth: parent.depth + 1}
			}
		}
		ranks[id] = r
		return r
	}
	ordered := append([]Entity{}, pages...)
	sort.SliceStable(ordered, func(i, j int) bool {
		ri, rj := rankOf(ordered[i].Id, 0), rankOf(ordered[j].Id, 0)
		if ri.blockTier != rj.blockTier {
			return ri.blockTier < rj.blockTier
		}
		return ri.depth < rj.depth
	})
	return ordered
}

// maxDeferredPages bounds how many fetched pages may wait for their parent
// (memory: a deferred page holds its whole block tree). Past it the oldest
// emits without waiting and loses at most its ref.
const maxDeferredPages = 64

// emitOrder holds back a fetched page until the page it lives in has been
// emitted, which a static order cannot guarantee: a page nested under a
// block is only placed once some page's tree turns out to hold that block,
// and pages nest under blocks of block-nested pages arbitrarily deep.
type emitOrder struct {
	inRun    map[string]bool // pages of this pipeline
	emitted  map[string]bool
	deferred []*fetchedPage
}

func newEmitOrder(pages []Entity) *emitOrder {
	inRun := make(map[string]bool, len(pages))
	for _, page := range pages {
		inRun[page.Id] = true
	}
	return &emitOrder{inRun: inRun, emitted: map[string]bool{}}
}

// waitsForParent reports whether the page's parent is still to come in this
// run: a page_id parent not emitted yet, or a block_id parent no emitted
// page has been seen to hold while pages remain.
func (c *Converter) waitsForParent(stub Entity, order *emitOrder) bool {
	switch stub.Parent.Type {
	case "page_id":
		return order.inRun[stub.Parent.PageId] && !order.emitted[stub.Parent.PageId]
	case "block_id":
		for _, link := range c.childLinks[stub.Id] {
			if link.holdsParentBlock {
				return false
			}
		}
		return len(order.emitted) < len(order.inRun)
	}
	return false
}

// emitInParentOrder emits f, or defers it until its parent is emitted;
// every emission may release deferred pages.
func (c *Converter) emitInParentOrder(ctx context.Context, f *fetchedPage, order *emitOrder, sink importv2.Sink) error {
	if c.waitsForParent(f.stub, order) {
		order.deferred = append(order.deferred, f)
		if len(order.deferred) <= maxDeferredPages {
			return nil
		}
		f, order.deferred = order.deferred[0], order.deferred[1:]
	}
	if err := c.emitDeferred(ctx, f, order, sink); err != nil {
		return err
	}
	return c.releaseDeferred(ctx, order, sink)
}

// releaseDeferred emits deferred pages whose parent has arrived, repeating
// until no more are released (one release may unblock another).
func (c *Converter) releaseDeferred(ctx context.Context, order *emitOrder, sink importv2.Sink) error {
	for released := true; released; {
		released = false
		for i, f := range order.deferred {
			if c.waitsForParent(f.stub, order) {
				continue
			}
			order.deferred = append(order.deferred[:i:i], order.deferred[i+1:]...)
			if err := c.emitDeferred(ctx, f, order, sink); err != nil {
				return err
			}
			released = true
			break
		}
	}
	return nil
}

// flushDeferred emits the pages still waiting once the stream ends: whatever
// became ready first, then the oldest waiting page (its parent never
// arrived, so it emits without one) — which may in turn release others.
func (c *Converter) flushDeferred(ctx context.Context, order *emitOrder, sink importv2.Sink) error {
	for {
		if err := c.releaseDeferred(ctx, order, sink); err != nil {
			return err
		}
		if len(order.deferred) == 0 {
			return nil
		}
		f := order.deferred[0]
		order.deferred = order.deferred[1:]
		if err := c.emitDeferred(ctx, f, order, sink); err != nil {
			return err
		}
	}
}

func (c *Converter) emitDeferred(ctx context.Context, f *fetchedPage, order *emitOrder, sink importv2.Sink) error {
	if err := c.emitFetchedPage(ctx, f, sink); err != nil {
		return err
	}
	order.emitted[f.stub.Id] = true
	return nil
}
