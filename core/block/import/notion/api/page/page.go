package page

import (
	"context"
	"time"

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
	"github.com/gogo/protobuf/types"
	"github.com/textileio/go-threads/core/thread"
)

const ObjectType = "page"

type Service struct {
	propertyService *property.Service
}

func New(client *client.Client) *Service {
	return &Service{
		propertyService: property.New(client),
	}
}

type Page struct {
	Object         string              `json:"object"`
	ID             string              `json:"id"`
	CreatedTime    time.Time           `json:"created_time"`
	LastEditedTime time.Time           `json:"last_edited_time"`
	CreatedBy      api.User            `json:"created_by,omitempty"`
	LastEditedBy   api.User            `json:"last_edited_by,omitempty"`
	Title          []api.RichText      `json:"title"`
	Parent         api.Parent          `json:"parent"`
	URL            string              `json:"url"`
	Properties     property.Properties `json:"properties"`
	Archived       bool                `json:"archived"`
	Icon           *api.Icon           `json:"icon,omitempty"`
	Cover          *block.Image        `json:"cover,omitempty"`
}

func (p Page) GetObjectType() string {
	return ObjectType
}

func (ds *Service) GetPages(ctx context.Context, apiKey string, mode pb.RpcObjectImportRequestMode, pages []Page) *converter.Response {
	var convereterError converter.ConvertError
	return ds.mapPagesToSnaphots(ctx, apiKey, mode, pages, convereterError)
}

func (ds *Service) mapPagesToSnaphots(ctx context.Context, apiKey string, mode pb.RpcObjectImportRequestMode, pages []Page, convereterError converter.ConvertError) *converter.Response {
	var allSnapshots = make([]*converter.Snapshot, 0)
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
		snapshot := ds.transformPages(apiKey, &p, mode)

		allSnapshots = append(allSnapshots, &converter.Snapshot{
			Id:       tid.String(),
			FileName: p.URL,
			Snapshot: snapshot,
		})
	}
	if convereterError.IsEmpty() {
		return &converter.Response{Snapshots: allSnapshots, Error: nil}
	}
	return &converter.Response{Snapshots: allSnapshots, Error: convereterError}
}

func (ds *Service) transformPages(apiKey string, d *Page, mode pb.RpcObjectImportRequestMode) *model.SmartBlockSnapshotBase {
	details := make(map[string]*types.Value, 0)
	details[bundle.RelationKeySource.String()] = pbtypes.String(d.URL)
	if len(d.Title) > 0 {
		details[bundle.RelationKeyName.String()] = pbtypes.String(d.Title[0].PlainText)
	}
	if d.Icon != nil && d.Icon.Emoji != nil {
		details[bundle.RelationKeyIconEmoji.String()] = pbtypes.String(*d.Icon.Emoji)
	}
	details[bundle.RelationKeyIsArchived.String()] = pbtypes.Bool(d.Archived)

	ds.handlePageProperties(apiKey, d.ID, d.Properties, details, mode)
	snapshot := &model.SmartBlockSnapshotBase{
		Blocks:      []*model.Block{},
		Details:     &types.Struct{Fields: details},
		ObjectTypes: []string{bundle.TypeKeyPage.URL()},
		Collections: nil,
	}

	return snapshot
}

func (ds *Service) handlePageProperties(apiKey, pageID string, p property.Properties, d map[string]*types.Value, mode pb.RpcObjectImportRequestMode) *converter.ConvertError {
	var ce converter.ConvertError
	for k, v := range p {
		object, err := ds.propertyService.GetPropertyObject(context.TODO(), pageID, v.GetID(), apiKey, v.GetPropertyType())
		if err != nil {
			ce.Add("property: " + v.GetID(), err)
			if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return &ce
			}
		}
		err = ds.propertyService.SetDetailValue(k, v.GetPropertyType(), object, d)
		if err != nil && mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			ce.Add("property: " + v.GetID(), err)
			if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return &ce
			}
		}
	}
	return nil
}
