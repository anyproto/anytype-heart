package basic

import (
	"context"
	"fmt"

	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
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
	CreateSmartBlockFromState(ctx context.Context, spaceID string, objectTypeKeys []domain.TypeKey, createState *state.State) (id string, newDetails *domain.Details, err error)
}

type templateStateCreator interface {
	CreateTemplateStateWithDetails(
		spaceId, templateId, typeId string, layout model.ObjectTypeLayout, details *domain.Details, withTemplateValidation bool,
	) (st *state.State, err error)
	CreateTemplateStateFromSmartBlock(sb smartblock.SmartBlock, typeId string, layout model.ObjectTypeLayout, details *domain.Details) *state.State
}

// ExtractBlocksToObjects extracts child blocks from the object to separate objects and
// replaces these blocks to the links to these objects
func (bs *basic) ExtractBlocksToObjects(
	ctx session.Context,
	objectCreator ObjectCreator,
	templateStateCreator templateStateCreator,
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
	uk domain.UniqueKey, root simple.Block, creator templateStateCreator, req pb.RpcBlockListConvertToObjectsRequest,
) (*state.State, error) {
	objType, err := bs.objectStore.GetObjectByUniqueKey(uk)
	if err != nil {
		return nil, fmt.Errorf("failed to get type from store: %w", err)
	}
	var (
		// nolint:gosec
		layout  = model.ObjectTypeLayout(objType.GetInt64(bundle.RelationKeyRecommendedLayout))
		typeId  = objType.GetString(bundle.RelationKeyId)
		details = createTargetObjectDetails(root.Model().GetText().GetText(), layout)
	)

	if req.ContextId == req.TemplateId {
		return creator.CreateTemplateStateFromSmartBlock(bs, typeId, layout, details), nil
	}

	return creator.CreateTemplateStateWithDetails(bs.SpaceID(), req.TemplateId, typeId, layout, details, true)
}

func insertBlocksToState(
	srcState *state.State,
	srcSubtreeRoot simple.Block,
	targetState *state.State,
) {
	srcRootId := srcSubtreeRoot.Model().Id
	descendants := srcState.Descendants(srcRootId)
	newSubtreeRootId, newBlocks := copySubtreeOfBlocks(srcState, srcRootId, append(descendants, srcSubtreeRoot))

	// remove descendant blocks from source object
	removeBlocks(srcState, descendants)

	for _, newBlock := range newBlocks {
		targetState.Add(newBlock)
	}

	targetRootBlock := targetState.Pick(targetState.RootId()).Model()
	if hasNoteLayout(targetState) {
		targetRootBlock.ChildrenIds = append(targetRootBlock.ChildrenIds, newSubtreeRootId)
	} else {
		// text in newSubtree root has already been added to the title
		children := targetState.Pick(newSubtreeRootId).Model().ChildrenIds
		targetRootBlock.ChildrenIds = append(targetRootBlock.ChildrenIds, children...)
	}

	targetState.Set(simple.New(targetRootBlock))
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

func createTargetObjectDetails(nameText string, layout model.ObjectTypeLayout) *domain.Details {
	details := domain.NewDetails()
	details.SetInt64(bundle.RelationKeyResolvedLayout, int64(layout))
	// Without this check title will be duplicated in template.WithNameToFirstBlock
	if layout != model.ObjectType_note {
		details.SetString(bundle.RelationKeyName, nameText)
	}
	return details
}

// copySubtreeOfBlocks makes a copy of a subtree of blocks and assign a new id for each block
func copySubtreeOfBlocks(s *state.State, oldRootId string, oldBlocks []simple.Block) (string, []simple.Block) {
	copiedBlocks := make([]simple.Block, 0, len(oldBlocks))
	oldToNewIds := map[string]string{}
	newProcessedIds := map[string]struct{}{}

	// duplicate blocks that can be duplicated
	for _, oldBlock := range oldBlocks {
		if d, ok := oldBlock.(duplicatable); ok {
			newRootId, oldVisitedIds, newBlocks, err := d.Duplicate(s)
			if err != nil {
				log.Errorf("failed to perform newProcessedIds duplicate: %v", err)
				continue
			}

			for _, newBlock := range newBlocks {
				copiedBlocks = append(copiedBlocks, newBlock)
				newProcessedIds[newBlock.Model().Id] = struct{}{}
			}

			for _, id := range oldVisitedIds {
				// mark id as visited and already set
				oldToNewIds[id] = ""
			}
			oldToNewIds[oldBlock.Model().Id] = newRootId
		}
	}

	// copy blocks that can't be duplicated
	for _, oldBlock := range oldBlocks {
		_, found := oldToNewIds[oldBlock.Model().Id]
		if found {
			continue
		}

		newId := bson.NewObjectId().Hex()
		oldToNewIds[oldBlock.Model().Id] = newId

		newBlock := oldBlock.Copy()
		newBlock.Model().Id = newId

		copiedBlocks = append(copiedBlocks, newBlock)
	}

	// update children ids for copied blocks
	for _, copiedBlock := range copiedBlocks {
		if _, hasCorrectChildren := newProcessedIds[copiedBlock.Model().Id]; hasCorrectChildren {
			continue
		}

		for i, id := range copiedBlock.Model().ChildrenIds {
			newChildId := oldToNewIds[id]
			if newChildId == "" {
				log.With("old id", id).
					With("parent new id", copiedBlock.Model().Id).
					With("parent old id", oldToNewIds[copiedBlock.Model().Id]).
					Warn("empty id is set as new")
			}
			copiedBlock.Model().ChildrenIds[i] = newChildId
		}
	}

	return oldToNewIds[oldRootId], copiedBlocks
}

func hasNoteLayout(s *state.State) bool {
	//nolint:gosec
	return model.ObjectTypeLayout(s.LocalDetails().GetInt64(bundle.RelationKeyResolvedLayout)) == model.ObjectType_note
}
