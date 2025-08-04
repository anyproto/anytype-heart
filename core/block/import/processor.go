package importer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	creator "github.com/anyproto/anytype-heart/core/block/import/common/objectcreator"
	"github.com/anyproto/anytype-heart/core/block/import/common/workerpool"
	"github.com/anyproto/anytype-heart/core/block/import/web"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/metrics/anymetry"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type ImportProcessor interface {
	Execute(ctx context.Context) *ImportResponse
	ExecuteWebImport(ctx context.Context) (string, *domain.Details, error)
}

func NewProcessor(deps *Dependencies, req *ImportRequest) ImportProcessor {
	return &importProcessor{
		deps:     deps,
		request:  req,
		response: &ImportResponse{},
	}
}

type importProcessor struct {
	deps     *Dependencies
	request  *ImportRequest
	response *ImportResponse

	converterResponse *common.Response
	errors            *common.ConvertError

	oldIDToNew           map[string]string
	createPayloads       map[string]treestorage.TreeStorageCreatePayload
	relationKeysToFormat map[domain.RelationKey]int32
}

func (p *importProcessor) Execute(ctx context.Context) *ImportResponse {
	if p.request.SpaceId == "" {
		p.response.Err = fmt.Errorf("spaceId is empty")
		return p.response
	}

	importId := uuid.New().String()
	recordEvent(&metrics.ImportStartedEvent{ID: importId, ImportType: p.request.Type.String()})

	defer func() {
		p.finalize(ctx, importId)
	}()

	if p.request.Progress == nil {
		p.setupProgressBar()
	}

	if p.deps.blockService != nil && !p.request.GetNoProgress() {
		if err := p.deps.blockService.ProcessAdd(p.request.Progress); err != nil {
			p.response.Err = fmt.Errorf("failed to add process: %w", err)
			return p.response
		}
	}

	p.response.ProcessId = p.request.Progress.Id()

	if p.request.Type == model.Import_External {
		return p.handleExternalImport(ctx)
	}
	return p.handleBuiltinConverterImport(ctx)
}

func (p *importProcessor) setupProgressBar() {
	var progressBarType pb.IsModelProcessMessage = &pb.ModelProcessMessageOfImport{
		Import: &pb.ModelProcessImport{},
	}
	if p.request.IsMigration {
		progressBarType = &pb.ModelProcessMessageOfMigration{
			Migration: &pb.ModelProcessMigration{},
		}
	}

	var progress process.Progress
	if p.request.GetNoProgress() {
		progress = process.NewNoOp()
	} else {
		progress = process.NewProgress(progressBarType)
		if p.request.SendNotification {
			progress = process.NewNotificationProcess(progressBarType, p.deps.notificationService)
		}
	}

	p.request.Progress = progress
}

func (p *importProcessor) handleExternalImport(ctx context.Context) *ImportResponse {
	if p.request.Snapshots == nil {
		p.response.Err = common.ErrNoSnapshotToImport
		return p.response
	}

	sn := make([]*common.Snapshot, len(p.request.Snapshots))
	for i, s := range p.request.Snapshots {
		sn[i] = &common.Snapshot{
			Id: s.GetId(),
			Snapshot: &common.SnapshotModel{
				Data: common.NewStateSnapshotFromProto(s.Snapshot),
			},
		}
	}

	p.request.Origin = objectorigin.Import(model.Import_External)
	if err := p.initConversionFields(&common.Response{Snapshots: sn}, nil); err != nil {
		p.response.Err = fmt.Errorf("failed to build import context, error: %s", err.Error())
		return p.response
	}

	details, _ := p.createObjects(ctx)
	if !p.errors.IsEmpty() {
		p.response.Err = p.errors.GetResultError(p.request.Type)
		return p.response
	}

	p.response.ObjectsCount = int64(len(details))
	return p.response
}

func (p *importProcessor) handleBuiltinConverterImport(ctx context.Context) *ImportResponse {
	converter, exists := p.deps.converters[p.request.Type.String()]
	if !exists {
		p.response.Err = fmt.Errorf("unknown import type %s", p.request.Type)
		return p.response
	}

	allErrors := common.NewError(p.request.Mode)
	response, convErr := converter.GetSnapshots(ctx, p.request.RpcObjectImportRequest, p.request.Progress)
	if !convErr.IsEmpty() {
		resultErr := convErr.GetResultError(p.request.Type)
		if shouldReturnError(resultErr, response, p.request.RpcObjectImportRequest) {
			p.response.Err = resultErr
			return p.response
		}
		allErrors.Merge(convErr)
	}

	if response == nil || len(response.Snapshots) == 0 {
		p.response.Err = fmt.Errorf("source path doesn't contain %s resources to import", p.request.Type)
		return p.response
	}

	p.createTypeWidgets(response.TypesCreated)

	if err := p.initConversionFields(response, allErrors); err != nil {
		allErrors.Add(fmt.Errorf("failed to build import context: %w", err))
		p.response.Err = allErrors.GetResultError(p.request.Type)
		return p.response
	}

	// Create objects
	details, rootCollectionID := p.createObjects(ctx)
	resultErr := p.errors.GetResultError(p.request.Type)

	if resultErr != nil {
		rootCollectionID = ""
	}

	p.response.RootCollectionId = rootCollectionID
	p.response.RootWidgetLayout = response.RootObjectWidgetType
	p.response.ObjectsCount = calculateObjectCount(details, rootCollectionID)
	p.response.Err = resultErr

	return p.response
}

func (p *importProcessor) createTypeWidgets(typeKeys []domain.TypeKey) {
	err := p.deps.blockService.CreateTypeWidgetsIfMissing(context.Background(), p.request.SpaceId, typeKeys, true)
	if err != nil {
		log.Errorf("failed to create widget from root collection, error: %s", err.Error())
	}
}

func (p *importProcessor) initConversionFields(converterResponse *common.Response, errors *common.ConvertError) error {
	if converterResponse == nil {
		return fmt.Errorf("import request and converter response should not be nil")
	}
	if errors == nil {
		errors = common.NewError(p.request.Mode)
	}
	p.converterResponse = converterResponse
	p.errors = errors
	p.oldIDToNew = make(map[string]string, len(converterResponse.Snapshots))
	p.createPayloads = make(map[string]treestorage.TreeStorageCreatePayload, len(converterResponse.Snapshots))
	p.relationKeysToFormat = make(map[domain.RelationKey]int32, len(converterResponse.Snapshots))
	return nil
}

func (p *importProcessor) createObjects(ctx context.Context) (map[string]*domain.Details, string) {
	if err := p.assignIdsToAllObjects(ctx); err != nil {
		return nil, ""
	}

	workerCount := workerPoolSize
	if len(p.converterResponse.Snapshots) < workerCount {
		workerCount = 1
	}
	dataObject := creator.NewDataObject(
		ctx,
		p.oldIDToNew,
		p.createPayloads,
		p.relationKeysToFormat,
		p.request.Origin,
		p.request.SpaceId,
	)
	pool := workerpool.NewPool(workerCount)

	p.request.Progress.SetProgressMessage("Create objects")

	go p.addWork(pool)
	go pool.Start(dataObject)

	details := p.readResults(pool)
	rootCollectionID := p.oldIDToNew[p.converterResponse.RootObjectID]

	return details, rootCollectionID
}

func (p *importProcessor) assignIdsToAllObjects(ctx context.Context) error {
	relationOptions := make([]*common.Snapshot, 0)
	for _, snapshot := range p.converterResponse.Snapshots {
		// we will process relation options after relations when all relation keys are collected
		if lo.Contains(snapshot.Snapshot.Data.ObjectTypes, bundle.TypeKeyRelationOption.String()) {
			relationOptions = append(relationOptions, snapshot)
			continue
		}
		err := p.processSnapshot(ctx, snapshot)
		if err != nil {
			p.errors.Add(err)
			if p.request.Mode != pb.RpcObjectImportRequest_IGNORE_ERRORS {
				return err
			}
			log.With(zap.String("object name", snapshot.Id)).Error(err)
		}
	}
	for _, option := range relationOptions {
		replaceRelationKeyValue(option, p.oldIDToNew)
		err := p.processSnapshot(ctx, option)
		if err != nil {
			p.errors.Add(err)
			if p.request.Mode != pb.RpcObjectImportRequest_IGNORE_ERRORS {
				return err
			}
			log.With(zap.String("object name", option.Id)).Error(err)
		}
	}
	return nil
}

func (p *importProcessor) processSnapshot(ctx context.Context, snapshot *common.Snapshot) (err error) {
	if err = p.preloadFileKeys(snapshot); err != nil {
		return err
	}

	id, payload, err := p.deps.idProvider.GetIDAndPayload(ctx, p.request.SpaceId, snapshot, time.Now(), p.request.UpdateExistingObjects, p.request.Origin)
	if err != nil {
		return err
	}
	p.oldIDToNew[snapshot.Id] = id
	var isBundled bool
	switch snapshot.Snapshot.SbType {
	case smartblock.SmartBlockTypeObjectType:
		isBundled = bundle.HasObjectTypeByKey(domain.TypeKey(snapshot.Snapshot.Data.Key))
	case smartblock.SmartBlockTypeRelation:
		key := domain.RelationKey(snapshot.Snapshot.Data.Key)
		isBundled = bundle.HasRelation(key)
		if !isBundled {
			p.relationKeysToFormat[key] = int32(snapshot.Snapshot.Data.Details.GetInt64(bundle.RelationKeyRelationFormat)) //nolint:gosec
		}
	}
	// bundled types will be created and then updated, because they can be installed asynchronously
	if payload.RootRawChange != nil && !isBundled {
		p.createPayloads[id] = payload
	}
	return p.extractInternalKey(snapshot)
}

func (p *importProcessor) extractInternalKey(snapshot *common.Snapshot) error {
	newUniqueKey := p.deps.idProvider.GetInternalKey(snapshot.Snapshot.SbType)
	if newUniqueKey == "" {
		return nil
	}

	oldUniqueKey := snapshot.Snapshot.Data.Details.GetString(bundle.RelationKeyUniqueKey)
	if oldUniqueKey == "" {
		oldUniqueKey = snapshot.Snapshot.Data.Key
	}

	if oldUniqueKey != "" {
		p.oldIDToNew[oldUniqueKey] = newUniqueKey
	}

	return nil
}

func (p *importProcessor) preloadFileKeys(snapshot *common.Snapshot) error {
	for _, fileKeys := range snapshot.Snapshot.FileKeys {
		err := p.deps.objectStore.AddFileKeys(domain.FileEncryptionKeys{
			FileId:         domain.FileId(fileKeys.Hash),
			EncryptionKeys: fileKeys.Keys,
		})
		if err != nil {
			return fmt.Errorf("add file keys: %w", err)
		}
	}
	if fileInfo := snapshot.Snapshot.Data.FileInfo; fileInfo != nil {
		keys := make(map[string]string, len(fileInfo.EncryptionKeys))
		for _, key := range fileInfo.EncryptionKeys {
			keys[key.Path] = key.Key
		}
		err := p.deps.objectStore.AddFileKeys(domain.FileEncryptionKeys{
			FileId:         domain.FileId(fileInfo.FileId),
			EncryptionKeys: keys,
		})
		if err != nil {
			return fmt.Errorf("add file keys from file info: %w", err)
		}
	}
	return nil
}

func (p *importProcessor) addWork(pool *workerpool.WorkerPool) {
	for _, snapshot := range p.converterResponse.Snapshots {
		t := creator.NewTask(snapshot, p.deps.objectCreator)
		stop := pool.AddWork(t)
		if stop {
			break
		}
	}
	pool.CloseTask()
}

func (p *importProcessor) readResults(pool *workerpool.WorkerPool) map[string]*domain.Details {
	details := make(map[string]*domain.Details)

	for r := range pool.Results() {
		if err := p.request.Progress.TryStep(1); err != nil {
			p.errors.Add(fmt.Errorf("%w: %s", common.ErrCancel, err.Error()))
			pool.Stop()
			return nil
		}

		result := r.(*creator.Result)
		if result.Err != nil {
			p.errors.Add(result.Err)
			if p.request.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				pool.Stop()
				return nil
			}
		}

		details[result.NewID] = result.Details
	}

	return details
}

func (p *importProcessor) finalize(ctx context.Context, importId string) {
	p.finishImportProcess()
	p.sendFileEvents()
	p.addRootCollectionWidget(ctx)
	recordEvent(&metrics.ImportFinishedEvent{ID: importId, ImportType: p.request.Type.String()})
	p.sendImportFinishEvent()
}

func (p *importProcessor) finishImportProcess() {
	if notificationProgress, ok := p.request.Progress.(process.Notificationable); ok {
		notification := p.buildNotification()
		notificationProgress.FinishWithNotification(notification, p.response.Err)
	} else {
		p.request.Progress.Finish(p.response.Err)
	}
}

func (p *importProcessor) buildNotification() *model.Notification {
	return &model.Notification{
		Id:      uuid.New().String(),
		Status:  model.Notification_Created,
		IsLocal: true,
		Space:   p.request.SpaceId,
		Payload: &model.NotificationPayloadOfImport{
			Import: &model.NotificationImport{
				ProcessId:  p.request.Progress.Id(),
				ErrorCode:  common.GetImportNotificationErrorCode(p.response.Err),
				ImportType: p.request.Type,
				SpaceId:    p.request.SpaceId,
			},
		},
	}
}

func (p *importProcessor) sendFileEvents() {
	if p.response.Err == nil {
		p.deps.fileSync.SendImportEvents()
	}
	p.deps.fileSync.ClearImportEvents()
}

func (p *importProcessor) addRootCollectionWidget(ctx context.Context) {
	if p.response.RootCollectionId == "" {
		return
	}

	spc, err := p.deps.spaceService.Get(ctx, p.request.SpaceId)
	if err != nil {
		log.Errorf("failed to create widget from root collection, error: %s", err.Error())
	} else {
		_, err = p.deps.blockService.CreateWidgetBlock(nil, &pb.RpcBlockCreateWidgetRequest{
			ContextId:    spc.DerivedIDs().Widgets,
			WidgetLayout: p.response.RootWidgetLayout,
			Block: &model.Block{
				Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{
					TargetBlockId: p.response.RootCollectionId,
				}},
			},
		}, true)
		if err != nil {
			log.Errorf("failed to create widget from root collection, error: %s", err.Error())
		}
	}
}

func (p *importProcessor) sendImportFinishEvent() {
	if p.request.IsSync {
		return
	}

	p.deps.eventSender.Broadcast(event.NewEventSingleMessage("", &pb.EventMessageValueOfImportFinish{
		ImportFinish: &pb.EventImportFinish{
			RootCollectionID: p.response.RootCollectionId,
			ObjectsCount:     p.response.ObjectsCount,
			ImportType:       p.request.Type,
		},
	}))
}

func (p *importProcessor) ExecuteWebImport(ctx context.Context) (string, *domain.Details, error) {
	if p.request.Progress == nil {
		p.setupProgressBar()
	}

	if p.deps.blockService != nil {
		if err := p.deps.blockService.ProcessAdd(p.request.Progress); err != nil {
			return "", nil, fmt.Errorf("failed to add process: %w", err)
		}
	}

	defer p.request.Progress.Finish(nil)
	p.request.Progress.SetProgressMessage("Parse url")

	w := p.deps.converters[web.Name]
	converterResponse, converterError := w.GetSnapshots(ctx, p.request.RpcObjectImportRequest, p.request.Progress)
	if converterError != nil {
		return "", nil, converterError.Error()
	}

	if len(converterResponse.Snapshots) == 0 {
		return "", nil, fmt.Errorf("snpashots are empty")
	}

	p.request.Progress.SetProgressMessage("Create objects")

	if err := p.initConversionFields(converterResponse, nil); err != nil {
		return "", nil, err
	}

	details, _ := p.createObjects(ctx)
	if !p.errors.IsEmpty() {
		return "", nil, fmt.Errorf("couldn't create objects")
	}
	return converterResponse.Snapshots[0].Id, details[converterResponse.Snapshots[0].Id], nil
}

func shouldReturnError(e error, res *common.Response, req *pb.RpcObjectImportRequest) bool {
	return (e != nil && req.Mode != pb.RpcObjectImportRequest_IGNORE_ERRORS) ||
		errors.Is(e, common.ErrNotionServerExceedRateLimit) || errors.Is(e, common.ErrCsvLimitExceeded) ||
		(common.IsNoObjectError(e) && (res == nil || len(res.Snapshots) == 0)) || // return error only if we don't have object to import
		errors.Is(e, common.ErrCancel)
}

func recordEvent(event anymetry.Event) {
	metrics.Service.Send(event)
}

func calculateObjectCount(details map[string]*domain.Details, rootCollectionID string) int64 {
	objectsCount := int64(len(details))
	if rootCollectionID != "" && objectsCount > 0 {
		objectsCount-- // exclude root collection object from counter
	}
	return objectsCount
}

func replaceRelationKeyValue(snapshot *common.Snapshot, oldIDToNew map[string]string) {
	if snapshot.Snapshot.Data.Details == nil || snapshot.Snapshot.Data.Details.Len() == 0 {
		return
	}
	key := snapshot.Snapshot.Data.Details.GetString(bundle.RelationKeyRelationKey)
	if newRelationID, ok := oldIDToNew[key]; ok {
		key = strings.TrimPrefix(newRelationID, addr.RelationKeyToIdPrefix)
	}
	snapshot.Snapshot.Data.Details.SetString(bundle.RelationKeyRelationKey, key)
}
