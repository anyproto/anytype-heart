package database

import (
	"context"
	"time"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/block"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
	"github.com/textileio/go-threads/core/thread"
)

const ObjectType = "database"

type Service struct {}

func New() *Service {
	return &Service{}
}

type Database struct {
	Object         string         `json:"object"`
	ID             string         `json:"id"`
	CreatedTime    time.Time      `json:"created_time"`
	LastEditedTime time.Time      `json:"last_edited_time"`
	CreatedBy      api.User       `json:"created_by,omitempty"`
	LastEditedBy   api.User       `json:"last_edited_by,omitempty"`
	Title          []api.RichText `json:"title"`
	Parent         api.Parent     `json:"parent"`
	URL            string         `json:"url"`
	Properties     interface{}    `json:"properties"` // can't support it for databases yet
	Description    []api.RichText `json:"description"`
	IsInline       bool           `json:"is_inline"`
	Archived       bool           `json:"archived"`
	Icon           *api.Icon      `json:"icon,omitempty"`
	Cover          *block.Image   `json:"cover,omitempty"`
}

func (p Database) GetObjectType() string {
	return ObjectType
}

func (ds *Service) GetDatabase(ctx context.Context, mode pb.RpcObjectImportRequestMode, databases []Database) *converter.Response {
	var convereterError converter.ConvertError
	return ds.mapDatabasesToSnaphots(ctx, mode, databases, convereterError)
}

func (ds *Service) mapDatabasesToSnaphots(ctx context.Context, mode pb.RpcObjectImportRequestMode, databases []Database, convereterError converter.ConvertError) *converter.Response {
	var allSnapshots = make([]*converter.Snapshot, 0)
	for _, d := range databases {
		tid, err := threads.ThreadCreateID(thread.AccessControlled, smartblock.SmartBlockTypePage)
		if err != nil {
			convereterError.Add(d.ID, err)
			if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return &converter.Response{Error: convereterError}
			} else {
				continue
			}
		}
		snapshot := ds.transformDatabase(d)
		
		allSnapshots = append(allSnapshots, &converter.Snapshot{
			Id:       tid.String(),
			FileName: d.URL,
			Snapshot: snapshot,
		})
	}
	if convereterError.IsEmpty() {
		return &converter.Response{Snapshots: allSnapshots, Error: nil} 
	}
	return &converter.Response{Snapshots: allSnapshots, Error: convereterError} 
}

func (ds *Service) transformDatabase(d Database) *model.SmartBlockSnapshotBase {
	details := make(map[string]*types.Value, 0)
	details[bundle.RelationKeySource.String()] = pbtypes.String(d.URL)
	if len(d.Title) > 0{
		details[bundle.RelationKeyName.String()] = pbtypes.String(d.Title[0].PlainText)
	}
	if d.Icon != nil && d.Icon.Emoji != nil {
		details[bundle.RelationKeyIconEmoji.String()] = pbtypes.String(*d.Icon.Emoji)
	}
	details[bundle.RelationKeyCreatedDate.String()] = pbtypes.String(d.CreatedTime.String())
	details[bundle.RelationKeyCreator.String()] = pbtypes.String(d.CreatedBy.Name)
	details[bundle.RelationKeyIsArchived.String()] = pbtypes.Bool(d.Archived)
	details[bundle.RelationKeyLastModifiedDate.String()] = pbtypes.String(d.LastEditedTime.String())
	details[bundle.RelationKeyLastModifiedBy.String()] = pbtypes.String(d.LastEditedBy.Name)
	details[bundle.RelationKeyDescription.String()] = pbtypes.String(api.RichTextToDescription(d.Description))
	details[bundle.RelationKeyIsFavorite.String()] = pbtypes.Bool(true) 

	snapshot := &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{},
		Details: &types.Struct{Fields: details},
		ObjectTypes: []string{bundle.TypeKeyPage.URL()},
		Collections: nil,
	}

	return snapshot
}
