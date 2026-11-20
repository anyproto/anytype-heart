package bookmark

import (
	"context"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/block/simple/bookmark"
	"github.com/anyproto/anytype-heart/core/domain"
)

const CName = "bookmark"

// ContentFuture represents asynchronous result of getting bookmark content.
type ContentFuture func() *bookmark.ObjectContent

// Service is the offline no-op implementation.
type Service interface {
	CreateObjectAndFetch(
		ctx context.Context, spaceId, templateId string, details *domain.Details,
	) (objectID string, newDetails *domain.Details, err error)
	CreateBookmarkObject(
		ctx context.Context, spaceId, templateId string, details *domain.Details, getContent ContentFuture,
	) (objectId string, newDetails *domain.Details, err error)

	UpdateObject(objectId string, getContent *bookmark.ObjectContent) error
	FetchAsync(spaceID string, blockID string, params bookmark.FetchParams)
	FetchBookmarkContent(spaceID string, url string, parseBlock bool) ContentFuture
	ContentUpdaters(spaceID string, url string, parseBlock bool) (chan func(contentBookmark *bookmark.ObjectContent), error)

	app.Component
}

type service struct{}

func New() Service {
	return &service{}
}

func (s *service) Name() string { return CName }

func (s *service) Init(a *app.App) error { return nil }

func (s *service) CreateObjectAndFetch(
	ctx context.Context, spaceId, templateId string, details *domain.Details,
) (objectID string, newDetails *domain.Details, err error) {
	return "", nil, nil
}

func (s *service) CreateBookmarkObject(
	ctx context.Context, spaceId, templateId string, details *domain.Details, getContent ContentFuture,
) (objectId string, newDetails *domain.Details, err error) {
	return "", nil, nil
}

func (s *service) UpdateObject(objectId string, getContent *bookmark.ObjectContent) error { return nil }

func (s *service) FetchAsync(spaceID string, blockID string, params bookmark.FetchParams) {}

func (s *service) FetchBookmarkContent(spaceID string, url string, parseBlock bool) ContentFuture {
	return func() *bookmark.ObjectContent { return nil }
}

func (s *service) ContentUpdaters(spaceID string, url string, parseBlock bool) (chan func(contentBookmark *bookmark.ObjectContent), error) {
	ch := make(chan func(contentBookmark *bookmark.ObjectContent))
	close(ch)
	return ch, nil
}
