package core

import (
	"errors"
)

type ProfileInfo interface {
	LocalProfile() (Profile, error)
	ProfileID() string
}

type Profile struct {
	AccountAddr string
	Name        string
	IconImage   string
	IconColor   string
}

func (a *Anytype) LocalProfile() (Profile, error) {
	var (
		profile   = Profile{AccountAddr: a.wallet.GetAccountPrivkey().GetPublic().Account()}
		profileId = a.predefinedBlockIds.Profile
	)
	if a.objectStore == nil {
		return profile, errors.New("objectstore not available")
	}

	profileDetails, err := a.objectStore.GetDetails(profileId)
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

func (a *Anytype) ProfileID() string {
	return a.predefinedBlockIds.Profile
}
