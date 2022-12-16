package database

import (
	"context"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/textileio/go-threads/core/thread"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

const ObjectType = "database"

type Service struct{}

// New is a constructor for Service
func New() *Service {
	return &Service{}
}

// Database represent Database object from Notion https://developers.notion.com/reference/database
type Database struct {
	Object         string          `json:"object"`
	ID             string          `json:"id"`
	CreatedTime    time.Time       `json:"created_time"`
	LastEditedTime time.Time       `json:"last_edited_time"`
	CreatedBy      api.User        `json:"created_by,omitempty"`
	LastEditedBy   api.User        `json:"last_edited_by,omitempty"`
	Title          []api.RichText  `json:"title"`
	Parent         api.Parent      `json:"parent"`
	URL            string          `json:"url"`
	Properties     interface{}     `json:"properties"` // can't support it for databases yet
	Description    []*api.RichText `json:"description"`
	IsInline       bool            `json:"is_inline"`
	Archived       bool            `json:"archived"`
	Icon           *api.Icon       `json:"icon,omitempty"`
	Cover          *api.FileObject `json:"cover,omitempty"`
}

func (p *Database) GetObjectType() string {
	return ObjectType
}

// GetDatabase makes snaphots from notion Database objects
func (ds *Service) GetDatabase(ctx context.Context,
	mode pb.RpcObjectImportRequestMode,
	databases []Database,
	progress *process.Progress) (*converter.Response, map[string]string, map[string]string, converter.ConvertError) {
	var (
		allSnapshots       = make([]*converter.Snapshot, 0)
		notionIdsToAnytype = make(map[string]string, 0)
		databaseNameToID   = make(map[string]string, 0)
		convereterError    = converter.ConvertError{}
	)

	progress.SetProgressMessage("Start creating pages from notion databases")
	for _, d := range databases {
		if err := progress.TryStep(1); err != nil {
			ce := converter.NewFromError(d.ID, err)
			return nil, nil, nil, ce
		}

		tid, err := threads.ThreadCreateID(thread.AccessControlled, smartblock.SmartBlockTypePage)
		if err != nil {
			convereterError.Add(d.ID, err)
			if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return nil, nil, nil, convereterError
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
		notionIdsToAnytype[d.ID] = tid.String()
		databaseNameToID[d.ID] = pbtypes.GetString(snapshot.Details, bundle.RelationKeyName.String())
	}
	if convereterError.IsEmpty() {
		return &converter.Response{Snapshots: allSnapshots}, notionIdsToAnytype, databaseNameToID, nil
	}

	return &converter.Response{Snapshots: allSnapshots}, notionIdsToAnytype, databaseNameToID, convereterError
}

func (ds *Service) transformDatabase(d Database) *model.SmartBlockSnapshotBase {
	details := make(map[string]*types.Value, 0)
	relations := make([]*converter.Relation, 0)
	details[bundle.RelationKeySource.String()] = pbtypes.String(d.URL)
	if len(d.Title) > 0 {
		details[bundle.RelationKeyName.String()] = pbtypes.String(d.Title[0].PlainText)
	}
	if d.Icon != nil && d.Icon.Emoji != nil {
		details[bundle.RelationKeyIconEmoji.String()] = pbtypes.String(*d.Icon.Emoji)
	}

	if d.Cover != nil {
		var relation *converter.Relation

		if d.Cover.Type == api.External {
			details[bundle.RelationKeyCoverId.String()] = pbtypes.String(d.Cover.External.URL)
			details[bundle.RelationKeyCoverType.String()] = pbtypes.Float64(1)
			relation = &converter.Relation{
				Name:   bundle.RelationKeyCoverId.String(),
				Format: model.RelationFormat_file,
			}
		}

		if d.Cover.Type == api.File {
			details[bundle.RelationKeyCoverId.String()] = pbtypes.String(d.Cover.File.URL)
			details[bundle.RelationKeyCoverType.String()] = pbtypes.Float64(1)
			relation = &converter.Relation{
				Name:   bundle.RelationKeyCoverId.String(),
				Format: model.RelationFormat_file,
			}
		}

		relations = append(relations, relation)
	}
	details[bundle.RelationKeyCreatedDate.String()] = pbtypes.String(d.CreatedTime.String())
	details[bundle.RelationKeyCreator.String()] = pbtypes.String(d.CreatedBy.Name)
	details[bundle.RelationKeyIsArchived.String()] = pbtypes.Bool(d.Archived)
	details[bundle.RelationKeyLastModifiedDate.String()] = pbtypes.String(d.LastEditedTime.String())
	details[bundle.RelationKeyLastModifiedBy.String()] = pbtypes.String(d.LastEditedBy.Name)
	details[bundle.RelationKeyDescription.String()] = pbtypes.String(api.RichTextToDescription(d.Description))
	details[bundle.RelationKeyIsFavorite.String()] = pbtypes.Bool(true)

	snapshot := &model.SmartBlockSnapshotBase{
		Blocks:      []*model.Block{},
		Details:     &types.Struct{Fields: details},
		ObjectTypes: []string{bundle.TypeKeyPage.URL()},
		Collections: nil,
	}

	return snapshot
}
