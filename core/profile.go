package core

import (
	"context"
	"fmt"
	"io"

	"github.com/anytypeio/go-anytype-library/cafe/pb"
	"github.com/gogo/status"
	"google.golang.org/grpc/codes"
)

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
		return fmt.Errorf("cafe client not set")
	}

	s, err := a.cafe.ProfileFind(ctx, &pb.ProfileFindRequest{
		AccountAddrs: AccountAddrs,
	})
	if err != nil {
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
