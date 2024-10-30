package export

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/anyproto/any-sync/app"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"github.com/google/uuid"
	"github.com/gosimple/slug"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/block/cache"
	sb "github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
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
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
	"github.com/anyproto/anytype-heart/util/anyerror"
	"github.com/anyproto/anytype-heart/util/constant"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/text"
)

const CName = "export"

const (
	tempFileName              = "temp_anytype_backup"
	spaceDirectory            = "spaces"
	typesDirectory            = "types"
	objectsDirectory          = "objects"
	relationsDirectory        = "relations"
	relationsOptionsDirectory = "relationsOptions"
	templatesDirectory        = "templates"
	filesObjects              = "filesObjects"
)

var log = logging.Logger("anytype-mw-export")

type Export interface {
	Export(ctx context.Context, req pb.RpcObjectListExportRequest) (path string, succeed int, err error)
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
	return
}

func (e *export) Name() (name string) {
	return CName
}

func (e *export) Export(ctx context.Context, req pb.RpcObjectListExportRequest) (path string, succeed int, err error) {
	queue := e.processService.NewQueue(pb.ModelProcess{
		Id:    bson.NewObjectId().Hex(),
		Type:  pb.ModelProcess_Export,
		State: 0,
	}, 4)
	queue.SetMessage("prepare")

	if err = queue.Start(); err != nil {
		return
	}
	defer func() {
		queue.Stop(err)
		e.sendNotification(err, req)
	}()

	exportCtx := newExportContext(e, req)
	return exportCtx.exportObjects(ctx, queue)
}

func (e *export) sendNotification(err error, req pb.RpcObjectListExportRequest) {
	errCode := model.NotificationExport_NULL
	if err != nil {
		errCode = model.NotificationExport_UNKNOWN_ERROR
	}
	notificationSendErr := e.notificationService.CreateAndSend(&model.Notification{
		Id:      uuid.New().String(),
		Status:  model.Notification_Created,
		IsLocal: true,
		Payload: &model.NotificationPayloadOfExport{Export: &model.NotificationExport{
			ErrorCode:  errCode,
			ExportType: req.Format,
		}},
		Space: req.SpaceId,
	})
	if notificationSendErr != nil {
		log.Errorf("failed to send notification: %v", notificationSendErr)
	}
}

type exportContext struct {
	spaceId        string
	docs           map[string]*types.Struct
	includeArchive bool
	includeNested  bool
	includeFiles   bool
	format         model.ExportFormat
	isJson         bool
	reqIds         []string
	zip            bool
	path           string

	*export
}

func newExportContext(e *export, req pb.RpcObjectListExportRequest) *exportContext {
	return &exportContext{
		path:           req.Path,
		spaceId:        req.SpaceId,
		docs:           map[string]*types.Struct{},
		includeArchive: req.IncludeArchived,
		includeNested:  req.IncludeNested,
		includeFiles:   req.IncludeFiles,
		format:         req.Format,
		isJson:         req.IsJson,
		reqIds:         req.ObjectIds,
		zip:            req.Zip,
		export:         e,
	}
}

func (e *exportContext) copy() *exportContext {
	return &exportContext{
		spaceId:        e.spaceId,
		docs:           e.docs,
		includeArchive: e.includeArchive,
		includeNested:  e.includeNested,
		includeFiles:   e.includeFiles,
		format:         e.format,
		isJson:         e.isJson,
		reqIds:         e.reqIds,
		export:         e.export,
	}
}

func (e *exportContext) exportObjects(ctx context.Context, queue process.Queue) (string, int, error) {
	err := e.docsForExport()
	if err != nil {
		return "", 0, err
	}
	var wr writer
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
		return e.renameZipArchive(wr, succeed)
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
	}
	if err := queue.Finalize(); err != nil {
		cleanupFile(wr)
		return 0, err
	}
	return succeed, nil
}

func (e *exportContext) exportDocs(ctx context.Context,
	wr writer,
	succeed *int64,
	tasks []process.Task,
) []process.Task {
	for docId := range e.docs {
		did := docId
		task := func() {
			if werr := e.writeDoc(ctx, wr, did); werr != nil {
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
	mc.SetKnownDocs(e.docs)
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
	mc.SetKnownDocs(e.docs)
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
		return "", 0, nil
	}
	return zipName, succeed, nil
}

func isAnyblockExport(format model.ExportFormat) bool {
	return format == model.Export_Protobuf || format == model.Export_JSON
}

func (e *exportContext) docsForExport() (err error) {
	isProtobuf := isAnyblockExport(e.format)
	if len(e.reqIds) == 0 {
		return e.getExistedObjects(isProtobuf)
	}

	if len(e.reqIds) > 0 {
		return e.getObjectsByIDs(isProtobuf)
	}
	return
}

func (e *exportContext) getObjectsByIDs(isProtobuf bool) error {
	res, err := e.queryAndFilterObjectsByRelation(e.spaceId, e.reqIds, bundle.RelationKeyId.String())
	if err != nil {
		return err
	}
	for _, object := range res {
		id := pbtypes.GetString(object.Details, bundle.RelationKeyId.String())
		e.docs[id] = object.Details
	}
	if isProtobuf {
		return e.processProtobuf()
	}
	return e.processNotProtobuf()
}

func (e *exportContext) queryAndFilterObjectsByRelation(spaceId string, reqIds []string, relationFilter string) ([]database.Record, error) {
	var allObjects []database.Record
	const singleBatchCount = 50
	for j := 0; j < len(reqIds); {
		if j+singleBatchCount < len(reqIds) {
			records, err := e.queryObjectsByIds(spaceId, reqIds[j:j+singleBatchCount], relationFilter)
			if err != nil {
				return nil, err
			}
			allObjects = append(allObjects, records...)
		} else {
			records, err := e.queryObjectsByIds(spaceId, reqIds[j:], relationFilter)
			if err != nil {
				return nil, err
			}
			allObjects = append(allObjects, records...)
		}
		j += singleBatchCount
	}
	return allObjects, nil
}

func (e *exportContext) queryObjectsByIds(spaceId string, reqIds []string, relationFilter string) ([]database.Record, error) {
	return e.objectStore.SpaceIndex(spaceId).Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: relationFilter,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       pbtypes.StringList(reqIds),
			},
		},
	})
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
			e.addNestedObject(id, map[string]*types.Struct{})
		}
	}
	return nil
}

func (e *exportContext) processProtobuf() error {
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
	allRelations, allTypes, allSetOfList, err := e.getRelationsAndTypes(e.docs, processedObjects)
	if err != nil {
		return err
	}
	templateRelations, templateTypes, templateSetOfList, err := e.getTemplatesRelationsAndTypes(lo.Union(allTypes, allSetOfList), processedObjects)
	if err != nil {
		return err
	}
	allRelations = lo.Union(allRelations, templateRelations)
	allTypes = lo.Union(allTypes, templateTypes)
	allSetOfList = lo.Union(allSetOfList, templateSetOfList)
	err = e.addRelationsAndTypes(allTypes, allRelations, allSetOfList)
	if err != nil {
		return err
	}
	return nil
}

func (e *exportContext) getRelationsAndTypes(notProcessedObjects map[string]*types.Struct, processedObjects map[string]struct{}) ([]string, []string, []string, error) {
	allRelations, allTypes, allSetOfList, err := e.collectDerivedObjects(notProcessedObjects)
	if err != nil {
		return nil, nil, nil, err
	}
	// get derived objects only from types,
	// because relations currently have only system relations and object type
	if len(allTypes) > 0 || len(allSetOfList) > 0 {
		relations, objectTypes, setOfList, err := e.getDerivedObjectsForTypes(lo.Union(allTypes, allSetOfList), processedObjects)
		if err != nil {
			return nil, nil, nil, err
		}
		allRelations = lo.Union(allRelations, relations)
		allTypes = lo.Union(allTypes, objectTypes)
		allSetOfList = lo.Union(allSetOfList, setOfList)
	}
	return allRelations, allTypes, allSetOfList, nil
}

func (e *exportContext) collectDerivedObjects(objects map[string]*types.Struct) ([]string, []string, []string, error) {
	var relations, objectsTypes, setOf []string
	for id := range objects {
		err := cache.Do(e.picker, id, func(b sb.SmartBlock) error {
			state := b.NewState()
			relations = lo.Union(relations, getObjectRelations(state))
			details := state.CombinedDetails()
			if isObjectWithDataview(details) {
				dataviewRelations, err := getDataviewRelations(state)
				if err != nil {
					return err
				}
				relations = lo.Union(relations, dataviewRelations)
			}
			objectTypeId := pbtypes.GetString(details, bundle.RelationKeyType.String())
			objectsTypes = lo.Union(objectsTypes, []string{objectTypeId})
			setOfList := pbtypes.GetStringList(details, bundle.RelationKeySetOf.String())
			setOf = lo.Union(setOf, setOfList)
			return nil
		})
		if err != nil {
			return nil, nil, nil, err
		}
	}
	return relations, objectsTypes, setOf, nil
}

func getObjectRelations(state *state.State) []string {
	relationLinks := state.GetRelationLinks()
	relations := make([]string, 0, len(relationLinks))
	for _, link := range relationLinks {
		relations = append(relations, link.Key)
	}
	return relations
}

func isObjectWithDataview(details *types.Struct) bool {
	return pbtypes.GetFloat64(details, bundle.RelationKeyLayout.String()) == float64(model.ObjectType_collection) ||
		pbtypes.GetFloat64(details, bundle.RelationKeyLayout.String()) == float64(model.ObjectType_set)
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

func (e *exportContext) getDerivedObjectsForTypes(allTypes []string, processedObjects map[string]struct{}) ([]string, []string, []string, error) {
	notProceedTypes := make(map[string]*types.Struct)
	var relations, objectTypes []string
	for _, object := range allTypes {
		if _, ok := processedObjects[object]; ok {
			continue
		}
		notProceedTypes[object] = nil
		processedObjects[object] = struct{}{}
	}
	if len(notProceedTypes) == 0 {
		return relations, objectTypes, nil, nil
	}
	relations, objectTypes, setOfList, err := e.getRelationsAndTypes(notProceedTypes, processedObjects)
	if err != nil {
		return nil, nil, nil, err
	}
	return relations, objectTypes, setOfList, nil
}

func (e *exportContext) getTemplatesRelationsAndTypes(allTypes []string, processedObjects map[string]struct{}) ([]string, []string, []string, error) {
	templates, err := e.queryAndFilterObjectsByRelation(e.spaceId, allTypes, bundle.RelationKeyTargetObjectType.String())
	if err != nil {
		return nil, nil, nil, err
	}
	if len(templates) == 0 {
		return nil, nil, nil, nil
	}
	templatesToProcess := make(map[string]*types.Struct, len(templates))
	for _, template := range templates {
		id := pbtypes.GetString(template.Details, bundle.RelationKeyId.String())
		if _, ok := e.docs[id]; !ok {
			e.docs[id] = template.Details
			templatesToProcess[id] = template.Details
		}
	}
	templateRelations, templateType, templateSetOfList, err := e.getRelationsAndTypes(templatesToProcess, processedObjects)
	if err != nil {
		return nil, nil, nil, err
	}
	return templateRelations, templateType, templateSetOfList, nil
}

func (e *exportContext) addRelationsAndTypes(types, relations, setOfList []string) error {
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
	storeRelations, err := e.queryAndFilterObjectsByRelation(e.spaceId, uniqueKeys, bundle.RelationKeyUniqueKey.String())
	if err != nil {
		return nil, err
	}
	return storeRelations, nil
}

func (e *exportContext) addRelation(relation database.Record) {
	if relationKey := pbtypes.GetString(relation.Details, bundle.RelationKeyRelationKey.String()); relationKey != "" {
		if !bundle.HasRelation(relationKey) {
			id := pbtypes.GetString(relation.Details, bundle.RelationKeyId.String())
			e.docs[id] = relation.Details
		}
	}
}

func (e *exportContext) addOptionIfTag(relation database.Record) error {
	format := pbtypes.GetInt64(relation.Details, bundle.RelationKeyRelationFormat.String())
	relationKey := pbtypes.GetString(relation.Details, bundle.RelationKeyRelationKey.String())
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
		id := pbtypes.GetString(option.Details, bundle.RelationKeyId.String())
		e.docs[id] = option.Details
	}
	return nil
}

func (e *exportContext) getRelationOptions(relationKey string) ([]database.Record, error) {
	relationOptionsDetails, err := e.objectStore.SpaceIndex(e.spaceId).Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_relationOption)),
			},
			{
				RelationKey: bundle.RelationKeyRelationKey.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(relationKey),
			},
		},
	})
	if err != nil {
		return nil, err
	}
	return relationOptionsDetails, nil
}

func (e *exportContext) processObjectTypesAndSetOfList(objectTypes, setOfList []string) error {
	objectDetails, err := e.queryAndFilterObjectsByRelation(e.spaceId, lo.Union(objectTypes, setOfList), bundle.RelationKeyId.String())
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
		rawUniqueKey := pbtypes.GetString(objectTypes[i].Details, bundle.RelationKeyUniqueKey.String())
		uniqueKey, err := domain.UnmarshalUniqueKey(rawUniqueKey)
		if err != nil {
			return nil, err
		}
		id := pbtypes.GetString(objectTypes[i].Details, bundle.RelationKeyId.String())
		e.docs[id] = objectTypes[i].Details
		if uniqueKey.SmartblockType() == smartblock.SmartBlockTypeObjectType {
			key, err := domain.GetTypeKeyFromRawUniqueKey(rawUniqueKey)
			if err != nil {
				return nil, err
			}
			if bundle.IsInternalType(key) {
				continue
			}
			recommendedRelations = append(recommendedRelations, pbtypes.GetStringList(objectTypes[i].Details, bundle.RelationKeyRecommendedRelations.String())...)
		}
	}
	return recommendedRelations, nil
}

func (e *exportContext) addRecommendedRelations(recommendedRelations []string) error {
	relations, err := e.queryAndFilterObjectsByRelation(e.spaceId, recommendedRelations, bundle.RelationKeyId.String())
	if err != nil {
		return err
	}
	for _, relation := range relations {
		id := pbtypes.GetString(relation.Details, bundle.RelationKeyId.String())
		if id == addr.MissingObject {
			continue
		}

		relationKey := pbtypes.GetString(relation.Details, bundle.RelationKeyUniqueKey.String())
		uniqueKey, err := domain.UnmarshalUniqueKey(relationKey)
		if err != nil {
			return err
		}
		if bundle.IsSystemRelation(domain.RelationKey(uniqueKey.InternalKey())) {
			continue
		}
		e.docs[id] = relation.Details
	}
	return nil
}

func (e *exportContext) addNestedObjects(ids []string) error {
	nestedDocs := make(map[string]*types.Struct, 0)
	for _, id := range ids {
		e.addNestedObject(id, nestedDocs)
	}
	if len(nestedDocs) == 0 {
		return nil
	}
	exportCtxChild := e.copy()
	exportCtxChild.includeNested = false
	exportCtxChild.docs = nestedDocs
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

func (e *exportContext) addNestedObject(id string, nestedDocs map[string]*types.Struct) {
	links, err := e.objectStore.SpaceIndex(e.spaceId).GetOutboundLinksById(id)
	if err != nil {
		log.Errorf("export failed to get outbound links for id: %s", err)
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
				nestedDocs[link] = rec[0].Details
				e.docs[link] = rec[0].Details
				e.addNestedObject(link, nestedDocs)
			}
		}
	}
}

func (e *exportContext) fillLinkedFiles(id string) ([]string, error) {
	spaceIndex := e.objectStore.SpaceIndex(e.spaceId)
	var fileObjectsIds []string
	err := cache.Do(e.picker, id, func(b sb.SmartBlock) error {
		b.NewState().IterateLinkedFiles(func(fileObjectId string) {
			res, err := spaceIndex.Query(database.Query{
				Filters: []*model.BlockContentDataviewFilter{
					{
						RelationKey: bundle.RelationKeyId.String(),
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.String(fileObjectId),
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
			e.docs[fileObjectId] = res[0].Details
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
	e.docs = make(map[string]*types.Struct, len(res))
	for _, info := range res {
		objectSpaceID := e.spaceId
		if objectSpaceID == "" {
			objectSpaceID = pbtypes.GetString(info.Details, bundle.RelationKeySpaceId.String())
		}
		sbType, err := e.sbtProvider.Type(objectSpaceID, info.Id)
		if err != nil {
			log.With("objectId", info.Id).Errorf("failed to get smartblock type: %v", err)
			continue
		}
		if !objectValid(sbType, info, e.includeArchive, isProtobuf) {
			continue
		}
		e.docs[info.Id] = info.Details

	}
	return nil
}

func (e *exportContext) listTargetTypesFromTemplates(ids []string) []string {
	for id, object := range e.docs {
		if pbtypes.Get(object, bundle.RelationKeyTargetObjectType.String()) != nil {
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
				if e.includeFiles && b.Type() == smartblock.SmartBlockTypeFileObject {
					fileName, err := e.saveFile(ctx, wr, b, false)
					if err != nil {
						return fmt.Errorf("save file: %w", err)
					}
					st.SetDetailAndBundledRelation(bundle.RelationKeySource, pbtypes.String(fileName))
				}
				if err = mw.Add(b.Space(), st); err != nil {
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

func (e *exportContext) writeDoc(ctx context.Context, wr writer, docId string) (err error) {
	return cache.Do(e.picker, docId, func(b sb.SmartBlock) error {
		st := b.NewState()
		if pbtypes.GetBool(st.CombinedDetails(), bundle.RelationKeyIsDeleted.String()) {
			return nil
		}

		if e.includeFiles && b.Type() == smartblock.SmartBlockTypeFileObject {
			fileName, err := e.saveFile(ctx, wr, b, e.spaceId == "")
			if err != nil {
				return fmt.Errorf("save file: %w", err)
			}
			st.SetDetailAndBundledRelation(bundle.RelationKeySource, pbtypes.String(fileName))
			// Don't save file objects in markdown
			if e.format == model.Export_Markdown {
				return nil
			}
		}

		var conv converter.Converter
		switch e.format {
		case model.Export_Markdown:
			conv = md.NewMDConverter(st, wr.Namer())
		case model.Export_Protobuf:
			conv = pbc.NewConverter(st, e.isJson)
		case model.Export_JSON:
			conv = pbjson.NewConverter(st)
		}
		conv.SetKnownDocs(e.docs)
		result := conv.Convert(b.Type().ToProto())
		var filename string
		if e.format == model.Export_Markdown {
			filename = makeMarkdownName(st, wr, docId, conv.Ext(), e.spaceId)
		} else if docId == b.Space().DerivedIDs().Home {
			filename = "index" + conv.Ext()
		} else {
			filename = makeFileName(docId, e.spaceId, conv.Ext(), st, b.Type())
		}
		lastModifiedDate := pbtypes.GetInt64(st.LocalDetails(), bundle.RelationKeyLastModifiedDate.String())
		if err = wr.WriteFile(filename, bytes.NewReader(result), lastModifiedDate); err != nil {
			return err
		}
		return nil
	})
}

func (e *exportContext) saveFile(ctx context.Context, wr writer, fileObject sb.SmartBlock, exportAllSpaces bool) (fileName string, err error) {
	fullId := domain.FullFileId{
		SpaceId: fileObject.Space().Id(),
		FileId:  domain.FileId(pbtypes.GetString(fileObject.Details(), bundle.RelationKeyFileId.String())),
	}

	file, err := e.fileService.FileByHash(ctx, fullId)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(file.Info().Media, "image") {
		image, err := e.fileService.ImageByHash(context.TODO(), fullId)
		if err != nil {
			return "", err
		}
		file, err = image.GetOriginalFile()
		if err != nil {
			return "", err
		}
	}
	origName := file.Meta().Name
	rootPath := "files"
	if exportAllSpaces {
		rootPath = filepath.Join(spaceDirectory, fileObject.Space().Id(), rootPath)
	}
	fileName = wr.Namer().Get(rootPath, fileObject.Id(), filepath.Base(origName), filepath.Ext(origName))
	rd, err := file.Reader(context.Background())
	if err != nil {
		return "", err
	}
	return fileName, wr.WriteFile(fileName, rd, file.Info().LastModifiedDate)
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
		spaceDashBoardID = pbtypes.GetString(b.CombinedDetails(), bundle.RelationKeySpaceDashboardId.String())
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
	name := pbtypes.GetString(s.Details(), bundle.RelationKeyName.String())
	if name == "" {
		name = s.Snippet()
	}
	path := ""
	// space can be empty in case user want to export all spaces
	if spaceId == "" {
		spaceId := pbtypes.GetString(s.LocalDetails(), bundle.RelationKeySpaceId.String())
		path = filepath.Join(spaceDirectory, spaceId)
	}
	return wr.Namer().Get(path, docID, name, ext)
}

func makeFileName(docId, spaceId, ext string, st *state.State, blockType smartblock.SmartBlockType) string {
	dir := provideFileDirectory(blockType)
	filename := filepath.Join(dir, docId+ext)
	// space can be empty in case user want to export all spaces
	if spaceId == "" {
		spaceId := pbtypes.GetString(st.LocalDetails(), bundle.RelationKeySpaceId.String())
		filename = filepath.Join(spaceDirectory, spaceId, filename)
	}
	return filename
}

func provideFileDirectory(blockType smartblock.SmartBlockType) string {
	switch blockType {
	case smartblock.SmartBlockTypeRelation:
		return relationsDirectory
	case smartblock.SmartBlockTypeRelationOption:
		return relationsOptionsDirectory
	case smartblock.SmartBlockTypeObjectType:
		return typesDirectory
	case smartblock.SmartBlockTypeTemplate:
		return templatesDirectory
	case smartblock.SmartBlockTypeFile, smartblock.SmartBlockTypeFileObject:
		return filesObjects
	default:
		return objectsDirectory
	}
}

func objectValid(sbType smartblock.SmartBlockType, info *model.ObjectInfo, includeArchived bool, isProtobuf bool) bool {
	if info.Id == addr.AnytypeProfileId {
		return false
	}
	if !isProtobuf && !validTypeForNonProtobuf(sbType) && !validLayoutForNonProtobuf(info.Details) {
		return false
	}
	if isProtobuf && !validType(sbType) {
		return false
	}
	if strings.HasPrefix(info.Id, addr.BundledObjectTypeURLPrefix) || strings.HasPrefix(info.Id, addr.BundledRelationURLPrefix) {
		return false
	}
	if pbtypes.GetBool(info.Details, bundle.RelationKeyIsArchived.String()) && !includeArchived {
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
	return sbType == smartblock.SmartBlockTypeHome ||
		sbType == smartblock.SmartBlockTypeProfilePage ||
		sbType == smartblock.SmartBlockTypePage ||
		sbType == smartblock.SmartBlockTypeSubObject ||
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

func validLayoutForNonProtobuf(details *types.Struct) bool {
	return pbtypes.GetFloat64(details, bundle.RelationKeyLayout.String()) != float64(model.ObjectType_collection) &&
		pbtypes.GetFloat64(details, bundle.RelationKeyLayout.String()) != float64(model.ObjectType_set)
}

func cleanupFile(wr writer) {
	wr.Close()
	os.Remove(wr.Path())
}

func listObjectIds(docs map[string]*types.Struct) []string {
	ids := make([]string, 0, len(docs))
	for id := range docs {
		ids = append(ids, id)
	}
	return ids
}

func isLinkedObjectExist(rec []database.Record) bool {
	return len(rec) > 0 && !pbtypes.GetBool(rec[0].Details, bundle.RelationKeyIsDeleted.String())
}
