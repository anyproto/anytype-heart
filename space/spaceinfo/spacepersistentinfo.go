package spaceinfo

import (
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

type SpacePersistentInfo struct {
	SpaceID                    string
	AccountStatus              *AccountStatus
	AclHeadId                  string
	EncodedKey                 string
	Name                       string
	OneToOneIdentity           string
	OneToOneRequestMetadataKey string
	OneToOneInboxSentStatus    OneToOneInboxSentStatus
}

func NewSpacePersistentInfo(spaceId string) SpacePersistentInfo {
	return SpacePersistentInfo{SpaceID: spaceId}
}

func NewSpacePersistentInfoFromState(st state.Doc) SpacePersistentInfo {
	details := st.CombinedDetails()
	spaceInfo := NewSpacePersistentInfo(details.GetString(bundle.RelationKeyTargetSpaceId))
	spaceInfo.SetAccountStatus(AccountStatus(details.GetInt64(bundle.RelationKeySpaceAccountStatus))).
		SetAclHeadId(details.GetString(bundle.RelationKeyLatestAclHeadId)).
		SetEncodedKey(details.GetString(bundle.RelationKeyGuestKey))
	return spaceInfo
}

func (s *SpacePersistentInfo) UpdateDetails(st *state.State) *SpacePersistentInfo {
	st.SetDetailAndBundledRelation(bundle.RelationKeyTargetSpaceId, domain.String(s.SpaceID))
	if s.AccountStatus != nil {
		st.SetDetailAndBundledRelation(bundle.RelationKeySpaceAccountStatus, domain.Int64(*s.AccountStatus))
	}
	if s.AclHeadId != "" {
		st.SetDetailAndBundledRelation(bundle.RelationKeyLatestAclHeadId, domain.String(s.AclHeadId))
	}
	if s.EncodedKey != "" {
		st.SetDetail(bundle.RelationKeyGuestKey, domain.String(s.EncodedKey))
	}
	if s.OneToOneIdentity != "" {
		st.SetDetail(bundle.RelationKeyOneToOneIdentity, domain.String(s.OneToOneIdentity))
	}
	if s.OneToOneRequestMetadataKey != "" {
		st.SetDetail(bundle.RelationKeyOneToOneRequestMetadataKey, domain.String(s.OneToOneRequestMetadataKey))
	}
	if s.OneToOneInboxSentStatus != OneToOneInboxSentStatus_None {
		st.SetDetail(bundle.RelationKeyOneToOneInboxSentStatus, domain.Int64(s.OneToOneInboxSentStatus))
	}

	if s.Name != "" {
		st.SetDetail(bundle.RelationKeyName, domain.String(s.Name))
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

func (s *SpacePersistentInfo) SetEncodedKey(encodedKey string) *SpacePersistentInfo {
	s.EncodedKey = encodedKey
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
