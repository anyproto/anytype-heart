package editor

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
)

type PageCreator interface {
	Do(id string, apply func(b smartblock.SmartBlock) error) error
	CreatePageFromState(ctx *state.Context, groupId string, req pb.RpcBlockCreatePageRequest, state *state.State) (linkId string, pageId string, err error)
}

// ExtractBlocksToPages extracts child blocks from the page to separate pages and
// replaces these blocks to the links to these pages
func ExtractBlocksToPages(s PageCreator, req pb.RpcBlockListConvertChildrenToPagesRequest) (linkIds []string, err error) {
	blocks := make(map[string]*model.Block)
	err = s.Do(req.ContextId, func(contextBlock smartblock.SmartBlock) error {
		for _, b := range contextBlock.Blocks() {
			blocks[b.Id] = b
		}
		return nil
	})
	if err != nil {
		return linkIds, err
	}

	var toRemove []string

	type linkAdd struct {
		blockId string
		pageId  string
	}
	var linksToAdd []linkAdd

	visited := map[string]struct{}{}
	for _, blockId := range req.BlockIds {
		if blocks[blockId] == nil || blocks[blockId].GetText() == nil {
			continue
		}

		subtree := extractBlocksSubtree(blockId, blocks, visited)
		if len(subtree) == 0 {
			continue
		}
		// Collect all child blocks to remove later
		for _, b := range subtree {
			if b.Id != blockId {
				toRemove = append(toRemove, b.Id)
			}
		}

		newRoot, newBlocks := reassignSubtreeIds(blockId, subtree)

		// Build a state for the new page from child blocks
		st := state.NewDoc("", nil).NewState()
		for _, b := range newBlocks {
			st.Add(base.NewBase(b))
		}
		st.Add(base.NewBase(&model.Block{
			// This id will be replaced by id of the new page
			Id:          "_root",
			ChildrenIds: []string{newRoot},
		}))

		fields := map[string]*types.Value{
			"name": pbtypes.String(blocks[blockId].GetText().Text),
		}
		if req.ObjectType != "" {
			fields[bundle.RelationKeyType.String()] = pbtypes.String(req.ObjectType)
		}
		_, pageId, err := s.CreatePageFromState(nil, "", pb.RpcBlockCreatePageRequest{
			ContextId: req.ContextId,
			Details: &types.Struct{
				Fields: fields,
			},
		}, st)
		if err != nil {
			return linkIds, err
		}
		linksToAdd = append(linksToAdd, linkAdd{
			blockId: blockId,
			pageId:  pageId,
		})
	}

	err = s.Do(req.ContextId, func(b smartblock.SmartBlock) error {
		st := b.NewState()

		t := basic.NewStateTransformer(st)

		for _, l := range linksToAdd {
			linkId, err := t.CreateBlock("", pb.RpcBlockCreateRequest{
				TargetId: l.blockId,
				Block: &model.Block{
					Content: &model.BlockContentOfLink{
						Link: &model.BlockContentLink{
							TargetBlockId: l.pageId,
							Style:         model.BlockContentLink_Page,
						},
					},
				},
				Position: model.Block_Replace,
			})
			if err != nil {
				return fmt.Errorf("create link to page %s: %w", l.pageId, err)
			}
			linkIds = append(linkIds, linkId)
		}
		t.CutBlocks(toRemove)

		return b.Apply(st)
	})

	return linkIds, err
}

// extractBlocksSubtree extracts a subtree with specific root from blocks and marks visited blocks
func extractBlocksSubtree(root string, blocks map[string]*model.Block, visited map[string]struct{}) []*model.Block {
	var (
		queue     = []string{root}
		extracted []*model.Block
	)

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		b, ok := blocks[cur]
		if !ok {
			continue
		}
		if _, ok := visited[cur]; ok {
			continue
		}
		visited[cur] = struct{}{}

		extracted = append(extracted, b)
		queue = append(queue, b.ChildrenIds...)
	}

	return extracted
}

// reassignSubtreeIds makes a copy of a subtree of blocks and assign a new id for each block
func reassignSubtreeIds(root string, blocks []*model.Block) (string, []*model.Block) {
	res := make([]*model.Block, 0, len(blocks))
	mapping := map[string]string{}
	for _, b := range blocks {
		newId := bson.NewObjectId().Hex()
		mapping[b.Id] = newId

		newBlock := pbtypes.CopyBlock(b)
		newBlock.Id = newId
		res = append(res, newBlock)
	}

	for _, b := range res {
		for i, id := range b.ChildrenIds {
			b.ChildrenIds[i] = mapping[id]
		}
	}
	return mapping[root], res
}
