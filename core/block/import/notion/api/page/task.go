package page

import (
	"context"
	"sync"

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

type Task struct {
	propertyService *property.Service
	blockService    *block.Service
	p               Page
	wg              *sync.WaitGroup
}

func (pt *Task) ID() string {
	return pt.p.ID
}

func (pt *Task) Execute(ctx context.Context,
	apiKey string,
	mode pb.RpcObjectImportRequestMode,
	request *block.MapRequest) ([]*converter.Snapshot, []*converter.Relation, converter.ConvertError) {
	defer pt.wg.Done()
	var allSnapshots []*converter.Snapshot
	snapshot, relations, ce := pt.transformPages(ctx, apiKey, pt.p, mode, request)
	if ce != nil {
		if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, nil, ce
		}
	}
	pageID := request.NotionPageIdsToAnytype[pt.p.ID]
	allSnapshots = append(allSnapshots, &converter.Snapshot{
		Id:       pageID,
		FileName: pt.p.URL,
		Snapshot: &pb.ChangeSnapshot{Data: snapshot},
		SbType:   smartblock.SmartBlockTypePage,
	})
	return allSnapshots, relations, nil
}

func (pt *Task) transformPages(ctx context.Context,
	apiKey string,
	p Page,
	mode pb.RpcObjectImportRequestMode,
	request *block.MapRequest) (*model.SmartBlockSnapshotBase, []*converter.Relation, converter.ConvertError) {
	details := make(map[string]*types.Value, 0)
	details[bundle.RelationKeySource.String()] = pbtypes.String(p.URL)
	if p.Icon != nil && p.Icon.Emoji != nil {
		details[bundle.RelationKeyIconEmoji.String()] = pbtypes.String(*p.Icon.Emoji)
	}
	details[bundle.RelationKeyIsArchived.String()] = pbtypes.Bool(p.Archived)
	details[bundle.RelationKeyIsFavorite.String()] = pbtypes.Bool(false)

	allErrors := converter.ConvertError{}
	relations := pt.handlePageProperties(ctx, apiKey, p.ID, p.Properties, details, request)
	addFCoverDetail(p, details)
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

// handlePageProperties gets properties values by their ids from notion api
// and transforms them to Details and RelationLinks
func (pt *Task) handlePageProperties(ctx context.Context,
	apiKey, pageID string,
	p property.Properties,
	d map[string]*types.Value,
	req *block.MapRequest) []*converter.Relation {
	relations := make([]*converter.Relation, 0)
	for k, v := range p {
		if isPropertyPaginated(v) {
			properties, err := pt.propertyService.GetPropertyObject(ctx, pageID, v.GetID(), apiKey, v.GetPropertyType())
			if err != nil {
				logger.With("method", "handlePageProperties").Errorf("failed to get paginated property, %s, %s", v.GetPropertyType(), err)
				continue
			}
			pt.handlePaginatedProperty(v, properties)
		}
		if r, ok := v.(*property.RelationItem); ok {
			linkRelationsIDWithAnytypeID(r, req.NotionPageIdsToAnytype, req.NotionDatabaseIdsToAnytype)
		}
		var (
			ds property.DetailSetter
			ok bool
		)
		if ds, ok = v.(property.DetailSetter); !ok {
			logger.With("method", "handlePageProperties").
				Errorf("failed to convert to interface DetailSetter, %s", v.GetPropertyType())
			continue
		}
		ds.SetDetail(k, d)

		rel := &converter.Relation{
			Relation: &model.Relation{
				Name:   k,
				Format: v.GetFormat(),
			},
		}
		if isPropertyTag(v) {
			setOptionsForListRelation(v, rel.Relation)
		}
		relations = append(relations, rel)
	}
	return relations
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

func addFCoverDetail(p Page, details map[string]*types.Value) {
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
