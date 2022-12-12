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
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

var logger = logging.Logger("notion-page")

const (
	ObjectType = "page"
	pageSize   = 100
)

type Service struct {
	blockService *block.Service
	client       *client.Client
}

// New is a constructor for Service
func New(client *client.Client) *Service {
	return &Service{
		blockService: block.New(client),
		client:       client,
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

		tid, err := threads.ThreadCreateID(thread.AccessControlled, smartblock.SmartBlockTypePage)
		if err != nil {
			convereterError.Add(p.ID, err)
			if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return nil, nil, convereterError
			} else {
				continue
			}
		}
		notionPagesIdsToAnytype[p.ID] = tid.String()
	}

	progress.SetProgressMessage("Start creating blocks")
	relationsToPageID := make(map[string][]*converter.Relation)
	// Need to collect pages title and notion ids mapping for such blocks as ChildPage and ChildDatabase,
	// because we only get title in those blocks from API
	pageNameToID := make(map[string]string, 0)
	for _, p := range pages {
		for _, v := range p.Properties {
			if t, ok := v.(*property.TitleItem); ok {
				title := api.RichTextToDescription(t.Title)
				pageNameToID[p.ID] = title
				break
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
			} else {
				continue
			}
		}
		pageID := notionPagesIdsToAnytype[p.ID]
		allSnapshots = append(allSnapshots, &converter.Snapshot{
			Id:       pageID,
			FileName: p.URL,
			Snapshot: snapshot,
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
	details[bundle.RelationKeyIsFavorite.String()] = pbtypes.Bool(true)

	allErrors := converter.ConvertError{}
	relations := ds.handlePageProperties(apiKey, p.ID, p.Properties, details, request.NotionPageIdsToAnytype, request.NotionDatabaseIdsToAnytype)

	notionBlocks, blocksAndChildrenErr := ds.blockService.GetBlocksAndChildren(ctx, p.ID, apiKey, pageSize, mode)
	if blocksAndChildrenErr != nil {
		allErrors.Merge(blocksAndChildrenErr)
		if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, nil, allErrors
		}
	}

	request.Blocks = notionBlocks
	resp := ds.blockService.MapNotionBlocksToAnytype(request)
	resp.MergeDetails(details)
	relations = append(relations, resp.Relations...)
	snapshot := &model.SmartBlockSnapshotBase{
		Blocks:      resp.Blocks,
		Details:     &types.Struct{Fields: resp.Details},
		ObjectTypes: []string{bundle.TypeKeyPage.URL()},
	}

	return snapshot, relations, nil
}

// handlePageProperties gets properties values by their ids from notion api and transforms them to Details and RelationLinks
func (ds *Service) handlePageProperties(apiKey, pageID string,
	p property.Properties,
	d map[string]*types.Value,
	notionPagesIdsToAnytype, notionDatabaseIdsToAnytype map[string]string) []*converter.Relation {
	relations := make([]*converter.Relation, 0)
	for k, v := range p {
		if rel, ok := v.(*property.RelationItem); ok {
			linkRelationsIDWithAnytypeID(rel, notionPagesIdsToAnytype, notionDatabaseIdsToAnytype)
		}
		var (
			ds property.DetailSetter
			ok bool
		)
		if ds, ok = v.(property.DetailSetter); !ok {
			logger.With("method", "handlePageProperties").Errorf("failed to convert to interface DetailSetter, %s", v.GetPropertyType())
			continue
		}
		ds.SetDetail(k, d)
		relations = append(relations, &converter.Relation{
			Name:   k,
			Format: v.GetFormat(),
		})
	}
	return relations
}

// linkRelationsIDWithAnytypeID take anytype ID based on page/database ID from Notin. In property we get id from Notion, so we somehow need to
// map this ID with anytype for correct Relation. We use two maps notionPagesIdsToAnytype, notionDatabaseIdsToAnytype for this
func linkRelationsIDWithAnytypeID(rel *property.RelationItem, notionPagesIdsToAnytype, notionDatabaseIdsToAnytype map[string]string) {
	for _, r := range rel.Relation {
		if anytypeID, ok := notionPagesIdsToAnytype[r.ID]; ok {
			r.ID = anytypeID
		}
		if anytypeID, ok := notionDatabaseIdsToAnytype[r.ID]; ok {
			r.ID = anytypeID
		}
	}
}
