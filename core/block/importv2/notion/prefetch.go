package notion

import (
	"context"
	"fmt"
	"net/http"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
)

// prefetchInFlight bounds concurrently fetched pages. Throughput is capped by
// the client's shared 3 rps pacer either way — this only needs to cover the
// latency-bandwidth product so the pacer, not the round-trip time, is the
// bottleneck (serial fetching ran at ~1.5-2 rps against a 3 rps allowance;
// v1 used a 10-worker pool with no client-side pacer at all).
const prefetchInFlight = 6

// fetchedPage carries one page's network payload from a prefetch worker to
// the ordered emit loop. Issues raised during the fetch are buffered and
// replayed at consumption time, so run output stays deterministic (converter
// contract rule 5) regardless of how fetches interleave.
type fetchedPage struct {
	stub      Entity
	page      pageObject
	pageErr   error
	blocks    []notionBlock
	blockIds  map[string]struct{}
	blocksErr error
	issues    []importv2.Issue
	done      chan struct{}
}

// fetchSink is the fetch-phase sink shim. Only Issue (buffered per page) and
// the status signals (thread-safe per the Sink contract) are legal off the
// converter goroutine; Object and Claim during a fetch are contract
// violations.
type fetchSink struct {
	page *fetchedPage
	sink importv2.Sink
}

func (s *fetchSink) Object(ctx context.Context, o *importv2.Object) error {
	return importv2.Issue{
		Severity: importv2.SeverityFatal,
		Code:     importv2.IssueInvariant,
		Message:  "object emitted during prefetch (fetch phase must not emit)",
	}
}

func (s *fetchSink) Claim(ctx context.Context, claim importv2.IdentityClaim) error {
	return fmt.Errorf("identity claimed during prefetch (fetch phase must not claim)")
}

func (s *fetchSink) Issue(i importv2.Issue) { s.page.issues = append(s.page.issues, i) }

func (s *fetchSink) Phase(p importv2.Phase) { s.sink.Phase(p) }

// Item is deliberately NOT forwarded: prefetch workers run ahead of the
// ordered emit loop, so a worker's title would name a page the user will
// only see minutes later. The emit loop announces the current item itself,
// in stub order.
func (s *fetchSink) Item(importv2.DisplayText) {}

// prefetchPages pipelines page fetches ahead of the emit loop: up to
// prefetchInFlight pages fetch concurrently while the consumer emits strictly
// in stub order. The returned channel yields every page exactly once, in
// order; the consumer must wait on each page's done channel. Cancellation
// unblocks both sides (the engine always cancels the run context on abort).
func (c *Converter) prefetchPages(ctx context.Context, pages []Entity, sink importv2.Sink) <-chan *fetchedPage {
	out := make(chan *fetchedPage)
	sem := make(chan struct{}, prefetchInFlight)
	go func() {
		defer close(out)
		for i := range pages {
			f := &fetchedPage{stub: pages[i], done: make(chan struct{})}
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			go func() {
				defer func() { <-sem }()
				defer close(f.done)
				// Converter-internal goroutine: the engine's panic firewall
				// covers the Convert goroutine, not this one — a panicking
				// fetch fails one page, never the process.
				defer func() {
					if rec := recover(); rec != nil {
						f.pageErr = fmt.Errorf("prefetch panic: %v", rec)
					}
				}()
				c.fetchPageData(ctx, f, sink)
			}()
			select {
			case out <- f:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// fetchPageData is the network half of a page conversion: the page object
// and its block tree. It touches no shared converter state (the client is
// thread-safe, seenIds is per-page, issues buffer per-page) — everything
// stateful happens later in emitFetchedPage on the converter goroutine.
func (c *Converter) fetchPageData(ctx context.Context, f *fetchedPage, sink importv2.Sink) {
	buffer := &fetchSink{page: f, sink: sink}
	if err := c.client.Request(ctx, http.MethodGet, "/pages/"+f.stub.Id, nil, &f.page); err != nil {
		f.pageErr = err
		return
	}
	f.blockIds = map[string]struct{}{}
	f.blocks, f.blocksErr = c.fetchBlockTree(ctx, f.stub.Id, f.blockIds, buffer)
}
