package bookmarkimporter

import (
	"github.com/anyproto/any-sync/app"
	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"

	bookmarksvc "github.com/anyproto/anytype-heart/core/block/bookmark"
	"github.com/anyproto/anytype-heart/core/block/import"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/util/pbtypes"
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
