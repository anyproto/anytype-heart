package history

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/objecttreebuilder"
	"github.com/gogo/protobuf/proto"
	"golang.org/x/exp/slices"

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

const (
	BlockAdded = iota
	BlockChanged
	BlockRemoved
	BlockMoved
	Nothing
)

func New() History {
	return new(history)
}

type History interface {
	Show(id domain.FullID, versionId string) (bs *model.ObjectView, ver *pb.RpcHistoryVersion, err error)
	Versions(id domain.FullID, lastVersionId string, limit int) (resp []*pb.RpcHistoryVersion, err error)
	SetVersion(id domain.FullID, versionId string) (err error)
	Diff(id domain.FullID, beforeVersion, afterVersion string) (diff *model.StateDiff, err error)
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

func (h *history) Diff(id domain.FullID, beforeVersion, afterVersion string) (diff *model.StateDiff, err error) {
	changed, removed, added, moved, err := h.diffVersions(id, beforeVersion, afterVersion)
	if err != nil {
		return
	}
	return &model.StateDiff{
		Blocks: &model.StateDiffBlocks{
			ChangedIds: changed,
			RemovedIds: removed,
			AddedIds:   added,
			MovedIds:   moved,
		},
	}, nil
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

func (h *history) treeWithHeads(id domain.FullID, heads []string) (ht objecttree.HistoryTree, sbt smartblock.SmartBlockType, err error) {
	spc, err := h.spaceService.Get(context.Background(), id.SpaceID)
	if err != nil {
		return
	}
	ht, err = spc.TreeBuilder().BuildHistoryTree(context.Background(), id.ObjectID, objecttreebuilder.HistoryTreeOpts{
		HeadIds: heads,
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

func (h *history) diffHeads(id domain.FullID, beforeHeads, afterHeads []string) (changed, removed, added, moved []string, err error) {
	before, _, err := h.treeWithHeads(id, beforeHeads)
	if err != nil {
		return
	}
	after, _, err := h.treeWithHeads(id, afterHeads)
	if err != nil {
		return
	}
	return h.diffTrees(id.SpaceID, before, after)
}

func (h *history) diffVersions(id domain.FullID, beforeId, afterId string) (changed, removed, added, moved []string, err error) {
	before, _, err := h.treeWithId(id, beforeId, true)
	if err != nil {
		return
	}
	after, _, err := h.treeWithId(id, afterId, true)
	if err != nil {
		return
	}
	return h.diffTrees(id.SpaceID, before, after)
}

func (h *history) diffTrees(spaceId string, before, after objecttree.HistoryTree) (changed, removed, added, moved []string, err error) {
	buildState := func(tree objecttree.HistoryTree) (st *state.State, err error) {
		st, _, _, err = source.BuildState(spaceId, nil, tree)
		if err != nil {
			return
		}
		if _, _, err = state.ApplyStateFast(st); err != nil {
			return
		}
		st.BlocksInit(st)
		return
	}
	beforeState, err := buildState(before)
	if err != nil {
		return
	}
	afterState, err := buildState(after)
	if err != nil {
		return
	}
	afterState.SetParent(beforeState)
	_, _, err = state.ApplyState(afterState, true)
	if err != nil {
		return
	}
	changes := afterState.GetChanges()
	for _, ch := range changes {
		ids, action := h.changeType(ch)
		switch action {
		case BlockAdded:
			added = append(added, ids...)
		case BlockChanged:
			for _, id := range ids {
				if !slices.Contains(changed, id) {
					changed = append(changed, id)
				}
			}
		case BlockMoved:
			moved = append(moved, ids...)
		case BlockRemoved:
			removed = append(removed, ids...)
		default:
		}
	}
	return
}

func (h *history) idFromMessage(msg *pb.EventMessage) (id string) {
	switch msg.Value.(type) {
	case *pb.EventMessageValueOfBlockSetAlign:
		return msg.Value.(*pb.EventMessageValueOfBlockSetAlign).BlockSetAlign.Id
	case *pb.EventMessageValueOfBlockSetBackgroundColor:
		return msg.Value.(*pb.EventMessageValueOfBlockSetBackgroundColor).BlockSetBackgroundColor.Id
	case *pb.EventMessageValueOfBlockSetBookmark:
		return msg.Value.(*pb.EventMessageValueOfBlockSetBookmark).BlockSetBookmark.Id
	case *pb.EventMessageValueOfBlockSetVerticalAlign:
		return msg.Value.(*pb.EventMessageValueOfBlockSetVerticalAlign).BlockSetVerticalAlign.Id
	case *pb.EventMessageValueOfBlockSetDiv:
		return msg.Value.(*pb.EventMessageValueOfBlockSetDiv).BlockSetDiv.Id
	case *pb.EventMessageValueOfBlockSetText:
		return msg.Value.(*pb.EventMessageValueOfBlockSetText).BlockSetText.Id
	case *pb.EventMessageValueOfBlockSetFields:
		return msg.Value.(*pb.EventMessageValueOfBlockSetFields).BlockSetFields.Id
	case *pb.EventMessageValueOfBlockSetFile:
		return msg.Value.(*pb.EventMessageValueOfBlockSetFile).BlockSetFile.Id
	case *pb.EventMessageValueOfBlockSetLink:
		return msg.Value.(*pb.EventMessageValueOfBlockSetLink).BlockSetLink.Id
	case *pb.EventMessageValueOfBlockSetRelation:
		return msg.Value.(*pb.EventMessageValueOfBlockSetRelation).BlockSetRelation.Id
	case *pb.EventMessageValueOfBlockSetLatex:
		return msg.Value.(*pb.EventMessageValueOfBlockSetLatex).BlockSetLatex.Id
	case *pb.EventMessageValueOfBlockSetWidget:
		return msg.Value.(*pb.EventMessageValueOfBlockSetWidget).BlockSetWidget.Id
	case *pb.EventMessageValueOfBlockDataviewSourceSet:
		return msg.Value.(*pb.EventMessageValueOfBlockDataviewSourceSet).BlockDataviewSourceSet.Id
	case *pb.EventMessageValueOfBlockDataviewViewSet:
		return msg.Value.(*pb.EventMessageValueOfBlockDataviewViewSet).BlockDataviewViewSet.Id
	case *pb.EventMessageValueOfBlockDataviewViewOrder:
		return msg.Value.(*pb.EventMessageValueOfBlockDataviewViewOrder).BlockDataviewViewOrder.Id
	case *pb.EventMessageValueOfBlockDataviewViewDelete:
		return msg.Value.(*pb.EventMessageValueOfBlockDataviewViewDelete).BlockDataviewViewDelete.Id
	case *pb.EventMessageValueOfBlockDataviewRelationSet:
		return msg.Value.(*pb.EventMessageValueOfBlockDataviewRelationSet).BlockDataviewRelationSet.Id
	case *pb.EventMessageValueOfBlockDataviewRelationDelete:
		return msg.Value.(*pb.EventMessageValueOfBlockDataviewRelationDelete).BlockDataviewRelationDelete.Id
	case *pb.EventMessageValueOfBlockDataViewGroupOrderUpdate:
		return msg.Value.(*pb.EventMessageValueOfBlockDataViewGroupOrderUpdate).BlockDataViewGroupOrderUpdate.Id
	case *pb.EventMessageValueOfObjectRelationsAmend:
		return msg.Value.(*pb.EventMessageValueOfObjectRelationsAmend).ObjectRelationsAmend.Id
	case *pb.EventMessageValueOfObjectRelationsRemove:
		return msg.Value.(*pb.EventMessageValueOfObjectRelationsRemove).ObjectRelationsRemove.Id
	case *pb.EventMessageValueOfBlockDataViewObjectOrderUpdate:
		return msg.Value.(*pb.EventMessageValueOfBlockDataViewObjectOrderUpdate).BlockDataViewObjectOrderUpdate.Id
	case *pb.EventMessageValueOfBlockDataviewViewUpdate:
		return msg.Value.(*pb.EventMessageValueOfBlockDataviewViewUpdate).BlockDataviewViewUpdate.Id
	case *pb.EventMessageValueOfBlockDataviewTargetObjectIdSet:
		return msg.Value.(*pb.EventMessageValueOfBlockDataviewTargetObjectIdSet).BlockDataviewTargetObjectIdSet.Id
	case *pb.EventMessageValueOfBlockDataviewIsCollectionSet:
		return msg.Value.(*pb.EventMessageValueOfBlockDataviewIsCollectionSet).BlockDataviewIsCollectionSet.Id
	case *pb.EventMessageValueOfBlockSetRestrictions:
		return msg.Value.(*pb.EventMessageValueOfBlockSetRestrictions).BlockSetRestrictions.Id
	default:
		return ""
	}
}

func (h *history) changeType(ch *pb.ChangeContent) (ids []string, action int) {
	switch {
	case ch.GetBlockCreate() != nil:
		for _, id := range ch.GetBlockCreate().Blocks {
			ids = append(ids, id.Id)
		}
		return ids, BlockAdded
	case ch.GetBlockRemove() != nil:
		return ch.GetBlockRemove().Ids, BlockRemoved
	case ch.GetBlockUpdate() != nil:
		for _, ev := range ch.GetBlockUpdate().Events {
			id := h.idFromMessage(ev)
			if id != "" {
				ids = append(ids, id)
			}
		}
		return ids, BlockChanged
	case ch.GetBlockMove() != nil:
		return ch.GetBlockMove().Ids, BlockMoved
	default:
		return nil, Nothing
	}
}

func (h *history) buildState(id domain.FullID, versionId string) (st *state.State, sbType smartblock.SmartBlockType, ver *pb.RpcHistoryVersion, err error) {
	tree, sbType, err := h.treeWithId(id, versionId, true)
	if err != nil {
		return
	}

	st, _, _, err = source.BuildState(id.SpaceID, nil, tree)
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
