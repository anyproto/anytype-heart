package profilefinder

import (
	"context"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/cafe"
	cafePb "github.com/anytypeio/go-anytype-middleware/pkg/lib/cafe/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io"
	"sync"

	"github.com/anytypeio/go-anytype-middleware/app"
)

const CName = "process"

var log = logging.Logger("anytype-profilefinder")

type Service interface {
	app.Component
	FindProfilesByAccountIDs(ctx context.Context, AccountAddrs []string, ch chan core.Profile) error
}

func New() Service {
	return &service{}
}

type service struct {
	cafe cafe.Client
	m    sync.Mutex
}

func (s *service) Init(a *app.App) (err error) {
	s.cafe = a.MustComponent(cafe.CName).(cafe.Client)
	return nil
}

func (s *service) Name() (name string) {
	return CName
}

func (a *service) FindProfilesByAccountIDs(ctx context.Context, AccountAddrs []string, ch chan core.Profile) error {
	var errDeadlineExceeded = status.Error(codes.DeadlineExceeded, "deadline exceeded")

	if a.cafe == nil {
		close(ch)
		return fmt.Errorf("cafe client not set")
	}

	s, err := a.cafe.ProfileFind(ctx, &cafePb.ProfileFindRequest{
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

			ch <- core.Profile{
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
