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

func (pt *Task) transformPages(ctx context.Context,
	apiKey string,
	p Page,
	mode pb.RpcObjectImportRequestMode,
	request *block.MapRequest) (*model.SmartBlockSnapshotBase, []*converter.Relation, converter.ConvertError) {
	details := pt.prepareDetails(p)
	allErrors := converter.ConvertError{}
	relations := pt.handlePageProperties(ctx, apiKey, p.ID, p.Properties, details, request)
	addCoverDetail(p, details)
	notionBlocks, blocksAndChildrenErr := pt.blockService.GetBlocksAndChildren(ctx, p.ID, apiKey, pageSize, mode)
	if blocksAndChildrenErr != nil {
		allErrors.Merge(blocksAndChildrenErr)
		if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, nil, allErrors
		}
	}

	request.Blocks = notionBlocks
	resp := pt.blockService.MapNotionBlocksToAnytype(request)
	snapshot := &model.SmartBlockSnapshotBase{
		Blocks:      resp.Blocks,
		Details:     &types.Struct{Fields: details},
		ObjectTypes: []string{bundle.TypeKeyPage.URL()},
	}

	return snapshot, relations, nil
}

func (pt *Task) prepareDetails(p Page) map[string]*types.Value {
	details := make(map[string]*types.Value, 0)
	details[bundle.RelationKeySource.String()] = pbtypes.String(p.URL)
	if p.Icon != nil && p.Icon.Emoji != nil {
		details[bundle.RelationKeyIconEmoji.String()] = pbtypes.String(*p.Icon.Emoji)
	}
	details[bundle.RelationKeyIsArchived.String()] = pbtypes.Bool(p.Archived)
	details[bundle.RelationKeyIsFavorite.String()] = pbtypes.Bool(false)
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
		relation, err := pt.handleProperty(ctx, apiKey, pageID, k, v, req, d)
		if err != nil {
			logger.With("method", "handlePageProperties").Error(err)
			continue
		}
		relations = append(relations, relation)
	}
	return relations
}

func (pt *Task) handleProperty(ctx context.Context,
	apiKey, pageID, key string,
	propObject property.Object, req *block.MapRequest,
	details map[string]*types.Value) (*converter.Relation, error) {
	if isPropertyPaginated(propObject) {
		properties, err := pt.propertyService.GetPropertyObject(ctx, pageID, propObject.GetID(), apiKey, propObject.GetPropertyType())
		if err != nil {
			return nil, fmt.Errorf("failed to get paginated property, %s, %s", propObject.GetPropertyType(), err)
		}
		pt.handlePaginatedProperty(propObject, properties)
	}
	if r, ok := propObject.(*property.RelationItem); ok {
		linkRelationsIDWithAnytypeID(r, req.NotionPageIdsToAnytype, req.NotionDatabaseIdsToAnytype)
	}
	var (
		ds property.DetailSetter
		ok bool
	)
	if ds, ok = propObject.(property.DetailSetter); !ok {
		return nil, fmt.Errorf("failed to convert to interface DetailSetter, %s", propObject.GetPropertyType())
	}
	ds.SetDetail(key, details)

	rel := &converter.Relation{
		Relation: &model.Relation{
			Name:   key,
			Format: propObject.GetFormat(),
		},
	}
	if isPropertyTag(propObject) {
		setOptionsForListRelation(propObject, rel.Relation)
	}
	return rel, nil
}

func (pt *Task) handlePaginatedProperty(v property.Object, properties []interface{}) {
	switch pr := v.(type) {
	case *property.RelationItem:
		relationItems := make([]*property.Relation, 0, len(properties))
		for _, o := range properties {
			relationItems = append(relationItems, o.(*property.Relation))
		}
		pr.Relation = relationItems
	case *property.RichTextItem:
		richText := make([]*api.RichText, 0, len(properties))
		for _, o := range properties {
			richText = append(richText, o.(*api.RichText))
		}
		pr.RichText = richText
	case *property.PeopleItem:
		pList := make([]*api.User, 0, len(properties))
		for _, o := range properties {
			pList = append(pList, o.(*api.User))
		}
		pr.People = pList
	}
}

// linkRelationsIDWithAnytypeID take anytype ID based on page/database ID from Notin.
// In property we get id from Notion, so we somehow need to map this ID with anytype for correct Relation.
// We use two maps notionPagesIdsToAnytype, notionDatabaseIdsToAnytype for this
func linkRelationsIDWithAnytypeID(rel *property.RelationItem,
	notionPagesIdsToAnytype, notionDatabaseIdsToAnytype map[string]string) {
	for _, r := range rel.Relation {
		if anytypeID, ok := notionPagesIdsToAnytype[r.ID]; ok {
			r.ID = anytypeID
		}
		if anytypeID, ok := notionDatabaseIdsToAnytype[r.ID]; ok {
			r.ID = anytypeID
		}
	}
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
	var text, color []string
	switch property := pr.(type) {
	case *property.StatusItem:
		text = append(text, property.Status.Name)
		color = append(color, api.NotionColorToAnytype[property.Status.Color])
	case *property.SelectItem:
		text = append(text, property.Select.Name)
		color = append(color, api.NotionColorToAnytype[property.Select.Color])
	case *property.MultiSelectItem:
		for _, so := range property.MultiSelect {
			text = append(text, so.Name)
			color = append(color, api.NotionColorToAnytype[so.Color])
		}
	case *property.PeopleItem:
		for _, so := range property.People {
			text = append(text, so.Name)
			color = append(color, api.DefaultColor)
		}
	}

	for i := 0; i < len(text); i++ {
		rel.SelectDict = append(rel.SelectDict, &model.RelationOption{
			Text:  text[i],
			Color: color[i],
		})
	}
}
