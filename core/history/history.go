package history

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"slices"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/objecttreebuilder"
	"github.com/gogo/protobuf/proto"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/cache"
	smartblock2 "github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	history2 "github.com/anyproto/anytype-heart/core/block/history"
	"github.com/anyproto/anytype-heart/core/block/object/objectlink"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/source"
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

type History interface {
	Show(id domain.FullID, versionId string) (bs *model.ObjectView, ver *pb.RpcHistoryVersion, err error)
	Versions(id domain.FullID, lastVersionId string, limit int) (resp []*pb.RpcHistoryVersion, err error)
	SetVersion(id domain.FullID, versionId string) (err error)
	DiffVersions(req *pb.RpcHistoryDiffVersionsRequest) ([]*pb.EventMessage, *model.ObjectView, error)
	GetBlocksParticipants(id domain.FullID, versionId string, blocks []*model.Block) ([]*model.ObjectViewBlockParticipant, error)
	app.Component
}

type history struct {
	picker       cache.ObjectGetter
	objectStore  objectstore.ObjectStore
	spaceService space.Service
	heads        map[string]string
}

func (h *history) Init(a *app.App) (err error) {
	h.picker = app.MustComponent[cache.ObjectGetter](a)
	h.objectStore = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	h.spaceService = app.MustComponent[space.Service](a)
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
	meta, _ := h.objectStore.QueryByID(dependentObjectIDs)

	meta = append(meta, database.Record{Details: s.CombinedDetails()})
	details := make([]*model.ObjectViewDetailsSet, 0, len(meta))
	for _, m := range meta {
		details = append(details, &model.ObjectViewDetailsSet{
			Id:      pbtypes.GetString(m.Details, bundle.RelationKeyId.String()),
			Details: m.Details,
		})
	}

	relations, err := h.objectStore.FetchRelationByLinks(id.SpaceID, s.PickRelationLinks())
	if err != nil {
		return nil, nil, fmt.Errorf("fetch relations by links: %w", err)
	}
	blocksParticipants, err := h.GetBlocksParticipants(id, versionID, s.Blocks())
	if err != nil {
		return nil, nil, fmt.Errorf("get blocks modifiers: %w", err)
	}
	return &model.ObjectView{
		RootId:            id.ObjectID,
		Type:              model.SmartBlockType(sbType),
		Blocks:            s.Blocks(),
		Details:           details,
		RelationLinks:     relations.RelationLinks(),
		BlockParticipants: blocksParticipants,
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

	curHeads := make(map[string]struct{})
	for len(resp) < limit {
		tree, _, e := h.treeWithId(id, lastVersionId, includeLastId)
		if e != nil {
			return nil, e
		}
		var data []*pb.RpcHistoryVersion

		e = tree.IterateFrom(tree.Root().Id, source.UnmarshalChange, func(c *objecttree.Change) (isContinue bool) {
			participantId := domain.NewParticipantId(id.SpaceID, c.Identity.Account())
			curHeads[c.Id] = struct{}{}
			for _, previousId := range c.PreviousIds {
				delete(curHeads, previousId)
			}
			version := &pb.RpcHistoryVersion{
				Id:          c.Id,
				PreviousIds: c.PreviousIds,
				AuthorId:    participantId,
				Time:        c.Timestamp,
			}
			if len(curHeads) > 1 {
				var (
					resultHead bytes.Buffer
					count      int
				)
				for head := range curHeads {
					resultHead.WriteString(head)
					count++
					if count != len(curHeads)-1 {
						resultHead.WriteString(" ")
					}
				}
				hash := md5.New()
				hashSum := string(hash.Sum(resultHead.Bytes()))
				h.heads[hashSum] = resultHead.String()
				version.Id = hashSum
			}
			data = append(data, version)
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

func (h *history) DiffVersions(req *pb.RpcHistoryDiffVersionsRequest) ([]*pb.EventMessage, *model.ObjectView, error) {
	id := domain.FullID{
		ObjectID: req.ObjectId,
		SpaceID:  req.SpaceId,
	}
	previousState, _, _, err := h.buildState(id, req.PreviousVersion)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get state of versions %s: %w", req.PreviousVersion, err)
	}

	currState, sbType, _, err := h.buildState(id, req.CurrentVersion)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get state of versions %s: %w", req.CurrentVersion, err)
	}

	currState.SetParent(previousState)
	msg, _, err := state.ApplyState(currState, false)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get history events for versions %s, %s: %w", req.CurrentVersion, req.PreviousVersion, err)
	}

	historyEvents := filterHistoryEvents(msg)
	spc, err := h.spaceService.Get(context.Background(), id.SpaceID)
	if err != nil {
		return nil, nil, fmt.Errorf("get space: %w", err)
	}
	dependentObjectIDs := objectlink.DependentObjectIDs(currState, spc, true, true, false, true, false)
	meta, err := h.objectStore.QueryByID(dependentObjectIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("get dependencies: %w", err)
	}

	meta = append(meta, database.Record{Details: currState.CombinedDetails()})
	details := make([]*model.ObjectViewDetailsSet, 0, len(meta))
	for _, m := range meta {
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
	return historyEvents, objectView, nil
}

func filterHistoryEvents(msg []simple.EventMessage) []*pb.EventMessage {
	var response []*pb.EventMessage
	for _, message := range msg {
		if message.Virtual {
			continue
		}
		if isSuitableChange(message) {
			response = append(response, message.Msg)
		}
	}
	return response
}

func isSuitableChange(message simple.EventMessage) bool {
	return isDataviewChange(message) ||
		isDetailsChange(message) ||
		isRelationsChange(message) ||
		isBlockPropertiesChange(message) ||
		isSimpleBlockChange(message) ||
		isBasicBlockChange(message)
}

func isDataviewChange(message simple.EventMessage) bool {
	return message.Msg.GetBlockDataviewRelationDelete() != nil ||
		message.Msg.GetBlockDataviewSourceSet() != nil ||
		message.Msg.GetBlockDataviewRelationSet() != nil ||
		message.Msg.GetBlockDataviewViewSet() != nil ||
		message.Msg.GetBlockDataviewViewOrder() != nil ||
		message.Msg.GetBlockDataviewViewDelete() != nil ||
		message.Msg.GetBlockDataViewObjectOrderUpdate() != nil ||
		message.Msg.GetBlockDataViewGroupOrderUpdate() != nil ||
		message.Msg.GetBlockDataviewViewUpdate() != nil ||
		message.Msg.GetBlockDataviewTargetObjectIdSet() != nil
}

func isRelationsChange(message simple.EventMessage) bool {
	filterLocalAndDerivedRelations(message.Msg.GetObjectRelationsAmend())
	filterLocalAndDerivedRelationsByKey(message.Msg.GetObjectRelationsRemove())
	return (message.Msg.GetObjectRelationsAmend() != nil && len(message.Msg.GetObjectRelationsAmend().RelationLinks) > 0) ||
		(message.Msg.GetObjectRelationsRemove() != nil && len(message.Msg.GetObjectRelationsRemove().RelationKeys) > 0)
}

func filterLocalAndDerivedRelationsByKey(removedRelations *pb.EventObjectRelationsRemove) {
	if removedRelations == nil {
		return
	}
	var relKeysWithoutLocal []string
	for _, key := range removedRelations.RelationKeys {
		if !slices.Contains(bundle.LocalAndDerivedRelationKeys, key) {
			relKeysWithoutLocal = append(relKeysWithoutLocal, key)
		}
	}
	removedRelations.RelationKeys = relKeysWithoutLocal
}

func filterLocalAndDerivedRelations(addedRelations *pb.EventObjectRelationsAmend) {
	if addedRelations == nil {
		return
	}
	var relLinksWithoutLocal pbtypes.RelationLinks
	for _, link := range addedRelations.RelationLinks {
		if !slices.Contains(bundle.LocalAndDerivedRelationKeys, link.Key) {
			relLinksWithoutLocal = relLinksWithoutLocal.Append(link)
		}
	}
	addedRelations.RelationLinks = relLinksWithoutLocal
}

func isDetailsChange(message simple.EventMessage) bool {
	return message.Msg.GetObjectDetailsAmend() != nil ||
		message.Msg.GetObjectDetailsUnset() != nil
}

func isBlockPropertiesChange(message simple.EventMessage) bool {
	return message.Msg.GetBlockSetAlign() != nil ||
		message.Msg.GetBlockSetChildrenIds() != nil ||
		message.Msg.GetBlockSetBackgroundColor() != nil ||
		message.Msg.GetBlockSetFields() != nil ||
		message.Msg.GetBlockSetVerticalAlign() != nil
}

func isSimpleBlockChange(message simple.EventMessage) bool {
	return message.Msg.GetBlockSetTableRow() != nil ||
		message.Msg.GetBlockSetRelation() != nil ||
		message.Msg.GetBlockSetText() != nil ||
		message.Msg.GetBlockSetLink() != nil ||
		message.Msg.GetBlockSetLatex() != nil ||
		message.Msg.GetBlockSetFile() != nil ||
		message.Msg.GetBlockSetDiv() != nil ||
		message.Msg.GetBlockSetBookmark() != nil
}

func isBasicBlockChange(message simple.EventMessage) bool {
	return message.Msg.GetBlockAdd() != nil ||
		message.Msg.GetBlockDelete() != nil
}

func (h *history) GetBlocksParticipants(id domain.FullID, versionId string, blocks []*model.Block) ([]*model.ObjectViewBlockParticipant, error) {
	if len(blocks) == 0 {
		return nil, nil
	}
	existingBlocks := lo.SliceToMap(blocks, func(item *model.Block) (string, struct{}) { return item.GetId(), struct{}{} })
	tree, _, err := h.treeWithId(id, versionId, true)
	if err != nil {
		return nil, err
	}

	blocksParticipantsMap := make(map[string]string, 0)
	err = tree.IterateFrom(tree.Root().Id, source.UnmarshalChange, func(c *objecttree.Change) (isContinue bool) {
		h.fillBlockParticipantMap(c, id, blocksParticipantsMap, existingBlocks)
		return true
	})
	if err != nil {
		return nil, err
	}

	blocksParticipants := make([]*model.ObjectViewBlockParticipant, 0)
	for blockId, participantId := range blocksParticipantsMap {
		blocksParticipants = append(blocksParticipants, &model.ObjectViewBlockParticipant{
			BlockId:       blockId,
			ParticipantId: participantId,
		})
	}
	return blocksParticipants, nil
}

func (h *history) fillBlockParticipantMap(c *objecttree.Change,
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
	if setBackgroundColor := event.GetBlockSetBackgroundColor(); setBackgroundColor != nil {
		blockId = append(blockId, setBackgroundColor.Id)
	}
	if setFields := event.GetBlockSetFields(); setFields != nil {
		blockId = append(blockId, setFields.Id)
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
	return cache.Do(h.picker, id.ObjectID, func(sb smartblock2.SmartBlock) error {
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

	st, _, _, err = source.BuildState(id.SpaceID, nil, tree, true)
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
