package notion

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
)

// maxBlockDepth caps the block-tree recursion; v1 had no bound (a
// pathological or cyclic tree could recurse without limit).
const maxBlockDepth = 64

// notionBlock decodes one block object generically: the type discriminator
// plus the type-specific payload kept raw until mapping.
type notionBlock struct {
	Id          string          `json:"id"`
	Type        string          `json:"type"`
	HasChildren bool            `json:"has_children"`
	payload     json.RawMessage // the type-keyed member
	children    []notionBlock
	// origId keeps the real Notion id when Id got a per-occurrence suffix
	// (hoisted duplicate synced-block content).
	origId string
}

// notionId is the block's real Notion id — the one API references (child_page
// ids, block parents) point at — regardless of any Id suffixing.
func (b *notionBlock) notionId() string {
	if b.origId != "" {
		return b.origId
	}
	return b.Id
}

func (b *notionBlock) UnmarshalJSON(data []byte) error {
	type header struct {
		Id          string `json:"id"`
		Type        string `json:"type"`
		HasChildren bool   `json:"has_children"`
	}
	var head header
	if err := json.Unmarshal(data, &head); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	b.Id, b.Type, b.HasChildren = head.Id, head.Type, head.HasChildren
	b.payload = fields[head.Type]
	return nil
}

func (b *notionBlock) decode(out any) error {
	if b.payload == nil {
		return fmt.Errorf("block %s has no %q payload", b.Id, b.Type)
	}
	return json.Unmarshal(b.payload, out)
}

type blockListResponse struct {
	Results    []notionBlock `json:"results"`
	HasMore    bool          `json:"has_more"`
	NextCursor *string       `json:"next_cursor"`
}

// fetchBlockTree loads a page's full block tree: paginated children, bounded
// recursion, synced-block originals resolved (approved decision — v1 lost
// synced content entirely). seenIds collects every fetched block id — the
// ownership set for resolving block-parented child entities.
func (c *Converter) fetchBlockTree(ctx context.Context, pageId string, seenIds map[string]struct{}, sink importv2.Sink) ([]notionBlock, error) {
	return c.fetchChildren(ctx, pageId, 0, seenIds, sink)
}

// fetchChildren walks one block's children. Issues raised here are keyed to
// the page: pageId is the ROOT of this walk (fetchBlockTree passes it as the
// first blockId), which is what a reader can open — a block id resolves to
// nothing and cannot be named.
func (c *Converter) fetchChildren(ctx context.Context, blockId string, depth int, seenIds map[string]struct{}, sink importv2.Sink) ([]notionBlock, error) {
	if depth > maxBlockDepth {
		sink.Issue(importv2.Warning(importv2.IssueDataLoss, blockId,
			fmt.Sprintf("Content nested deeper than %d levels was not imported", maxBlockDepth)))
		return nil, nil
	}
	var blocks []notionBlock
	cursor := ""
	for {
		path := fmt.Sprintf("/blocks/%s/children?page_size=100", blockId)
		if cursor != "" {
			// Cursors are opaque: escape rather than assume URL-safety.
			path += "&start_cursor=" + url.QueryEscape(cursor)
		}
		var response blockListResponse
		if err := c.client.Request(ctx, http.MethodGet, path, nil, &response); err != nil {
			return nil, fmt.Errorf("fetch children of %s: %w", blockId, err)
		}
		blocks = append(blocks, response.Results...)
		if !response.HasMore {
			break
		}
		if response.NextCursor == nil || *response.NextCursor == "" {
			return nil, fmt.Errorf("block children: has_more with empty next_cursor")
		}
		cursor = *response.NextCursor
	}

	for i := range blocks {
		block := &blocks[i]
		if seenIds != nil {
			seenIds[block.Id] = struct{}{}
		}
		// child_page/child_database mark subtree boundaries: their content
		// is imported as its own object, so descending would re-crawl every
		// nested subpage under every ancestor (O(depth) redundant requests)
		// and pollute seenIds with other pages' block ids, breaking the
		// child-entity ownership check.
		if block.Type == "child_page" || block.Type == "child_database" {
			continue
		}
		childSource := ""
		if block.HasChildren {
			childSource = block.Id
		}
		syncedDuplicate := false
		if block.Type == "synced_block" {
			var synced struct {
				SyncedFrom *struct {
					BlockId string `json:"block_id"`
				} `json:"synced_from"`
			}
			if err := block.decode(&synced); err == nil && synced.SyncedFrom != nil {
				// Duplicate synced block: content lives under the original.
				childSource = synced.SyncedFrom.BlockId
				syncedDuplicate = true
			}
		}
		if childSource == "" {
			continue
		}
		var children []notionBlock
		var err error
		if syncedDuplicate {
			children, err = c.syncedOriginal(ctx, childSource, depth+1, seenIds, sink)
		} else {
			children, err = c.fetchChildren(ctx, childSource, depth+1, seenIds, sink)
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil, err
			}
			// One unreadable subtree (e.g. a synced block whose original is
			// not shared with the integration → 404) must not drop the
			// whole page: degrade to a placeholder child.
			sink.Issue(importv2.Issue{
				Severity: importv2.SeverityWarning, Code: importv2.IssueDataLoss, SourceKey: block.Id,
				Message: "Part of this page could not be fetched from Notion (a synced block whose original is not shared with the integration, most often); a placeholder marks the gap",
				Err:     err,
			})
			block.children = []notionBlock{{Id: block.Id + "-lostchildren", Type: "unreadable"}}
			continue
		}
		if syncedDuplicate {
			// The same original may be referenced by several duplicates in
			// one page (or coexist with the original itself), so the hoisted
			// subtree cannot reuse the original's ids verbatim: the snapshot
			// would carry one block id under several parents and state would
			// collapse all copies into one.
			suffixHoistedIds(children, "-"+dashless(block.Id))
		}
		block.children = children
	}
	return blocks, nil
}

// syncedOriginal fetches the content of a synced block's original, once per
// run. A workspace built from Notion templates puts the same synced nav bar
// on every page: one recorded workspace referenced 6 originals from 138
// places, and re-walking each original's subtree per reference cost 303 of
// its 1286 block requests — a third of the crawl, at Notion's ~3 requests a
// second, fetching bytes it already had.
//
// Callers get a COPY. The caller re-ids a hoisted subtree in place
// (suffixHoistedIds) so that one original appearing several times in a page
// does not produce one block id under several parents, and handing out the
// cached slice would rewrite the cache with the first caller's suffix.
func (c *Converter) syncedOriginal(ctx context.Context, blockId string, depth int, seenIds map[string]struct{}, sink importv2.Sink) ([]notionBlock, error) {
	c.syncedMu.Lock()
	cached, ok := c.syncedOriginals[blockId]
	c.syncedMu.Unlock()
	if ok {
		// The fetch that filled the cache recorded these ids in ITS page's
		// ownership set; this page needs them too, or a child entity
		// parented to one of these blocks looks like it belongs elsewhere.
		recordBlockIds(cached, seenIds)
		return cloneBlocks(cached), nil
	}
	children, err := c.fetchChildren(ctx, blockId, depth, seenIds, sink)
	if err != nil {
		return nil, err
	}
	c.syncedMu.Lock()
	c.syncedOriginals[blockId] = cloneBlocks(children)
	c.syncedMu.Unlock()
	return children, nil
}

// cloneBlocks deep-copies a subtree. The payload is shared: it is only ever
// decoded, never written.
func cloneBlocks(blocks []notionBlock) []notionBlock {
	if blocks == nil {
		return nil
	}
	out := make([]notionBlock, len(blocks))
	copy(out, blocks)
	for i := range out {
		out[i].children = cloneBlocks(out[i].children)
	}
	return out
}

// recordBlockIds adds a subtree's real Notion ids to a page's ownership set.
func recordBlockIds(blocks []notionBlock, seenIds map[string]struct{}) {
	for i := range blocks {
		seenIds[blocks[i].notionId()] = struct{}{}
		recordBlockIds(blocks[i].children, seenIds)
	}
}

// suffixHoistedIds rewrites a duplicate synced block's hoisted subtree with a
// per-occurrence id suffix, keeping the real Notion id for reference
// resolution (nested duplicates accumulate one suffix per hoisting level).
func suffixHoistedIds(blocks []notionBlock, suffix string) {
	for i := range blocks {
		block := &blocks[i]
		if block.origId == "" {
			block.origId = block.Id
		}
		block.Id += suffix
		suffixHoistedIds(block.children, suffix)
	}
}
