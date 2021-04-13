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
		profile   = Profile{AccountAddr: a.Account()}
		profileId = a.predefinedBlockIds.Profile
	)

	ps := a.objectStore
	if ps == nil {
		return profile, errors.New("no pagestore available")
	}

	profileDetails, err := ps.GetDetails(profileId)
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
