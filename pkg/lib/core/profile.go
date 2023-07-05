package core

import (
	"errors"
)

type ProfileInfo interface {
	LocalProfile(spaceID string) (Profile, error)
	ProfileID(spaceID string) string
}

type Profile struct {
	AccountAddr string
	Name        string
	IconImage   string
	IconColor   string
}

func (a *Anytype) LocalProfile(spaceID string) (Profile, error) {
	profile := Profile{AccountAddr: a.wallet.GetAccountPrivkey().GetPublic().Account()}
	profileID := a.PredefinedObjects(spaceID).Profile

	if a.objectStore == nil {
		return profile, errors.New("objectstore not available")
	}

	profileDetails, err := a.objectStore.GetDetails(profileID)
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

func (a *Anytype) ProfileID(spaceID string) string {
	return a.PredefinedObjects(spaceID).Profile
}
