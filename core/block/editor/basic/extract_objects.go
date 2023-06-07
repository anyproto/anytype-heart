package basic

import (
	"context"
	"fmt"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/base"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type ObjectCreator interface {
	CreateSmartBlockFromState(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, createState *state.State) (id string, newDetails *types.Struct, err error)
	InjectWorkspaceID(details *types.Struct, objectID string)
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

		layout, _ := st.Layout()
		details := extractDetailsFields(req.ObjectType, root.Model().GetText().Text, layout)

		s.InjectWorkspaceID(details, req.ContextId)
		objectID, _, err := s.CreateSmartBlockFromState(context.TODO(), coresb.SmartBlockTypePage, details, objState)
		if err != nil {
			return nil, fmt.Errorf("create child object: %w", err)
		}

		linkId, err := bs.CreateBlock(st, pb.RpcBlockCreateRequest{
			TargetId: root.Model().Id,
			Block: &model.Block{
				Content: &model.BlockContentOfLink{
					Link: &model.BlockContentLink{
						TargetBlockId: objectID,
						Style:         model.BlockContentLink_Page,
					},
				},
			},
			Position: model.Block_Replace,
		})
		if err != nil {
			return nil, fmt.Errorf("create link to object %s: %w", objectID, err)
		}

		linkIds = append(linkIds, linkId)
	}

	return linkIds, bs.Apply(st)
}

func extractDetailsFields(objectType string, nameText string, layout model.ObjectTypeLayout) *types.Struct {
	fields := map[string]*types.Value{}

	// Without this check title will be duplicated in template.WithNameToFirstBlock
	if layout != model.ObjectType_note {
		fields[bundle.RelationKeyName.String()] = pbtypes.String(nameText)
	}

	if objectType != "" {
		fields[bundle.RelationKeyType.String()] = pbtypes.String(objectType)
	}

	details := &types.Struct{Fields: fields}
	return details
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
