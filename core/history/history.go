package history

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/objecttreebuilder"
	"github.com/gogo/protobuf/proto"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/block"
	smartblock2 "github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	history2 "github.com/anyproto/anytype-heart/core/block/history"
	"github.com/anyproto/anytype-heart/core/block/object/objectlink"
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
	return &model.ObjectView{
		RootId:        id.ObjectID,
		Type:          model.SmartBlockType(sbType),
		Blocks:        s.Blocks(),
		Details:       details,
		RelationLinks: relations.RelationLinks(),
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
			var blockId, fileObjectIds, relationKeys []string
			if changeModel, ok := c.Model.(*pb.Change); ok {
				for _, content := range changeModel.GetContent() {
					blockId = h.handleBlockChanges(content, blockId)
					fileObjectIds = h.handleFileChanges(content, fileObjectIds)
					relationKeys = h.handleRelationChanges(content, relationKeys)
				}
			}

			data = append(data, &pb.RpcHistoryVersion{
				Id:            c.Id,
				PreviousIds:   c.PreviousIds,
				AuthorId:      participantId,
				Time:          c.Timestamp,
				BlockIds:      blockId,
				FileObjectIds: fileObjectIds,
				RelationKeys:  relationKeys,
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

func (h *history) handleRelationChanges(content *pb.ChangeContent, relationKeys []string) []string {
	if c := content.GetDetailsSet(); c != nil {
		relationKeys = append(relationKeys, c.Key)
	}
	if c := content.GetDetailsUnset(); c != nil {
		relationKeys = append(relationKeys, c.Key)
	}
	if c := content.GetRelationAdd(); c != nil {
		for _, link := range c.RelationLinks {
			relationKeys = append(relationKeys, link.Key)
		}
	}
	if c := content.GetRelationRemove(); c != nil {
		relationKeys = append(relationKeys, c.RelationKey...)
	}
	return relationKeys
}

func (h *history) handleFileChanges(content *pb.ChangeContent, fileObjectIds []string) []string {
	if c := content.GetSetFileInfo(); c != nil {
		if c.FileInfo != nil {
			fileObjectIds = append(fileObjectIds, c.FileInfo.FileId)
		}
	}
	return fileObjectIds
}

func (h *history) handleBlockChanges(content *pb.ChangeContent, blockId []string) []string {
	if c := content.GetBlockCreate(); c != nil {
		for _, bl := range c.Blocks {
			blockId = append(blockId, bl.Id)
		}
	}
	if c := content.GetBlockDuplicate(); c != nil {
		blockId = append(blockId, c.Ids...)
	}
	if c := content.GetBlockMove(); c != nil {
		blockId = append(blockId, c.Ids...)
	}
	if c := content.GetBlockRemove(); c != nil {
		blockId = append(blockId, c.Ids...)
	}
	if c := content.GetBlockUpdate(); c != nil {
		for _, event := range c.Events {
			blockId = h.handleAddAndDeleteEvents(event, blockId)
			blockId = h.handleBlockSettingsEvents(event, blockId)
			blockId = h.handleSimpleBlockEvents(event, blockId)
			blockId = h.handleDataviewEvents(event, blockId)
		}
	}
	return blockId
}

func (h *history) handleAddAndDeleteEvents(event *pb.EventMessage, blockId []string) []string {
	if blockAdd := event.GetBlockAdd(); blockAdd != nil {
		for _, bl := range event.GetBlockAdd().Blocks {
			blockId = append(blockId, bl.Id)
		}
	}
	if blockDelete := event.GetBlockDelete(); blockDelete != nil {
		blockId = append(blockId, blockDelete.BlockIds...)
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
	if setText := event.GetBlockSetText(); setText != nil {
		blockId = append(blockId, setText.Id)
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

func (h *history) handleDataviewEvents(event *pb.EventMessage, blockId []string) []string {
	if dvUpdate := event.GetBlockDataviewViewUpdate(); dvUpdate != nil {
		blockId = append(blockId, dvUpdate.Id)
	}
	if dvUpdate := event.GetBlockDataViewGroupOrderUpdate(); dvUpdate != nil {
		blockId = append(blockId, dvUpdate.Id)
	}
	if dvUpdate := event.GetBlockDataViewObjectOrderUpdate(); dvUpdate != nil {
		blockId = append(blockId, dvUpdate.Id)
	}
	if dvUpdate := event.GetBlockDataviewViewDelete(); dvUpdate != nil {
		blockId = append(blockId, dvUpdate.Id)
	}
	if dvUpdate := event.GetBlockDataviewViewOrder(); dvUpdate != nil {
		blockId = append(blockId, dvUpdate.Id)
	}
	if dvUpdate := event.GetBlockDataviewViewSet(); dvUpdate != nil {
		blockId = append(blockId, dvUpdate.Id)
	}
	if dvUpdate := event.GetBlockDataviewSourceSet(); dvUpdate != nil {
		blockId = append(blockId, dvUpdate.Id)
	}
	if dvUpdate := event.GetBlockDataviewRelationDelete(); dvUpdate != nil {
		blockId = append(blockId, dvUpdate.Id)
	}
	if dvUpdate := event.GetBlockDataviewRelationSet(); dvUpdate != nil {
		blockId = append(blockId, dvUpdate.Id)
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
