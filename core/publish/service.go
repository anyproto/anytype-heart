package publish

import (
	"context"
	"errors"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/pb"
)

const CName = "common.core.publishservice"

var (
	ErrLimitExceeded   = errors.New("limit exceeded")
	ErrUrlAlreadyTaken = errors.New("url is already taken by another page")
)

// PublishResult is returned by Publish in offline mode.
type PublishResult struct {
	Url string
}

// Service is the offline no-op implementation.
type Service interface {
	app.ComponentRunnable
	Publish(ctx context.Context, spaceId, pageObjId, uri string, joinSpace bool) (res PublishResult, err error)
	Unpublish(ctx context.Context, spaceId, pageObjId string) error
	PublishList(ctx context.Context, id string) ([]*pb.RpcPublishingPublishState, error)
	ResolveUri(ctx context.Context, uri string) (*pb.RpcPublishingPublishState, error)
	GetStatus(ctx context.Context, spaceId string, objectId string) (*pb.RpcPublishingPublishState, error)
}

type service struct{}

func New() Service {
	return &service{}
}

func (s *service) Name() string { return CName }

func (s *service) Init(a *app.App) error { return nil }

func (s *service) Run(ctx context.Context) error { return nil }

func (s *service) Close(ctx context.Context) error { return nil }

func (s *service) Publish(ctx context.Context, spaceId, pageObjId, uri string, joinSpace bool) (PublishResult, error) {
	return PublishResult{}, nil
}

func (s *service) Unpublish(ctx context.Context, spaceId, pageObjId string) error { return nil }

func (s *service) PublishList(ctx context.Context, id string) ([]*pb.RpcPublishingPublishState, error) {
	return nil, nil
}

func (s *service) ResolveUri(ctx context.Context, uri string) (*pb.RpcPublishingPublishState, error) {
	return nil, nil
}

func (s *service) GetStatus(ctx context.Context, spaceId string, objectId string) (*pb.RpcPublishingPublishState, error) {
	return nil, nil
}
