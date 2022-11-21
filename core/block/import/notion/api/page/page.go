package page

import (
	"context"

	"github.com/gogo/protobuf/types"
	"github.com/textileio/go-threads/core/thread"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/client"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/property"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

const (
	ObjectType = "page"
	pageSize   = 100
)

type Service struct {
	propertyService *property.Service
	detailSetter    *property.DetailValueSetter
	blockService    *block.Service
	client          *client.Client
}

// New is a constructor for Service
func New(client *client.Client) *Service {
	return &Service{
		propertyService: property.New(client),
		detailSetter:    property.NewDetailSetter(),
		blockService:    block.New(client),
		client:          client,
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
	Cover          *block.ImageBlock   `json:"cover,omitempty"`
	URL            string              `json:"url,omitempty"`
}

func (p *Page) GetObjectType() string {
	return ObjectType
}

// GetPages transform Page objects from Notion to snaphots
func (ds *Service) GetPages(ctx context.Context, apiKey string, mode pb.RpcObjectImportRequestMode, pages []Page, notionDatabaseIdsToAnytype, databaseNameToID map[string]string) *converter.Response {
	convereterError := converter.ConvertError{}
	return ds.mapPagesToSnaphots(ctx, apiKey, mode, pages, convereterError, notionDatabaseIdsToAnytype, databaseNameToID)
}

func (ds *Service) mapPagesToSnaphots(ctx context.Context, apiKey string, mode pb.RpcObjectImportRequestMode, pages []Page, convereterError converter.ConvertError, notionDatabaseIdsToAnytype, databaseNameToID map[string]string) *converter.Response {
	var allSnapshots = make([]*converter.Snapshot, 0)
	var notionPagesIdsToAnytype = make(map[string]string, 0)
	for _, p := range pages {
		tid, err := threads.ThreadCreateID(thread.AccessControlled, smartblock.SmartBlockTypePage)
		if err != nil {
			convereterError.Add(p.ID, err)
			if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return &converter.Response{Error: convereterError}
			} else {
				continue
			}
		}
		notionPagesIdsToAnytype[p.ID] = tid.String()
	}
	for _, p := range pages {
		snapshot, ce := ds.transformPages(ctx, apiKey, p, mode, notionPagesIdsToAnytype, notionDatabaseIdsToAnytype, databaseNameToID)
		if ce != nil {
			convereterError.Merge(*ce)
			if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return &converter.Response{Error: convereterError}
			} else {
				continue
			}
		}
	
		allSnapshots = append(allSnapshots, &converter.Snapshot{
			Id:       notionPagesIdsToAnytype[p.ID],
			FileName: p.URL,
			Snapshot: snapshot,
		})
	}
	if convereterError.IsEmpty() {
		return &converter.Response{Snapshots: allSnapshots, Error: nil}
	}
	return &converter.Response{Snapshots: allSnapshots, Error: convereterError}
}

func (ds *Service) transformPages(ctx context.Context, apiKey string, d Page, mode pb.RpcObjectImportRequestMode, notionPagesIdsToAnytype, notionDatabaseIdsToAnytype, databaseNameToID map[string]string) (*model.SmartBlockSnapshotBase, *converter.ConvertError) {
	details := make(map[string]*types.Value, 0)
	details[bundle.RelationKeySource.String()] = pbtypes.String(d.URL)
	if d.Icon != nil && d.Icon.Emoji != nil {
		details[bundle.RelationKeyIconEmoji.String()] = pbtypes.String(*d.Icon.Emoji)
	}
	details[bundle.RelationKeyIsArchived.String()] = pbtypes.Bool(d.Archived)

	var (
		allErrors = &converter.ConvertError{}
		relations []*model.RelationLink
	)
	relations, pageNameToID, ce := ds.handlePageProperties(apiKey, d.ID, d.Properties, details, mode)
	if ce != nil {
		allErrors.Merge(*ce)
		if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, allErrors
		}
	}

	notionBlocks, blocksAndChildrenErr := ds.blockService.GetBlocksAndChildren(ctx, d.ID, apiKey, pageSize, mode)
	if blocksAndChildrenErr != nil {
		allErrors.Merge(*blocksAndChildrenErr)
		if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, allErrors
		}
	}

	anytypeBlocks := ds.blockService.MapNotionBlocksToAnytype(notionBlocks, notionPagesIdsToAnytype, notionDatabaseIdsToAnytype, pageNameToID, databaseNameToID)
	snapshot := &model.SmartBlockSnapshotBase{
		Blocks:        anytypeBlocks,
		Details:       &types.Struct{Fields: details},
		ObjectTypes:   []string{bundle.TypeKeyPage.URL()},
		RelationLinks: relations,
	}

	return snapshot, nil
}

// handlePageProperties gets properties values by their ids from notion api and transforms them to Details and RelationLinks
func (ds *Service) handlePageProperties(apiKey, pageID string, p property.Properties, d map[string]*types.Value, mode pb.RpcObjectImportRequestMode) ([]*model.RelationLink, map[string]string, *converter.ConvertError) {
	ce := converter.ConvertError{}
	relations := make([]*model.RelationLink, 0)
	pageNameToID := make(map[string]string, 0)
	for k, v := range p {
		object, err := ds.propertyService.GetPropertyObject(context.TODO(), pageID, v.GetID(), apiKey, v.GetPropertyType())
		if err != nil {
			ce.Add("property: " + v.GetID(), err)
			if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return relations, pageNameToID, &ce
			}
		}
		err = ds.detailSetter.SetDetailValue(k, v.GetPropertyType(), object, d)
		if err != nil && mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			ce.Add("property: " + v.GetID(), err)
			if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return relations, pageNameToID, &ce
			}
		}
		relations = append(relations, &model.RelationLink{
			Key:    k,
			Format: v.GetFormat(),
		})
		if v.GetPropertyType() == property.PropertyConfigTypeTitle {
			if name, ok := d[bundle.RelationKeyName.String()]; ok {
				pageNameToID[pageID] = name.GetStringValue()
			}
		}
	}
	return relations, pageNameToID, nil
}
