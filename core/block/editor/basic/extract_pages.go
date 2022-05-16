package basic

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
)

type PageCreator interface {
	CreatePageFromState(ctx *state.Context, contextBlock smartblock.SmartBlock, groupId string, req pb.RpcBlockCreatePageRequest, state *state.State) (linkId string, pageId string, err error)
}

// ExtractBlocksToPages extracts child blocks from the page to separate pages and
// replaces these blocks to the links to these pages
func (bs *basic) ExtractBlocksToPages(ctx *state.Context, s PageCreator, req pb.RpcBlockListConvertChildrenToPagesRequest) (linkIds []string, err error) {
	st := bs.NewState()

	roots := listRoots(st, req.BlockIds)
	for _, root := range roots {
		descendants := st.Descendants(root.Model().Id)
		newRoot, newBlocks := reassignSubtreeIds(root.Model().Id, append(descendants, root))

		// Remove children
		for _, b := range descendants {
			st.Unlink(b.Model().Id)
		}

		// Build a state for the new page from child blocks
		pageState := state.NewDoc("", nil).NewState()
		for _, b := range newBlocks {
			pageState.Add(b)
		}
		pageState.Add(base.NewBase(&model.Block{
			// This id will be replaced by id of the new page
			Id:          "_root",
			ChildrenIds: []string{newRoot},
		}))

		fields := map[string]*types.Value{
			"name": pbtypes.String(root.Model().GetText().Text),
		}
		if req.ObjectType != "" {
			fields[bundle.RelationKeyType.String()] = pbtypes.String(req.ObjectType)
		}
		_, pageId, err := s.CreatePageFromState(ctx, bs, "", pb.RpcBlockCreatePageRequest{
			ContextId: req.ContextId,
			Details: &types.Struct{
				Fields: fields,
			},
		}, pageState)
		if err != nil {
			return nil, fmt.Errorf("create child page: %w", err)
		}

		linkId, err := CreateBlock(st, "", pb.RpcBlockCreateRequest{
			TargetId: root.Model().Id,
			Block: &model.Block{
				Content: &model.BlockContentOfLink{
					Link: &model.BlockContentLink{
						TargetBlockId: pageId,
						Style:         model.BlockContentLink_Page,
					},
				},
			},
			Position: model.Block_Replace,
		})
		if err != nil {
			return nil, fmt.Errorf("create link to page %s: %w", pageId, err)
		}

		linkIds = append(linkIds, linkId)
	}

	return linkIds, bs.Apply(st)
}

// listRoots returns unique root blocks that are listed in blockIds
func listRoots(st *state.State, blockIds []string) []simple.Block {
	visited := map[string]struct{}{}

	// Mark children as visited
	queue := blockIds
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]

		b := st.Pick(id)
		if b == nil {
			continue
		}

		childrenIds := b.Model().ChildrenIds
		for _, chId := range childrenIds {
			visited[chId] = struct{}{}
			queue = append(queue, childrenIds...)
		}
	}

	// Unvisited blocks are roots
	var roots []simple.Block
	for _, id := range blockIds {
		if _, ok := visited[id]; ok {
			continue
		}
		b := st.Pick(id)
		if b == nil {
			continue
		}

		roots = append(roots, b)
	}
	return roots
}

// reassignSubtreeIds makes a copy of a subtree of blocks and assign a new id for each block
func reassignSubtreeIds(rootId string, blocks []simple.Block) (string, []simple.Block) {
	res := make([]simple.Block, 0, len(blocks))
	mapping := map[string]string{}
	for _, b := range blocks {
		newId := bson.NewObjectId().Hex()
		mapping[b.Model().Id] = newId

		newBlock := b.Copy()
		newBlock.Model().Id = newId
		res = append(res, newBlock)
	}

	for _, b := range res {
		for i, id := range b.Model().ChildrenIds {
			b.Model().ChildrenIds[i] = mapping[id]
		}
	}
	return mapping[rootId], res
}
