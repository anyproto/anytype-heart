package basic

import (
	"context"
	"fmt"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

type ObjectCreator interface {
	CreateSmartBlockFromState(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, relationIds []string, createState *state.State) (id string, newDetails *types.Struct, err error)
	InjectWorkspaceId(details *types.Struct, objectId string)
}

// ExtractBlocksToObjects extracts child blocks from the object to separate objects and
// replaces these blocks to the links to these objects
func (bs *basic) ExtractBlocksToObjects(ctx *session.Context, s ObjectCreator, req pb.RpcBlockListConvertToObjectsRequest) (linkIds []string, err error) {
	st := bs.NewStateCtx(ctx)

	rootIds := st.SelectRoots(req.BlockIds)
	for _, id := range rootIds {
		root := st.Pick(id)
		descendants := st.Descendants(id)
		newRoot, newBlocks := reassignSubtreeIds(id, append(descendants, root))

		// Remove children
		for _, b := range descendants {
			st.Unlink(b.Model().Id)
		}

		// Build a state for the new object from child blocks
		objState := state.NewDoc("", nil).NewState()
		for _, b := range newBlocks {
			objState.Add(b)
		}

		// For note objects we have to create special block structure to
		// avoid messing up with note content
		if req.ObjectType == bundle.TypeKeyNote.URL() {
			objState.Add(base.NewBase(&model.Block{
				// This id will be replaced by id of the new object
				Id:          "_root",
				ChildrenIds: []string{newRoot},
			}))
		}

		// Root block have to have Smartblock content
		rootId := objState.RootId()
		rootBlock := objState.Get(rootId).Model()
		rootBlock.Content = &model.BlockContentOfSmartblock{
			Smartblock: &model.BlockContentSmartblock{},
		}
		objState.Set(simple.New(rootBlock))

		fields := map[string]*types.Value{
			bundle.RelationKeyName.String(): pbtypes.String(root.Model().GetText().Text),
		}
		if req.ObjectType != "" {
			fields[bundle.RelationKeyType.String()] = pbtypes.String(req.ObjectType)
		}
		det := &types.Struct{Fields: fields}

		s.InjectWorkspaceId(det, req.ContextId)
		objectId, _, err := s.CreateSmartBlockFromState(context.TODO(), coresb.SmartBlockTypePage, det, nil, objState)
		if err != nil {
			return nil, fmt.Errorf("create child object: %w", err)
		}

		linkId, err := bs.CreateBlock(st, pb.RpcBlockCreateRequest{
			TargetId: root.Model().Id,
			Block: &model.Block{
				Content: &model.BlockContentOfLink{
					Link: &model.BlockContentLink{
						TargetBlockId: objectId,
						Style:         model.BlockContentLink_Page,
					},
				},
			},
			Position: model.Block_Replace,
		})
		if err != nil {
			return nil, fmt.Errorf("create link to object %s: %w", objectId, err)
		}

		linkIds = append(linkIds, linkId)
	}

	return linkIds, bs.Apply(st)
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
