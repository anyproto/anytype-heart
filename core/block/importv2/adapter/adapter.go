// Package adapter exposes the v2 import engine behind the existing wire
// contract: it translates pb.RpcObjectImportRequest into an engine run,
// registers the progress process, joins process-cancel into the run context,
// and maps the engine result onto the v1 notification/event surface. Thin by
// design — no import logic lives here.
package adapter

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/engine"
	"github.com/anyproto/anytype-heart/core/block/importv2/identity"
	"github.com/anyproto/anytype-heart/core/block/importv2/markdown"
	"github.com/anyproto/anytype-heart/core/block/importv2/persist"
	"github.com/anyproto/anytype-heart/core/block/importv2/resolve"
	"github.com/anyproto/anytype-heart/core/block/importv2/source"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/core/files/filesync"
	"github.com/anyproto/anytype-heart/core/notifications"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/clientspace"

	objectcreator "github.com/anyproto/anytype-heart/core/block/object/objectcreator"

	detailservice "github.com/anyproto/anytype-heart/core/block/detailservice"
)

const CName = "importv2"

var log = logging.Logger("import-v2")

// Importer is the adapter surface used by the gRPC handler.
type Importer interface {
	app.ComponentRunnable
	// Handles reports whether the v2 engine serves this import type (the
	// per-format config flags).
	Handles(importType model.ImportType) bool
	// Import runs one import asynchronously (v1 handler semantics: empty
	// reply, result via notification + EventImportFinish).
	Import(req *pb.RpcObjectImportRequest)
}

type service struct {
	config            *config.Config
	spaceService      space.Service
	objectStore       objectstore.ObjectStore
	blockService      *block.Service
	fileObjectService fileobject.Service
	installer         objectcreator.Service
	detailsService    detailservice.Service
	collectionService *collection.Service
	notificationsSvc  notifications.Notifications
	eventSender       event.Sender
	fileSync          filesync.FileSync

	componentCtx    context.Context
	componentCancel context.CancelFunc
	runs            sync.WaitGroup
}

func New() Importer {
	return &service{}
}

func (s *service) Name() string { return CName }

func (s *service) Init(a *app.App) error {
	s.config = app.MustComponent[*config.Config](a)
	s.spaceService = app.MustComponent[space.Service](a)
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	s.blockService = app.MustComponent[*block.Service](a)
	s.fileObjectService = app.MustComponent[fileobject.Service](a)
	s.installer = app.MustComponent[objectcreator.Service](a)
	s.detailsService = app.MustComponent[detailservice.Service](a)
	s.collectionService = app.MustComponent[*collection.Service](a)
	s.notificationsSvc = app.MustComponent[notifications.Notifications](a)
	s.eventSender = app.MustComponent[event.Sender](a)
	s.fileSync = app.MustComponent[filesync.FileSync](a)
	s.componentCtx, s.componentCancel = context.WithCancel(context.Background())
	return nil
}

func (s *service) Run(ctx context.Context) error { return nil }

// Close cancels in-flight runs and waits (bounded) for their compensation.
func (s *service) Close(ctx context.Context) error {
	s.componentCancel()
	done := make(chan struct{})
	go func() {
		s.runs.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		log.Warn("import runs did not drain within close grace period")
	}
	return nil
}

func (s *service) Handles(importType model.ImportType) bool {
	switch importType {
	case model.Import_Markdown, model.Import_Obsidian:
		return s.config.ImportV2Markdown
	case model.Import_Notion:
		return false // arrives with the notion converter phase
	default:
		return false
	}
}

func (s *service) Import(req *pb.RpcObjectImportRequest) {
	s.runs.Add(1)
	go func() {
		defer s.runs.Done()
		s.runImport(req)
	}()
}

func (s *service) runImport(req *pb.RpcObjectImportRequest) {
	progress := s.setupProgress(req)
	runCtx, cancel := context.WithCancel(s.componentCtx)
	defer cancel()
	watchDone := make(chan struct{})
	defer close(watchDone)
	go func() {
		select {
		case <-progress.Canceled():
			cancel()
		case <-watchDone:
		}
	}()

	result := s.execute(runCtx, req, progress)

	s.finishProgress(progress, req, result)
	if result.Err == nil {
		s.fileSync.SendImportEvents()
	}
	s.fileSync.ClearImportEvents()
	s.eventSender.Broadcast(event.NewEventSingleMessage("", &pb.EventMessageValueOfImportFinish{
		ImportFinish: &pb.EventImportFinish{
			RootCollectionID: result.RootCollectionId,
			ObjectsCount:     result.ObjectsCount(),
			ImportType:       req.Type,
		},
	}))
}

func (s *service) execute(ctx context.Context, req *pb.RpcObjectImportRequest, progress process.Progress) *importv2.Result {
	if req.SpaceId == "" {
		return &importv2.Result{Err: importv2.Fatal(importv2.IssueSourceInvalid, fmt.Errorf("spaceId is required"))}
	}
	spc, err := s.spaceService.Get(ctx, req.SpaceId)
	if err != nil {
		return &importv2.Result{Err: importv2.Fatal(importv2.IssueStoreError, fmt.Errorf("get space: %w", err))}
	}

	paths, params, err := markdownParams(req)
	if err != nil {
		return &importv2.Result{Err: importv2.Fatal(importv2.IssueSourceInvalid, err)}
	}
	spillDir, err := os.MkdirTemp("", "anytype-import-v2-*")
	if err != nil {
		return &importv2.Result{Err: importv2.Fatal(importv2.IssueSourceInvalid, fmt.Errorf("create spill dir: %w", err))}
	}
	defer os.RemoveAll(spillDir)

	origin := objectorigin.Import(req.Type)
	request := importv2.Request{
		SpaceID:        req.SpaceId,
		Origin:         origin,
		Mode:           modeFromProto(req.Mode),
		UpdateExisting: req.UpdateExistingObjects,
		NoCollection:   params.NoCollection,
	}

	// Multiple paths run as independent sequential engine runs (v1 built one
	// merged run; parity for multi-path selections lands with increment 2).
	combined := &importv2.Result{}
	for _, importPath := range paths {
		result := s.runOne(ctx, request, spc, importPath, params, spillDir, progress)
		combined.Created += result.Created
		combined.Updated += result.Updated
		combined.Skipped += result.Skipped
		combined.Failed += result.Failed
		combined.Compensated += result.Compensated
		combined.Leaked += result.Leaked
		combined.Issues = append(combined.Issues, result.Issues...)
		combined.IssuesDropped += result.IssuesDropped
		if result.RootCollectionId != "" {
			combined.RootCollectionId = result.RootCollectionId
			combined.WidgetLayout = result.WidgetLayout
		}
		if result.Err != nil {
			combined.Err = result.Err
			break
		}
	}
	if combined.Err == nil && combined.RootCollectionId != "" {
		s.createRootWidget(spc.DerivedIDs().Widgets, combined)
	}
	return combined
}

type mdParams struct {
	NoCollection             bool
	CreateDirectoryPages     bool
	IncludePropertiesAsBlock bool
}

func markdownParams(req *pb.RpcObjectImportRequest) ([]string, mdParams, error) {
	markdownReq := req.GetMarkdownParams()
	if markdownReq == nil || len(markdownReq.Path) == 0 {
		return nil, mdParams{}, fmt.Errorf("markdown import requires at least one path")
	}
	return markdownReq.Path, mdParams{
		NoCollection:             markdownReq.NoCollection,
		CreateDirectoryPages:     markdownReq.CreateDirectoryPages,
		IncludePropertiesAsBlock: markdownReq.IncludePropertiesAsBlock,
	}, nil
}

func (s *service) runOne(ctx context.Context, request importv2.Request, spc clientspace.Space, importPath string, params mdParams, spillDir string, progress process.Progress) *importv2.Result {
	src, err := source.Open(importPath)
	if err != nil {
		return &importv2.Result{Err: importv2.Fatal(importv2.IssueSourceInvalid, fmt.Errorf("open source: %w", err))}
	}
	defer src.Close()

	journal := persist.NewJournal()
	formats := resolve.NewFormats()
	keys := engine.NewKeyTable()
	identitySvc := identity.NewService(spc, s.objectStore.SpaceIndex(request.SpaceID), request.UpdateExisting, time.Now())
	resolver := resolve.New(identitySvc, keys, formats)
	persister := persist.New(
		request.SpaceID,
		request.Origin,
		spc,
		s.blockService,
		&uploaderAdapter{blockService: s.blockService, fileObjectService: s.fileObjectService},
		s.detailsService,
		resolver,
		persist.NewInstallCoordinator(&installerAdapter{installer: s.installer, space: spc}),
		journal,
		spillDir,
	)
	converter := markdown.New(src, markdown.Params{
		CreateDirectoryPages:     params.CreateDirectoryPages,
		IncludePropertiesAsBlock: params.IncludePropertiesAsBlock,
	}, &collectionFactory{service: s.collectionService})

	return engine.Run(ctx, request, converter, engine.Deps{
		Identity:  identitySvc,
		Persister: persister,
		Journal:   journal,
		Objects:   s.blockService,
		Links:     s.objectStore.SpaceIndex(request.SpaceID),
		Formats:   formats,
		Keys:      keys,
		// The run's root collection carries the import date in its name.
		Collection: &collectionFactory{service: s.collectionService, addDate: true},
		Reporter:   &progressReporter{progress: progress},
	})
}

func (s *service) createRootWidget(widgetsId string, result *importv2.Result) {
	_, err := s.blockService.CreateWidgetBlock(nil, &pb.RpcBlockCreateWidgetRequest{
		ContextId:    widgetsId,
		WidgetLayout: result.WidgetLayout,
		Block: &model.Block{
			Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{
				TargetBlockId: result.RootCollectionId,
				Style:         model.BlockContentLink_Page,
			}},
		},
	}, true)
	if err != nil {
		log.Errorf("create root collection widget: %s", err)
	}
}

func (s *service) setupProgress(req *pb.RpcObjectImportRequest) process.Progress {
	var progress process.Progress
	if req.GetNoProgress() {
		progress = process.NewNoOp()
	} else {
		processMessage := pb.IsModelProcessMessage(&pb.ModelProcessMessageOfImport{Import: &pb.ModelProcessImport{}})
		if req.IsMigration {
			processMessage = &pb.ModelProcessMessageOfMigration{Migration: &pb.ModelProcessMigration{}}
		}
		progress = process.NewNotificationProcess(processMessage, s.notificationsSvc)
	}
	if err := s.blockService.ProcessAdd(progress); err != nil {
		log.Errorf("register import process: %s", err)
	}
	return progress
}

func (s *service) finishProgress(progress process.Progress, req *pb.RpcObjectImportRequest, result *importv2.Result) {
	notificationable, ok := progress.(process.Notificationable)
	if !ok {
		progress.Finish(result.Err)
		return
	}
	notificationable.FinishWithNotification(&model.Notification{
		Status:  model.Notification_Created,
		IsLocal: true,
		Space:   req.SpaceId,
		Payload: &model.NotificationPayloadOfImport{Import: &model.NotificationImport{
			ProcessId:  progress.Id(),
			ErrorCode:  errorCode(result.Err, req),
			ImportType: req.Type,
			SpaceId:    req.SpaceId,
		}},
	}, result.Err)
}

func modeFromProto(mode pb.RpcObjectImportRequestMode) importv2.Mode {
	if mode == pb.RpcObjectImportRequest_IGNORE_ERRORS {
		return importv2.ModeContinueOnError
	}
	return importv2.ModeAllOrNothing
}

// errorCode maps the run's fatal issue onto the wire enum the frontend
// already understands.
func errorCode(err error, req *pb.RpcObjectImportRequest) model.ImportErrorCode {
	if err == nil {
		return model.Import_NULL
	}
	issue := importv2.AsIssue(err, importv2.SeverityFatal, importv2.IssueStoreError)
	switch issue.Code {
	case importv2.IssueCancelled:
		return model.Import_IMPORT_IS_CANCELED
	case importv2.IssueNoObjects:
		if isZipImport(req) {
			return model.Import_FILE_IMPORT_NO_OBJECTS_IN_ZIP_ARCHIVE
		}
		return model.Import_FILE_IMPORT_NO_OBJECTS_IN_DIRECTORY
	case importv2.IssueFileFetchFailed:
		return model.Import_FILE_LOAD_ERROR
	case importv2.IssueRateLimited:
		return model.Import_NOTION_RATE_LIMIT_EXCEEDED
	case importv2.IssueAuthFailed:
		return model.Import_INSUFFICIENT_PERMISSIONS
	default:
		return model.Import_INTERNAL_ERROR
	}
}

func isZipImport(req *pb.RpcObjectImportRequest) bool {
	if params := req.GetMarkdownParams(); params != nil {
		for _, p := range params.Path {
			if strings.EqualFold(filepath.Ext(p), ".zip") {
				return true
			}
		}
	}
	return false
}
