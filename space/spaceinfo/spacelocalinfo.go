package spaceinfo

import (
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

type SpaceLocalInfo struct {
	SpaceId         string
	localStatus     *LocalStatus
	remoteStatus    *RemoteStatus
	shareableStatus *ShareableStatus
	writeLimit      *uint32
	readLimit       *uint32
}

func NewSpaceLocalInfo(spaceId string) SpaceLocalInfo {
	return SpaceLocalInfo{SpaceId: spaceId}
}

func NewSpaceLocalInfoFromState(s state.Doc) SpaceLocalInfo {
	details := s.LocalDetails()
	spaceInfo := NewSpaceLocalInfo(details.GetString(bundle.RelationKeyTargetSpaceId))
	spaceInfo.SetReadLimit(uint32(details.GetInt64(bundle.RelationKeyReadersLimit))).
		SetWriteLimit(uint32(details.GetInt64(bundle.RelationKeyWritersLimit))).
		SetLocalStatus(LocalStatus(details.GetInt64(bundle.RelationKeySpaceLocalStatus))).
		SetRemoteStatus(RemoteStatus(details.GetInt64(bundle.RelationKeySpaceRemoteStatus))).
		SetShareableStatus(ShareableStatus(details.GetInt64(bundle.RelationKeySpaceShareableStatus)))
	return spaceInfo
}

func (s *SpaceLocalInfo) GetLocalStatus() LocalStatus {
	if s.localStatus == nil {
		return LocalStatusUnknown
	}
	return *s.localStatus
}

func (s *SpaceLocalInfo) GetRemoteStatus() RemoteStatus {
	if s.remoteStatus == nil {
		return RemoteStatusUnknown
	}
	return *s.remoteStatus
}

func (s *SpaceLocalInfo) GetShareableStatus() ShareableStatus {
	if s.shareableStatus == nil {
		return ShareableStatusUnknown
	}
	return *s.shareableStatus
}

func (s *SpaceLocalInfo) GetWriteLimit() uint32 {
	if s.writeLimit == nil {
		return 0
	}
	return *s.writeLimit
}

func (s *SpaceLocalInfo) GetReadLimit() uint32 {
	if s.readLimit == nil {
		return 0
	}
	return *s.readLimit
}

func (s *SpaceLocalInfo) SetLocalStatus(status LocalStatus) *SpaceLocalInfo {
	s.localStatus = &status
	return s
}

func (s *SpaceLocalInfo) SetRemoteStatus(status RemoteStatus) *SpaceLocalInfo {
	s.remoteStatus = &status
	return s
}

func (s *SpaceLocalInfo) SetShareableStatus(status ShareableStatus) *SpaceLocalInfo {
	s.shareableStatus = &status
	return s
}

func (s *SpaceLocalInfo) SetWriteLimit(limit uint32) *SpaceLocalInfo {
	s.writeLimit = &limit
	return s
}

func (s *SpaceLocalInfo) SetReadLimit(limit uint32) *SpaceLocalInfo {
	s.readLimit = &limit
	return s
}

func (s *SpaceLocalInfo) UpdateDetails(st *state.State) *SpaceLocalInfo {
	st.SetDetail(bundle.RelationKeyTargetSpaceId, domain.String(s.SpaceId))
	if s.localStatus != nil {
		st.SetDetail(bundle.RelationKeySpaceLocalStatus, domain.Int64(*s.localStatus))
	}
	if s.remoteStatus != nil {
		st.SetDetail(bundle.RelationKeySpaceRemoteStatus, domain.Int64(*s.remoteStatus))
	}
	if s.shareableStatus != nil {
		st.SetDetail(bundle.RelationKeySpaceShareableStatus, domain.Int64(*s.shareableStatus))
	}
	if s.writeLimit != nil {
		st.SetDetail(bundle.RelationKeyWritersLimit, domain.Int64(*s.writeLimit))
	}
	if s.readLimit != nil {
		st.SetDetail(bundle.RelationKeyReadersLimit, domain.Int64(*s.readLimit))
	}
	return s
}

func (s *SpaceLocalInfo) Log(log *logging.Sugared) *SpaceLocalInfo {
	log = log.With("spaceId", s.SpaceId)
	if s.localStatus != nil {
		log = log.With("localStatus", s.localStatus.String())
	}
	if s.remoteStatus != nil {
		log = log.With("remoteStatus", s.remoteStatus.String())
	}
	if s.shareableStatus != nil {
		log = log.With("shareableStatus", s.shareableStatus.String())
	}
	if s.writeLimit != nil {
		log = log.With("writeLimit", *s.writeLimit)
	}
	if s.readLimit != nil {
		log = log.With("readLimit", *s.readLimit)
	}
	log.Info("set local info")
	return s
}
