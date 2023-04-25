package bookmarkimporter

import (
	"go.uber.org/zap"

	"github.com/anytypeio/go-anytype-infrastructure-experiments/common/app"
	bookmarksvc "github.com/anytypeio/go-anytype-middleware/core/block/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/block/import"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
)

const CName = "bookmark-importer"

var log = logging.Logger("bookmark-importer")

type Importer interface {
	ImportWeb(ctx *session.Context, req *pb.RpcObjectImportRequest) (string, *types.Struct, error)
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

func (bd *BookmarkImporterDecorator) CreateBookmarkObject(details *types.Struct, getContent bookmarksvc.ContentFuture) (objectId string, newDetails *types.Struct, err error) {
	url := pbtypes.GetString(details, bundle.RelationKeySource.String())
	if objectId, newDetails, err = bd.Importer.ImportWeb(nil, &pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfBookmarksParams{BookmarksParams: &pb.RpcObjectImportRequestBookmarksParams{Url: url}},
		UpdateExistingObjects: true,
	}); err != nil {
		log.With(zap.String("function", "BookmarkFetch")).With(zap.String("message", "failed to import bookmark")).Error(err)
		return bd.Service.CreateBookmarkObject(details, getContent)
	}
	err = bd.Service.UpdateBookmarkObject(objectId, getContent)
	if err != nil {
		return "", nil, err
	}
	return objectId, newDetails, nil
}

func (bd *BookmarkImporterDecorator) Name() (name string) {
	return CName
}
