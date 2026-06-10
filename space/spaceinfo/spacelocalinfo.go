package spaceinfo

import (
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var spaceInfoLog = logging.Logger("space.spaceinfo")

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
	st.SetDetailAndBundledRelation(bundle.RelationKeyTargetSpaceId, domain.String(s.SpaceId))
	// Log local/remote status transitions at the single chokepoint every status write passes
	// through, so cold-start status churn (GO-7289) can be diagnosed. Only emitted when a value
	// actually changes, to avoid noise from idempotent re-writes.
	if s.localStatus != nil || s.remoteStatus != nil {
		oldLocal := LocalStatus(st.LocalDetails().GetInt64(bundle.RelationKeySpaceLocalStatus))
		oldRemote := RemoteStatus(st.LocalDetails().GetInt64(bundle.RelationKeySpaceRemoteStatus))
		newLocal := oldLocal
		if s.localStatus != nil {
			newLocal = *s.localStatus
		}
		newRemote := oldRemote
		if s.remoteStatus != nil {
			newRemote = *s.remoteStatus
		}
		changed := (s.localStatus != nil && newLocal != oldLocal) ||
			(s.remoteStatus != nil && newRemote != oldRemote)
		if changed {
			spaceInfoLog.With("spaceId", s.SpaceId).
				With("localStatus", oldLocal.String()+"->"+newLocal.String()).
				With("remoteStatus", oldRemote.String()+"->"+newRemote.String()).
				Debug("space status transition in objectstore")
		}
	}
	if s.localStatus != nil {
		st.SetDetailAndBundledRelation(bundle.RelationKeySpaceLocalStatus, domain.Int64(*s.localStatus))
	}
	if s.remoteStatus != nil {
		st.SetDetailAndBundledRelation(bundle.RelationKeySpaceRemoteStatus, domain.Int64(*s.remoteStatus))
	}
	if s.shareableStatus != nil {
		st.SetDetailAndBundledRelation(bundle.RelationKeySpaceShareableStatus, domain.Int64(*s.shareableStatus))
	}
	if s.writeLimit != nil {
		st.SetDetailAndBundledRelation(bundle.RelationKeyWritersLimit, domain.Int64(*s.writeLimit))
	}
	if s.readLimit != nil {
		st.SetDetailAndBundledRelation(bundle.RelationKeyReadersLimit, domain.Int64(*s.readLimit))
	}
	return s
}

// Equal reports whether every field set on s already matches the corresponding
// detail in details. Unset (nil) fields are ignored, mirroring UpdateDetails.
// It lets callers skip a no-op state apply when the status hasn't changed.
func (s *SpaceLocalInfo) Equal(details *domain.Details) bool {
	if details == nil {
		return false
	}
	if details.GetString(bundle.RelationKeyTargetSpaceId) != s.SpaceId {
		return false
	}
	if s.localStatus != nil && details.GetInt64(bundle.RelationKeySpaceLocalStatus) != int64(*s.localStatus) {
		return false
	}
	if s.remoteStatus != nil && details.GetInt64(bundle.RelationKeySpaceRemoteStatus) != int64(*s.remoteStatus) {
		return false
	}
	if s.shareableStatus != nil && details.GetInt64(bundle.RelationKeySpaceShareableStatus) != int64(*s.shareableStatus) {
		return false
	}
	if s.writeLimit != nil && details.GetInt64(bundle.RelationKeyWritersLimit) != int64(*s.writeLimit) {
		return false
	}
	if s.readLimit != nil && details.GetInt64(bundle.RelationKeyReadersLimit) != int64(*s.readLimit) {
		return false
	}
	return true
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
