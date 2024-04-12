package spaceinfo

import (
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type SpacePersistentInfo struct {
	SpaceID       string
	AccountStatus *AccountStatus
	AclHeadId     string
}

func NewSpacePersistentInfo(spaceId string) SpacePersistentInfo {
	return SpacePersistentInfo{SpaceID: spaceId}
}

func NewSpacePersistentInfoFromState(st state.Doc) SpacePersistentInfo {
	details := st.CombinedDetails()
	spaceInfo := NewSpacePersistentInfo(pbtypes.GetString(details, bundle.RelationKeyTargetSpaceId.String()))
	spaceInfo.SetAccountStatus(AccountStatus(pbtypes.GetInt64(details, bundle.RelationKeySpaceAccountStatus.String()))).
		SetAclHeadId(pbtypes.GetString(details, bundle.RelationKeyLatestAclHeadId.String()))
	return spaceInfo
}

func (s *SpacePersistentInfo) UpdateDetails(st *state.State) *SpacePersistentInfo {
	st.SetDetailAndBundledRelation(bundle.RelationKeyTargetSpaceId, pbtypes.String(s.SpaceID))
	if s.AccountStatus != nil {
		st.SetDetailAndBundledRelation(bundle.RelationKeySpaceAccountStatus, pbtypes.Int64(int64(*s.AccountStatus)))
	}
	if s.AclHeadId != "" {
		st.SetDetailAndBundledRelation(bundle.RelationKeyLatestAclHeadId, pbtypes.String(s.AclHeadId))
	}
	return s
}

func (s *SpacePersistentInfo) Log(log *logging.Sugared) *SpacePersistentInfo {
	log = log.With("spaceId", s.SpaceID)
	if s.AccountStatus != nil {
		log = log.With("accountStatus", s.AccountStatus.String())
	}
	if s.AclHeadId != "" {
		log = log.With("aclHeadId", s.AclHeadId)
	}
	log.Info("set space persistent info")
	return s
}

func (s *SpacePersistentInfo) SetAccountStatus(status AccountStatus) *SpacePersistentInfo {
	s.AccountStatus = &status
	return s
}

func (s *SpacePersistentInfo) SetAclHeadId(aclHeadId string) *SpacePersistentInfo {
	s.AclHeadId = aclHeadId
	return s
}

func (s *SpacePersistentInfo) GetAccountStatus() AccountStatus {
	if s.AccountStatus == nil {
		return AccountStatusUnknown
	}
	return *s.AccountStatus
}

func (s *SpacePersistentInfo) GetAclHeadId() string {
	return s.AclHeadId
}
