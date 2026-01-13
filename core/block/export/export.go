package export

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/anyproto/any-sync/app"
	"github.com/globalsign/mgo/bson"
	"github.com/google/uuid"
	"github.com/gosimple/slug"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/fileobject"
	sb "github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/object/objectlink"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/block/simple"
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
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/gateway"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
	"github.com/anyproto/anytype-heart/util/anyerror"
	"github.com/anyproto/anytype-heart/util/constant"
	"github.com/anyproto/anytype-heart/util/slice"
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
	app.Component
}

type export struct {
	picker              cache.ObjectGetter
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
	e.picker = app.MustComponent[cache.ObjectGetter](a)
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
	}, 4, req.NoProgress, e.notificationService)
	queue.SetMessage("prepare")

	if err = queue.Start(); err != nil {
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

type Doc struct {
	Details *domain.Details
	isLink  bool
}

type Docs map[string]*Doc

func (d Docs) transformToDetailsMap() map[string]*domain.Details {
	details := make(map[string]*domain.Details, len(d))
	for id, doc := range d {
		details[id] = doc.Details
	}
	return details
}

type exportContext struct {
	spaceId                      string
	docs                         Docs
	includeArchive               bool
	includeNested                bool
	includeFiles                 bool
	format                       model.ExportFormat
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
	if doc, ok := e.docs[id]; ok && doc.isLink {
		return e.linkStateFilters
	}
	return nil
}

// exportObject synchronously exports a single object and return the bytes slice
func (e *exportContext) exportObject(ctx context.Context, objectId string) (string, error) {
	err := e.docsForExport(ctx)
	if err != nil {
		return "", err
	}

	var docNamer Namer
	if e.format == model.Export_Markdown && e.gatewayUrl != "" {
		u, err := url.Parse(e.gatewayUrl)
		if err != nil {
			return "", err
		}
		docNamer = &deepLinkNamer{gatewayUrl: *u, spaceId: e.spaceId}
	} else {
		docNamer = newNamer()
	}
	inMemoryWriter := &InMemoryWriter{fn: docNamer}
	details, err := e.objectStore.SpaceIndex(e.spaceId).GetDetails(objectId)
	if err != nil {
		return "", err
	}

	// do not allow file export for in-memory writer
	// nolint: gosec
	switch model.ObjectTypeLayout(details.GetInt64(bundle.RelationKeyResolvedLayout)) {
	case model.ObjectType_file, model.ObjectType_image, model.ObjectType_video, model.ObjectType_audio, model.ObjectType_pdf:
		return "", fmt.Errorf("file export is not allowed for in-memory writer")
	}

	err = e.writeDoc(ctx, inMemoryWriter, objectId, e.docs.transformToDetailsMap())
	if err != nil {
		return "", err
	}

	for _, v := range inMemoryWriter.data {
		if e.format == model.Export_Protobuf {
			return base64.StdEncoding.EncodeToString(v), nil
		}
		return string(v), nil
	}

	return "", nil
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
		return "", 0, err
	}
	wr, err = e.getWriter()
	if err != nil {
		return "", 0, err
	}
	succeed, err := e.exportByFormat(ctx, wr, queue)
	if err != nil {
		return "", 0, err
	}
	wr.Close()
	if e.zip {
		path, succeed, err = e.renameZipArchive(wr, succeed)
		return path, succeed, err
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
			err = anyerror.CleanupError(err)
			return nil, err
		}
	} else {
		if wr, err = newDirWriter(e.path, e.includeFiles); err != nil {
			err = anyerror.CleanupError(err)
			return nil, err
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
	docsDetails := e.docs.transformToDetailsMap()
	for docId := range e.docs {
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
	mc.SetKnownDocs(e.docs.transformToDetailsMap())
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
	mc.SetKnownDocs(e.docs.transformToDetailsMap())
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
		return "", 0, err
	}
	return zipName, succeed, nil
}

func isAnyblockExport(format model.ExportFormat) bool {
	return format == model.Export_Protobuf || format == model.Export_JSON
}

func (e *exportContext) docsForExport(ctx context.Context) (err error) {
	isProtobuf := isAnyblockExport(e.format)
	if len(e.reqIds) == 0 {
		return e.getExistedObjects(isProtobuf)
	}

	if len(e.reqIds) > 0 {
		return e.getObjectsByIDs(ctx, isProtobuf)
	}
	return
}

func (e *exportContext) getObjectsByIDs(ctx context.Context, isProtobuf bool) error {
	res, err := e.queryAndFilterObjectsByRelation(e.spaceId, e.reqIds, bundle.RelationKeyId)
	if err != nil {
		return err
	}
	for _, object := range res {
		id := object.Details.GetString(bundle.RelationKeyId)
		e.docs[id] = &Doc{Details: object.Details}
	}
	if e.includeSpace {
		err = e.addSpaceToDocs(ctx)
		if err != nil {
			return err
		}
	}
	if isProtobuf {
		return e.processProtobuf()
	}
	return e.processNotProtobuf()
}

func (e *exportContext) queryAndFilterObjectsByRelation(spaceId string, reqIds []string, relationKey domain.RelationKey) ([]database.Record, error) {
	var allObjects []database.Record
	const singleBatchCount = 50
	for j := 0; j < len(reqIds); {
		if j+singleBatchCount < len(reqIds) {
			records, err := e.queryObjectsByRelation(spaceId, reqIds[j:j+singleBatchCount], relationKey)
			if err != nil {
				return nil, err
			}
			allObjects = append(allObjects, records...)
		} else {
			records, err := e.queryObjectsByRelation(spaceId, reqIds[j:], relationKey)
			if err != nil {
				return nil, err
			}
			allObjects = append(allObjects, records...)
		}
		j += singleBatchCount
	}
	return allObjects, nil
}

func (e *exportContext) queryObjectsByRelation(spaceId string, reqIds []string, relationKey domain.RelationKey) ([]database.Record, error) {
	return e.objectStore.SpaceIndex(spaceId).Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.StringList(reqIds),
			},
		},
	})
}

func (e *exportContext) addSpaceToDocs(ctx context.Context) error {
	space, err := e.spaceService.Get(ctx, e.spaceId)
	if err != nil {
		return err
	}
	workspaceId := space.DerivedIDs().Workspace
	records, err := e.objectStore.SpaceIndex(e.spaceId).QueryByIds([]string{workspaceId})
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return fmt.Errorf("no objects found for space %s", workspaceId)
	}
	e.docs[workspaceId] = &Doc{Details: records[0].Details, isLink: true}
	return nil
}

func (e *exportContext) processNotProtobuf() error {
	ids := listObjectIds(e.docs)
	if e.includeFiles {
		fileObjectsIds, err := e.processFiles(ids)
		if err != nil {
			return err
		}
		ids = append(ids, fileObjectsIds...)
	}
	if e.includeNested {
		for _, id := range ids {
			e.addNestedObject(id, map[string]*Doc{})
		}
	}
	return nil
}

func (e *exportContext) processProtobuf() error {
	if !e.includeNested {
		err := e.addDependentObjectsFromDataview()
		if err != nil {
			return err
		}
	}
	ids := listObjectIds(e.docs)
	if e.includeFiles {
		err := e.addFileObjects(ids)
		if err != nil {
			return err
		}
	}

	err := e.addDerivedObjects()
	if err != nil {
		return err
	}
	ids = e.listTargetTypesFromTemplates(ids)
	if e.includeNested {
		err = e.addNestedObjects(ids)
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *exportContext) addDependentObjectsFromDataview() error {
	var (
		viewDependentObjectsIds []string
		err                     error
	)
	for id, details := range e.docs {
		if isObjectWithDataview(details.Details) {
			viewDependentObjectsIds, err = e.getViewDependentObjects(id, viewDependentObjectsIds)
			if err != nil {
				return err
			}
		}
	}
	viewDependentObjects, err := e.queryAndFilterObjectsByRelation(e.spaceId, viewDependentObjectsIds, bundle.RelationKeyId)
	if err != nil {
		return err
	}
	templates, err := e.queryAndFilterObjectsByRelation(e.spaceId, viewDependentObjectsIds, bundle.RelationKeyTargetObjectType)
	if err != nil {
		return err
	}
	for _, object := range append(viewDependentObjects, templates...) {
		id := object.Details.GetString(bundle.RelationKeyId)
		e.docs[id] = &Doc{
			Details: object.Details,
			isLink:  e.isLinkProcess,
		}
	}
	return nil
}

func (e *exportContext) getViewDependentObjects(id string, viewDependentObjectsIds []string) ([]string, error) {
	err := cache.Do(e.picker, id, func(sb sb.SmartBlock) error {
		st := sb.NewState().Copy().Filter(e.getStateFilters(id))
		viewDependentObjectsIds = append(viewDependentObjectsIds,
			objectlink.DependentObjectIDs(st, sb.Space(), e.formatFetcher, objectlink.Flags{Blocks: true})...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return viewDependentObjectsIds, nil
}

func (e *exportContext) addFileObjects(ids []string) error {
	fileObjectsIds, err := e.processFiles(ids)
	if err != nil {
		return err
	}
	if e.includeNested {
		err = e.addNestedObjects(fileObjectsIds)
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *exportContext) processFiles(ids []string) ([]string, error) {
	var fileObjectsIds []string
	for _, id := range ids {
		objectFiles, err := e.fillLinkedFiles(id)
		if err != nil {
			return nil, err
		}
		fileObjectsIds = lo.Union(fileObjectsIds, objectFiles)
	}
	return fileObjectsIds, nil
}

func (e *exportContext) addDerivedObjects() error {
	processedObjects := make(map[string]struct{}, 0)
	err := e.getRelationsAndTypes(e.docs, processedObjects)
	if err != nil {
		return err
	}

	err = e.getTemplatesRelationsAndTypes(processedObjects)
	if err != nil {
		return err
	}
	err = e.addRelationsAndTypes()
	if err != nil {
		return err
	}
	return nil
}

func (e *exportContext) getRelationsAndTypes(notProcessedObjects map[string]*Doc, processedObjects map[string]struct{}) error {
	err := e.collectDerivedObjects(notProcessedObjects)
	if err != nil {
		return err
	}
	// get derived objects only from types,
	// because relations currently have only system relations and object type
	if len(e.objectTypes) > 0 || len(e.setOfList) > 0 {
		err = e.getDerivedObjectsForTypes(processedObjects)
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *exportContext) collectDerivedObjects(objects map[string]*Doc) error {
	for id := range objects {
		err := cache.Do(e.picker, id, func(b sb.SmartBlock) error {
			state := b.NewState().Copy().Filter(e.getStateFilters(id))
			objectRelations := state.AllRelationKeys()
			fillObjectsMap(e.relations, slice.IntoStrings(objectRelations))
			details := state.CombinedDetails()
			if isObjectWithDataview(details) {
				dataviewRelations, err := getDataviewRelations(state)
				if err != nil {
					return err
				}
				fillObjectsMap(e.relations, dataviewRelations)
			}
			var objectTypes []string
			if details.Has(bundle.RelationKeyType) {
				objectTypes = append(objectTypes, details.GetString(bundle.RelationKeyType))
			}
			if details.Has(bundle.RelationKeyTargetObjectType) {
				objectTypes = append(objectTypes, details.GetString(bundle.RelationKeyTargetObjectType))
			}
			fillObjectsMap(e.objectTypes, objectTypes)
			setOfList := details.GetStringList(bundle.RelationKeySetOf)
			fillObjectsMap(e.setOfList, setOfList)
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func fillObjectsMap(dst map[string]struct{}, objectsToAdd []string) {
	for _, objectId := range objectsToAdd {
		dst[objectId] = struct{}{}
	}
}

func isObjectWithDataview(details *domain.Details) bool {
	return details.GetInt64(bundle.RelationKeyResolvedLayout) == int64(model.ObjectType_collection) ||
		details.GetInt64(bundle.RelationKeyResolvedLayout) == int64(model.ObjectType_set)
}

func getDataviewRelations(state *state.State) ([]string, error) {
	var relations []string
	err := state.Iterate(func(b simple.Block) (isContinue bool) {
		if dataview := b.Model().GetDataview(); dataview != nil {
			for _, view := range dataview.Views {
				for _, relation := range view.Relations {
					relations = append(relations, relation.Key)
				}
			}
		}
		return true
	})
	return relations, err
}

func (e *exportContext) getDerivedObjectsForTypes(processedObjects map[string]struct{}) error {
	notProceedTypes := make(map[string]*Doc)
	for object := range e.objectTypes {
		e.fillNotProcessedTypes(processedObjects, object, notProceedTypes)
	}
	for object := range e.setOfList {
		e.fillNotProcessedTypes(processedObjects, object, notProceedTypes)
	}
	if len(notProceedTypes) == 0 {
		return nil
	}
	err := e.getRelationsAndTypes(notProceedTypes, processedObjects)
	if err != nil {
		return err
	}
	return nil
}

func (e *exportContext) fillNotProcessedTypes(processedObjects map[string]struct{}, object string, notProceedTypes map[string]*Doc) {
	if _, ok := processedObjects[object]; ok {
		return
	}
	notProceedTypes[object] = nil
	processedObjects[object] = struct{}{}
}

func (e *exportContext) getTemplatesRelationsAndTypes(processedObjects map[string]struct{}) error {
	allTypes := lo.MapToSlice(e.objectTypes, func(key string, value struct{}) string { return key })
	templates, err := e.queryAndFilterObjectsByRelation(e.spaceId, allTypes, bundle.RelationKeyTargetObjectType)
	if err != nil {
		return nil
	}
	if len(templates) == 0 {
		return nil
	}
	templatesToProcess := make(map[string]*Doc, len(templates))
	for _, template := range templates {
		id := template.Details.GetString(bundle.RelationKeyId)
		if _, ok := e.docs[id]; !ok {
			templateDoc := &Doc{Details: template.Details, isLink: e.isLinkProcess}
			e.docs[id] = templateDoc
			templatesToProcess[id] = templateDoc
		}
	}
	err = e.getRelationsAndTypes(templatesToProcess, processedObjects)
	if err != nil {
		return err
	}
	return nil
}

func (e *exportContext) addRelationsAndTypes() error {
	types := lo.MapToSlice(e.objectTypes, func(key string, value struct{}) string { return key })
	setOfList := lo.MapToSlice(e.setOfList, func(key string, value struct{}) string { return key })
	relations := lo.MapToSlice(e.relations, func(key string, value struct{}) string { return key })

	err := e.addRelations(relations)
	if err != nil {
		return err
	}
	err = e.processObjectTypesAndSetOfList(types, setOfList)
	if err != nil {
		return err
	}
	return nil
}

func (e *exportContext) addRelations(relations []string) error {
	storeRelations, err := e.getRelationsFromStore(relations)
	if err != nil {
		return err
	}
	for _, storeRelation := range storeRelations {
		e.addRelation(storeRelation)
		err := e.addOptionIfTag(storeRelation)
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *exportContext) getRelationsFromStore(relations []string) ([]database.Record, error) {
	uniqueKeys := make([]string, 0, len(relations))
	for _, relation := range relations {
		uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, relation)
		if err != nil {
			return nil, err
		}
		uniqueKeys = append(uniqueKeys, uniqueKey.Marshal())
	}
	storeRelations, err := e.queryAndFilterObjectsByRelation(e.spaceId, uniqueKeys, bundle.RelationKeyUniqueKey)
	if err != nil {
		return nil, err
	}
	return storeRelations, nil
}

func (e *exportContext) addRelation(relation database.Record) {
	relationKey := domain.RelationKey(relation.Details.GetString(bundle.RelationKeyRelationKey))
	if relationKey != "" {
		id := relation.Details.GetString(bundle.RelationKeyId)
		e.docs[id] = &Doc{Details: relation.Details, isLink: e.isLinkProcess}
	}
}

func (e *exportContext) addOptionIfTag(relation database.Record) error {
	format := relation.Details.GetInt64(bundle.RelationKeyRelationFormat)
	relationKey := relation.Details.GetString(bundle.RelationKeyRelationKey)
	if format == int64(model.RelationFormat_tag) || format == int64(model.RelationFormat_status) {
		err := e.addRelationOptions(relationKey)
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *exportContext) addRelationOptions(relationKey string) error {
	relationOptions, err := e.getRelationOptions(relationKey)
	if err != nil {
		return err
	}
	for _, option := range relationOptions {
		id := option.Details.GetString(bundle.RelationKeyId)
		e.docs[id] = &Doc{Details: option.Details, isLink: e.isLinkProcess}
	}
	return nil
}

func (e *exportContext) getRelationOptions(relationKey string) ([]database.Record, error) {
	relationOptionsDetails, err := e.objectStore.SpaceIndex(e.spaceId).Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(model.ObjectType_relationOption),
			},
			{
				RelationKey: bundle.RelationKeyRelationKey,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(relationKey),
			},
		},
	})
	if err != nil {
		return nil, err
	}
	return relationOptionsDetails, nil
}

func (e *exportContext) processObjectTypesAndSetOfList(objectTypes, setOfList []string) error {
	objectDetails, err := e.queryAndFilterObjectsByRelation(e.spaceId, lo.Union(objectTypes, setOfList), bundle.RelationKeyId)
	if err != nil {
		return err
	}
	if len(objectDetails) == 0 {
		return nil
	}
	recommendedRelations, err := e.addObjectsAndCollectRecommendedRelations(objectDetails)
	if err != nil {
		return err
	}
	err = e.addRecommendedRelations(recommendedRelations)
	if err != nil {
		return err
	}
	return nil
}

func (e *exportContext) addObjectsAndCollectRecommendedRelations(objectTypes []database.Record) ([]string, error) {
	recommendedRelations := make([]string, 0, len(objectTypes))
	for i := 0; i < len(objectTypes); i++ {
		rawUniqueKey := objectTypes[i].Details.GetString(bundle.RelationKeyUniqueKey)
		uniqueKey, err := domain.UnmarshalUniqueKey(rawUniqueKey)
		if err != nil {
			return nil, err
		}
		id := objectTypes[i].Details.GetString(bundle.RelationKeyId)
		e.docs[id] = &Doc{Details: objectTypes[i].Details, isLink: e.isLinkProcess}
		if uniqueKey.SmartblockType() == smartblock.SmartBlockTypeObjectType {
			key, err := domain.GetTypeKeyFromRawUniqueKey(rawUniqueKey)
			if err != nil {
				return nil, err
			}
			if bundle.IsInternalType(key) {
				continue
			}
			recommendedRelations = lo.Uniq(slices.Concat(recommendedRelations,
				objectTypes[i].Details.GetStringList(bundle.RelationKeyRecommendedRelations),
				objectTypes[i].Details.GetStringList(bundle.RelationKeyRecommendedHiddenRelations),
				objectTypes[i].Details.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations),
				objectTypes[i].Details.GetStringList(bundle.RelationKeyRecommendedFileRelations),
			))
		}
	}
	return recommendedRelations, nil
}

func (e *exportContext) addRecommendedRelations(recommendedRelations []string) error {
	relations, err := e.queryAndFilterObjectsByRelation(e.spaceId, recommendedRelations, bundle.RelationKeyId)
	if err != nil {
		return err
	}
	for _, relation := range relations {
		id := relation.Details.GetString(bundle.RelationKeyId)
		if id == addr.MissingObject {
			continue
		}

		relationKey := relation.Details.GetString(bundle.RelationKeyUniqueKey)
		uniqueKey, err := domain.UnmarshalUniqueKey(relationKey)
		if err != nil {
			return err
		}
		if bundle.IsSystemRelation(domain.RelationKey(uniqueKey.InternalKey())) {
			continue
		}
		e.docs[id] = &Doc{Details: relation.Details, isLink: e.isLinkProcess}
	}
	return nil
}

func (e *exportContext) addNestedObjects(ids []string) error {
	nestedDocs := make(map[string]*Doc, 0)
	for _, id := range ids {
		e.addNestedObject(id, nestedDocs)
	}
	if len(nestedDocs) == 0 {
		return nil
	}
	exportCtxChild := e.copy()
	exportCtxChild.includeNested = false
	exportCtxChild.docs = nestedDocs
	exportCtxChild.isLinkProcess = true
	err := exportCtxChild.processProtobuf()
	if err != nil {
		return err
	}
	for id, object := range exportCtxChild.docs {
		if _, ok := e.docs[id]; !ok {
			e.docs[id] = object
		}
	}
	return nil
}

func (e *exportContext) addNestedObject(id string, nestedDocs map[string]*Doc) {
	var links []string
	err := cache.Do(e.picker, id, func(sb sb.SmartBlock) error {
		st := sb.NewState().Copy().Filter(e.getStateFilters(id))
		links = objectlink.DependentObjectIDs(st, sb.Space(), e.formatFetcher, objectlink.Flags{
			Blocks:                   true,
			Details:                  true,
			Collection:               true,
			NoHiddenBundledRelations: true,
			NoBackLinks:              !e.includeBackLinks,
			CreatorModifierWorkspace: true,
		})
		return nil
	})
	if err != nil {
		return
	}
	for _, link := range links {
		if _, exists := e.docs[link]; !exists {
			sbt, sbtErr := e.sbtProvider.Type(e.spaceId, link)
			if sbtErr != nil {
				log.Errorf("failed to get smartblocktype of id %s", link)
				continue
			}
			if !validType(sbt) {
				continue
			}
			rec, qErr := e.objectStore.SpaceIndex(e.spaceId).QueryByIds([]string{link})
			if qErr != nil {
				log.Errorf("failed to query id %s, err: %s", qErr, err)
				continue
			}
			if isLinkedObjectExist(rec) {
				exportDoc := &Doc{Details: rec[0].Details, isLink: true}
				nestedDocs[link] = exportDoc
				e.docs[link] = exportDoc
				e.addNestedObject(link, nestedDocs)
			}
		}
	}
}

func (e *exportContext) fillLinkedFiles(id string) ([]string, error) {
	spaceIndex := e.objectStore.SpaceIndex(e.spaceId)
	var fileObjectsIds []string
	err := cache.Do(e.picker, id, func(b sb.SmartBlock) error {
		b.NewState().Copy().Filter(e.getStateFilters(id)).IterateLinkedFiles(e.formatFetcher, func(fileObjectId string) {
			res, err := spaceIndex.Query(database.Query{
				Filters: []database.FilterRequest{
					{
						RelationKey: bundle.RelationKeyId,
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       domain.String(fileObjectId),
					},
				},
			})
			if err != nil {
				log.Errorf("failed to get details for file object id %s: %v", fileObjectId, err)
				return
			}
			if len(res) == 0 {
				return
			}
			e.docs[fileObjectId] = &Doc{Details: res[0].Details, isLink: e.isLinkProcess}
			fileObjectsIds = append(fileObjectsIds, fileObjectId)
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return fileObjectsIds, nil
}

func (e *exportContext) getExistedObjects(isProtobuf bool) error {
	spaceIndex := e.objectStore.SpaceIndex(e.spaceId)
	res, err := spaceIndex.List(false)
	if err != nil {
		return err
	}
	if e.includeArchive {
		archivedObjects, err := spaceIndex.List(true)
		if err != nil {
			return err
		}
		res = append(res, archivedObjects...)
	}
	e.docs = make(map[string]*Doc, len(res))
	for _, info := range res {
		objectSpaceID := e.spaceId
		if objectSpaceID == "" {
			objectSpaceID = info.Details.GetString(bundle.RelationKeySpaceId)
		}
		sbType, err := e.sbtProvider.Type(objectSpaceID, info.Id)
		if err != nil {
			log.With("objectId", info.Id).Errorf("failed to get smartblock type: %v", err)
			continue
		}
		if !objectValid(sbType, info, e.includeArchive, isProtobuf) {
			continue
		}
		e.docs[info.Id] = &Doc{Details: info.Details}
	}
	return nil
}

func (e *exportContext) listTargetTypesFromTemplates(ids []string) []string {
	for id, object := range e.docs {
		if object.Details.Has(bundle.RelationKeyTargetObjectType) {
			ids = append(ids, id)
		}
	}
	return ids
}

func (e *exportContext) writeMultiDoc(ctx context.Context, mw converter.MultiConverter, wr writer, queue process.Queue) (succeed int, err error) {
	for did := range e.docs {
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
					return err
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
		return 0, err
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
			return err
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
			return "", err
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
		return "", err
	}
	return fileName, wr.WriteFile(fileName, rd, file.LastModifiedDate())
}

func (e *exportContext) createProfileFile(spaceID string, wr writer) error {
	spc, err := e.spaceService.Get(context.Background(), spaceID)
	if err != nil {
		return err
	}
	var spaceDashBoardID string

	pr, err := e.accountService.ProfileInfo()
	if err != nil {
		return err
	}
	err = cache.Do(e.picker, spc.DerivedIDs().Workspace, func(b sb.SmartBlock) error {
		spaceDashBoardID = b.CombinedDetails().GetString(bundle.RelationKeySpaceDashboardId)
		return nil
	})
	if err != nil {
		return err
	}
	profile := &pb.Profile{
		SpaceDashboardId: spaceDashBoardID,
		Address:          pr.AccountId,
		Name:             pr.Name,
		Avatar:           pr.IconImage,
		ProfileId:        pr.Id,
	}
	data, err := profile.Marshal()
	if err != nil {
		return err
	}
	err = wr.WriteFile(constant.ProfileFile, bytes.NewReader(data), 0)
	if err != nil {
		return err
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

func objectValid(sbType smartblock.SmartBlockType, info *database.ObjectInfo, includeArchived bool, isProtobuf bool) bool {
	if info.Id == addr.AnytypeProfileId {
		return false
	}
	if !isProtobuf && (!validTypeForNonProtobuf(sbType) || !validLayoutForNonProtobuf(info.Details)) {
		return false
	}
	if isProtobuf && !validType(sbType) {
		return false
	}
	if strings.HasPrefix(info.Id, addr.BundledObjectTypeURLPrefix) || strings.HasPrefix(info.Id, addr.BundledRelationURLPrefix) {
		return false
	}
	if info.Details.GetBool(bundle.RelationKeyIsArchived) && !includeArchived {
		return false
	}
	return true
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

func validType(sbType smartblock.SmartBlockType) bool {
	return sbType == smartblock.SmartBlockTypeProfilePage ||
		sbType == smartblock.SmartBlockTypePage ||
		sbType == smartblock.SmartBlockTypeTemplate ||
		sbType == smartblock.SmartBlockTypeWorkspace ||
		sbType == smartblock.SmartBlockTypeWidget ||
		sbType == smartblock.SmartBlockTypeObjectType ||
		sbType == smartblock.SmartBlockTypeRelation ||
		sbType == smartblock.SmartBlockTypeRelationOption ||
		sbType == smartblock.SmartBlockTypeFileObject ||
		sbType == smartblock.SmartBlockTypeParticipant
}

func validTypeForNonProtobuf(sbType smartblock.SmartBlockType) bool {
	return sbType == smartblock.SmartBlockTypeProfilePage ||
		sbType == smartblock.SmartBlockTypePage ||
		sbType == smartblock.SmartBlockTypeFileObject
}

func validLayoutForNonProtobuf(details *domain.Details) bool {
	return details.GetInt64(bundle.RelationKeyResolvedLayout) != int64(model.ObjectType_collection) &&
		details.GetInt64(bundle.RelationKeyResolvedLayout) != int64(model.ObjectType_set)
}

func cleanupFile(wr writer) {
	if wr == nil {
		return
	}
	wr.Close()
	os.Remove(wr.Path())
}

func listObjectIds(docs map[string]*Doc) []string {
	ids := make([]string, 0, len(docs))
	for id := range docs {
		ids = append(ids, id)
	}
	return ids
}

func isLinkedObjectExist(rec []database.Record) bool {
	return len(rec) > 0 && !rec[0].Details.GetBool(bundle.RelationKeyIsDeleted)
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
	knownObjects := e.docs.transformToDetailsMap()
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
