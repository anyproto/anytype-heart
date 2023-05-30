package page

import (
	"context"
	"fmt"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/block"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/property"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type DataObject struct {
	apiKey  string
	mode    pb.RpcObjectImportRequestMode
	request *block.MapRequest
	ctx     context.Context
}

func NewDataObject(ctx context.Context, apiKey string, mode pb.RpcObjectImportRequestMode, request *block.MapRequest) *DataObject {
	return &DataObject{apiKey: apiKey, mode: mode, request: request, ctx: ctx}
}

type Result struct {
	snapshot []*converter.Snapshot
	ce       converter.ConvertError
}

type Task struct {
	propertyService *property.Service
	blockService    *block.Service
	p               Page
}

func (pt *Task) ID() string {
	return pt.p.ID
}

func (pt *Task) Execute(data interface{}) interface{} {
	do := data.(*DataObject)
	snapshot, subObjectsSnapshots, ce := pt.makeSnapshotFromPages(do.ctx, do.apiKey, pt.p, do.mode, do.request)
	if ce != nil {
		if do.mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return &Result{ce: ce}
		}
	}
	pageID := do.request.NotionPageIdsToAnytype[pt.p.ID]
	resultSnapshots := make([]*converter.Snapshot, 0, 1+len(subObjectsSnapshots))
	sn := &converter.Snapshot{
		Id:       pageID,
		FileName: pt.p.URL,
		Snapshot: &pb.ChangeSnapshot{Data: snapshot},
		SbType:   smartblock.SmartBlockTypePage,
	}
	resultSnapshots = append(resultSnapshots, sn)
	for _, objectsSnapshot := range subObjectsSnapshots {
		id := pbtypes.GetString(objectsSnapshot.Details, bundle.RelationKeyId.String())
		resultSnapshots = append(resultSnapshots, &converter.Snapshot{
			Id:       id,
			SbType:   smartblock.SmartBlockTypeSubObject,
			Snapshot: &pb.ChangeSnapshot{Data: objectsSnapshot},
		})
	}
	return &Result{snapshot: resultSnapshots, ce: ce}
}

func (pt *Task) makeSnapshotFromPages(
	ctx context.Context,
	apiKey string,
	p Page,
	mode pb.RpcObjectImportRequestMode,
	request *block.MapRequest) (*model.SmartBlockSnapshotBase, []*model.SmartBlockSnapshotBase, converter.ConvertError,
) {

	allErrors := converter.ConvertError{}
	details, subObjectsSnapshots, relationLinks := pt.provideDetails(ctx, apiKey, p, request)

	notionBlocks, blocksAndChildrenErr := pt.blockService.GetBlocksAndChildren(ctx, p.ID, apiKey, pageSize, mode)
	if blocksAndChildrenErr != nil {
		allErrors.Merge(blocksAndChildrenErr)
		if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, nil, allErrors
		}
	}

	snapshot := pt.provideSnapshot(request, notionBlocks, details, relationLinks)

	return snapshot, subObjectsSnapshots, nil
}

func (pt *Task) provideDetails(ctx context.Context, apiKey string, p Page, request *block.MapRequest) (map[string]*types.Value, []*model.SmartBlockSnapshotBase, []*model.RelationLink) {
	details := pt.prepareDetails(p)
	relations, relationLinks := pt.handlePageProperties(ctx, apiKey, p.ID, p.Properties, details, request)
	addCoverDetail(p, details)
	return details, relations, relationLinks
}

func (pt *Task) provideSnapshot(request *block.MapRequest, notionBlocks []interface{}, details map[string]*types.Value, relationLinks []*model.RelationLink) *model.SmartBlockSnapshotBase {
	request.Blocks = notionBlocks
	resp := pt.blockService.MapNotionBlocksToAnytype(request)
	snapshot := &model.SmartBlockSnapshotBase{
		Blocks:        resp.Blocks,
		Details:       &types.Struct{Fields: details},
		ObjectTypes:   []string{bundle.TypeKeyPage.URL()},
		RelationLinks: relationLinks,
	}
	return snapshot
}

func (pt *Task) prepareDetails(p Page) map[string]*types.Value {
	details := make(map[string]*types.Value, 0)
	details[bundle.RelationKeySourceFilePath.String()] = pbtypes.String(p.URL)
	if p.Icon != nil && p.Icon.Emoji != nil {
		details[bundle.RelationKeyIconEmoji.String()] = pbtypes.String(*p.Icon.Emoji)
	}
	details[bundle.RelationKeyIsArchived.String()] = pbtypes.Bool(p.Archived)
	details[bundle.RelationKeyIsFavorite.String()] = pbtypes.Bool(false)
	createdTime := converter.ConvertStringToTime(p.CreatedTime)
	lastEditedTime := converter.ConvertStringToTime(p.LastEditedTime)
	details[bundle.RelationKeyLastModifiedDate.String()] = pbtypes.Float64(float64(lastEditedTime))
	details[bundle.RelationKeyCreatedDate.String()] = pbtypes.Float64(float64(createdTime))
	return details
}

// handlePageProperties gets properties values by their ids from notion api
// and transforms them to Details and RelationLinks
func (pt *Task) handlePageProperties(ctx context.Context,
	apiKey, pageID string,
	p property.Properties,
	d map[string]*types.Value,
	req *block.MapRequest) ([]*model.SmartBlockSnapshotBase, []*model.RelationLink) {
	relations := make([]*model.SmartBlockSnapshotBase, 0)
	relationsLinks := make([]*model.RelationLink, 0)
	for k, v := range p {
		relation, relationLink, err := pt.retrieveRelation(ctx, apiKey, pageID, k, v, req, d)
		if err != nil {
			logger.With("method", "handlePageProperties").Error(err)
			continue
		}
		relations = append(relations, relation...)
		relationsLinks = append(relationsLinks, relationLink)
	}
	return relations, relationsLinks
}

func (pt *Task) retrieveRelation(ctx context.Context,
	apiKey, pageID, key string,
	propObject property.Object,
	req *block.MapRequest,
	details map[string]*types.Value) ([]*model.SmartBlockSnapshotBase, *model.RelationLink, error) {
	if err := pt.handlePagination(ctx, apiKey, pageID, propObject); err != nil {
		return nil, nil, err
	}
	pt.handleLinkRelationsIDWithAnytypeID(propObject, req)
	if snapshot := req.ReadRelationsMap(propObject.GetID()); snapshot != nil {
		id := pbtypes.GetString(snapshot.Details, bundle.RelationKeyRelationKey.String())
		subObjectsSnapshots := pt.getRelationOptionsSnapshots(id, propObject, req)
		if err := pt.setDetails(propObject, id, details); err != nil {
			return nil, nil, err
		}
		relationLink := &model.RelationLink{
			Key:    id,
			Format: propObject.GetFormat(),
		}
		return subObjectsSnapshots, relationLink, nil
	}
	id := bson.NewObjectId().Hex()
	subObjectsSnapshots := pt.getRelationOptionsSnapshots(id, propObject, req)
	if err := pt.setDetails(propObject, id, details); err != nil {
		return nil, nil, err
	}
	relation := pt.getRelationSnapshot(id, key, propObject)
	req.WriteToRelationsMap(propObject.GetID(), relation)
	subObjectsSnapshots = append(subObjectsSnapshots, relation)
	relationLink := &model.RelationLink{
		Key:    id,
		Format: propObject.GetFormat(),
	}
	return subObjectsSnapshots, relationLink, nil
}

func (pt *Task) getRelationSnapshot(id string, key string, propObject property.Object) *model.SmartBlockSnapshotBase {
	details := pt.getRelationDetails(id, key, propObject)
	rel := &model.SmartBlockSnapshotBase{
		Details:     details,
		ObjectTypes: []string{bundle.TypeKeyRelation.URL()},
	}
	return rel
}

func (pt *Task) getRelationOptionsSnapshots(id string, propObject property.Object, req *block.MapRequest) []*model.SmartBlockSnapshotBase {
	subObjectsSnapshots := make([]*model.SmartBlockSnapshotBase, 0)
	if isPropertyTag(propObject) {
		subObjectsSnapshots = append(subObjectsSnapshots, getRelationOptions(propObject, id, req)...)
	}
	return subObjectsSnapshots
}

func (pt *Task) getRelationDetails(id string, key string, propObject property.Object) *types.Struct {
	details := &types.Struct{Fields: map[string]*types.Value{}}
	details.Fields[bundle.RelationKeyRelationFormat.String()] = pbtypes.Float64(float64(propObject.GetFormat()))
	details.Fields[bundle.RelationKeyName.String()] = pbtypes.String(key)
	details.Fields[bundle.RelationKeyId.String()] = pbtypes.String(addr.RelationKeyToIdPrefix + id)
	details.Fields[bundle.RelationKeyRelationKey.String()] = pbtypes.String(id)
	details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_relation))
	return details
}

// linkRelationsIDWithAnytypeID take anytype ID based on page/database ID from Notin.
// In property, we get id from Notion, so we somehow need to map this ID with anytype for correct Relation.
// We use two maps notionPagesIdsToAnytype, notionDatabaseIdsToAnytype for this
func (pt *Task) handleLinkRelationsIDWithAnytypeID(propObject property.Object, req *block.MapRequest) {
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

func (pt *Task) handlePagination(ctx context.Context, apiKey string, pageID string, propObject property.Object) error {
	if isPropertyPaginated(propObject) {
		var (
			properties []interface{}
			err        error
		)
		if properties, err =
			pt.propertyService.GetPropertyObject(
				ctx,
				pageID,
				propObject.GetID(),
				apiKey,
				propObject.GetPropertyType(),
			); err != nil {
			return fmt.Errorf("failed to get paginated property, %s, %s", propObject.GetPropertyType(), err)
		}
		pt.handlePaginatedProperties(propObject, properties)
	}
	return nil
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

func (pt *Task) setDetails(propObject property.Object, key string, details map[string]*types.Value) error {
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

func addCoverDetail(p Page, details map[string]*types.Value) {
	if p.Cover != nil {
		if p.Cover.Type == api.External {
			details[bundle.RelationKeyCoverId.String()] = pbtypes.String(p.Cover.External.URL)
			details[bundle.RelationKeyCoverType.String()] = pbtypes.Float64(1)
		}

		if p.Cover.Type == api.File {
			details[bundle.RelationKeyCoverId.String()] = pbtypes.String(p.Cover.File.URL)
			details[bundle.RelationKeyCoverType.String()] = pbtypes.Float64(1)
		}
	}
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

func getRelationOptions(pr property.Object, rel string, req *block.MapRequest) []*model.SmartBlockSnapshotBase {
	var opts []*model.SmartBlockSnapshotBase
	switch property := pr.(type) {
	case *property.StatusItem:
		options := statusItemOptions(property, rel, req)
		if options != nil {
			opts = append(opts, options)
		}
	case *property.SelectItem:
		options := selectItemOptions(property, rel, req)
		if options != nil {
			opts = append(opts, options)
		}
	case *property.MultiSelectItem:
		opts = append(opts, multiselectItemOptions(property, rel, req)...)
	case *property.PeopleItem:
		opts = append(opts, peopleItemOptions(property, rel, req)...)
	}
	return opts
}

func peopleItemOptions(property *property.PeopleItem, rel string, req *block.MapRequest) []*model.SmartBlockSnapshotBase {
	peopleOptions := make([]*model.SmartBlockSnapshotBase, 0, len(property.People))
	for _, po := range property.People {
		if po.Name == "" {
			return nil
		}
		exist, optionID := isOptionAlreadyExist(po.Name, rel, req)
		if exist {
			po.ID = optionID
			continue
		}
		details, optSnapshot := provideRelationOptionSnapshot(po.Name, "", rel)
		peopleOptions = append(peopleOptions, optSnapshot)
		optionID = pbtypes.GetString(details, bundle.RelationKeyId.String())
		po.ID = optionID
	}
	req.WriteToRelationsOptionsMap(rel, peopleOptions)
	return peopleOptions
}

func multiselectItemOptions(property *property.MultiSelectItem, rel string, req *block.MapRequest) []*model.SmartBlockSnapshotBase {
	multiSelectOptions := make([]*model.SmartBlockSnapshotBase, 0, len(property.MultiSelect))
	for _, so := range property.MultiSelect {
		if so.Name == "" {
			return nil
		}
		exist, optionID := isOptionAlreadyExist(so.Name, rel, req)
		if exist {
			so.ID = optionID
			continue
		}
		details, optSnapshot := provideRelationOptionSnapshot(so.Name, so.Color, rel)
		optionID = pbtypes.GetString(details, bundle.RelationKeyId.String())
		so.ID = optionID
		multiSelectOptions = append(multiSelectOptions, optSnapshot)
	}
	req.WriteToRelationsOptionsMap(rel, multiSelectOptions)
	return multiSelectOptions
}

func selectItemOptions(property *property.SelectItem, rel string, req *block.MapRequest) *model.SmartBlockSnapshotBase {
	if property.Select.Name == "" {
		return nil
	}
	exist, optionID := isOptionAlreadyExist(property.Select.Name, rel, req)
	if exist {
		property.Select.ID = optionID
		return nil
	}
	details, optSnapshot := provideRelationOptionSnapshot(property.Select.Name, property.Select.Color, rel)
	optionID = pbtypes.GetString(details, bundle.RelationKeyId.String())
	property.Select.ID = optionID
	req.WriteToRelationsOptionsMap(rel, []*model.SmartBlockSnapshotBase{optSnapshot})
	return optSnapshot
}

func statusItemOptions(property *property.StatusItem, rel string, req *block.MapRequest) *model.SmartBlockSnapshotBase {
	if property.Status.Name == "" {
		return nil
	}
	exist, optionID := isOptionAlreadyExist(property.Status.Name, rel, req)
	if exist {
		property.Status.ID = optionID
		return nil
	}
	details, optSnapshot := provideRelationOptionSnapshot(property.Status.Name, property.Status.Color, rel)
	optionID = pbtypes.GetString(details, bundle.RelationKeyId.String())
	property.Status.ID = optionID
	req.WriteToRelationsOptionsMap(rel, []*model.SmartBlockSnapshotBase{optSnapshot})
	return optSnapshot
}

func isOptionAlreadyExist(optName, rel string, req *block.MapRequest) (bool, string) {
	options := req.ReadRelationsOptionsMap(rel)
	for _, option := range options {
		name := pbtypes.GetString(option.Details, bundle.RelationKeyName.String())
		id := pbtypes.GetString(option.Details, bundle.RelationKeyId.String())
		if optName == name {
			return true, id
		}
	}
	return false, ""
}

func provideRelationOptionSnapshot(name, color, rel string) (*types.Struct, *model.SmartBlockSnapshotBase) {
	details := getDetailsForRelationOption(name, rel)
	details.Fields[bundle.RelationKeyRelationOptionColor.String()] = pbtypes.String(api.NotionColorToAnytype[color])
	optSnapshot := &model.SmartBlockSnapshotBase{
		Details:     details,
		ObjectTypes: []string{bundle.TypeKeyRelationOption.URL()},
	}
	return details, optSnapshot
}

func getDetailsForRelationOption(name, rel string) *types.Struct {
	details := &types.Struct{Fields: map[string]*types.Value{}}
	details.Fields[bundle.RelationKeyName.String()] = pbtypes.String(name)
	details.Fields[bundle.RelationKeyRelationKey.String()] = pbtypes.String(rel)
	details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_relationOption))
	details.Fields[bundle.RelationKeyId.String()] = pbtypes.String(bson.NewObjectId().Hex())
	return details
}
