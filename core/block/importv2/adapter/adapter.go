// Package adapter exposes the v2 import engine behind the existing wire
// contract: it translates pb.RpcObjectImportRequest into an engine run,
// registers the progress process, joins process-cancel into the run context,
// and maps the engine result onto the v1 notification/event surface. Thin by
// design — no import logic lives here.
package adapter

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"runtime/debug"
	"sort"
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
	"github.com/anyproto/anytype-heart/core/block/importv2/notion"
	notionclient "github.com/anyproto/anytype-heart/core/block/importv2/notion/client"
	"github.com/anyproto/anytype-heart/core/block/importv2/persist"
	"github.com/anyproto/anytype-heart/core/block/importv2/resolve"
	"github.com/anyproto/anytype-heart/core/block/importv2/source"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/session"
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
	// ValidateNotionToken probes the Notion API with the given token.
	ValidateNotionToken(ctx context.Context, req *pb.RpcObjectImportNotionValidateTokenRequest) pb.RpcObjectImportNotionValidateTokenResponseErrorCode
}

// Narrow seams over the app components the adapter's lifecycle paths touch,
// so the lifecycle harness (§13.4) can construct the service with fakes and
// actually drive Import/Close/sweep — the paths earlier reviews could only
// reason about. Init wires all of them from the real components.
type spaceGetter interface {
	Get(ctx context.Context, spaceId string) (clientspace.Space, error)
}

type processAdder interface {
	ProcessAdd(p process.Process) error
}

type widgetCreator interface {
	CreateWidgetBlock(ctx session.Context, req *pb.RpcBlockCreateWidgetRequest, checkDuplicatedTarget bool) (string, error)
}

type engineRunFn func(ctx context.Context, request importv2.Request, converter importv2.Converter, spc clientspace.Space, lc *runLifecycle, progress process.Progress) *importv2.Result

type service struct {
	config            *config.Config
	spaceService      spaceGetter
	objectStore       objectstore.ObjectStore
	blockService      *block.Service
	processes         processAdder
	widgets           widgetCreator
	objects           persist.ObjectAccess
	engineRunner      engineRunFn
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

	// activeRuns tracks in-flight runs' cancel-cause funcs so Close can
	// suspend them (importv2.ErrSuspended) instead of tearing them down.
	activeRunsMu sync.Mutex
	activeRuns   map[int64]context.CancelCauseFunc
	runSeq       int64
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
	s.processes = s.blockService
	s.widgets = s.blockService
	s.objects = s.blockService
	s.engineRunner = s.runEngine
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

func (s *service) Run(ctx context.Context) error {
	if s.config.RepoPath != "" {
		// Settle runs a previous process left behind (spec §6.1): finish
		// deleting terminal ones, compensate crashed/suspended ones. Joined
		// into s.runs so Close waits for the sweep like any run; its ctx
		// check between dirs stops it promptly on shutdown.
		s.runs.Add(1)
		go func() {
			defer s.runs.Done()
			s.sweepAbandoned()
		}()
	}
	return nil
}

// Close suspends in-flight runs (their durable state is kept for the
// startup sweep, spec §6.4 — no compensation races the shutdown grace) and
// waits (bounded) for them to drain and flush.
func (s *service) Close(ctx context.Context) error {
	// Suspend BEFORE the component cancel: the first cancellation wins the
	// cause, and it must be ErrSuspended, not a plain Canceled.
	s.suspendRuns()
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
		return s.config.ImportV2Notion
	default:
		return false
	}
}

func (s *service) Import(req *pb.RpcObjectImportRequest) {
	s.runs.Add(1)
	go func() {
		defer s.runs.Done()
		// The engine firewalls its own run; this catches adapter-level
		// panics (progress/notification plumbing) so a fire-and-forget
		// import can never crash the process.
		defer func() {
			if rec := recover(); rec != nil {
				log.Errorf("import run panic: %v\n%s", rec, debug.Stack())
			}
		}()
		s.runImport(req)
	}()
}

func (s *service) runImport(req *pb.RpcObjectImportRequest) {
	progress := s.setupProgress(req)
	runCtx, cancel := context.WithCancelCause(s.componentCtx)
	defer cancel(nil)
	handle := s.registerRun(cancel)
	defer s.unregisterRun(handle)
	watchDone := make(chan struct{})
	defer close(watchDone)
	go func() {
		select {
		case <-progress.Canceled():
			// User cancellation: a plain cause — the engine compensates.
			cancel(nil)
		case <-watchDone:
		}
	}()

	result := s.execute(runCtx, req, progress)

	if result.Suspended {
		// Shutdown suspend (the engine's own verdict — it skipped
		// compensation and the state is on disk for the startup sweep). No
		// finish notification, no import events — the process is going away
		// and the run is not over.
		log.With("importType", req.Type.String(), "spaceId", req.SpaceId).
			Warnf("import suspended for shutdown")
		s.fileSync.ClearImportEvents()
		progress.Finish(result.Err)
		return
	}

	logRunResult(req, result)
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
			ReportObjectId:   result.ReportObjectId,
			IssuesCount:      issuesCount(result),
		},
	}))
}

// issuesCount is the wire-facing issue count: warning-or-worse issues plus
// the overflow the capped list did not retain (info diagnostics excluded).
func issuesCount(result *importv2.Result) int64 {
	count := result.IssuesDropped
	for _, issue := range result.Issues {
		if issue.Severity >= importv2.SeverityWarning {
			count++
		}
	}
	return count
}

// logRunResult emits the one structured end-of-run line the issue taxonomy
// feeds into telemetry (§16 item 1): counts by severity/code make a Sentry
// or Graylog event attributable to a converter, object class, and failure
// kind instead of an opaque "import failed".
func logRunResult(req *pb.RpcObjectImportRequest, result *importv2.Result) {
	counts := map[string]int64{}
	for _, issue := range result.Issues {
		counts[issue.Severity.String()+"/"+string(issue.Code)]++
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, counts[key]))
	}
	logger := log.With(
		"importType", req.Type.String(),
		"spaceId", req.SpaceId,
		"created", result.Created,
		"updated", result.Updated,
		"skipped", result.Skipped,
		"failed", result.Failed,
		"issues", strings.Join(parts, " "),
		"issuesDropped", result.IssuesDropped,
		"reportObjectId", result.ReportObjectId,
	)
	if result.Err != nil {
		logger.Warnf("import finished with error: %s", result.Err)
		return
	}
	logger.Infof("import finished")
}

func (s *service) execute(ctx context.Context, req *pb.RpcObjectImportRequest, progress process.Progress) *importv2.Result {
	if req.SpaceId == "" {
		return &importv2.Result{Err: importv2.Fatal(importv2.IssueSourceInvalid, fmt.Errorf("spaceId is required"))}
	}
	spc, err := s.spaceService.Get(ctx, req.SpaceId)
	if err != nil {
		return &importv2.Result{Err: importv2.Fatal(importv2.IssueStoreError, fmt.Errorf("get space: %w", err))}
	}

	request := importv2.Request{
		SpaceID:        req.SpaceId,
		Origin:         objectorigin.Import(req.Type),
		Mode:           modeFromProto(req.Mode),
		UpdateExisting: req.UpdateExistingObjects,
	}

	var combined *importv2.Result
	switch req.Type {
	case model.Import_Notion:
		combined = s.executeNotion(ctx, request, req, spc, progress)
	default:
		combined = s.executeMarkdown(ctx, request, req, spc, progress)
	}
	if combined.Err == nil && combined.RootCollectionId != "" {
		s.createRootWidget(spc.DerivedIDs().Widgets, combined)
	}
	return combined
}

func (s *service) executeMarkdown(ctx context.Context, request importv2.Request, req *pb.RpcObjectImportRequest, spc clientspace.Space, progress process.Progress) *importv2.Result {
	paths, params, err := markdownParams(req)
	if err != nil {
		return &importv2.Result{Err: importv2.Fatal(importv2.IssueSourceInvalid, err)}
	}
	params.Planner = plannerFromRequest(req)
	request.NoCollection = params.NoCollection

	// Multiple paths run as independent sequential engine runs (v1 built one
	// merged run; parity for multi-path selections is a phase-3 item).
	combined := &importv2.Result{}
	for pathIndex, importPath := range paths {
		result := s.runOne(ctx, request, spc, importPath, pathIndex, params, progress)
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
		if result.ReportObjectId != "" {
			combined.ReportObjectId = result.ReportObjectId
		}
		if result.Err != nil {
			combined.Err = result.Err
			combined.Suspended = result.Suspended
			break
		}
	}
	return combined
}

func (s *service) executeNotion(ctx context.Context, request importv2.Request, req *pb.RpcObjectImportRequest, spc clientspace.Space, progress process.Progress) *importv2.Result {
	params := req.GetNotionParams()
	if params == nil || params.GetApiKey() == "" {
		return &importv2.Result{Err: importv2.Fatal(importv2.IssueAuthFailed, fmt.Errorf("notion import requires an api key"))}
	}
	lc, err := s.beginRun(ctx, request, "Notion", 0)
	if err != nil {
		return &importv2.Result{Err: importv2.Fatal(importv2.IssueStoreError, err)}
	}
	defer lc.release() // Invariant 3: a panic below must not leak the registry hold
	apiClient := notionclient.NewClient(params.GetApiKey())
	var opts []notion.Option
	if planner := plannerFromRequest(req); planner.planner != nil {
		opts = append(opts, notion.WithPlanner(planner.planner))
		if planner.includeSamples {
			opts = append(opts, notion.WithContentSamples())
		}
	}
	converter := notion.New(apiClient, notionclient.NewFileFetcher(),
		&collectionFactory{service: s.collectionService}, lc.spillDir, opts...)
	result := s.engineRunner(ctx, request, converter, spc, lc, progress)
	s.finishRun(lc, result)
	return result
}

type mdParams struct {
	NoCollection             bool
	CreateDirectoryPages     bool
	IncludePropertiesAsBlock bool
	Flavour                  string
	Planner                  plannerParams
}

func markdownParams(req *pb.RpcObjectImportRequest) ([]string, mdParams, error) {
	markdownReq := req.GetMarkdownParams()
	if markdownReq == nil || len(markdownReq.Path) == 0 {
		return nil, mdParams{}, fmt.Errorf("markdown import requires at least one path")
	}
	// An explicit Obsidian import forces the profile; plain markdown
	// imports detect the flavour from the listing (§11.4).
	flavour := ""
	if req.Type == model.Import_Obsidian {
		flavour = markdown.FlavourObsidian
	}
	return markdownReq.Path, mdParams{
		NoCollection:             markdownReq.NoCollection,
		CreateDirectoryPages:     markdownReq.CreateDirectoryPages,
		IncludePropertiesAsBlock: markdownReq.IncludePropertiesAsBlock,
		Flavour:                  flavour,
	}, nil
}

func (s *service) runOne(ctx context.Context, request importv2.Request, spc clientspace.Space, importPath string, pathIndex int, params mdParams, progress process.Progress) *importv2.Result {
	src, err := source.Open(importPath)
	if err != nil {
		return &importv2.Result{Err: importv2.Fatal(importv2.IssueSourceInvalid, fmt.Errorf("open source: %w", err))}
	}
	defer src.Close()

	lc, err := s.beginRun(ctx, request, "Markdown", pathIndex)
	if err != nil {
		return &importv2.Result{Err: importv2.Fatal(importv2.IssueStoreError, err)}
	}
	defer lc.release() // Invariant 3: a panic below must not leak the registry hold
	converter := markdown.New(src, markdown.Params{
		CreateDirectoryPages:     params.CreateDirectoryPages,
		IncludePropertiesAsBlock: params.IncludePropertiesAsBlock,
		Flavour:                  params.Flavour,
		Planner:                  params.Planner.planner,
		IncludeContentSamples:    params.Planner.includeSamples,
	}, &collectionFactory{service: s.collectionService})
	result := s.engineRunner(ctx, request, converter, spc, lc, progress)
	s.finishRun(lc, result)
	return result
}

// runEngine wires one engine run's per-run components over the app seams.
func (s *service) runEngine(ctx context.Context, request importv2.Request, converter importv2.Converter, spc clientspace.Space, lc *runLifecycle, progress process.Progress) *importv2.Result {
	journal := persist.NewJournal()
	if lc.store != nil {
		journal = persist.NewJournalWithLedger(lc.store)
	}
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
		&existsChecker{store: s.objectStore.SpaceIndex(request.SpaceID)},
		lc.spillDir,
	)
	return engine.Run(ctx, request, converter, engine.Deps{
		Identity:  identitySvc,
		Persister: persister,
		Journal:   journal,
		Objects:   s.blockService,
		Formats:   formats,
		Keys:      keys,
		// The run's root collection carries the import date in its name.
		Collection:     &collectionFactory{service: s.collectionService, addDate: true},
		Reporter:       &progressReporter{progress: progress},
		OnCompensating: s.onCompensating(lc),
	})
}

// ValidateNotionToken probes the API with the given token (the frontend
// calls this before starting an import).
func (s *service) ValidateNotionToken(ctx context.Context, req *pb.RpcObjectImportNotionValidateTokenRequest) pb.RpcObjectImportNotionValidateTokenResponseErrorCode {
	apiClient := notionclient.NewClient(req.GetToken(), notionclient.WithRetryPolicy(notionclient.RetryPolicy{
		MaxAttempts: 1, BaseDelay: time.Second, MaxDelay: time.Second, TotalBudget: 30 * time.Second,
	}))
	err := apiClient.Request(ctx, http.MethodGet, "/users?page_size=1", nil, nil)
	switch {
	case err == nil:
		return pb.RpcObjectImportNotionValidateTokenResponseError_NULL
	case errors.Is(err, notionclient.ErrUnauthorized):
		return pb.RpcObjectImportNotionValidateTokenResponseError_UNAUTHORIZED
	case errors.Is(err, notionclient.ErrForbidden):
		return pb.RpcObjectImportNotionValidateTokenResponseError_FORBIDDEN
	case errors.Is(err, notionclient.ErrUnavailable), errors.Is(err, notionclient.ErrRateLimited):
		return pb.RpcObjectImportNotionValidateTokenResponseError_SERVICE_UNAVAILABLE
	default:
		return pb.RpcObjectImportNotionValidateTokenResponseError_INTERNAL_ERROR
	}
}

func (s *service) createRootWidget(widgetsId string, result *importv2.Result) {
	_, err := s.widgets.CreateWidgetBlock(nil, &pb.RpcBlockCreateWidgetRequest{
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
	if err := s.processes.ProcessAdd(progress); err != nil {
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
			ProcessId:      progress.Id(),
			ErrorCode:      errorCode(result.Err, req),
			ImportType:     req.Type,
			SpaceId:        req.SpaceId,
			ReportObjectId: result.ReportObjectId,
			IssuesCount:    issuesCount(result),
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
	switch {
	case errors.Is(err, notionclient.ErrRateLimited):
		return model.Import_NOTION_RATE_LIMIT_EXCEEDED
	case errors.Is(err, notionclient.ErrUnavailable):
		return model.Import_NOTION_SERVER_IS_UNAVAILABLE
	case errors.Is(err, notionclient.ErrUnauthorized), errors.Is(err, notionclient.ErrForbidden):
		return model.Import_INSUFFICIENT_PERMISSIONS
	}
	issue := importv2.AsIssue(err, importv2.SeverityFatal, importv2.IssueStoreError)
	switch issue.Code {
	case importv2.IssueCancelled:
		return model.Import_IMPORT_IS_CANCELED
	case importv2.IssueNoObjects:
		if req.Type == model.Import_Notion {
			return model.Import_NOTION_NO_OBJECTS_IN_INTEGRATION
		}
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
