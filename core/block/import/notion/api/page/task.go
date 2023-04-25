package page

import (
	"context"
	"fmt"

	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/property"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
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
	snapshot  *converter.Snapshot
	relations []*converter.Relation
	ce        converter.ConvertError
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

	snapshot, relations, ce := pt.transformPages(do.ctx, do.apiKey, pt.p, do.mode, do.request)
	if ce != nil {
		if do.mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return &Result{ce: ce}
		}
	}
	pageID := do.request.NotionPageIdsToAnytype[pt.p.ID]
	sn := &converter.Snapshot{
		Id:       pageID,
		FileName: pt.p.URL,
		Snapshot: &pb.ChangeSnapshot{Data: snapshot},
		SbType:   smartblock.SmartBlockTypePage,
	}
	return &Result{snapshot: sn, relations: relations, ce: ce}
}

func (pt *Task) transformPages(
	ctx context.Context,
	apiKey string,
	p Page,
	mode pb.RpcObjectImportRequestMode,
	request *block.MapRequest) (*model.SmartBlockSnapshotBase, []*converter.Relation, converter.ConvertError,
) {

	allErrors := converter.ConvertError{}
	details, relations := pt.handleDetails(ctx, apiKey, p, request)

	notionBlocks, blocksAndChildrenErr := pt.blockService.GetBlocksAndChildren(ctx, p.ID, apiKey, pageSize, mode)
	if blocksAndChildrenErr != nil {
		allErrors.Merge(blocksAndChildrenErr)
		if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, nil, allErrors
		}
	}

	snapshot := pt.provideSnapshot(request, notionBlocks, details)

	return snapshot, relations, nil
}

func (pt *Task) handleDetails(ctx context.Context, apiKey string, p Page, request *block.MapRequest) (map[string]*types.Value, []*converter.Relation) {
	details := pt.prepareDetails(p)
	relations := pt.handlePageProperties(ctx, apiKey, p.ID, p.Properties, details, request)
	addCoverDetail(p, details)
	return details, relations
}

func (pt *Task) provideSnapshot(request *block.MapRequest, notionBlocks []interface{}, details map[string]*types.Value) *model.SmartBlockSnapshotBase {
	request.Blocks = notionBlocks
	resp := pt.blockService.MapNotionBlocksToAnytype(request)
	snapshot := &model.SmartBlockSnapshotBase{
		Blocks:      resp.Blocks,
		Details:     &types.Struct{Fields: details},
		ObjectTypes: []string{bundle.TypeKeyPage.URL()},
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
	details[bundle.RelationKeyOriginalCreatedDate.String()] = pbtypes.Float64(float64(createdTime))
	return details
}

// handlePageProperties gets properties values by their ids from notion api
// and transforms them to Details and RelationLinks
func (pt *Task) handlePageProperties(ctx context.Context,
	apiKey, pageID string,
	p property.Properties,
	d map[string]*types.Value,
	req *block.MapRequest) []*converter.Relation {
	relations := make([]*converter.Relation, 0)
	for k, v := range p {
		relation, err := pt.retrieveRelation(ctx, apiKey, pageID, k, v, req, d)
		if err != nil {
			logger.With("method", "handlePageProperties").Error(err)
			continue
		}
		relations = append(relations, relation)
	}
	return relations
}

func (pt *Task) retrieveRelation(ctx context.Context,
	apiKey, pageID, key string,
	propObject property.Object, req *block.MapRequest,
	details map[string]*types.Value) (*converter.Relation, error) {

	if err := pt.handlePagination(ctx, apiKey, pageID, propObject); err != nil {
		return nil, err
	}
	pt.handleLinkRelationsIDWithAnytypeID(propObject, req)

	if err := pt.setDetails(propObject, key, details); err != nil {
		return nil, err
	}
	return pt.handleRelation(key, propObject), nil
}

// linkRelationsIDWithAnytypeID take anytype ID based on page/database ID from Notin.
// In property we get id from Notion, so we somehow need to map this ID with anytype for correct Relation.
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

func (pt *Task) handleRelation(key string, propObject property.Object) *converter.Relation {
	rel := &converter.Relation{
		Relation: &model.Relation{
			Name:   key,
			Format: propObject.GetFormat(),
		},
	}
	if isPropertyTag(propObject) {
		setOptionsForListRelation(propObject, rel.Relation)
	}
	return rel
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

func setOptionsForListRelation(pr property.Object, rel *model.Relation) {
	appendToSelectDict(rel, getRelationOptions(pr))
}

func getRelationOptions(pr property.Object) []*model.RelationOption {
	var opts []*model.RelationOption
	switch property := pr.(type) {
	case *property.StatusItem:
		opts = append(opts, statusItemOptions(property))
	case *property.SelectItem:
		opts = append(opts, selectItemOptions(property))
	case *property.MultiSelectItem:
		opts = append(opts, multiselectItemOptions(property)...)
	case *property.PeopleItem:
		opts = append(opts, peopleItemOptions(property)...)
	}
	return opts
}

func appendToSelectDict(rel *model.Relation, opts []*model.RelationOption) {
	rel.SelectDict = append(rel.SelectDict, opts...)
}

func peopleItemOptions(property *property.PeopleItem) []*model.RelationOption {
	res := make([]*model.RelationOption, 0, len(property.People))
	for _, so := range property.People {
		res = append(res, &model.RelationOption{
			Text:  so.Name,
			Color: api.DefaultColor,
		})
	}
	return res
}

func multiselectItemOptions(property *property.MultiSelectItem) []*model.RelationOption {
	res := make([]*model.RelationOption, 0, len(property.MultiSelect))
	for _, so := range property.MultiSelect {
		res = append(res, &model.RelationOption{
			Text:  so.Name,
			Color: api.NotionColorToAnytype[so.Color],
		})
	}
	return res
}

func selectItemOptions(property *property.SelectItem) *model.RelationOption {
	return &model.RelationOption{
		Text:  property.Select.Name,
		Color: api.NotionColorToAnytype[property.Select.Color],
	}
}

func statusItemOptions(property *property.StatusItem) *model.RelationOption {
	return &model.RelationOption{
		Text:  property.Status.Name,
		Color: api.NotionColorToAnytype[property.Status.Color],
	}
}
