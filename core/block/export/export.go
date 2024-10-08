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
	"github.com/google/uuid"
	"github.com/gosimple/slug"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/block"
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
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
	"github.com/anyproto/anytype-heart/util/anyerror"
	"github.com/anyproto/anytype-heart/util/constant"
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
	blockService        *block.Service
	picker              cache.ObjectGetter
	objectStore         objectstore.ObjectStore
	sbtProvider         typeprovider.SmartBlockTypeProvider
	fileService         files.Service
	spaceService        space.Service
	accountService      account.Service
	notificationService notifications.Notifications
}

func New() Export {
	return &export{}
}

func (e *export) Init(a *app.App) (err error) {
	e.blockService = a.MustComponent(block.CName).(*block.Service)
	e.objectStore = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
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
	queue := e.blockService.Process().NewQueue(pb.ModelProcess{
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

	docs, err := e.docsForExport(req.SpaceId, req)
	if err != nil {
		return
	}

	var wr writer
	if req.Zip {
		if wr, err = newZipWriter(req.Path, tempFileName); err != nil {
			err = anyerror.CleanupError(err)
			return
		}
	} else {
		if wr, err = newDirWriter(req.Path, req.IncludeFiles); err != nil {
			err = anyerror.CleanupError(err)
			return
		}
	}

	queue.SetMessage("export docs")
	if req.Format == model.Export_Protobuf && len(req.ObjectIds) == 0 {
		if err = e.createProfileFile(req.SpaceId, wr); err != nil {
			log.Errorf("failed to create profile file: %s", err)
		}
	}
	if req.Format == model.Export_DOT || req.Format == model.Export_SVG {
		succeed = e.exportDotAndSVG(ctx, req, docs, succeed, wr, queue)
	} else if req.Format == model.Export_GRAPH_JSON {
		succeed = e.exportGraphJson(ctx, req, docs, succeed, wr, queue)
	} else {
		tasks := make([]process.Task, 0, len(docs))
		var succeedAsync int64
		tasks = e.exportDocs(ctx, req, docs, wr, queue, &succeedAsync, tasks)
		err := queue.Wait(tasks...)
		if err != nil {
			e.cleanupFile(wr)
			return "", 0, err
		}
		succeed += int(succeedAsync)
	}
	if err = queue.Finalize(); err != nil {
		e.cleanupFile(wr)
		return "", 0, nil
	}
	wr.Close()
	if req.Zip {
		return e.renameZipArchive(req, wr, succeed)
	}
	return wr.Path(), succeed, nil
}

func (e *export) exportDocs(ctx context.Context,
	req pb.RpcObjectListExportRequest,
	docs map[string]*domain.Details,
	wr writer, queue process.Queue,
	succeed *int64,
	tasks []process.Task,
) []process.Task {
	for docId := range docs {
		did := docId
		task := func() {
			if werr := e.writeDoc(ctx, &req, wr, docs, queue, did); werr != nil {
				log.With("objectID", did).Warnf("can't export doc: %v", werr)
			} else {
				atomic.AddInt64(succeed, 1)
			}
		}
		tasks = append(tasks, task)
	}
	return tasks
}

func (e *export) exportGraphJson(ctx context.Context, req pb.RpcObjectListExportRequest, docs map[string]*domain.Details, succeed int, wr writer, queue process.Queue) int {
	mc := graphjson.NewMultiConverter(e.sbtProvider)
	mc.SetKnownDocs(docs)
	var werr error
	if succeed, werr = e.writeMultiDoc(ctx, mc, wr, docs, queue, req.IncludeFiles); werr != nil {
		log.Warnf("can't export docs: %v", werr)
	}
	return succeed
}

func (e *export) exportDotAndSVG(ctx context.Context, req pb.RpcObjectListExportRequest, docs map[string]*domain.Details, succeed int, wr writer, queue process.Queue) int {
	var format = dot.ExportFormatDOT
	if req.Format == model.Export_SVG {
		format = dot.ExportFormatSVG
	}
	mc := dot.NewMultiConverter(format, e.sbtProvider)
	mc.SetKnownDocs(docs)
	var werr error
	if succeed, werr = e.writeMultiDoc(ctx, mc, wr, docs, queue, req.IncludeFiles); werr != nil {
		log.Warnf("can't export docs: %v", werr)
	}
	return succeed
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

func (e *export) renameZipArchive(req pb.RpcObjectListExportRequest, wr writer, succeed int) (string, int, error) {
	zipName := getZipName(req.Path)
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

func (e *export) docsForExport(spaceID string, req pb.RpcObjectListExportRequest) (docs map[string]*domain.Details, err error) {
	isProtobuf := isAnyblockExport(req.Format)
	if len(req.ObjectIds) == 0 {
		return e.getExistedObjects(spaceID, req.IncludeArchived, isProtobuf)
	}

	if len(req.ObjectIds) > 0 {
		return e.getObjectsByIDs(spaceID, req.ObjectIds, req.IncludeNested, req.IncludeFiles, isProtobuf)
	}
	return
}

func (e *export) getObjectsByIDs(spaceId string, reqIds []string, includeNested, includeFiles, isProtobuf bool) (map[string]*domain.Details, error) {
	res, err := e.queryAndFilterObjectsByRelation(spaceId, reqIds, bundle.RelationKeyId)
	if err != nil {
		return nil, err
	}
	docs := make(map[string]*domain.Details)
	for _, object := range res {
		id := object.Details.GetString(bundle.RelationKeyId)
		docs[id] = object.Details
	}
	if isProtobuf {
		return e.processProtobuf(spaceId, docs, includeNested, includeFiles)
	}
	return e.processNotProtobuf(spaceId, docs, includeNested, includeFiles)
}

func (e *export) queryAndFilterObjectsByRelation(spaceId string, reqIds []string, relationFilter domain.RelationKey) ([]database.Record, error) {
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

func (e *export) queryObjectsByIds(spaceId string, reqIds []string, relationFilter domain.RelationKey) ([]database.Record, error) {
	return e.objectStore.SpaceIndex(spaceId).Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: relationFilter,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.StringList(reqIds),
			},
		},
	})
}

func (e *export) processNotProtobuf(spaceId string, docs map[string]*domain.Details, includeNested, includeFiles bool) (map[string]*domain.Details, error) {
	ids := e.fillObjectsIds(docs)
	if includeFiles {
		fileObjectsIds, err := e.processFiles(spaceId, ids, docs)
		if err != nil {
			return nil, err
		}
		ids = append(ids, fileObjectsIds...)
	}
	if includeNested {
		for _, id := range ids {
			e.addNestedObject(spaceId, id, docs, map[string]*domain.Details{})
		}
	}
	return docs, nil
}

func (e *export) processProtobuf(spaceId string, docs map[string]*domain.Details, includeNested, includeFiles bool) (map[string]*domain.Details, error) {
	ids := e.fillObjectsIds(docs)
	if includeFiles {
		err := e.addFileObjects(spaceId, docs, ids, includeNested)
		if err != nil {
			return nil, err
		}
	}
	err := e.addDerivedObjects(spaceId, docs)
	if err != nil {
		return nil, err
	}
	ids = e.fillTemplateIds(docs, ids)
	if includeNested {
		err = e.addNestedObjects(spaceId, docs, ids, includeFiles)
		if err != nil {
			return nil, err
		}
	}
	return docs, nil
}

func (e *export) fillObjectsIds(docs map[string]*domain.Details) []string {
	ids := make([]string, 0, len(docs))
	for id := range docs {
		ids = append(ids, id)
	}
	return ids
}

func (e *export) addFileObjects(spaceId string, docs map[string]*domain.Details, ids []string, includeNested bool) error {
	fileObjectsIds, err := e.processFiles(spaceId, ids, docs)
	if err != nil {
		return err
	}
	if includeNested {
		err = e.addNestedObjects(spaceId, docs, fileObjectsIds, true)
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *export) processFiles(spaceId string, ids []string, docs map[string]*domain.Details) ([]string, error) {
	spc, err := e.spaceService.Get(context.Background(), spaceId)
	if err != nil {
		return nil, fmt.Errorf("get space: %w", err)
	}
	var fileObjectsIds []string
	for _, id := range ids {
		objectFiles, err := e.fillLinkedFiles(spc, id, docs)
		if err != nil {
			return nil, err
		}
		fileObjectsIds = lo.Union(fileObjectsIds, objectFiles)
	}
	return fileObjectsIds, nil
}

func (e *export) addDerivedObjects(spaceId string, docs map[string]*domain.Details) error {
	processedObjects := make(map[string]struct{}, 0)
	allRelations, allTypes, allSetOfList, err := e.getRelationsAndTypes(spaceId, docs, processedObjects)
	if err != nil {
		return err
	}
	templateRelations, templateTypes, templateSetOfList, err := e.getTemplatesRelationsAndTypes(spaceId, docs, lo.Union(allTypes, allSetOfList), processedObjects)
	if err != nil {
		return err
	}
	allRelations = lo.Union(allRelations, templateRelations)
	allTypes = lo.Union(allTypes, templateTypes)
	allSetOfList = lo.Union(allSetOfList, templateSetOfList)
	err = e.addRelationsAndTypes(spaceId, docs, allTypes, allRelations, allSetOfList)
	if err != nil {
		return err
	}
	return nil
}

func (e *export) getRelationsAndTypes(
	spaceId string,
	objects map[string]*domain.Details,
	processedObjects map[string]struct{},
) ([]string, []string, []string, error) {
	allRelations, allTypes, allSetOfList, err := e.collectDerivedObjects(objects)
	if err != nil {
		return nil, nil, nil, err
	}
	// get derived objects only from types,
	// because relations currently have only system relations and object type
	if len(allTypes) > 0 || len(allSetOfList) > 0 {
		relations, objectTypes, setOfList, err := e.getDerivedObjectsForTypes(spaceId, lo.Union(allTypes, allSetOfList), processedObjects)
		if err != nil {
			return nil, nil, nil, err
		}
		allRelations = lo.Union(allRelations, relations)
		allTypes = lo.Union(allTypes, objectTypes)
		allSetOfList = lo.Union(allSetOfList, setOfList)
	}
	return allRelations, allTypes, allSetOfList, nil
}

func (e *export) collectDerivedObjects(objects map[string]*domain.Details) ([]string, []string, []string, error) {
	var relations, objectsTypes, setOf []string
	for id := range objects {
		err := cache.Do(e.picker, id, func(b sb.SmartBlock) error {
			state := b.NewState()
			relations = lo.Union(relations, e.getObjectRelations(state))
			details := state.CombinedDetails()
			if e.isObjectWithDataview(details) {
				dataviewRelations, err := e.getDataviewRelations(state)
				if err != nil {
					return err
				}
				relations = lo.Union(relations, dataviewRelations)
			}
			objectTypeId := details.GetString(bundle.RelationKeyType)
			objectsTypes = lo.Union(objectsTypes, []string{objectTypeId})
			setOfList := details.GetStringList(bundle.RelationKeySetOf)
			setOf = lo.Union(setOf, setOfList)
			return nil
		})
		if err != nil {
			return nil, nil, nil, err
		}
	}
	return relations, objectsTypes, setOf, nil
}

func (e *export) getObjectRelations(state *state.State) []string {
	relationLinks := state.GetRelationLinks()
	relations := make([]string, 0, len(relationLinks))
	for _, link := range relationLinks {
		relations = append(relations, link.Key)
	}
	return relations
}

func (e *export) isObjectWithDataview(details *domain.Details) bool {
	return details.GetInt64(bundle.RelationKeyLayout) == int64(model.ObjectType_collection) ||
		details.GetInt64(bundle.RelationKeyLayout) == int64(model.ObjectType_set)
}

func (e *export) getDataviewRelations(state *state.State) ([]string, error) {
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

func (e *export) getDerivedObjectsForTypes(
	spaceId string,
	allTypes []string,
	processedObjects map[string]struct{},
) ([]string, []string, []string, error) {
	notProceedTypes := make(map[string]*domain.Details, 0)
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
	relations, objectTypes, setOfList, err := e.getRelationsAndTypes(spaceId, notProceedTypes, processedObjects)
	if err != nil {
		return nil, nil, nil, err
	}
	return relations, objectTypes, setOfList, nil
}

func (e *export) getTemplatesRelationsAndTypes(
	spaceId string,
	allObjects map[string]*domain.Details,
	allTypes []string,
	processedObjects map[string]struct{},
) ([]string, []string, []string, error) {

	templates, err := e.queryAndFilterObjectsByRelation(spaceId, allTypes, bundle.RelationKeyTargetObjectType)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(templates) == 0 {
		return nil, nil, nil, nil
	}
	templatesToProcess := make(map[string]*domain.Details, len(templates))
	for _, template := range templates {
		id := template.Details.GetString(bundle.RelationKeyId)
		if _, ok := allObjects[id]; !ok {
			allObjects[id] = template.Details
			templatesToProcess[id] = template.Details
		}
	}
	templateRelations, templateType, templateSetOfList, err := e.getRelationsAndTypes(spaceId, templatesToProcess, processedObjects)
	if err != nil {
		return nil, nil, nil, err
	}
	return templateRelations, templateType, templateSetOfList, nil
}

func (e *export) addRelationsAndTypes(
	spaceId string,
	allObjects map[string]*domain.Details,
	types []string,
	relations []string,
	setOfList []string,
) error {
	err := e.addRelations(spaceId, allObjects, relations)
	if err != nil {
		return err
	}
	err = e.processObjectTypesAndSetOfList(spaceId, types, allObjects, setOfList)
	if err != nil {
		return err
	}
	return nil
}

func (e *export) addRelations(spaceId string, allObjects map[string]*domain.Details, relations []string) error {
	storeRelations, err := e.getRelationsFromStore(spaceId, relations)
	if err != nil {
		return err
	}
	for _, storeRelation := range storeRelations {
		e.addRelation(storeRelation, allObjects)
		err := e.addOptionIfTag(spaceId, storeRelation, allObjects)
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *export) getRelationsFromStore(spaceId string, relations []string) ([]database.Record, error) {
	uniqueKeys := make([]string, 0, len(relations))
	for _, relation := range relations {
		uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, relation)
		if err != nil {
			return nil, err
		}
		uniqueKeys = append(uniqueKeys, uniqueKey.Marshal())
	}
	storeRelations, err := e.queryAndFilterObjectsByRelation(spaceId, uniqueKeys, bundle.RelationKeyUniqueKey)
	if err != nil {
		return nil, err
	}
	return storeRelations, nil
}

func (e *export) addRelation(relation database.Record, derivedObjects map[string]*domain.Details) {
	if relationKey := relation.Details.GetString(bundle.RelationKeyRelationKey); relationKey != "" {
		if !bundle.HasRelation(domain.RelationKey(relationKey)) {
			id := relation.Details.GetString(bundle.RelationKeyId)
			derivedObjects[id] = relation.Details
		}
	}
}

func (e *export) addOptionIfTag(spaceId string, relation database.Record, derivedObjects map[string]*domain.Details) error {
	format := relation.Details.GetInt64(bundle.RelationKeyRelationFormat)
	relationKey := relation.Details.GetString(bundle.RelationKeyRelationKey)
	if format == int64(model.RelationFormat_tag) || format == int64(model.RelationFormat_status) {
		err := e.addRelationOptions(spaceId, relationKey, derivedObjects)
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *export) addRelationOptions(spaceId string, relationKey string, derivedObjects map[string]*domain.Details) error {
	relationOptions, err := e.getRelationOptions(spaceId, relationKey)
	if err != nil {
		return err
	}
	for _, option := range relationOptions {
		id := option.Details.GetString(bundle.RelationKeyId)
		derivedObjects[id] = option.Details
	}
	return nil
}

func (e *export) getRelationOptions(spaceId, relationKey string) ([]database.Record, error) {
	relationOptionsDetails, err := e.objectStore.SpaceIndex(spaceId).Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_relationOption)),
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

func (e *export) processObjectTypesAndSetOfList(spaceId string, objectTypes []string, allObjects map[string]*domain.Details, setOfList []string) error {
	objectDetails, err := e.queryAndFilterObjectsByRelation(spaceId, lo.Union(objectTypes, setOfList), bundle.RelationKeyId)
	if err != nil {
		return err
	}
	if len(objectDetails) == 0 {
		return nil
	}
	recommendedRelations, err := e.addObjectsAndCollectRecommendedRelations(objectDetails, allObjects)
	if err != nil {
		return err
	}
	err = e.addRecommendedRelations(spaceId, recommendedRelations, allObjects)
	if err != nil {
		return err
	}
	return nil
}

func (e *export) addObjectsAndCollectRecommendedRelations(
	objectTypes []database.Record,
	allObjects map[string]*domain.Details,
) ([]string, error) {
	recommendedRelations := make([]string, 0, len(objectTypes))
	for i := 0; i < len(objectTypes); i++ {
		rawUniqueKey := objectTypes[i].Details.GetString(bundle.RelationKeyUniqueKey)
		uniqueKey, err := domain.UnmarshalUniqueKey(rawUniqueKey)
		if err != nil {
			return nil, err
		}
		id := objectTypes[i].Details.GetString(bundle.RelationKeyId)
		allObjects[id] = objectTypes[i].Details
		if uniqueKey.SmartblockType() == smartblock.SmartBlockTypeObjectType {
			key, err := domain.GetTypeKeyFromRawUniqueKey(rawUniqueKey)
			if err != nil {
				return nil, err
			}
			if bundle.IsInternalType(key) {
				continue
			}
			recommendedRelations = append(recommendedRelations, objectTypes[i].Details.GetStringList(bundle.RelationKeyRecommendedRelations)...)
		}
	}
	return recommendedRelations, nil
}

func (e *export) addRecommendedRelations(spaceId string, recommendedRelations []string, allObjects map[string]*domain.Details) error {
	relations, err := e.queryAndFilterObjectsByRelation(spaceId, recommendedRelations, bundle.RelationKeyId)
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
		allObjects[id] = relation.Details
	}
	return nil
}

func (e *export) addNestedObjects(spaceId string, docs map[string]*domain.Details, ids []string, includeFiles bool) error {
	nestedDocs := make(map[string]*domain.Details, 0)
	for _, id := range ids {
		e.addNestedObject(spaceId, id, docs, nestedDocs)
	}
	if len(nestedDocs) == 0 {
		return nil
	}
	nestedDocs, err := e.processProtobuf(spaceId, nestedDocs, false, includeFiles)
	if err != nil {
		return err
	}
	for id, object := range nestedDocs {
		if _, ok := docs[id]; !ok {
			docs[id] = object
		}
	}
	return nil
}

func (e *export) addNestedObject(spaceId string, id string, docs map[string]*domain.Details, nestedDocs map[string]*domain.Details) {
	links, err := e.objectStore.SpaceIndex(spaceId).GetOutboundLinksById(id)
	if err != nil {
		log.Errorf("export failed to get outbound links for id: %s", err)
		return
	}
	for _, link := range links {
		if _, exists := docs[link]; !exists {
			sbt, sbtErr := e.sbtProvider.Type(spaceId, link)
			if sbtErr != nil {
				log.Errorf("failed to get smartblocktype of id %s", link)
				continue
			}
			if !validType(sbt) {
				continue
			}
			rec, qErr := e.objectStore.SpaceIndex(spaceId).QueryByIds([]string{link})
			if qErr != nil {
				log.Errorf("failed to query id %s, err: %s", qErr, err)
				continue
			}
			if isLinkedObjectExist(rec) {
				nestedDocs[link] = rec[0].Details
				docs[link] = rec[0].Details
				e.addNestedObject(spaceId, link, docs, nestedDocs)
			}
		}
	}
}

func (e *export) fillLinkedFiles(space clientspace.Space, id string, docs map[string]*domain.Details) ([]string, error) {
	spaceIndex := e.objectStore.SpaceIndex(space.Id())
	var fileObjectsIds []string
	err := space.Do(id, func(b sb.SmartBlock) error {
		b.NewState().IterateLinkedFiles(func(fileObjectId string) {
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
			docs[fileObjectId] = res[0].Details
			fileObjectsIds = append(fileObjectsIds, fileObjectId)
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return fileObjectsIds, nil
}

func (e *export) getExistedObjects(spaceId string, includeArchived bool, isProtobuf bool) (map[string]*domain.Details, error) {
	spaceIndex := e.objectStore.SpaceIndex(spaceId)
	res, err := spaceIndex.List(false)
	if err != nil {
		return nil, err
	}
	if includeArchived {
		archivedObjects, err := spaceIndex.List(true)
		if err != nil {
			return nil, err
		}
		res = append(res, archivedObjects...)
	}
	objectDetails := make(map[string]*domain.Details, len(res))
	for _, info := range res {
		objectSpaceID := spaceId
		if spaceId == "" {
			objectSpaceID = info.Details.GetString(bundle.RelationKeySpaceId)
		}
		sbType, err := e.sbtProvider.Type(objectSpaceID, info.Id)
		if err != nil {
			log.With("objectId", info.Id).Errorf("failed to get smartblock type: %v", err)
			continue
		}
		if !e.objectValid(sbType, info, includeArchived, isProtobuf) {
			continue
		}
		objectDetails[info.Id] = info.Details

	}
	return objectDetails, nil
}

func (e *export) writeMultiDoc(ctx context.Context,
	mw converter.MultiConverter,
	wr writer,
	docs map[string]*domain.Details,
	queue process.Queue,
	includeFiles bool,
) (succeed int, err error) {
	for did := range docs {
		if err = queue.Wait(func() {
			log.With("objectID", did).Debugf("write doc")
			werr := cache.Do(e.picker, did, func(b sb.SmartBlock) error {
				st := b.NewState().Copy()
				if includeFiles && b.Type() == smartblock.SmartBlockTypeFileObject {
					fileName, err := e.saveFile(ctx, wr, b, false)
					if err != nil {
						return fmt.Errorf("save file: %w", err)
					}
					st.SetDetailAndBundledRelation(bundle.RelationKeySource, domain.String(fileName))
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

func (e *export) writeDoc(ctx context.Context, req *pb.RpcObjectListExportRequest, wr writer, docInfo map[string]*domain.Details, queue process.Queue, docId string) (err error) {
	return cache.Do(e.picker, docId, func(b sb.SmartBlock) error {
		st := b.NewState()
		if st.LocalDetails().GetBool(bundle.RelationKeyIsDeleted) {
			return nil
		}

		if req.IncludeFiles && b.Type() == smartblock.SmartBlockTypeFileObject {
			fileName, err := e.saveFile(ctx, wr, b, req.SpaceId == "")
			if err != nil {
				return fmt.Errorf("save file: %w", err)
			}
			st.SetDetailAndBundledRelation(bundle.RelationKeySource, domain.String(fileName))
			// Don't save file objects in markdown
			if req.Format == model.Export_Markdown {
				return nil
			}
		}

		var conv converter.Converter
		switch req.Format {
		case model.Export_Markdown:
			conv = md.NewMDConverter(st, wr.Namer())
		case model.Export_Protobuf:
			conv = pbc.NewConverter(st, req.IsJson)
		case model.Export_JSON:
			conv = pbjson.NewConverter(st)
		}
		conv.SetKnownDocs(docInfo)
		result := conv.Convert(b.Type().ToProto())
		var filename string
		if req.Format == model.Export_Markdown {
			filename = e.makeMarkdownName(st, wr, docId, conv, req.SpaceId)
		} else if docId == b.Space().DerivedIDs().Home {
			filename = "index" + conv.Ext()
		} else {
			filename = e.makeFileName(docId, req.SpaceId, conv, st, b.Type())
		}
		lastModifiedDate := st.LocalDetails().GetInt64(bundle.RelationKeyLastModifiedDate)
		if err = wr.WriteFile(filename, bytes.NewReader(result), lastModifiedDate); err != nil {
			return err
		}
		return nil
	})
}

func (e *export) makeMarkdownName(s *state.State, wr writer, docID string, conv converter.Converter, spaceId string) string {
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
	return wr.Namer().Get(path, docID, name, conv.Ext())
}

func (e *export) makeFileName(docId, spaceId string, conv converter.Converter, st *state.State, blockType smartblock.SmartBlockType) string {
	dir := e.provideFileDirectory(blockType)
	filename := filepath.Join(dir, docId+conv.Ext())
	// space can be empty in case user want to export all spaces
	if spaceId == "" {
		spaceId := st.LocalDetails().GetString(bundle.RelationKeySpaceId)
		filename = filepath.Join(spaceDirectory, spaceId, filename)
	}
	return filename
}

func (e *export) provideFileDirectory(blockType smartblock.SmartBlockType) string {
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

func (e *export) saveFile(ctx context.Context, wr writer, fileObject sb.SmartBlock, exportAllSpaces bool) (fileName string, err error) {
	fullId := domain.FullFileId{
		SpaceId: fileObject.Space().Id(),
		FileId:  domain.FileId(fileObject.Details().GetString(bundle.RelationKeyFileId)),
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

func (e *export) createProfileFile(spaceID string, wr writer) error {
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
		ids := b.CombinedDetails().GetStringList(bundle.RelationKeySpaceDashboardId)
		if len(ids) > 0 {
			spaceDashBoardID = ids[0]
		}
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

func (e *export) objectValid(sbType smartblock.SmartBlockType, info *database.ObjectInfo, includeArchived bool, isProtobuf bool) bool {
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

func validLayoutForNonProtobuf(details *domain.Details) bool {
	return details.GetInt64(bundle.RelationKeyLayout) != int64(model.ObjectType_collection) &&
		details.GetInt64(bundle.RelationKeyLayout) != int64(model.ObjectType_set)
}

func (e *export) cleanupFile(wr writer) {
	wr.Close()
	os.Remove(wr.Path())
}

func (e *export) fillTemplateIds(docs map[string]*domain.Details, ids []string) []string {
	for id, object := range docs {
		if object.Has(bundle.RelationKeyTargetObjectType) {
			ids = append(ids, id)
		}
	}
	return ids
}

func isLinkedObjectExist(rec []database.Record) bool {
	return len(rec) > 0 && !rec[0].Details.GetBool(bundle.RelationKeyIsDeleted)
}
