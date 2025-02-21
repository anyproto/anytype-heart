package page

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/globalsign/mgo/bson"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/block"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/files"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/property"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type DataObject struct {
	apiKey    string
	mode      pb.RpcObjectImportRequestMode
	request   *api.NotionImportContext
	ctx       context.Context
	relations *property.PropertiesStore
}

func NewDataObject(ctx context.Context, apiKey string, mode pb.RpcObjectImportRequestMode, request *api.NotionImportContext, relations *property.PropertiesStore) *DataObject {
	return &DataObject{apiKey: apiKey, mode: mode, request: request, ctx: ctx, relations: relations}
}

type Result struct {
	snapshot []*common.Snapshot
	ce       *common.ConvertError
}

type Task struct {
	relationCreateMutex    *sync.Mutex
	relationOptCreateMutex *sync.Mutex
	propertyService        *property.Service
	blockService           *block.Service
	p                      Page
	fileDownloader         files.Downloader
}

func (pt *Task) ID() string {
	return pt.p.ID
}

func (pt *Task) Execute(data interface{}) interface{} {
	do := data.(*DataObject)
	allErrors := common.NewError(do.mode)
	snapshot, relationsAndOptionsSnapshots := pt.makeSnapshotFromPages(do, allErrors)
	if allErrors.ShouldAbortImport(0, model.Import_Notion) {
		return &Result{ce: allErrors}
	}
	pageId := do.request.NotionPageIdsToAnytype[pt.p.ID]
	resultSnapshots := make([]*common.Snapshot, 0, 1+len(relationsAndOptionsSnapshots))
	sn := &common.Snapshot{
		Id:       pageId,
		FileName: pt.p.URL,
		Snapshot: &common.SnapshotModel{
			SbType: smartblock.SmartBlockTypePage,
			Data:   snapshot,
		},
	}
	resultSnapshots = append(resultSnapshots, sn)
	for _, objectsSnapshot := range relationsAndOptionsSnapshots {
		sbType := pt.getRelationOrOptionType(objectsSnapshot)
		resultSnapshots = append(resultSnapshots, &common.Snapshot{
			Id: objectsSnapshot.Details.GetString(bundle.RelationKeyId),
			Snapshot: &common.SnapshotModel{
				SbType: sbType,
				Data:   objectsSnapshot,
			},
		})
	}
	return &Result{snapshot: resultSnapshots, ce: allErrors}
}

func (pt *Task) makeSnapshotFromPages(object *DataObject, allErrors *common.ConvertError) (*common.StateSnapshot, []*common.StateSnapshot) {
	details, relationsAndOptionsSnapshots, relationLinks := pt.provideDetails(object)
	notionBlocks, blocksAndChildrenErr := pt.blockService.GetBlocksAndChildren(object.ctx, pt.p.ID, object.apiKey, pageSize, object.mode)
	if blocksAndChildrenErr != nil {
		allErrors.Merge(blocksAndChildrenErr)
		if allErrors.ShouldAbortImport(0, model.Import_Notion) {
			return nil, nil
		}
	}
	resp := pt.blockService.MapNotionBlocksToAnytype(object.request, notionBlocks, pt.p.ID)
	pt.uploadFilesLocally(resp.Blocks)
	snapshot := pt.provideSnapshot(resp.Blocks, details, relationLinks)
	return snapshot, relationsAndOptionsSnapshots
}

func (pt *Task) uploadFilesLocally(blocks []*model.Block) {
	var (
		wg               sync.WaitGroup
		filesUploadTasks []func()
	)
	for _, block := range blocks {
		if block.GetFile() != nil && block.GetFile().GetName() != "" {
			task, stop := pt.uploadFileBlock(block, &wg)
			if stop {
				break
			}
			filesUploadTasks = append(filesUploadTasks, task)
		}
		if block.GetText() != nil && block.GetText().GetIconImage() != "" {
			task, stop := pt.uploadIconImage(block, &wg)
			if stop {
				break
			}
			filesUploadTasks = append(filesUploadTasks, task)
		}
	}
	for _, task := range filesUploadTasks {
		go task()
	}
	wg.Wait()
}

func (pt *Task) uploadFileBlock(block *model.Block, wg *sync.WaitGroup) (func(), bool) {
	file, stop := pt.fileDownloader.QueueFileForDownload(block.GetFile().GetName())
	if stop {
		return nil, true
	}
	wg.Add(1)
	return func() {
		defer wg.Done()
		localPath, err := file.WaitForLocalPath()
		if err != nil {
			log.Errorf("failed to download file: %s", err)
		}
		block.GetFile().Name = localPath
	}, false
}

func (pt *Task) uploadIconImage(block *model.Block, wg *sync.WaitGroup) (func(), bool) {
	file, stop := pt.fileDownloader.QueueFileForDownload(block.GetText().GetIconImage())
	if stop {
		return nil, true
	}
	wg.Add(1)
	return func() {
		defer wg.Done()
		localPath, err := file.WaitForLocalPath()
		if err != nil {
			log.Errorf("failed to download file: %s", err)
		}
		block.GetText().IconImage = localPath
	}, false
}

func (pt *Task) provideDetails(object *DataObject) (*domain.Details, []*common.StateSnapshot, []*model.RelationLink) {
	details, relationLinks := pt.prepareDetails()
	relationsSnapshots, notionRelationLinks := pt.handlePageProperties(object, details)
	relationLinks = append(relationLinks, notionRelationLinks...)
	api.UploadFileRelationLocally(pt.fileDownloader, details, relationLinks)
	return details, relationsSnapshots, relationLinks
}

func (pt *Task) provideSnapshot(notionBlocks []*model.Block, details *domain.Details, relationLinks []*model.RelationLink) *common.StateSnapshot {
	snapshot := &common.StateSnapshot{
		Blocks:        notionBlocks,
		Details:       details,
		ObjectTypes:   []string{bundle.TypeKeyPage.String()},
		RelationLinks: relationLinks,
	}
	return snapshot
}

func (pt *Task) prepareDetails() (*domain.Details, []*model.RelationLink) {
	details := domain.NewDetails()
	var relationLinks []*model.RelationLink
	details.SetString(bundle.RelationKeySourceFilePath, pt.p.ID)
	if pt.p.Icon != nil {
		if iconRelationLink := api.SetIcon(details, pt.p.Icon); iconRelationLink != nil {
			relationLinks = append(relationLinks, iconRelationLink)
		}
	}
	if pt.p.Cover != nil {
		api.SetCover(details, pt.p.Cover)
		relationLinks = append(relationLinks, &model.RelationLink{
			Key:    bundle.RelationKeyCoverId.String(),
			Format: model.RelationFormat_file,
		})
	}

	details.SetBool(bundle.RelationKeyIsArchived, pt.p.Archived)
	details.SetBool(bundle.RelationKeyIsFavorite, false)
	createdTime := common.ConvertStringToTime(pt.p.CreatedTime)
	lastEditedTime := common.ConvertStringToTime(pt.p.LastEditedTime)
	details.SetInt64(bundle.RelationKeyLastModifiedDate, lastEditedTime)
	details.SetInt64(bundle.RelationKeyCreatedDate, createdTime)
	details.SetInt64(bundle.RelationKeyResolvedLayout, int64(model.ObjectType_basic))
	return details, relationLinks
}

// handlePageProperties gets properties values by their ids from notion api
// and transforms them to Details and RelationLinks
func (pt *Task) handlePageProperties(object *DataObject, details *domain.Details) ([]*common.StateSnapshot, []*model.RelationLink) {
	relationsSnapshots := make([]*common.StateSnapshot, 0)
	relationsLinks := make([]*model.RelationLink, 0)
	hasTag := isPageContainsTagProperty(pt.p.Properties)
	var tagExist bool
	for name, prop := range pt.p.Properties {
		relation, relationLink, err := pt.retrieveRelation(object, name, prop, details, hasTag, tagExist)
		if err != nil {
			log.With("method", "handlePageProperties").Error(err)
			continue
		}
		relationsSnapshots = append(relationsSnapshots, relation...)
		relationsLinks = append(relationsLinks, relationLink)
		if shouldApplyTagPropertyToTagRelation(name, prop, hasTag, tagExist) {
			tagExist = true
		}
	}
	return relationsSnapshots, relationsLinks
}

func (pt *Task) retrieveRelation(object *DataObject, key string, propObject property.Object, details *domain.Details, hasTag bool, tagExist bool) ([]*common.StateSnapshot, *model.RelationLink, error) {
	if err := pt.handlePagination(object.ctx, object.apiKey, propObject); err != nil {
		return nil, nil, err
	}
	pt.handleLinkRelationsIDWithAnytypeID(propObject, object.request)
	return pt.makeRelationFromProperty(object.relations, propObject, details, key, hasTag, tagExist)
}

func (pt *Task) makeRelationFromProperty(relation *property.PropertiesStore,
	propObject property.Object,
	details *domain.Details,
	name string,
	hasTag, tagExist bool) ([]*common.StateSnapshot, *model.RelationLink, error) {
	pt.relationCreateMutex.Lock()
	defer pt.relationCreateMutex.Unlock()
	snapshot, key, relationsAndOptionsSnapshots := pt.provideRelationSnapshot(relation, propObject, name, hasTag, tagExist)
	if key == "" {
		key = snapshot.Details.GetString(bundle.RelationKeyRelationKey)
	}
	relationsAndOptionsSnapshots = append(relationsAndOptionsSnapshots, pt.provideRelationOptionsSnapshots(key, propObject, relation)...)
	if err := pt.setDetails(propObject, key, details); err != nil {
		return nil, nil, err
	}
	relationLink := &model.RelationLink{
		Key:    key,
		Format: propObject.GetFormat(),
	}
	return relationsAndOptionsSnapshots, relationLink, nil
}

func (pt *Task) provideRelationSnapshot(
	relation *property.PropertiesStore,
	propObject property.Object,
	name string,
	hasTag, tagExist bool,
) (*common.StateSnapshot, string, []*common.StateSnapshot) {
	var (
		key                          string
		relationsAndOptionsSnapshots []*common.StateSnapshot
	)
	snapshot := relation.GetSnapshotByNameAndFormat(name, int64(propObject.GetFormat()))
	if snapshot == nil {
		if snapshot = relation.ReadRelationsMap(propObject.GetID()); snapshot == nil {
			snapshot, key = pt.getRelationSnapshot(name, propObject, hasTag, tagExist)
			if snapshot != nil {
				relation.WriteToRelationsMap(propObject.GetID(), snapshot)
				relation.AddSnapshotByNameAndFormat(name, int64(propObject.GetFormat()), snapshot)
				relationsAndOptionsSnapshots = append(relationsAndOptionsSnapshots, snapshot)
			}
		}
	}
	return snapshot, key, relationsAndOptionsSnapshots
}

func (pt *Task) getRelationSnapshot(name string, propObject property.Object, hasTag, tagExist bool) (*common.StateSnapshot, string) {
	key := bson.NewObjectId().Hex()
	if propObject.GetPropertyType() == property.PropertyConfigTypeTitle {
		return nil, bundle.RelationKeyName.String()
	}
	if shouldApplyTagPropertyToTagRelation(name, propObject, hasTag, tagExist) {
		key = bundle.RelationKeyTag.String()
	}
	details := pt.getRelationDetails(key, name, propObject)
	rel := &common.StateSnapshot{
		Details:     details,
		ObjectTypes: []string{bundle.TypeKeyRelation.String()},
		Key:         key,
	}
	return rel, key
}

func (pt *Task) provideRelationOptionsSnapshots(id string, propObject property.Object, relation *property.PropertiesStore) []*common.StateSnapshot {
	pt.relationOptCreateMutex.Lock()
	defer pt.relationOptCreateMutex.Unlock()
	relationsAndOptionsSnapshots := make([]*common.StateSnapshot, 0)
	if isPropertyTag(propObject) {
		relationsAndOptionsSnapshots = append(relationsAndOptionsSnapshots, getRelationOptions(propObject, id, relation)...)
	}
	return relationsAndOptionsSnapshots
}

func (pt *Task) getRelationDetails(key string, name string, propObject property.Object) *domain.Details {
	if name == "" {
		name = property.UntitledProperty
	}
	details := domain.NewDetails()
	details.SetInt64(bundle.RelationKeyRelationFormat, int64(propObject.GetFormat()))
	details.SetString(bundle.RelationKeyName, name)
	details.SetString(bundle.RelationKeyRelationKey, key)
	details.SetInt64(bundle.RelationKeyResolvedLayout, int64(model.ObjectType_relation))
	details.SetInt64(bundle.RelationKeyLayout, int64(model.ObjectType_relation))
	details.SetString(bundle.RelationKeySourceFilePath, propObject.GetID())
	uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, key)
	if err != nil {
		log.Warnf("failed to create unique key for Notion relation: %v", err)
		return details
	}
	details.SetString(bundle.RelationKeyId, uniqueKey.Marshal())
	return details
}

// linkRelationsIDWithAnytypeID take anytype ID based on page/database ID from Notin.
// In property, we get id from Notion, so we somehow need to map this ID with anytype for correct Relation.
// We use two maps notionPagesIdsToAnytype, notionDatabaseIdsToAnytype for this
func (pt *Task) handleLinkRelationsIDWithAnytypeID(propObject property.Object, req *api.NotionImportContext) {
	if r, ok := propObject.(*property.RelationItem); ok {
		for _, r := range r.Relation {
			if anytypeID, ok := req.NotionPageIdsToAnytype[r.ID]; ok {
				r.ID = anytypeID
			}
			if anytypeID, ok := req.NotionDatabaseIdsToAnytype[r.ID]; ok {
				r.ID = anytypeID
			}
		}
	}
}

func (pt *Task) handlePagination(ctx context.Context, apiKey string, propObject property.Object) error {
	if isPropertyPaginated(propObject) {
		var (
			properties []interface{}
			err        error
		)
		if isEmpty(propObject) {
			return nil
		}
		if properties, err =
			pt.propertyService.GetPropertyObject(
				ctx,
				pt.p.ID,
				propObject.GetID(),
				apiKey,
				propObject.GetPropertyType(),
			); err != nil {
			return fmt.Errorf("failed to get paginated property, %s, %w", propObject.GetPropertyType(), err)
		}
		pt.handlePaginatedProperties(propObject, properties)
	}
	return nil
}

func isEmpty(object property.Object) bool {
	paginatedObject, ok := object.(property.PaginatedObject)
	if !ok {
		return false
	}
	return paginatedObject.IsEmpty()
}

func (pt *Task) handlePaginatedProperties(propObject property.Object, properties []interface{}) {
	switch pr := propObject.(type) {
	case *property.RelationItem:
		handleRelationItem(properties, pr)
	case *property.RichTextItem:
		handleRichTextItem(properties, pr)
	case *property.PeopleItem:
		handlePeopleItem(properties, pr)
	}
}

func (pt *Task) setDetails(propObject property.Object, key string, details *domain.Details) error {
	var (
		ds property.DetailSetter
		ok bool
	)
	if ds, ok = propObject.(property.DetailSetter); !ok {
		return fmt.Errorf("failed to convert to interface DetailSetter, %s", propObject.GetPropertyType())
	}
	ds.SetDetail(key, details)
	return nil
}

func (pt *Task) getRelationOrOptionType(objectSnapshot *common.StateSnapshot) smartblock.SmartBlockType {
	if lo.Contains(objectSnapshot.ObjectTypes, bundle.TypeKeyRelationOption.String()) {
		return smartblock.SmartBlockTypeRelationOption
	}
	return smartblock.SmartBlockTypeRelation
}

func handlePeopleItem(properties []interface{}, pr *property.PeopleItem) {
	pList := make([]*api.User, 0, len(properties))
	for _, o := range properties {
		pList = append(pList, o.(*api.User))
	}
	pr.People = pList
}

func handleRichTextItem(properties []interface{}, pr *property.RichTextItem) {
	richText := make([]*api.RichText, 0, len(properties))
	for _, o := range properties {
		richText = append(richText, o.(*api.RichText))
	}
	pr.RichText = richText
}

func handleRelationItem(properties []interface{}, pr *property.RelationItem) {
	relationItems := make([]*property.Relation, 0, len(properties))
	for _, o := range properties {
		relationItems = append(relationItems, o.(*property.Relation))
	}
	pr.Relation = relationItems
}

func isPropertyPaginated(pr property.Object) bool {
	if r, ok := pr.(*property.RelationItem); ok && r.HasMore {
		return true
	}
	return pr.GetPropertyType() == property.PropertyConfigTypeRichText ||
		pr.GetPropertyType() == property.PropertyConfigTypePeople
}

func isPropertyTag(pr property.Object) bool {
	return pr.GetPropertyType() == property.PropertyConfigTypeMultiSelect ||
		pr.GetPropertyType() == property.PropertyConfigTypeSelect ||
		pr.GetPropertyType() == property.PropertyConfigStatus ||
		pr.GetPropertyType() == property.PropertyConfigTypePeople
}

func getRelationOptions(pr property.Object, rel string, relation *property.PropertiesStore) []*common.StateSnapshot {
	var opts []*common.StateSnapshot
	switch property := pr.(type) {
	case *property.StatusItem:
		options := statusItemOptions(property, rel, relation)
		if options != nil {
			opts = append(opts, options)
		}
	case *property.SelectItem:
		options := selectItemOptions(property, rel, relation)
		if options != nil {
			opts = append(opts, options)
		}
	case *property.MultiSelectItem:
		opts = append(opts, multiselectItemOptions(property, rel, relation)...)
	case *property.PeopleItem:
		opts = append(opts, peopleItemOptions(property, rel, relation)...)
	}
	return opts
}

func peopleItemOptions(property *property.PeopleItem, rel string, relation *property.PropertiesStore) []*common.StateSnapshot {
	peopleOptions := make([]*common.StateSnapshot, 0, len(property.People))
	for _, po := range property.People {
		if po.Name == "" {
			continue
		}
		exist, optionID := isOptionAlreadyExist(po.Name, rel, relation)
		if exist {
			po.ID = optionID
			continue
		}
		details, optSnapshot := provideRelationOptionSnapshot(po.Name, "", rel)
		peopleOptions = append(peopleOptions, optSnapshot)
		optionID = details.GetString(bundle.RelationKeyId)
		po.ID = optionID
	}
	relation.WriteToRelationsOptionsMap(rel, peopleOptions)
	return peopleOptions
}

func multiselectItemOptions(property *property.MultiSelectItem, rel string, relation *property.PropertiesStore) []*common.StateSnapshot {
	multiSelectOptions := make([]*common.StateSnapshot, 0, len(property.MultiSelect))
	for _, so := range property.MultiSelect {
		if so.Name == "" {
			continue
		}
		exist, optionID := isOptionAlreadyExist(so.Name, rel, relation)
		if exist {
			so.ID = optionID
			continue
		}
		details, optSnapshot := provideRelationOptionSnapshot(so.Name, so.Color, rel)
		optionID = details.GetString(bundle.RelationKeyId)
		so.ID = optionID
		multiSelectOptions = append(multiSelectOptions, optSnapshot)
	}
	relation.WriteToRelationsOptionsMap(rel, multiSelectOptions)
	return multiSelectOptions
}

func selectItemOptions(property *property.SelectItem, rel string, relation *property.PropertiesStore) *common.StateSnapshot {
	if property.Select.Name == "" {
		return nil
	}
	exist, optionID := isOptionAlreadyExist(property.Select.Name, rel, relation)
	if exist {
		property.Select.ID = optionID
		return nil
	}
	details, optSnapshot := provideRelationOptionSnapshot(property.Select.Name, property.Select.Color, rel)
	optionID = details.GetString(bundle.RelationKeyId)
	property.Select.ID = optionID
	relation.WriteToRelationsOptionsMap(rel, []*common.StateSnapshot{optSnapshot})
	return optSnapshot
}

func statusItemOptions(property *property.StatusItem, rel string, relation *property.PropertiesStore) *common.StateSnapshot {
	if property.Status == nil || property.Status.Name == "" {
		return nil
	}
	exist, optionID := isOptionAlreadyExist(property.Status.Name, rel, relation)
	if exist {
		property.Status.ID = optionID
		return nil
	}
	details, optSnapshot := provideRelationOptionSnapshot(property.Status.Name, property.Status.Color, rel)
	optionID = details.GetString(bundle.RelationKeyId)
	property.Status.ID = optionID
	relation.WriteToRelationsOptionsMap(rel, []*common.StateSnapshot{optSnapshot})
	return optSnapshot
}

func isOptionAlreadyExist(optName, rel string, relation *property.PropertiesStore) (bool, string) {
	options := relation.ReadRelationsOptionsMap(rel)
	for _, option := range options {
		name := option.Details.GetString(bundle.RelationKeyName)
		id := option.Details.GetString(bundle.RelationKeyId)
		if optName == name {
			return true, id
		}
	}
	return false, ""
}

func provideRelationOptionSnapshot(name, color, rel string) (*domain.Details, *common.StateSnapshot) {
	id, details := getDetailsForRelationOption(name, rel)
	details.SetString(bundle.RelationKeyRelationOptionColor, api.NotionColorToAnytype[color])
	optSnapshot := &common.StateSnapshot{
		Details:     details,
		ObjectTypes: []string{bundle.TypeKeyRelationOption.String()},
		Key:         id,
	}
	return details, optSnapshot
}

func getDetailsForRelationOption(name, rel string) (string, *domain.Details) {
	id := bson.NewObjectId().Hex()
	details := domain.NewDetails()
	details.SetString(bundle.RelationKeyName, name)
	details.SetString(bundle.RelationKeyRelationKey, rel)
	details.SetInt64(bundle.RelationKeyResolvedLayout, int64(model.ObjectType_relationOption))
	details.SetInt64(bundle.RelationKeyLayout, int64(model.ObjectType_relationOption))
	details.SetInt64(bundle.RelationKeyCreatedDate, time.Now().Unix())
	uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelationOption, id)
	if err != nil {
		log.Warnf("failed to create unique key for Notion relation: %v", err)
		return id, details
	}
	details.SetString(bundle.RelationKeyId, uniqueKey.Marshal())
	return id, details
}

func isPageContainsTagProperty(properties property.Properties) bool {
	for key, pr := range properties {
		if _, ok := pr.(*property.MultiSelectItem); ok {
			if strings.TrimSpace(key) == property.TagNameProperty {
				return true
			}
		}
		if _, ok := pr.(*property.SelectItem); ok {
			if strings.TrimSpace(key) == property.TagNameProperty {
				return true
			}
		}
	}
	return false
}

func shouldApplyTagPropertyToTagRelation(name string, prop property.Object, hasTag, tagExist bool) bool {
	return (prop.GetPropertyType() == property.PropertyConfigTypeMultiSelect || prop.GetPropertyType() == property.PropertyConfigTypeSelect) &&
		property.IsPropertyMatchTagRelation(name, hasTag) && !tagExist
}
