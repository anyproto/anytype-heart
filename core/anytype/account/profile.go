package account

import (
	"errors"

	"github.com/anyproto/anytype-heart/core/domain"
)

type Profile struct {
	Id          string
	AccountAddr string
	Name        string
	IconImage   string
	IconColor   string
}

func (s *service) ParticipantId(spaceId string) string {
	return domain.NewParticipantId(spaceId, s.AccountID())
}

func (s *service) LocalProfile() (Profile, error) {
	// TODO Fix ID!!!
	profile := Profile{
		Id:          s.ParticipantId("TODO"),
		AccountAddr: s.wallet.GetAccountPrivkey().GetPublic().Account(),
	}

	if s.objectStore == nil {
		return profile, errors.New("objectstore not available")
	}

	profileDetails, err := s.objectStore.GetDetails(profile.Id)
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
