package history

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/objecttreebuilder"
	"github.com/gogo/protobuf/proto"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/block"
	smartblock2 "github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	history2 "github.com/anyproto/anytype-heart/core/block/history"
	"github.com/anyproto/anytype-heart/core/block/object/objectlink"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/block/undo"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const CName = "history"

const versionGroupInterval = time.Minute * 5

var log = logging.Logger("anytype-mw-history")

func New() History {
	return new(history)
}

type VersionDiff struct {
	CreatedBlockIds      []string
	ModifiedBlockIds     []string
	CreatedRelationKeys  []string
	ModifiedRelationKeys []string
}

type History interface {
	Show(id domain.FullID, versionId string) (bs *model.ObjectView, ver *pb.RpcHistoryVersion, err error)
	Versions(id domain.FullID, lastVersionId string, limit int) (resp []*pb.RpcHistoryVersion, err error)
	SetVersion(id domain.FullID, versionId string) (err error)
	DiffVersions(req *pb.RpcHistoryDiffVersionsRequest) (*VersionDiff, *model.ObjectView, error)
	GetBlocksModifiers(id domain.FullID, versionId string, blocks []*model.Block) ([]*model.ObjectViewBlockModifier, error)
	app.Component
}

type history struct {
	accountService account.Service
	picker         block.ObjectGetter
	objectStore    objectstore.ObjectStore
	spaceService   space.Service
}

func (h *history) Init(a *app.App) (err error) {
	h.picker = app.MustComponent[block.ObjectGetter](a)
	h.objectStore = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	h.spaceService = app.MustComponent[space.Service](a)
	h.accountService = app.MustComponent[account.Service](a)
	return
}

func (h *history) Name() (name string) {
	return CName
}

func (h *history) Show(id domain.FullID, versionID string) (bs *model.ObjectView, ver *pb.RpcHistoryVersion, err error) {
	space, err := h.spaceService.Get(context.Background(), id.SpaceID)
	if err != nil {
		return nil, nil, fmt.Errorf("get space: %w", err)
	}
	s, sbType, ver, err := h.buildState(id, versionID)
	if err != nil {
		return
	}
	s.SetDetailAndBundledRelation(bundle.RelationKeyId, pbtypes.String(id.ObjectID))
	s.SetDetailAndBundledRelation(bundle.RelationKeySpaceId, pbtypes.String(id.SpaceID))
	typeId, err := space.GetTypeIdByKey(context.Background(), s.ObjectTypeKey())
	if err != nil {
		return nil, nil, fmt.Errorf("get type id by key: %w", err)
	}
	s.SetDetailAndBundledRelation(bundle.RelationKeyType, pbtypes.String(typeId))

	dependentObjectIDs := objectlink.DependentObjectIDs(s, space, true, true, false, true, false)
	// nolint:errcheck
	metaD, _ := h.objectStore.QueryByID(dependentObjectIDs)
	details := make([]*model.ObjectViewDetailsSet, 0, len(metaD))

	metaD = append(metaD, database.Record{Details: s.CombinedDetails()})
	for _, m := range metaD {
		details = append(details, &model.ObjectViewDetailsSet{
			Id:      pbtypes.GetString(m.Details, bundle.RelationKeyId.String()),
			Details: m.Details,
		})
	}

	relations, err := h.objectStore.FetchRelationByLinks(id.SpaceID, s.PickRelationLinks())
	if err != nil {
		return nil, nil, fmt.Errorf("fetch relations by links: %w", err)
	}
	blocksModifiers, err := h.GetBlocksModifiers(id, versionID, s.Blocks())
	if err != nil {
		return nil, nil, fmt.Errorf("get blocks modifiers: %w", err)
	}
	return &model.ObjectView{
		RootId:          id.ObjectID,
		Type:            model.SmartBlockType(sbType),
		Blocks:          s.Blocks(),
		Details:         details,
		RelationLinks:   relations.RelationLinks(),
		BlocksModifiers: blocksModifiers,
	}, ver, nil
}

func (h *history) Versions(id domain.FullID, lastVersionId string, limit int) (resp []*pb.RpcHistoryVersion, err error) {
	if limit <= 0 {
		limit = 100
	}
	var includeLastId = true

	reverse := func(vers []*pb.RpcHistoryVersion) []*pb.RpcHistoryVersion {
		for i, j := 0, len(vers)-1; i < j; i, j = i+1, j-1 {
			vers[i], vers[j] = vers[j], vers[i]
		}
		return vers
	}

	for len(resp) < limit {
		tree, _, e := h.treeWithId(id, lastVersionId, includeLastId)
		if e != nil {
			return nil, e
		}
		var data []*pb.RpcHistoryVersion

		e = tree.IterateFrom(tree.Root().Id, source.UnmarshalChange, func(c *objecttree.Change) (isContinue bool) {
			participantId := domain.NewParticipantId(id.SpaceID, c.Identity.Account())
			data = append(data, &pb.RpcHistoryVersion{
				Id:          c.Id,
				PreviousIds: c.PreviousIds,
				AuthorId:    participantId,
				Time:        c.Timestamp,
			})
			return true
		})
		if e != nil {
			return nil, e
		}
		if len(data[0].PreviousIds) == 0 {
			if data[0].Id == tree.Id() {
				data = data[1:]
			}
			resp = append(data, resp...)
			break
		} else {
			resp = append(data, resp...)
			lastVersionId = tree.Root().Id
			includeLastId = false
		}

		if len(data) == 0 {
			break
		}
	}

	resp = reverse(resp)

	var groupId int64
	var nextVersionTimestamp int64

	for i := 0; i < len(resp); i++ {
		if nextVersionTimestamp-resp[i].Time > int64(versionGroupInterval.Seconds()) {
			groupId++
		}
		nextVersionTimestamp = resp[i].Time
		resp[i].GroupId = groupId
	}

	return
}

func (h *history) DiffVersions(req *pb.RpcHistoryDiffVersionsRequest) (*VersionDiff, *model.ObjectView, error) {
	id := domain.FullID{
		ObjectID: req.ObjectId,
		SpaceID:  req.SpaceId,
	}
	currState, sbType, _, err := h.buildState(id, req.CurrentVersion)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get state of versions %s: %s", req.CurrentVersion, err)
	}
	previousState, _, _, err := h.buildState(id, req.PreviousVersion)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get state of versions %s: %s", req.PreviousVersion, err)
	}

	currState.SetParent(previousState)
	_, actions, err := state.ApplyState(currState, false)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get history events for versions %s, %s: %s", req.CurrentVersion, req.PreviousVersion, err)
	}
	versionDiff := h.processActions(actions, id.ObjectID)

	spc, err := h.spaceService.Get(context.Background(), id.SpaceID)
	if err != nil {
		return nil, nil, fmt.Errorf("get space: %w", err)
	}
	dependentObjectIDs := objectlink.DependentObjectIDs(currState, spc, true, true, false, true, false)
	metaD, _ := h.objectStore.QueryByID(dependentObjectIDs)
	details := make([]*model.ObjectViewDetailsSet, 0, len(metaD))

	metaD = append(metaD, database.Record{Details: currState.CombinedDetails()})
	for _, m := range metaD {
		details = append(details, &model.ObjectViewDetailsSet{
			Id:      pbtypes.GetString(m.Details, bundle.RelationKeyId.String()),
			Details: m.Details,
		})
	}

	objectView := &model.ObjectView{
		RootId:        id.ObjectID,
		Type:          model.SmartBlockType(sbType),
		Blocks:        currState.Blocks(),
		Details:       details,
		RelationLinks: currState.GetRelationLinks(),
	}

	return versionDiff, objectView, nil
}

func (h *history) processActions(actions undo.Action, objectId string) *VersionDiff {
	var createdBlockIds, modifiedBlockIds, createdRelationKeys, modifiedRelationKeys []string
	for _, changed := range actions.Change {
		if changed.Before.Model().GetId() == objectId {
			continue
		}
		modifiedBlockIds = append(modifiedBlockIds, changed.Before.Model().GetId())
	}
	for _, add := range actions.Add {
		createdBlockIds = append(createdBlockIds, add.Model().GetId())
	}
	if actions.Details != nil {
		modifiedRelationKeys = h.diffDetails(actions)
	}
	if actions.RelationLinks != nil {
		createdRelationKeys = h.diffRelations(actions)
	}
	return &VersionDiff{
		CreatedBlockIds:      createdBlockIds,
		ModifiedBlockIds:     modifiedBlockIds,
		CreatedRelationKeys:  createdRelationKeys,
		ModifiedRelationKeys: modifiedRelationKeys,
	}
}

func (h *history) diffDetails(actions undo.Action) []string {
	var createdRelationKeys []string
	diff := pbtypes.StructDiff(actions.Details.Before, actions.Details.After)
	for key := range diff.GetFields() {
		createdRelationKeys = append(createdRelationKeys, key)
	}
	return createdRelationKeys
}

func (h *history) diffRelations(actions undo.Action) []string {
	before := actions.RelationLinks.Before
	after := actions.RelationLinks.After
	relationsBefore := lo.Map(before, func(item *model.RelationLink, index int) *model.Relation {
		return &model.Relation{
			Key: item.Key,
		}
	})
	relationsAfter := lo.Map(after, func(item *model.RelationLink, index int) *model.Relation {
		return &model.Relation{
			Key: item.Key,
		}
	})
	added, _, _ := pbtypes.RelationsDiff(relationsBefore, relationsAfter)
	var createdRelationKeys []string
	for _, relation := range added {
		createdRelationKeys = append(createdRelationKeys, relation.Key)
	}
	return createdRelationKeys
}

func (h *history) GetBlocksModifiers(id domain.FullID, versionId string, blocks []*model.Block) ([]*model.ObjectViewBlockModifier, error) {
	if len(blocks) == 0 {
		return nil, nil
	}
	existingBlocks := lo.SliceToMap(blocks, func(item *model.Block) (string, struct{}) { return item.GetId(), struct{}{} })
	tree, _, e := h.treeWithId(id, versionId, true)
	if e != nil {
		return nil, e
	}

	blocksModifiersMap := make(map[string]string, 0)
	e = tree.IterateFrom(tree.Root().Id, source.UnmarshalChange, func(c *objecttree.Change) (isContinue bool) {
		if lo.Contains(c.PreviousIds, id.ObjectID) {
			return true // skip first change
		}
		h.processChange(c, id, blocksModifiersMap, existingBlocks)
		return true
	})
	if e != nil {
		return nil, e
	}

	blocksModifiers := make([]*model.ObjectViewBlockModifier, 0)
	return blocksModifiers, nil
}

func (h *history) processChange(c *objecttree.Change,
	id domain.FullID,
	blocksToParticipant map[string]string,
	existingBlocks map[string]struct{},
) {
	participantId := domain.NewParticipantId(id.SpaceID, c.Identity.Account())
	if changeContent, ok := c.Model.(*pb.Change); ok {
		blockChanges := h.getChangedBlockIds(changeContent.Content)
		for _, block := range blockChanges {
			if _, ok := existingBlocks[block]; !ok {
				continue
			}
			blocksToParticipant[block] = participantId
		}
	}
}

func (h *history) getChangedBlockIds(changeList []*pb.ChangeContent) []string {
	var blocksIds []string
	for _, content := range changeList {
		if c := content.GetBlockCreate(); c != nil {
			for _, bl := range c.Blocks {
				blocksIds = append(blocksIds, bl.Id)
			}
		}
		if c := content.GetBlockMove(); c != nil {
			blocksIds = append(blocksIds, c.Ids...)
		}
		if c := content.GetBlockUpdate(); c != nil {
			for _, event := range c.Events {
				blocksIds = h.handleAddEvent(event, blocksIds)
				blocksIds = h.handleBlockSettingsEvents(event, blocksIds)
				blocksIds = h.handleSimpleBlockEvents(event, blocksIds)
			}
		}
	}
	return blocksIds
}

func (h *history) handleAddEvent(event *pb.EventMessage, blockId []string) []string {
	if blockAdd := event.GetBlockAdd(); blockAdd != nil {
		for _, bl := range event.GetBlockAdd().Blocks {
			blockId = append(blockId, bl.Id)
		}
	}
	return blockId
}

func (h *history) handleBlockSettingsEvents(event *pb.EventMessage, blockId []string) []string {
	if setVerticalAlign := event.GetBlockSetVerticalAlign(); setVerticalAlign != nil {
		blockId = append(blockId, setVerticalAlign.Id)
	}
	if setAlign := event.GetBlockSetAlign(); setAlign != nil {
		blockId = append(blockId, setAlign.Id)
	}
	if setChildrenIds := event.GetBlockSetChildrenIds(); setChildrenIds != nil {
		blockId = append(blockId, setChildrenIds.Id)
	}
	if setBackgroundColor := event.GetBlockSetBackgroundColor(); setBackgroundColor != nil {
		blockId = append(blockId, setBackgroundColor.Id)
	}
	return blockId
}

func (h *history) handleSimpleBlockEvents(event *pb.EventMessage, blockId []string) []string {
	if setTableRow := event.GetBlockSetTableRow(); setTableRow != nil {
		blockId = append(blockId, setTableRow.Id)
	}
	if setRelation := event.GetBlockSetRelation(); setRelation != nil {
		blockId = append(blockId, setRelation.Id)
	}
	if setText := event.GetBlockSetText(); setText != nil {
		blockId = append(blockId, setText.Id)
	}
	if setLink := event.GetBlockSetLink(); setLink != nil {
		blockId = append(blockId, setLink.Id)
	}
	if setLatex := event.GetBlockSetLatex(); setLatex != nil {
		blockId = append(blockId, setLatex.Id)
	}
	if setFile := event.GetBlockSetFile(); setFile != nil {
		blockId = append(blockId, setFile.Id)
	}
	if setDiv := event.GetBlockSetDiv(); setDiv != nil {
		blockId = append(blockId, setDiv.Id)
	}
	if setFields := event.GetBlockSetFields(); setFields != nil {
		blockId = append(blockId, setFields.Id)
	}
	if setBookmark := event.GetBlockSetBookmark(); setBookmark != nil {
		blockId = append(blockId, setBookmark.Id)
	}
	return blockId
}

func (h *history) SetVersion(id domain.FullID, versionId string) (err error) {
	s, _, _, err := h.buildState(id, versionId)
	if err != nil {
		return
	}
	return block.Do(h.picker, id.ObjectID, func(sb smartblock2.SmartBlock) error {
		return history2.ResetToVersion(sb, s)
	})
}

func (h *history) treeWithId(id domain.FullID, beforeId string, includeBeforeId bool) (ht objecttree.HistoryTree, sbt smartblock.SmartBlockType, err error) {
	spc, err := h.spaceService.Get(context.Background(), id.SpaceID)
	if err != nil {
		return
	}
	ht, err = spc.TreeBuilder().BuildHistoryTree(context.Background(), id.ObjectID, objecttreebuilder.HistoryTreeOpts{
		BeforeId: beforeId,
		Include:  includeBeforeId,
	})
	if err != nil {
		return
	}

	payload := &model.ObjectChangePayload{}
	err = proto.Unmarshal(ht.ChangeInfo().ChangePayload, payload)
	if err != nil {
		return
	}

	sbt = smartblock.SmartBlockType(payload.SmartBlockType)
	return
}

func (h *history) buildState(id domain.FullID, versionId string) (st *state.State, sbType smartblock.SmartBlockType, ver *pb.RpcHistoryVersion, err error) {
	tree, sbType, err := h.treeWithId(id, versionId, true)
	if err != nil {
		return
	}

	st, _, _, err = source.BuildState(id.SpaceID, nil, tree)
	defer st.ResetParentIdsCache()
	if err != nil {
		return
	}
	if _, _, err = state.ApplyStateFast(st); err != nil {
		return
	}

	st.BlocksInit(st)
	if ch, e := tree.GetChange(versionId); e == nil {
		participantId := domain.NewParticipantId(id.SpaceID, ch.Identity.Account())
		ver = &pb.RpcHistoryVersion{
			Id:          ch.Id,
			PreviousIds: ch.PreviousIds,
			AuthorId:    participantId,
			Time:        ch.Timestamp,
		}
	}
	return
}
