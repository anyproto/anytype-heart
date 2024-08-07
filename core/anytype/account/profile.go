package account

import (
	"context"
	"fmt"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

type Profile struct {
	Id        string
	AccountId string
	Name      string
	IconImage string
	IconColor string
}

func (s *service) MyParticipantId(spaceId string) string {
	return domain.NewParticipantId(spaceId, s.AccountID())
}

func (s *service) ProfileObjectId() (string, error) {
	ids, err := s.getDerivedIds(context.Background(), s.personalSpaceId)
	if err != nil {
		return "", err
	}
	return ids.Profile, nil
}

func (s *service) ProfileInfo() (Profile, error) {
	profileId, err := s.ProfileObjectId()
	if err != nil {
		return Profile{}, fmt.Errorf("get profile id: %w", err)
	}
	profile := Profile{
		Id:        profileId,
		AccountId: s.AccountID(),
	}

	profileDetails, err := s.objectStore.GetDetails(profile.Id)
	if err != nil {
		return profile, err
	}
	if profileDetails != nil {
		profile.Name = profileDetails.GetString(bundle.RelationKeyName)
		profile.IconImage = profileDetails.GetString(bundle.RelationKeyIconImage)
	}

	return profile, nil
}
