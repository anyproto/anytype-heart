package core

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/cafe/pb"
	"github.com/gogo/status"
	"google.golang.org/grpc/codes"
)

type ProfileInfo interface {
	FindProfilesByAccountIDs(ctx context.Context, AccountAddrs []string, ch chan Profile) error
	LocalProfile() (Profile, error)
	ProfileID() string
}

type Profile struct {
	AccountAddr string
	Name        string
	IconImage   string
	IconColor   string
}

func (a *Anytype) FindProfilesByAccountIDs(ctx context.Context, AccountAddrs []string, ch chan Profile) error {
	var errDeadlineExceeded = status.Error(codes.DeadlineExceeded, "deadline exceeded")
	select {
	case <-a.onlineCh:
	case <-ctx.Done():
	}

	if a.cafe == nil {
		close(ch)
		return fmt.Errorf("cafe client not set")
	}

	s, err := a.cafe.ProfileFind(ctx, &pb.ProfileFindRequest{
		AccountAddrs: AccountAddrs,
	})
	if err != nil {
		close(ch)
		return err
	}
	done := make(chan error)

	go func() {
		for {
			resp, err := s.Recv()
			if err != nil {
				close(ch)
				if err != io.EOF && err != errDeadlineExceeded {
					done <- err
					log.Errorf("failed to receive from cafe: %s", err.Error())
				}

				close(done)
				return
			}

			ch <- Profile{
				AccountAddr: resp.AccountAddr,
				Name:        resp.Name,
				IconImage:   resp.IconImage,
				IconColor:   resp.IconColor,
			}
		}
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return status.Error(codes.DeadlineExceeded, "timeouted")
	}

}

func (a *Anytype) LocalProfile() (Profile, error) {
	var (
		profile   = Profile{AccountAddr: a.Account()}
		profileId = a.predefinedBlockIds.Profile
	)

	ps := a.localStore.Pages
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
