package notion

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

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
// synced content entirely).
func (c *Converter) fetchBlockTree(ctx context.Context, pageId string, sink importv2.Sink) ([]notionBlock, error) {
	return c.fetchChildren(ctx, pageId, 0, sink)
}

func (c *Converter) fetchChildren(ctx context.Context, blockId string, depth int, sink importv2.Sink) ([]notionBlock, error) {
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
			path += "&start_cursor=" + cursor
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
		children, err := c.fetchChildren(ctx, childSource, depth+1, sink)
		if err != nil {
			return nil, err
		}
		block.children = children
	}
	return blocks, nil
}
