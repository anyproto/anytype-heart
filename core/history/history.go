package history

import (
	"context"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/objecttreebuilder"
	"github.com/gogo/protobuf/proto"
	"github.com/samber/lo"
	"github.com/zeebo/blake3"

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

var hashersPool = &sync.Pool{
	New: func() any {
		return blake3.New()
	},
}

func New() History {
	return &history{heads: make(map[string]string, 0)}
}

type History interface {
	Show(id domain.FullID, versionId string) (bs *model.ObjectView, ver *pb.RpcHistoryVersion, err error)
	Versions(id domain.FullID, lastVersionId string, limit int, notIncludeVersion bool) (resp []*pb.RpcHistoryVersion, err error)
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

	dependentObjectIDs := objectlink.DependentObjectIDs(s, space, objectlink.Flags{
		Blocks:    true,
		Details:   true,
		Relations: false,
		Types:     true,
	})
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

func (h *history) Versions(id domain.FullID, lastVersionId string, limit int, notIncludeVersion bool) (resp []*pb.RpcHistoryVersion, err error) {
	hasher := hashersPool.Get().(*blake3.Hasher)
	defer hashersPool.Put(hasher)
	if limit <= 0 {
		limit = 100
	}
	var includeLastId = !notIncludeVersion

	reverse := func(vers []*pb.RpcHistoryVersion) []*pb.RpcHistoryVersion {
		for i, j := 0, len(vers)-1; i < j; i, j = i+1, j-1 {
			vers[i], vers[j] = vers[j], vers[i]
		}
		return vers
	}

	for len(resp) < limit {
		curHeads := make(map[string]struct{})
		tree, _, e := h.treeWithId(id, lastVersionId, includeLastId)
		if e != nil {
			return nil, e
		}
		var data []*pb.RpcHistoryVersion

		e = tree.IterateFrom(tree.Root().Id, source.UnmarshalChange, func(c *objecttree.Change) (isContinue bool) {
			participantId := domain.NewParticipantId(id.SpaceID, c.Identity.Account())
			data = h.fillVersionData(c, curHeads, participantId, data, hasher)
			return true
		})
		if e != nil {
			return nil, e
		}
		if len(data) == 0 {
			break
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

	if len(resp) > limit {
		resp = resp[:limit]
	}
	return
}

func (h *history) retrieveHeads(versionId string) []string {
	if heads, ok := h.heads[versionId]; ok {
		return strings.Split(heads, " ")
	}
	return []string{versionId}
}

func (h *history) fillVersionData(change *objecttree.Change, curHeads map[string]struct{}, participantId string, data []*pb.RpcHistoryVersion, hasher *blake3.Hasher) []*pb.RpcHistoryVersion {
	curHeads[change.Id] = struct{}{}
	for _, previousId := range change.PreviousIds {
		delete(curHeads, previousId)
	}
	version := &pb.RpcHistoryVersion{
		Id:          change.Id,
		PreviousIds: change.PreviousIds,
		AuthorId:    participantId,
		Time:        change.Timestamp,
	}
	if len(curHeads) > 1 {
		var combinedHeads string
		for head := range curHeads {
			combinedHeads += head + " "
		}
		combinedHeads = strings.TrimSpace(combinedHeads)
		hasher.Reset()
		// nolint: errcheck
		hasher.Write([]byte(combinedHeads)) // it never returns an error
		hashSum := hex.EncodeToString(hasher.Sum(nil))
		h.heads[hashSum] = combinedHeads
		version.Id = hashSum
	}
	return append(data, version)
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
	dependentObjectIDs := objectlink.DependentObjectIDs(currState, spc, objectlink.Flags{
		Blocks:    true,
		Details:   true,
		Relations: false,
		Types:     true,
	})
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

func (h *history) treeWithId(id domain.FullID, versionId string, includeBeforeId bool) (ht objecttree.HistoryTree, sbt smartblock.SmartBlockType, err error) {
	heads := h.retrieveHeads(versionId)
	spc, err := h.spaceService.Get(context.Background(), id.SpaceID)
	if err != nil {
		return
	}
	ht, err = spc.TreeBuilder().BuildHistoryTree(context.Background(), id.ObjectID, objecttreebuilder.HistoryTreeOpts{
		Heads:   heads,
		Include: includeBeforeId,
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
