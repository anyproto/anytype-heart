package account

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/domain"
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
	return s.spaceService.TechSpace().AccountObjectId()
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

	profileDetails, err := s.objectStore.SpaceIndex(s.spaceService.TechSpaceId()).GetDetails(profile.Id)
	if err != nil {
		return profile, err
	}

	if profileDetails != nil && profileDetails.Details != nil && profileDetails.Details.Fields != nil {
		for _, s := range []struct {
			field    string
			receiver *string
		}{
			{"name", &profile.Name},
			{"iconImage", &profile.IconImage},
			{"iconColor", &profile.IconColor},
		} {
			if value, ok := profileDetails.Details.Fields[s.field]; ok {
				*s.receiver = value.GetStringValue()
			}
		}
	}

	return profile, nil
}
