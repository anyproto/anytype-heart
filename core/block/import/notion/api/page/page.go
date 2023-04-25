package page

import (
	"context"

	"github.com/gogo/protobuf/types"
	"github.com/google/uuid"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/client"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/property"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	sb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

var logger = logging.Logger("notion-page")

const (
	ObjectType = "page"
	pageSize   = 100
)

type Service struct {
	blockService    *block.Service
	client          *client.Client
	propertyService *property.Service
}

// New is a constructor for Service
func New(client *client.Client) *Service {
	return &Service{
		blockService:    block.New(client),
		client:          client,
		propertyService: property.New(client),
	}
}

// Page represents Page object from notion https://developers.notion.com/reference/page
type Page struct {
	Object         string              `json:"object"`
	ID             string              `json:"id"`
	CreatedTime    string              `json:"created_time"`
	LastEditedTime string              `json:"last_edited_time"`
	CreatedBy      api.User            `json:"created_by,omitempty"`
	LastEditedBy   api.User            `json:"last_edited_by,omitempty"`
	Parent         api.Parent          `json:"parent"`
	Properties     property.Properties `json:"properties"`
	Archived       bool                `json:"archived"`
	Icon           *api.Icon           `json:"icon,omitempty"`
	Cover          *api.FileObject     `json:"cover,omitempty"`
	URL            string              `json:"url,omitempty"`
}

func (p *Page) GetObjectType() string {
	return ObjectType
}

// GetPages transform Page objects from Notion to snaphots
func (ds *Service) GetPages(ctx context.Context,
	apiKey string,
	mode pb.RpcObjectImportRequestMode,
	pages []Page,
	request *block.MapRequest,
	progress *process.Progress) (*converter.Response, map[string]string, converter.ConvertError) {
	var (
		allSnapshots            = make([]*converter.Snapshot, 0)
		convereterError         converter.ConvertError
		notionPagesIdsToAnytype = make(map[string]string, 0)
	)

	progress.SetProgressMessage("Start creating pages from notion")

	for _, p := range pages {
		if err := progress.TryStep(1); err != nil {
			ce := converter.NewFromError(p.ID, err)
			return nil, nil, ce
		}

		notionPagesIdsToAnytype[p.ID] = uuid.New().String()
	}

	progress.SetProgressMessage("Start creating blocks")
	relationsToPageID := make(map[string][]*converter.Relation)
	// Need to collect pages title and notion ids mapping for such blocks as ChildPage and ChildDatabase,
	// because we only get title in those blocks from API
	pageNameToID := make(map[string]string, 0)
	for _, p := range pages {
		for _, v := range p.Properties {
			if t, ok := v.(*property.TitleItem); ok {
				properties, err := ds.propertyService.GetPropertyObject(ctx, p.ID, t.GetID(), apiKey, t.GetPropertyType())
				if err != nil {
					logger.With("method", "handlePageProperties").Errorf("failed to get paginated property, %s, %s", v.GetPropertyType(), err)
					continue
				}
				title := make([]*api.RichText, 0, len(properties))
				for _, o := range properties {
					if t, ok := o.(*api.RichText); ok {
						title = append(title, t)
					}
				}
				t.Title = title
				pageNameToID[p.ID] = t.GetTitle()
			}
		}
	}
	request.NotionPageIdsToAnytype = notionPagesIdsToAnytype
	request.PageNameToID = pageNameToID
	for _, p := range pages {
		if err := progress.TryStep(1); err != nil {
			ce := converter.NewFromError(p.ID, err)
			return nil, nil, ce
		}
		snapshot, relations, ce := ds.transformPages(ctx, apiKey, p, mode, request)
		if ce != nil {
			convereterError.Merge(ce)
			if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return nil, nil, convereterError
			}
			continue
		}
		pageID := notionPagesIdsToAnytype[p.ID]
		allSnapshots = append(allSnapshots, &converter.Snapshot{
			Id:       pageID,
			FileName: p.URL,
			Snapshot: &pb.ChangeSnapshot{Data: snapshot},
			SbType:   sb.SmartBlockTypePage,
		})
		relationsToPageID[pageID] = relations
	}
	if convereterError.IsEmpty() {
		return &converter.Response{Snapshots: allSnapshots, Relations: relationsToPageID}, notionPagesIdsToAnytype, nil
	}

	return &converter.Response{Snapshots: allSnapshots}, notionPagesIdsToAnytype, convereterError
}

func (ds *Service) transformPages(ctx context.Context,
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
	relations := ds.handlePageProperties(ctx, apiKey, p.ID, p.Properties, details, request)
	addFCoverDetail(p, details)
	notionBlocks, blocksAndChildrenErr := ds.blockService.GetBlocksAndChildren(ctx, p.ID, apiKey, pageSize, mode)
	if blocksAndChildrenErr != nil {
		allErrors.Merge(blocksAndChildrenErr)
		if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, nil, allErrors
		}
	}

	request.Blocks = notionBlocks
	resp := ds.blockService.MapNotionBlocksToAnytype(request)
	snapshot := &model.SmartBlockSnapshotBase{
		Blocks:      resp.Blocks,
		Details:     &types.Struct{Fields: details},
		ObjectTypes: []string{bundle.TypeKeyPage.URL()},
	}

	return snapshot, relations, nil
}

// handlePageProperties gets properties values by their ids from notion api
// and transforms them to Details and RelationLinks
func (ds *Service) handlePageProperties(ctx context.Context,
	apiKey, pageID string,
	p property.Properties,
	d map[string]*types.Value,
	req *block.MapRequest) []*converter.Relation {
	relations := make([]*converter.Relation, 0)
	for k, v := range p {
		if isPropertyPaginated(v) {
			properties, err := ds.propertyService.GetPropertyObject(ctx, pageID, v.GetID(), apiKey, v.GetPropertyType())
			if err != nil {
				logger.With("method", "handlePageProperties").Errorf("failed to get paginated property, %s, %s", v.GetPropertyType(), err)
				continue
			}
			ds.handlePaginatedProperty(v, properties)
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

func (*Service) handlePaginatedProperty(v property.Object, properties []interface{}) {
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
