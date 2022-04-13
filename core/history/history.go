package history

import (
	"fmt"
	"time"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/change"
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

const CName = "history"

const versionGroupInterval = time.Minute * 5

var log = logging.Logger("anytype-mw-history")

func New() History {
	return new(history)
}

type History interface {
	Show(pageId, versionId string) (bs *pb.EventObjectShow, ver *pb.RpcHistoryVersion, err error)
	Versions(pageId, lastVersionId string, limit int) (resp []*pb.RpcHistoryVersion, err error)
	SetVersion(pageId, versionId string) (err error)
	app.Component
}

type BlockService interface {
	ResetToState(pageId string, s *state.State) (err error)
}

type history struct {
	a            core.Service
	blockService BlockService
	objectStore  objectstore.ObjectStore
}

func (h *history) Init(a *app.App) (err error) {
	h.a = a.MustComponent(core.CName).(core.Service)
	h.blockService = a.MustComponent(block.CName).(BlockService)
	h.objectStore = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	return
}

func (h *history) Name() (name string) {
	return CName
}

func (h *history) Show(pageId, versionId string) (bs *pb.EventObjectShow, ver *pb.RpcHistoryVersion, err error) {
	s, ver, err := h.buildState(pageId, versionId)
	if err != nil {
		return
	}

	metaD, _ := h.objectStore.QueryById(s.DepSmartIds())
	details := make([]*pb.EventObjectDetailsSet, 0, len(metaD))
	var uniqueObjTypes []string
	sbType, err := smartblock.SmartBlockTypeFromID(pageId)
	if err != nil {
		return nil, nil, fmt.Errorf("incorrect sb type: %w", err)
	}
	metaD = append(metaD, database.Record{Details: s.CombinedDetails()})
	uniqueObjTypes = s.ObjectTypes()
	for _, m := range metaD {
		details = append(details, &pb.EventObjectDetailsSet{
			Id:      pbtypes.GetString(m.Details, bundle.RelationKeyId.String()),
			Details: m.Details,
		})

		if ot := pbtypes.GetString(m.Details, bundle.RelationKeyType.String()); ot != "" {
			if slice.FindPos(uniqueObjTypes, ot) == -1 {
				uniqueObjTypes = append(uniqueObjTypes, ot)
			}
		}
	}

	objectTypes, _ := objectstore.GetObjectTypes(h.objectStore, uniqueObjTypes)
	return &pb.EventObjectShow{
		RootId:      pageId,
		Type:        model.SmartBlockType(sbType),
		Blocks:      s.Blocks(),
		Details:     details,
		ObjectTypes: objectTypes,
		Relations:   s.ExtraRelations(),
	}, ver, nil
}

func (h *history) Versions(pageId, lastVersionId string, limit int) (resp []*pb.RpcHistoryVersion, err error) {
	if limit <= 0 {
		limit = 100
	}
	profileId, profileName, err := h.getProfileInfo()
	if err != nil {
		return
	}
	var includeLastId = true

	reverse := func(vers []*pb.RpcHistoryVersion) []*pb.RpcHistoryVersion {
		for i, j := 0, len(vers)-1; i < j; i, j = i+1, j-1 {
			vers[i], vers[j] = vers[j], vers[i]
		}
		return vers
	}

	for len(resp) < limit {
		tree, _, e := h.buildTree(pageId, lastVersionId, includeLastId)
		if e != nil {
			return nil, e
		}
		var data []*pb.RpcHistoryVersion

		tree.Iterate(tree.RootId(), func(c *change.Change) (isContinue bool) {
			data = append(data, &pb.RpcHistoryVersion{
				Id:          c.Id,
				PreviousIds: c.PreviousIds,
				AuthorId:    profileId,
				AuthorName:  profileName,
				Time:        c.Timestamp,
			})
			return true
		})
		if len(data[0].PreviousIds) == 0 {
			if h.isEmpty(tree.Get(data[0].Id)) {
				data = data[1:]
			}
			resp = append(data, resp...)
			break
		} else {
			resp = append(data, resp...)
			lastVersionId = tree.RootId()
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

func (h *history) isEmpty(c *change.Change) bool {
	if c.Snapshot != nil && c.Snapshot.Data != nil {
		if c.Snapshot.Data.Details != nil && c.Snapshot.Data.Details.Fields != nil && len(c.Snapshot.Data.Details.Fields) > 0 {
			return false
		}
		for _, b := range c.Snapshot.Data.Blocks {
			if b.GetSmartblock() != nil && b.GetLayout() != nil {
				return false
			}
		}
		return true
	}
	return false
}

func (h *history) SetVersion(pageId, versionId string) (err error) {
	s, _, err := h.buildState(pageId, versionId)
	if err != nil {
		return
	}
	return h.blockService.ResetToState(pageId, s)
}

func (h *history) buildTree(pageId, versionId string, includeLastId bool) (tree *change.Tree, blockType smartblock.SmartBlockType, err error) {
	sb, err := h.a.GetBlock(pageId)
	if err != nil {
		err = fmt.Errorf("history: anytype.GetBlock error: %v", err)
		return
	}
	if versionId != "" {
		if tree, err = change.BuildTreeBefore(sb, versionId, includeLastId); err != nil {
			return
		}
	} else {
		if tree, _, err = change.BuildTree(sb); err != nil {
			return
		}
	}
	return tree, sb.Type(), nil
}

func (h *history) buildState(pageId, versionId string) (s *state.State, ver *pb.RpcHistoryVersion, err error) {
	tree, sbType, err := h.buildTree(pageId, versionId, true)
	if err != nil {
		return
	}
	root := tree.Root()
	if root == nil || root.GetSnapshot() == nil {
		return nil, nil, fmt.Errorf("root missing or not a snapshot")
	}
	s = state.NewDocFromSnapshot(pageId, root.GetSnapshot()).(*state.State)
	s.SetChangeId(root.Id)
	st, err := change.BuildStateSimpleCRDT(s, tree)
	if err != nil {
		return
	}
	if _, _, err = state.ApplyStateFast(st); err != nil {
		return
	}
	switch sbType {
	case smartblock.SmartBlockTypePage, smartblock.SmartBlockTypeProfilePage, smartblock.SmartBlockTypeSet:
		// todo: set case not handled
		template.InitTemplate(s, template.WithTitle)
	}
	s.BlocksInit(s)
	if ch := tree.Get(versionId); ch != nil {
		profileId, profileName, e := h.getProfileInfo()
		if e != nil {
			err = e
			return
		}
		ver = &pb.RpcHistoryVersion{
			Id:          ch.Id,
			PreviousIds: ch.PreviousIds,
			AuthorId:    profileId,
			AuthorName:  profileName,
			Time:        ch.Timestamp,
		}
	}
	return
}

func (h *history) getProfileInfo() (profileId, profileName string, err error) {
	profileId = h.a.ProfileID()
	lp, err := h.a.LocalProfile()
	if err != nil {
		return
	}
	profileName = lp.Name
	return
}
