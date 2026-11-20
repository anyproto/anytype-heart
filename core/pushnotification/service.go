package pushnotification

import (
	"context"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/pb"
)

const CName = "core.pushnotification.service"

// Service is the offline no-op implementation.
type Service interface {
	app.ComponentRunnable
	RegisterToken(req *pb.RpcPushNotificationRegisterTokenRequest)
	RevokeToken(background context.Context) (err error)
	Notify(ctx context.Context, spaceId, groupId string, topic []string, payload []byte) (err error)
	NotifyRead(ctx context.Context, spaceId, groupId string) (err error)
}

type service struct{}

func New() Service {
	return &service{}
}

func (s *service) Name() string { return CName }

func (s *service) Init(a *app.App) error { return nil }

func (s *service) Run(ctx context.Context) error { return nil }

func (s *service) Close(ctx context.Context) error { return nil }

func (s *service) RegisterToken(req *pb.RpcPushNotificationRegisterTokenRequest) {}

func (s *service) RevokeToken(ctx context.Context) error { return nil }

func (s *service) Notify(ctx context.Context, spaceId, groupId string, topic []string, payload []byte) error {
	return nil
}

func (s *service) NotifyRead(ctx context.Context, spaceId, groupId string) error { return nil }
