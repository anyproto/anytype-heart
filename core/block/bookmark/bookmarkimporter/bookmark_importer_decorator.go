package bookmarkimporter

import (
	"context"

	"github.com/anyproto/any-sync/app"

	bookmarksvc "github.com/anyproto/anytype-heart/core/block/bookmark"
	"github.com/anyproto/anytype-heart/core/block/import"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

const CName = "bookmark-importer"

var log = logging.Logger("bookmark-importer")

type Importer interface {
	ImportWeb(ctx context.Context, req *importer.ImportRequest) (string, *domain.Details, error)
}

type BookmarkImporterDecorator struct {
	Importer
	bookmarksvc.Service
	app.Component
}

func New() *BookmarkImporterDecorator {
	return &BookmarkImporterDecorator{}
}

func (bd *BookmarkImporterDecorator) Init(a *app.App) (err error) {
	bd.Service = a.MustComponent(bookmarksvc.CName).(bookmarksvc.Service)
	bd.Importer = a.MustComponent(importer.CName).(importer.Importer)
	return nil
}

func (bd *BookmarkImporterDecorator) CreateBookmarkObject(ctx context.Context, spaceID, templateId string, details *domain.Details, getContent bookmarksvc.ContentFuture) (objectId string, newDetails *domain.Details, err error) {
	url := details.GetString(bundle.RelationKeySource)
	if objectId, newDetails, err = bd.Importer.ImportWeb(nil, &importer.ImportRequest{
		RpcObjectImportRequest: &pb.RpcObjectImportRequest{
			Params:                &pb.RpcObjectImportRequestParamsOfBookmarksParams{BookmarksParams: &pb.RpcObjectImportRequestBookmarksParams{Url: url}},
			UpdateExistingObjects: true,
		},
	}); err != nil {
		log.With(
			"function", "BookmarkFetch",
			"message", "failed to import bookmark",
		).Error(err)
		return bd.Service.CreateBookmarkObject(ctx, spaceID, templateId, details, getContent)
	}
	err = bd.Service.UpdateObject(objectId, getContent())
	if err != nil {
		return "", nil, err
	}
	return objectId, newDetails, nil
}

func (bd *BookmarkImporterDecorator) Name() (name string) {
	return CName
}
