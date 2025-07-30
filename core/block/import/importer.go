package importer

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/google/uuid"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/detailservice"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	creator "github.com/anyproto/anytype-heart/core/block/import/common/objectcreator"
	"github.com/anyproto/anytype-heart/core/block/import/common/objectid"
	"github.com/anyproto/anytype-heart/core/block/import/common/syncer"
	"github.com/anyproto/anytype-heart/core/block/import/csv"
	"github.com/anyproto/anytype-heart/core/block/import/html"
	"github.com/anyproto/anytype-heart/core/block/import/markdown"
	"github.com/anyproto/anytype-heart/core/block/import/notion"
	pbc "github.com/anyproto/anytype-heart/core/block/import/pb"
	"github.com/anyproto/anytype-heart/core/block/import/txt"
	"github.com/anyproto/anytype-heart/core/block/import/web"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/core/files/filesync"
	"github.com/anyproto/anytype-heart/core/notifications"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/conc"
)

var log = logging.Logger("import")

const CName = "importer"

const workerPoolSize = 10

type Import struct {
	deps *Dependencies

	componentCtx    context.Context
	componentCancel context.CancelFunc
}

func New() Importer {
	return &Import{
		deps: &Dependencies{
			converters: make(map[string]common.Converter),
		},
	}
}

func (i *Import) Init(a *app.App) (err error) {
	i.initDependencies(a)
	i.setupConverters(a)
	i.componentCtx, i.componentCancel = context.WithCancel(context.Background())
	return nil
}

func (i *Import) initDependencies(a *app.App) {
	i.deps.blockService = app.MustComponent[*block.Service](a)
	i.deps.spaceService = app.MustComponent[space.Service](a)
	i.deps.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	i.deps.tempDirProvider = app.MustComponent[core.TempDirProvider](a)
	i.deps.fileSync = app.MustComponent[filesync.FileSync](a)
	i.deps.notificationService = app.MustComponent[notifications.Notifications](a)
	i.deps.eventSender = app.MustComponent[event.Sender](a)

	fileObjectService := app.MustComponent[fileobject.Service](a)
	i.deps.idProvider = objectid.NewIDProvider(
		i.deps.objectStore,
		i.deps.spaceService,
		i.deps.blockService,
		fileObjectService,
	)

	factory := syncer.New(
		syncer.NewFileSyncer(i.deps.blockService, fileObjectService),
		syncer.NewBookmarkSyncer(i.deps.blockService),
		syncer.NewIconSyncer(i.deps.blockService, fileObjectService),
	)
	relationSyncer := syncer.NewFileRelationSyncer(i.deps.blockService, fileObjectService)
	objectCreator := app.MustComponent[objectcreator.Service](a)
	detailsService := app.MustComponent[detailservice.Service](a)

	i.deps.objectCreator = creator.New(
		detailsService,
		factory,
		i.deps.objectStore,
		relationSyncer,
		i.deps.spaceService,
		objectCreator,
		i.deps.blockService,
	)
}

func (i *Import) setupConverters(a *app.App) {
	accountService := app.MustComponent[account.Service](a)
	collectionService := app.MustComponent[*collection.Service](a)
	tempDirProvider := app.MustComponent[core.TempDirProvider](a)

	converters := []common.Converter{
		markdown.New(tempDirProvider, collectionService),
		notion.New(collectionService),
		pbc.New(collectionService, accountService, tempDirProvider),
		web.NewConverter(),
		html.New(collectionService, tempDirProvider),
		txt.New(collectionService),
		csv.New(collectionService),
	}
	for _, c := range converters {
		i.deps.converters[c.Name()] = c
	}
	// temporary, until we don't have specific logic for obsidian import
	i.deps.converters[model.Import_Obsidian.String()] = i.deps.converters[model.Import_Markdown.String()]
}

func (i *Import) Name() string {
	return CName
}

func (i *Import) Run(_ context.Context) (err error) {
	return
}

func (i *Import) Close(_ context.Context) (err error) {
	if i.componentCancel != nil {
		i.componentCancel()
	}
	return
}

// Import get snapshots from converter or external api and create smartblocks from them
func (i *Import) Import(ctx context.Context, importRequest *ImportRequest) *ImportResponse {
	if importRequest.IsSync {
		return i.importObjects(ctx, importRequest)
	}
	conc.Go(func() {
		res := i.importObjects(i.componentCtx, importRequest)
		if res.Err != nil {
			log.Errorf("import from %s failed with error: %s", importRequest.Type.String(), res.Err)
		}
	})
	return nil
}

func (i *Import) importObjects(ctx context.Context, req *ImportRequest) *ImportResponse {
	processor := NewProcessor(i.deps, req)
	return processor.Execute(ctx)
}

func (i *Import) provideNotification(returnedErr error, progress process.Progress, req *ImportRequest) *model.Notification {
	return &model.Notification{
		Id:      uuid.New().String(),
		Status:  model.Notification_Created,
		IsLocal: true,
		Space:   req.SpaceId,
		Payload: &model.NotificationPayloadOfImport{Import: &model.NotificationImport{
			ProcessId:  progress.Id(),
			ErrorCode:  common.GetImportNotificationErrorCode(returnedErr),
			ImportType: req.Type,
			SpaceId:    req.SpaceId,
		}},
	}
}

// ListImports return all registered import types
func (i *Import) ListImports(_ *pb.RpcObjectImportListRequest) ([]*pb.RpcObjectImportListImportResponse, error) {
	res := make([]*pb.RpcObjectImportListImportResponse, len(i.deps.converters))
	var idx int
	for _, c := range i.deps.converters {
		res[idx] = &pb.RpcObjectImportListImportResponse{Type: convertType(c.Name())}
		idx++
	}
	return res, nil
}

// ValidateNotionToken return all registered import types
func (i *Import) ValidateNotionToken(
	ctx context.Context, req *pb.RpcObjectImportNotionValidateTokenRequest,
) (pb.RpcObjectImportNotionValidateTokenResponseErrorCode, error) {
	tv := notion.NewTokenValidator()
	return tv.Validate(ctx, req.GetToken())
}

func (i *Import) ImportWeb(ctx context.Context, req *ImportRequest) (string, *domain.Details, error) {
	processor := NewProcessor(i.deps, req)
	return processor.ExecuteWebImport(ctx)
}

func convertType(cType string) pb.RpcObjectImportListImportResponseType {
	return pb.RpcObjectImportListImportResponseType(pb.RpcObjectImportListImportResponseType_value[cType])
}
