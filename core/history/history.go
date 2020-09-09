package history

import (
	"fmt"
	"time"

	"github.com/anytypeio/go-anytype-middleware/change"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

const versionGroupInterval = time.Minute * 5

func NewHistory(a anytype.Service, bs BlockService, m meta.Service) History {
	return &history{
		a:    a,
		bs:   bs,
		meta: m,
	}
}

type History interface {
	Show(pageId, versionId string) (bs *pb.EventBlockShow, ver *pb.RpcHistoryVersionsVersion, err error)
	Versions(pageId, lastVersionId string, limit int) (resp []*pb.RpcHistoryVersionsVersion, err error)
	SetVersion(pageId, versionId string) (err error)
}

type BlockService interface {
	ResetToState(pageId string, s *state.State) (err error)
}

type history struct {
	a    anytype.Service
	bs   BlockService
	meta meta.Service
}

func (h *history) Show(pageId, versionId string) (bs *pb.EventBlockShow, ver *pb.RpcHistoryVersionsVersion, err error) {
	s, ver, err := h.buildState(pageId, versionId)
	if err != nil {
		return
	}

	metaD := h.meta.FetchDetails(s.DepSmartIds())
	details := make([]*pb.EventBlockSetDetails, 0, len(metaD))
	for _, m := range metaD {
		details = append(details, &pb.EventBlockSetDetails{
			Id:      m.BlockId,
			Details: m.Details,
		})
	}
	details = append(details, &pb.EventBlockSetDetails{
		Id:      pageId,
		Details: s.Details(),
	})
	return &pb.EventBlockShow{
		RootId:  pageId,
		Blocks:  s.Blocks(),
		Details: details,
	}, ver, nil
}

func (h *history) Versions(pageId, lastVersionId string, limit int) (resp []*pb.RpcHistoryVersionsVersion, err error) {
	if limit <= 0 {
		limit = 100
	}
	profileId, profileName, err := h.getProfileInfo()
	if err != nil {
		return
	}
	var includeLastId = true
	var groupId int64
	var prevVersionTimestamp int64

	reverse := func(vers []*pb.RpcHistoryVersionsVersion) []*pb.RpcHistoryVersionsVersion {
		for i, j := 0, len(vers)-1; i < j; i, j = i+1, j-1 {
			vers[i], vers[j] = vers[j], vers[i]
		}
		return vers
	}

	for len(resp) < limit {
		tree, e := h.buildTree(pageId, lastVersionId, includeLastId)
		if e != nil {
			return nil, e
		}
		var data []*pb.RpcHistoryVersionsVersion

		tree.Iterate(tree.RootId(), func(c *change.Change) (isContinue bool) {
			if c.Timestamp-prevVersionTimestamp > int64(versionGroupInterval.Seconds()) {
				groupId++
			}
			prevVersionTimestamp = c.Timestamp

			data = append(data, &pb.RpcHistoryVersionsVersion{
				Id:          c.Id,
				PreviousIds: c.PreviousIds,
				AuthorId:    profileId,
				AuthorName:  profileName,
				Time:        c.Timestamp,
				GroupId:     groupId,
			})
			return true
		})
		resp = append(data, resp...)
		lastVersionId = tree.RootId()
		includeLastId = false
		if len(data) == 0 || len(data[0].PreviousIds) == 0 {
			resp = reverse(resp)
			return
		}
	}

	resp = reverse(resp)
	return
}

func (h *history) SetVersion(pageId, versionId string) (err error) {
	s, _, err := h.buildState(pageId, versionId)
	if err != nil {
		return
	}
	return h.bs.ResetToState(pageId, s)
}

func (h *history) buildTree(pageId, versionId string, includeLastId bool) (*change.Tree, error) {
	sb, err := h.a.GetBlock(pageId)
	if err != nil {
		err = fmt.Errorf("history: anytype.GetBlock error: %v", err)
		return nil, nil
	}
	return change.BuildTreeBefore(sb, versionId, includeLastId)
}

func (h *history) buildState(pageId, versionId string) (s *state.State, ver *pb.RpcHistoryVersionsVersion, err error) {
	tree, err := h.buildTree(pageId, versionId, true)
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
	if ch := tree.Get(versionId); ch != nil {
		profileId, profileName, e := h.getProfileInfo()
		if e != nil {
			err = e
			return
		}
		ver = &pb.RpcHistoryVersionsVersion{
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
	profileId = h.a.PredefinedBlocks().Profile
	ps := h.a.PageStore()
	if ps == nil {
		return
	}
	profileDetails, err := ps.GetDetails(profileId)
	if err != nil {
		return
	}
	if profileDetails != nil && profileDetails.Details != nil && profileDetails.Details.Fields != nil {
		if name, ok := profileDetails.Details.Fields["name"]; ok {
			profileName = name.GetStringValue()
		}
	}
	return
}
