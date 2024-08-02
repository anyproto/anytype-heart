package basic

import (
	"context"
	"fmt"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type ObjectCreator interface {
	CreateSmartBlockFromState(ctx context.Context, spaceID string, objectTypeKeys []domain.TypeKey, createState *state.State) (id string, newDetails *types.Struct, err error)
}

type TemplateStateCreator interface {
	CreateTemplateStateWithDetails(templateId string, details *types.Struct) (*state.State, error)
}

// ExtractBlocksToObjects extracts child blocks from the object to separate objects and
// replaces these blocks to the links to these objects
func (bs *basic) ExtractBlocksToObjects(
	ctx session.Context,
	objectCreator ObjectCreator,
	templateStateCreator TemplateStateCreator,
	req pb.RpcBlockListConvertToObjectsRequest,
) (linkIds []string, err error) {
	typeUniqueKey, err := domain.UnmarshalUniqueKey(req.ObjectTypeUniqueKey)
	if err != nil {
		return nil, fmt.Errorf("unmarshal unique key: %w", err)
	}
	typeKey := domain.TypeKey(typeUniqueKey.InternalKey())

	newState := bs.NewStateCtx(ctx)
	rootIds := newState.SelectRoots(req.BlockIds)

	for _, rootID := range rootIds {
		rootBlock := newState.Pick(rootID)

		objState, err := bs.prepareObjectState(typeUniqueKey, rootBlock, templateStateCreator, req)
		if err != nil {
			return nil, err
		}

		insertBlocksToState(newState, rootBlock, objState)

		objectID, _, err := objectCreator.CreateSmartBlockFromState(
			context.Background(),
			bs.SpaceID(),
			[]domain.TypeKey{typeKey},
			objState,
		)
		if err != nil {
			return nil, fmt.Errorf("create child object: %w", err)
		}

		linkID, err := bs.changeToBlockWithLink(newState, rootBlock, objectID, req.Block)
		if err != nil {
			return nil, fmt.Errorf("create link to object %s: %w", objectID, err)
		}

		linkIds = append(linkIds, linkID)
	}

	return linkIds, bs.Apply(newState)
}

func (bs *basic) prepareObjectState(
	uk domain.UniqueKey, root simple.Block, creator TemplateStateCreator, req pb.RpcBlockListConvertToObjectsRequest,
) (*state.State, error) {
	details, err := bs.prepareTargetObjectDetails(bs.SpaceID(), uk, root)
	if err != nil {
		return nil, fmt.Errorf("prepare target details: %w", err)
	}
	return creator.CreateTemplateStateWithDetails(req.TemplateId, details)
}

func (bs *basic) prepareTargetObjectDetails(
	spaceID string,
	typeUniqueKey domain.UniqueKey,
	rootBlock simple.Block,
) (*types.Struct, error) {
	objType, err := bs.objectStore.GetObjectByUniqueKey(spaceID, typeUniqueKey)
	if err != nil {
		return nil, err
	}
	rawLayout := pbtypes.GetInt64(objType.GetDetails(), bundle.RelationKeyRecommendedLayout.String())
	details := createTargetObjectDetails(rootBlock.Model().GetText().GetText(), model.ObjectTypeLayout(rawLayout))
	return details, nil
}

func insertBlocksToState(
	newState *state.State,
	rootBlock simple.Block,
	objState *state.State,
) {
	rootID := rootBlock.Model().Id
	descendants := newState.Descendants(rootID)
	newRoot, newBlocks := reassignSubtreeIds(rootID, append(descendants, rootBlock))

	// remove descendant blocks from source object
	removeBlocks(newState, descendants)

	for _, b := range newBlocks {
		objState.Add(b)
	}
	rootB := objState.Pick(objState.RootId()).Model()
	rootB.ChildrenIds = append(rootB.ChildrenIds, newRoot)
	objState.Set(simple.New(rootB))
}

func (bs *basic) changeToBlockWithLink(newState *state.State, blockToReplace simple.Block, objectID string, linkBlock *model.Block) (string, error) {
	return bs.CreateBlock(newState, pb.RpcBlockCreateRequest{
		TargetId: blockToReplace.Model().Id,
		Block:    buildBlock(linkBlock, objectID),
		Position: model.Block_Replace,
	})
}

func buildBlock(b *model.Block, targetID string) (result *model.Block) {
	fallback := &model.Block{
		Content: &model.BlockContentOfLink{
			Link: &model.BlockContentLink{
				TargetBlockId: targetID,
				Style:         model.BlockContentLink_Page,
			},
		},
	}

	if b == nil {
		return fallback
	}
	result = pbtypes.CopyBlock(b)

	switch v := result.Content.(type) {
	case *model.BlockContentOfLink:
		v.Link.TargetBlockId = targetID
	case *model.BlockContentOfBookmark:
		v.Bookmark.TargetObjectId = targetID
	case *model.BlockContentOfFile:
		v.File.TargetObjectId = targetID
	case *model.BlockContentOfDataview:
		v.Dataview.TargetObjectId = targetID
	default:
		result = fallback
	}

	return
}

func removeBlocks(state *state.State, descendants []simple.Block) {
	for _, b := range descendants {
		state.Unlink(b.Model().Id)
	}
}

func createTargetObjectDetails(nameText string, layout model.ObjectTypeLayout) *types.Struct {
	fields := map[string]*types.Value{}

	// Without this check title will be duplicated in template.WithNameToFirstBlock
	if layout != model.ObjectType_note {
		fields[bundle.RelationKeyName.String()] = pbtypes.String(nameText)
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
