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
func (bs *basic) ExtractBlocksToObjects(ctx *session.Context, objectCreator ObjectCreator, req pb.RpcBlockListConvertToObjectsRequest) (linkIds []string, err error) {
	newState := bs.NewStateCtx(ctx)
	rootIds := newState.SelectRoots(req.BlockIds)

	for _, rootID := range rootIds {
		rootBlock := newState.Pick(rootID)

		objState := prepareTargetObjectState(newState, rootID, rootBlock, req)

		details, err := bs.prepareTargetObjectDetails(req, rootBlock, objectCreator)
		if err != nil {
			return nil, fmt.Errorf("extract blocks to objects: %w", err)
		}

		objectID, _, err := objectCreator.CreateSmartBlockFromState(
			context.TODO(),
			coresb.SmartBlockTypePage,
			details,
			objState,
		)
		if err != nil {
			return nil, fmt.Errorf("create child object: %w", err)
		}

		linkID, err := bs.changeToBlockWithLink(newState, rootBlock, objectID)
		if err != nil {
			return nil, fmt.Errorf("create link to object %s: %w", objectID, err)
		}

		linkIds = append(linkIds, linkID)
	}

	return linkIds, bs.Apply(newState)
}

func (bs *basic) prepareTargetObjectDetails(
	req pb.RpcBlockListConvertToObjectsRequest,
	rootBlock simple.Block,
	objectCreator ObjectCreator,
) (*types.Struct, error) {
	objType, err := bs.objectStore.GetObjectType(req.ObjectType)
	if err != nil {
		return nil, err
	}

	details := createTargetObjectDetails(req.ObjectType, rootBlock.Model().GetText().GetText(), objType.Layout)
	objectCreator.InjectWorkspaceID(details, req.ContextId)
	return details, nil
}

func prepareTargetObjectState(newState *state.State, rootID string, rootBlock simple.Block, req pb.RpcBlockListConvertToObjectsRequest) *state.State {
	descendants := newState.Descendants(rootID)
	newRoot, newBlocks := reassignSubtreeIds(rootID, append(descendants, rootBlock))
	removeBlocks(newState, descendants)

	objState := buildStateFromBlocks(newBlocks)
	fixStateForNoteLayout(objState, req, newRoot)
	injectSmartBlockContentToRootBlock(objState)
	return objState
}

func (bs *basic) changeToBlockWithLink(newState *state.State, blockToChange simple.Block, objectID string) (string, error) {
	return bs.CreateBlock(newState, pb.RpcBlockCreateRequest{
		TargetId: blockToChange.Model().Id,
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
}

func injectSmartBlockContentToRootBlock(objState *state.State) {
	rootID := objState.RootId()
	rootBlock := objState.Get(rootID).Model()
	rootBlock.Content = &model.BlockContentOfSmartblock{
		Smartblock: &model.BlockContentSmartblock{},
	}
	objState.Set(simple.New(rootBlock))
}

func fixStateForNoteLayout(
	objState *state.State,
	req pb.RpcBlockListConvertToObjectsRequest,
	newRoot string,
) {
	if req.ObjectType == bundle.TypeKeyNote.URL() {
		objState.Add(base.NewBase(&model.Block{
			// This id will be replaced by id of the new object
			Id:          "_root",
			ChildrenIds: []string{newRoot},
		}))
	}
}

func buildStateFromBlocks(newBlocks []simple.Block) *state.State {
	objState := state.NewDoc("", nil).NewState()
	for _, b := range newBlocks {
		objState.Add(b)
	}
	return objState
}

func removeBlocks(state *state.State, descendants []simple.Block) {
	for _, b := range descendants {
		state.Unlink(b.Model().Id)
	}
}

func createTargetObjectDetails(objectType string, nameText string, layout model.ObjectTypeLayout) *types.Struct {
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
