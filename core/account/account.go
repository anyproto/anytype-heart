package account

import (
	"context"
	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/cafe"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/cafe/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
)

const CName = "account"

var log = logging.Logger("anytype-account")

type Service interface {
	app.Component
	DeleteAccount(ctx context.Context) error
}

func New() Service {
	return &service{}
}

type service struct {
	cafe cafe.Client
}

func (s *service) Init(a *app.App) (err error) {
	s.cafe = a.MustComponent(cafe.CName).(cafe.Client)
	return nil
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) DeleteAccount(ctx context.Context) error {
	_, err := s.cafe.AccountDelete(ctx, &pb.AccountDeleteRequest{})
	return err
}
