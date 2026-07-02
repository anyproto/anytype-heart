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

func (c *Converter) fetchChildren(ctx context.Context, blockId string, depth int, seenIds map[string]struct{}, sink importv2.Sink) ([]notionBlock, error) {
	if depth > maxBlockDepth {
		sink.Issue(importv2.Warning(importv2.IssueDataLoss, blockId,
			fmt.Sprintf("block tree deeper than %d levels; deeper content skipped", maxBlockDepth)))
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
		if block.Type == "synced_block" {
			var synced struct {
				SyncedFrom *struct {
					BlockId string `json:"block_id"`
				} `json:"synced_from"`
			}
			if err := block.decode(&synced); err == nil && synced.SyncedFrom != nil {
				// Duplicate synced block: content lives under the original.
				childSource = synced.SyncedFrom.BlockId
			}
		}
		if childSource == "" {
			continue
		}
		children, err := c.fetchChildren(ctx, childSource, depth+1, seenIds, sink)
		if err != nil {
			if ctx.Err() != nil {
				return nil, err
			}
			// One unreadable subtree (e.g. a synced block whose original is
			// not shared with the integration → 404) must not drop the
			// whole page: degrade to a placeholder child.
			sink.Issue(importv2.Warning(importv2.IssueDataLoss, block.Id,
				fmt.Sprintf("children of block %s could not be fetched: %s", block.Id, err)))
			block.children = []notionBlock{{Id: block.Id + "-lostchildren", Type: "unreadable"}}
			continue
		}
		block.children = children
	}
	return blocks, nil
}
