package export

/*
AI generated

Name: Object Exporter
Scope: global

## Responsibility
- Exports objects to various formats: Protobuf, JSON, Markdown, DOT, SVG, GraphJSON
- Resolves and includes dependent objects (nested links, files, relations, types, templates)
- Manages export output as zip archives or directory structures
- Provides in-memory single-object export for programmatic use

## Documentation
Export collects objects to export via two paths:
1. All objects: queries objectStore for all non-archived objects in space
2. Specific IDs: queries only requested objects, then expands based on flags

Dependency resolution for Protobuf format (most complete):
- Collects object types and relations used by exported objects
- Recursively resolves types referenced in setOf/targetObjectType
- Adds templates matching collected object types
- Includes relation options for tag/status relations
- Adds recommended relations from custom types

For nested objects (includeNested=true):
- Recursively follows links in blocks and details
- Creates exportContext copy with isLinkProcess=true for linked objects
- Linked objects may have filtered state via linkStateFilters
*/

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/anyproto/any-sync/app"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/google/uuid"
	"github.com/gosimple/slug"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/fileobject"
	sb "github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/export/anyblock"
	"github.com/anyproto/anytype-heart/core/block/export/collect"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/converter"
	"github.com/anyproto/anytype-heart/core/converter/dot"
	"github.com/anyproto/anytype-heart/core/converter/graphjson"
	"github.com/anyproto/anytype-heart/core/converter/md"
	"github.com/anyproto/anytype-heart/core/converter/pbc"
	"github.com/anyproto/anytype-heart/core/converter/pbjson"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/notifications"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/gateway"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
	"github.com/anyproto/anytype-heart/util/anyerror"
	"github.com/anyproto/anytype-heart/util/constant"
	"github.com/anyproto/anytype-heart/util/text"
)

const CName = "export"

const (
	tempFileName   = "temp_anytype_backup"
	spaceDirectory = "spaces"

	TypesDirectory            = "types"
	ObjectsDirectory          = "objects"
	RelationsDirectory        = "relations"
	RelationsOptionsDirectory = "relationsOptions"
	TemplatesDirectory        = "templates"
	FilesObjects              = "filesObjects"
	Files                     = "files"

	defaultFileName = "untitled"
)

var log = logging.Logger("anytype-mw-export")

type Export interface {
	Export(ctx context.Context, req pb.RpcObjectListExportRequest) (path string, succeed int, err error)
	ExportSingleInMemory(ctx context.Context, spaceId string, objectId string, format model.ExportFormat) (res string, err error)
	// Collector is the format-agnostic collection seam (collection.go):
	// the native AnyBlock JSON exporter consumes it and nothing behind it.
	collect.Collector
	app.Component
}

type export struct {
	// picker is typed CachedObjectGetter rather than ObjectGetter because
	// the native AnyBlock JSON exporter closes every object it loads out of
	// the cache — that is its memory model, not an optimisation — and
	// anyblock.Exporter takes the wider type so a picker that cannot close is
	// a compile error there. In
	// production this resolves to the same component the narrow type did:
	// core/block.Service is the only ObjectGetter the app registers, and it
	// already answers the wider interface for the indexer.
	picker              cache.CachedObjectGetter
	objectStore         objectstore.ObjectStore
	sbtProvider         typeprovider.SmartBlockTypeProvider
	fileService         files.Service
	spaceService        space.Service
	accountService      account.Service
	notificationService notifications.Notifications
	processService      process.Service
	gatewayService      gateway.Gateway
	formatFetcher       relationutils.RelationFormatFetcher
}

func New() Export {
	return &export{}
}

func (e *export) Init(a *app.App) (err error) {
	e.processService = app.MustComponent[process.Service](a)
	e.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	e.fileService = app.MustComponent[files.Service](a)
	e.picker = app.MustComponent[cache.CachedObjectGetter](a)
	e.sbtProvider = app.MustComponent[typeprovider.SmartBlockTypeProvider](a)
	e.spaceService = app.MustComponent[space.Service](a)
	e.accountService = app.MustComponent[account.Service](a)
	e.notificationService = app.MustComponent[notifications.Notifications](a)
	e.gatewayService, _ = app.GetComponent[gateway.Gateway](a)
	e.formatFetcher = app.MustComponent[relationutils.RelationFormatFetcher](a)
	return
}

func (e *export) Name() (name string) {
	return CName
}

func (e *export) Export(ctx context.Context, req pb.RpcObjectListExportRequest) (path string, succeed int, err error) {
	queue := e.processService.NewQueue(pb.ModelProcess{
		Id:      bson.NewObjectId().Hex(),
		State:   0,
		Message: &pb.ModelProcessMessageOfExport{Export: &pb.ModelProcessExport{}},
	}, exportWorkers(req.Format), req.NoProgress, e.notificationService)
	queue.SetMessage("prepare")

	if err = queue.Start(); err != nil {
		err = fmt.Errorf("start export queue: %w", err)
		return
	}
	exportCtx := newExportContext(e, req)
	return exportCtx.exportObjects(ctx, queue)
}

func (e *export) ExportSingleInMemory(ctx context.Context, spaceId string, objectId string, format model.ExportFormat) (res string, err error) {
	req := pb.RpcObjectListExportRequest{
		SpaceId:                      spaceId,
		ObjectIds:                    []string{objectId},
		IncludeFiles:                 false,
		Format:                       format,
		IncludeNested:                true,
		IncludeArchived:              true,
		NoProgress:                   true,
		MdIncludePropertiesAndSchema: false,
	}

	exportCtx := newExportContext(e, req)
	return exportCtx.exportObject(ctx, objectId)
}

func (e *export) finishWithNotification(spaceId string, exportFormat model.ExportFormat, queue process.Queue, err error) {
	errCode := model.NotificationExport_NULL
	if err != nil {
		errCode = model.NotificationExport_UNKNOWN_ERROR
	}
	queue.FinishWithNotification(&model.Notification{
		Id:      uuid.New().String(),
		Status:  model.Notification_Created,
		IsLocal: true,
		Payload: &model.NotificationPayloadOfExport{Export: &model.NotificationExport{
			ErrorCode:  errCode,
			ExportType: exportFormat,
		}},
		Space: spaceId,
	}, nil)
}

// Doc and Docs alias the collection layer's types
// (core/block/export/collect), so the legacy exporter, its tests and the
// collect seam share one set of pointers with no conversion at the boundary.
type Doc = collect.Doc

type Docs = collect.Docs

type exportContext struct {
	spaceId                      string
	docs                         Docs
	includeArchive               bool
	includeNested                bool
	includeFiles                 bool
	format                       model.ExportFormat
	closure                      collect.Closure
	isJson                       bool
	reqIds                       []string
	zip                          bool
	path                         string
	linkStateFilters             *state.Filters
	isLinkProcess                bool
	includeBackLinks             bool
	includeSpace                 bool
	mdIncludePropertiesAndSchema bool
	relations                    map[string]struct{}
	setOfList                    map[string]struct{}
	objectTypes                  map[string]struct{}
	gatewayUrl                   string
	*export
}

func newExportContext(e *export, req pb.RpcObjectListExportRequest) *exportContext {
	ec := &exportContext{
		path:                         req.Path,
		spaceId:                      req.SpaceId,
		docs:                         map[string]*Doc{},
		includeArchive:               req.IncludeArchived,
		includeNested:                req.IncludeNested,
		includeFiles:                 req.IncludeFiles,
		format:                       req.Format,
		closure:                      closureForFormat(req.Format),
		isJson:                       req.IsJson,
		reqIds:                       req.ObjectIds,
		zip:                          req.Zip,
		linkStateFilters:             pbFiltersToState(req.LinksStateFilters),
		includeBackLinks:             req.IncludeBacklinks,
		includeSpace:                 req.IncludeSpace,
		mdIncludePropertiesAndSchema: req.MdIncludePropertiesAndSchema,
		setOfList:                    make(map[string]struct{}),
		objectTypes:                  make(map[string]struct{}),
		relations:                    make(map[string]struct{}),
		export:                       e,
	}
	if e.gatewayService != nil {
		ec.gatewayUrl = "http://" + e.gatewayService.Addr()
	}
	return ec
}

func (e *exportContext) copy() *exportContext {
	return &exportContext{
		spaceId:          e.spaceId,
		docs:             e.docs,
		includeArchive:   e.includeArchive,
		includeNested:    e.includeNested,
		includeFiles:     e.includeFiles,
		format:           e.format,
		closure:          e.closure,
		isJson:           e.isJson,
		reqIds:           e.reqIds,
		export:           e.export,
		isLinkProcess:    e.isLinkProcess,
		linkStateFilters: e.linkStateFilters,
		includeBackLinks: e.includeBackLinks,
		relations:        e.relations,
		setOfList:        e.setOfList,
		objectTypes:      e.objectTypes,
		includeSpace:     e.includeSpace,
	}
}

func (e *exportContext) getStateFilters(id string) *state.Filters {
	if doc, ok := e.docs[id]; ok && doc.IsLink {
		return e.linkStateFilters
	}
	return nil
}

// exportObject synchronously exports a single object and return the bytes slice
func (e *exportContext) exportObject(ctx context.Context, objectId string) (string, error) {
	if e.format == model.Export_AnyBlockV2 {
		// one document, no bundle files, no dependency closure to collect
		// for it — see exportSingleAnyBlockDocument (anyblockjson.go)
		return e.exportSingleAnyBlockDocument(ctx, objectId)
	}
	err := e.docsForExport(ctx)
	if err != nil {
		return "", fmt.Errorf("collect docs for export: %w", err)
	}

	var docNamer Namer
	if e.format == model.Export_Markdown && e.gatewayUrl != "" {
		u, err := url.Parse(e.gatewayUrl)
		if err != nil {
			return "", fmt.Errorf("parse gateway url: %w", err)
		}
		docNamer = &deepLinkNamer{gatewayUrl: *u, spaceId: e.spaceId}
	} else {
		docNamer = newNamer()
	}
	inMemoryWriter := &InMemoryWriter{fn: docNamer}
	details, err := e.objectStore.SpaceIndex(e.spaceId).GetDetails(objectId)
	if err != nil {
		return "", fmt.Errorf("get object details: %w", err)
	}

	if err := refuseInMemoryFileObject(details); err != nil {
		return "", err
	}

	err = e.writeDoc(ctx, inMemoryWriter, objectId, e.docs.TransformToDetailsMap())
	if err != nil {
		return "", fmt.Errorf("write doc: %w", err)
	}

	for _, v := range inMemoryWriter.data {
		if e.format == model.Export_Protobuf {
			return base64.StdEncoding.EncodeToString(v), nil
		}
		return string(v), nil
	}

	return "", nil
}

// refuseInMemoryFileObject is the refusal every in-memory export owes a
// file object, in any format: the in-memory writer has nowhere to put the
// bytes, so a document that promises them would be a half answer.
func refuseInMemoryFileObject(details *domain.Details) error {
	// nolint: gosec
	switch model.ObjectTypeLayout(details.GetInt64(bundle.RelationKeyResolvedLayout)) {
	case model.ObjectType_file, model.ObjectType_image, model.ObjectType_video, model.ObjectType_audio, model.ObjectType_pdf:
		return fmt.Errorf("file export is not allowed for in-memory writer")
	}
	return nil
}

func (e *exportContext) exportObjects(ctx context.Context, queue process.Queue) (string, int, error) {
	var (
		err  error
		wr   writer
		path string
	)
	defer func() {
		e.finishWithNotification(e.spaceId, e.format, queue, err)
		if err = queue.Finalize(); err != nil {
			cleanupFile(wr)
		}
	}()
	err = e.docsForExport(ctx)
	if err != nil {
		return "", 0, fmt.Errorf("collect docs for export: %w", err)
	}
	wr, err = e.getWriter()
	if err != nil {
		return "", 0, fmt.Errorf("get writer: %w", err)
	}
	succeed, err := e.exportByFormat(ctx, wr, queue)
	if err != nil {
		return "", 0, fmt.Errorf("export by format: %w", err)
	}
	wr.Close()
	if e.zip {
		path, succeed, err = e.renameZipArchive(wr, succeed)
		if err != nil {
			return "", 0, fmt.Errorf("rename zip archive: %w", err)
		}
		return path, succeed, nil
	}
	return wr.Path(), succeed, nil
}

func (e *exportContext) getWriter() (writer, error) {
	var (
		wr  writer
		err error
	)
	if e.zip {
		if wr, err = newZipWriter(e.path, tempFileName); err != nil {
			return nil, fmt.Errorf("create zip writer: %w", anyerror.CleanupError(err))
		}
	} else {
		if wr, err = newDirWriter(e.path, e.includeFiles); err != nil {
			return nil, fmt.Errorf("create dir writer: %w", anyerror.CleanupError(err))
		}
	}
	return wr, nil
}

func (e *exportContext) exportByFormat(ctx context.Context, wr writer, queue process.Queue) (int, error) {
	queue.SetMessage("export docs")
	if e.format == model.Export_Protobuf && len(e.reqIds) == 0 {
		if err := e.createProfileFile(e.spaceId, wr); err != nil {
			log.Errorf("failed to create profile file: %s", err)
		}
	}
	var succeed int
	if e.format == model.Export_DOT || e.format == model.Export_SVG {
		succeed = e.exportDotAndSVG(ctx, succeed, wr, queue)
	} else if e.format == model.Export_GRAPH_JSON {
		succeed = e.exportGraphJson(ctx, succeed, wr, queue)
	} else if e.format == model.Export_AnyBlockV2 {
		// the native bundle exporter writes the whole tree itself — its own
		// plan, emit and bundle files (anyblockjson.go)
		return e.exportAnyBlockJSON(ctx, wr, queue)
	} else {
		tasks := make([]process.Task, 0, len(e.docs))
		var succeedAsync int64
		tasks = e.exportDocs(ctx, wr, &succeedAsync, tasks)
		err := queue.Wait(tasks...)
		if err != nil {
			cleanupFile(wr)
			return 0, nil
		}
		succeed += int(succeedAsync)

		if err := e.postProcess(ctx, wr); err != nil {
			log.Warnf("failed to generate all schemas: %v", err)
		}
	}
	return succeed, nil
}

func (e *exportContext) exportDocs(ctx context.Context,
	wr writer,
	succeed *int64,
	tasks []process.Task,
) []process.Task {
	docsDetails := e.docs.TransformToDetailsMap()
	for docId, doc := range e.docs {
		if isExcludedFromExport(doc.Details) {
			continue
		}
		did := docId
		task := func() {
			if werr := e.writeDoc(ctx, wr, did, docsDetails); werr != nil {
				log.With("objectID", did).Warnf("can't export doc: %v", werr)
			} else {
				atomic.AddInt64(succeed, 1)
			}
		}
		tasks = append(tasks, task)
	}
	return tasks
}

func (e *exportContext) exportGraphJson(ctx context.Context, succeed int, wr writer, queue process.Queue) int {
	mc := graphjson.NewMultiConverter(e.sbtProvider)
	mc.SetKnownDocs(e.docs.TransformToDetailsMap())
	var werr error
	if succeed, werr = e.writeMultiDoc(ctx, mc, wr, queue); werr != nil {
		log.Warnf("can't export docs: %v", werr)
	}
	return succeed
}

func (e *exportContext) exportDotAndSVG(ctx context.Context, succeed int, wr writer, queue process.Queue) int {
	var format = dot.ExportFormatDOT
	if e.format == model.Export_SVG {
		format = dot.ExportFormatSVG
	}
	mc := dot.NewMultiConverter(format, e.sbtProvider)
	mc.SetKnownDocs(e.docs.TransformToDetailsMap())
	var werr error
	if succeed, werr = e.writeMultiDoc(ctx, mc, wr, queue); werr != nil {
		log.Warnf("can't export docs: %v", werr)
	}
	return succeed
}

func (e *exportContext) renameZipArchive(wr writer, succeed int) (string, int, error) {
	zipName := getZipName(e.path)
	err := os.Rename(wr.Path(), zipName)
	if err != nil {
		os.Remove(wr.Path())
		return "", 0, fmt.Errorf("rename zip archive: %w", err)
	}
	return zipName, succeed, nil
}

// exportWorkers is the export queue's width. The legacy formats keep the 4
// they have always run at. The native AnyBlock JSON bundle runs its emit AS
// queue tasks, and each of those cold-builds one object into the space's
// object cache before closing it again, so for that format the queue's
// width IS the resident content set, which is half as wide on mobile.
func exportWorkers(format model.ExportFormat) int {
	if format == model.Export_AnyBlockV2 {
		return anyblock.EmitWidth()
	}
	return 4
}

// closureForFormat maps an export format onto the collection closure it
// runs. The SELF-CONTAINED formats — the two protobuf renderings and the
// native AnyBlock JSON bundle — take the derived closure, so types,
// relations, options and templates travel with the objects; markdown, dot,
// svg and graphjson take content only (collect.Closure, design §1.1).
//
// The predicate this replaced was spelled isAnyblockExport and meant
// "protobuf or pb.json" — a name that predates the AnyBlock JSON format and
// now reads as its exact opposite, five lines from the routing that decides
// what an AnyBlock JSON export collects. Listing the formats here costs one
// switch and cannot be misread.
func closureForFormat(format model.ExportFormat) collect.Closure {
	switch format {
	case model.Export_Protobuf, model.Export_JSON, model.Export_AnyBlockV2:
		return collect.ClosureDerived
	default:
		return collect.ClosureContent
	}
}

func (e *exportContext) docsForExport(ctx context.Context) (err error) {
	docs, err := e.export.Collect(ctx, collect.Request{
		SpaceId:          e.spaceId,
		Ids:              e.reqIds,
		Closure:          e.closure,
		IncludeNested:    e.includeNested,
		IncludeFiles:     e.includeFiles,
		IncludeArchived:  e.includeArchive,
		IncludeBacklinks: e.includeBackLinks,
		IncludeSpace:     e.includeSpace,
		StateFilters:     e.linkStateFilters,
	})
	if err != nil {
		return err
	}
	e.docs = docs
	return nil
}

func (e *exportContext) writeMultiDoc(ctx context.Context, mw converter.MultiConverter, wr writer, queue process.Queue) (succeed int, err error) {
	for did, doc := range e.docs {
		if isExcludedFromExport(doc.Details) {
			continue
		}
		if err = queue.Wait(func() {
			log.With("objectID", did).Debugf("write doc")
			werr := cache.Do(e.picker, did, func(b sb.SmartBlock) error {
				st := b.NewState().Copy()
				if isCollection(st) {
					e.collectionFilterMissing(st)
				}
				if e.includeFiles && b.Type() == smartblock.SmartBlockTypeFileObject {
					fileName, err := e.saveFile(ctx, wr, b, false)
					if err != nil {
						return fmt.Errorf("save file: %w", err)
					}
					st.SetDetailAndBundledRelation(bundle.RelationKeySource, domain.String(fileName))
				}
				if err = mw.Add(b.Space(), st, e.formatFetcher); err != nil {
					return fmt.Errorf("add to multi converter: %w", err)
				}
				return nil
			})
			if err != nil {
				log.With("objectID", did).Warnf("can't export doc: %v", werr)
			} else {
				succeed++
			}

		}); err != nil {
			return
		}
	}

	if err = wr.WriteFile("export"+mw.Ext(), bytes.NewReader(mw.Convert(0)), 0); err != nil {
		return 0, fmt.Errorf("write export file: %w", err)
	}
	err = nil
	return
}

func (e *exportContext) writeDoc(ctx context.Context, wr writer, docId string, details map[string]*domain.Details) (err error) {
	return cache.Do(e.picker, docId, func(b sb.SmartBlock) error {
		st := b.NewState()
		if st.CombinedDetails().GetBool(bundle.RelationKeyIsDeleted) {
			return nil
		}
		st = st.Copy().Filter(e.getStateFilters(docId))
		if isCollection(st) {
			e.collectionFilterMissing(st)
		}
		if e.includeFiles && b.Type() == smartblock.SmartBlockTypeFileObject {
			fileName, err := e.saveFile(ctx, wr, b, e.spaceId == "")
			if err != nil {
				return fmt.Errorf("save file: %w", err)
			}
			st.SetDetailAndBundledRelation(bundle.RelationKeySource, domain.String(fileName))
			// Don't save file objects in markdown
			if e.format == model.Export_Markdown {
				return nil
			}
		}

		var conv converter.Converter
		switch e.format {
		case model.Export_Markdown:
			// Create a lazy object resolver for markdown export
			resolver := newLazyObjectResolver(e.objectStore, e.spaceId)

			if e.mdIncludePropertiesAndSchema {
				conv = md.NewMDConverterWithResolver(st, wr.Namer(), true, true, resolver)
			} else {
				conv = md.NewMDConverterWithResolver(st, wr.Namer(), false, false, resolver)
			}
		case model.Export_Protobuf:
			conv = pbc.NewConverter(st, e.isJson)
		case model.Export_JSON:
			conv = pbjson.NewConverter(st)
		}
		conv.SetKnownDocs(details)
		result := conv.Convert(b.Type().ToProto())
		if result == nil {
			return nil
		}
		var filename string
		if e.format == model.Export_Markdown {
			filename = makeMarkdownName(st, wr, docId, conv.Ext(), e.spaceId)
		} else if docId == b.Space().DerivedIDs().Home {
			filename = "index" + conv.Ext()
		} else {
			filename = makeFileName(docId, e.spaceId, conv.Ext(), st, b.Type())
		}
		lastModifiedDate := st.LocalDetails().GetInt64(bundle.RelationKeyLastModifiedDate)
		if err = wr.WriteFile(filename, bytes.NewReader(result), lastModifiedDate); err != nil {
			return fmt.Errorf("write file: %w", err)
		}

		return nil
	})
}

func (e *exportContext) saveFile(ctx context.Context, wr writer, fileObject sb.SmartBlock, exportAllSpaces bool) (fileName string, err error) {
	fileObjectComponent, ok := fileObject.(fileobject.FileObject)
	if !ok {
		return "", fmt.Errorf("object is not a file object")
	}
	file, err := fileObjectComponent.GetFile()
	if err != nil {
		return "", fmt.Errorf("get file: %w", err)
	}
	if strings.HasPrefix(file.MimeType(), "image") {
		image, err := fileObjectComponent.GetImage()
		if err != nil {
			return "", fmt.Errorf("get image: %w", err)
		}
		file, err = image.GetOriginalFile()
		if err != nil {
			return "", fmt.Errorf("get original file: %w", err)
		}
	}
	origName := file.Meta().Name
	rootPath := Files
	if exportAllSpaces {
		rootPath = filepath.Join(spaceDirectory, fileObject.Space().Id(), rootPath)
	}
	fileName = wr.Namer().Get(rootPath, fileObject.Id(), filepath.Base(origName), filepath.Ext(origName))
	rd, err := file.Reader(context.Background())
	if err != nil {
		return "", fmt.Errorf("open file reader: %w", err)
	}
	if err := wr.WriteFile(fileName, rd, file.LastModifiedDate()); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}
	return fileName, nil
}

func (e *exportContext) createProfileFile(spaceID string, wr writer) error {
	spc, err := e.spaceService.Get(context.Background(), spaceID)
	if err != nil {
		return fmt.Errorf("get space: %w", err)
	}
	var homepage string

	pr, err := e.accountService.ProfileInfo()
	if err != nil {
		return fmt.Errorf("get profile info: %w", err)
	}
	err = cache.Do(e.picker, spc.DerivedIDs().Workspace, func(b sb.SmartBlock) error {
		homepage = b.Details().GetString(bundle.RelationKeyHomepage)
		return nil
	})
	if err != nil {
		return fmt.Errorf("get workspace: %w", err)
	}
	profile := &pb.Profile{
		SpaceDashboardId: homepage,
		Address:          pr.AccountId,
		Name:             pr.Name,
		Avatar:           pr.IconImage,
		ProfileId:        pr.Id,
	}
	var data []byte
	if e.isJson {
		m := jsonpb.Marshaler{Indent: " ", EmitDefaults: true}
		result, mErr := m.MarshalToString(profile)
		if mErr != nil {
			return fmt.Errorf("marshal profile to json: %w", mErr)
		}
		data = []byte(result)
	} else {
		data, err = profile.Marshal()
		if err != nil {
			return fmt.Errorf("marshal profile: %w", err)
		}
	}
	err = wr.WriteFile(constant.ProfileFile, bytes.NewReader(data), 0)
	if err != nil {
		return fmt.Errorf("write profile file: %w", err)
	}
	return nil
}

func makeMarkdownName(s *state.State, wr writer, docID, ext, spaceId string) string {
	name := s.Details().GetString(bundle.RelationKeyName)
	if name == "" {
		name = s.Snippet()
	}
	path := ""
	// space can be empty in case user want to export all spaces
	if spaceId == "" {
		spaceId := s.LocalDetails().GetString(bundle.RelationKeySpaceId)
		path = filepath.Join(spaceDirectory, spaceId)
	}
	return wr.Namer().Get(path, docID, name, ext)
}

func makeFileName(docId, spaceId, ext string, st *state.State, blockType smartblock.SmartBlockType) string {
	dir := provideFileDirectory(blockType)
	filename := filepath.Join(dir, docId+ext)
	// space can be empty in case user want to export all spaces
	if spaceId == "" {
		spaceId := st.LocalDetails().GetString(bundle.RelationKeySpaceId)
		filename = filepath.Join(spaceDirectory, spaceId, filename)
	}
	return filename
}

func provideFileDirectory(blockType smartblock.SmartBlockType) string {
	switch blockType {
	case smartblock.SmartBlockTypeRelation:
		return RelationsDirectory
	case smartblock.SmartBlockTypeRelationOption:
		return RelationsOptionsDirectory
	case smartblock.SmartBlockTypeObjectType:
		return TypesDirectory
	case smartblock.SmartBlockTypeTemplate:
		return TemplatesDirectory
	case smartblock.SmartBlockTypeFile, smartblock.SmartBlockTypeFileObject:
		return FilesObjects
	default:
		return ObjectsDirectory
	}
}

func newNamer() *namer {
	return &namer{
		names: make(map[string]string),
	}
}

type namer struct {
	// id -> name and name -> id
	names map[string]string
	mu    sync.Mutex
}

func (fn *namer) Get(path, hash, title, ext string) (name string) {
	const fileLenLimit = 48
	fn.mu.Lock()
	defer fn.mu.Unlock()
	var ok bool
	if name, ok = fn.names[hash]; ok {
		return name
	}
	title = slug.Make(strings.TrimSuffix(title, ext))
	name = text.TruncateEllipsized(title, fileLenLimit)
	name = strings.TrimSuffix(name, text.TruncateEllipsis)
	if name == "" {
		name = defaultFileName
	}
	var (
		i = 0
		b = 36
	)
	gname := filepath.Join(path, name+ext)
	for {
		if _, ok = fn.names[gname]; !ok {
			fn.names[hash] = gname
			fn.names[gname] = hash
			return gname
		}
		i++
		n := int64(i * b)
		gname = filepath.Join(path, name+"_"+strconv.FormatInt(rand.Int63n(n), b)+ext)
	}
}

func cleanupFile(wr writer) {
	if wr == nil {
		return
	}
	wr.Close()
	os.Remove(wr.Path())
}

func pbFiltersToState(filters *pb.RpcObjectListExportStateFilters) *state.Filters {
	if filters == nil {
		return nil
	}
	relationByLayoutList := state.RelationsByLayout{}
	for _, relationByLayout := range filters.RelationsWhiteList {
		allowedRelations := make([]domain.RelationKey, 0, len(relationByLayout.AllowedRelations))
		for _, relation := range relationByLayout.AllowedRelations {
			allowedRelations = append(allowedRelations, domain.RelationKey(relation))
		}
		relationByLayoutList[relationByLayout.Layout] = allowedRelations
	}
	return &state.Filters{
		RelationsWhiteList: relationByLayoutList,
		RemoveBlocks:       filters.RemoveBlocks,
	}
}

// generateAllSchemas generates JSON schemas for all object types found in the export
func (e *exportContext) postProcess(ctx context.Context, wr writer) error {
	if e.format != model.Export_Markdown || !e.mdIncludePropertiesAndSchema {
		// for now only needed for MD
		return nil
	}
	// Create a lazy object resolver
	knownObjects := e.docs.TransformToDetailsMap()
	resolver := newLazyObjectResolver(e.objectStore, e.spaceId)

	// Create markdown post-processor
	postProcessor := md.NewMDPostProcessor(resolver, wr.Namer())

	// Generate all schemas
	return postProcessor.Process(knownObjects, wr)
}

func (e *exportContext) collectionFilterMissing(st *state.State) {
	collectionIds := st.GetStoreSlice(template.CollectionStoreKey)
	existingIds := lo.Filter(collectionIds, func(item string, index int) bool {
		_, exists := e.docs[item]
		return exists
	})
	if len(existingIds) != len(collectionIds) {
		st.UpdateStoreSlice(template.CollectionStoreKey, existingIds)
	}
}

func isCollection(st state.Doc) bool {
	return st.CombinedDetails().GetInt64(bundle.RelationKeyResolvedLayout) == int64(model.ObjectType_collection)
}
