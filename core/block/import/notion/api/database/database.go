package database

import (
	"context"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	simpleDataview "github.com/anytypeio/go-anytype-middleware/core/block/simple/dataview"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/google/uuid"

	"github.com/anytypeio/go-anytype-middleware/core/block/collection"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/page"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/property"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	sb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

const ObjectType = "database"

const rootCollection = "Notion Import"

type Service struct {
	collectionService *collection.Service
}

// New is a constructor for Service
func New(c *collection.Service) *Service {
	return &Service{
		collectionService: c,
	}
}

// Database represent Database object from Notion https://developers.notion.com/reference/database
type Database struct {
	Object         string                      `json:"object"`
	ID             string                      `json:"id"`
	CreatedTime    time.Time                   `json:"created_time"`
	LastEditedTime time.Time                   `json:"last_edited_time"`
	CreatedBy      api.User                    `json:"created_by,omitempty"`
	LastEditedBy   api.User                    `json:"last_edited_by,omitempty"`
	Title          []api.RichText              `json:"title"`
	Parent         api.Parent                  `json:"parent"`
	URL            string                      `json:"url"`
	Properties     property.DatabaseProperties `json:"properties"`
	Description    []*api.RichText             `json:"description"`
	IsInline       bool                        `json:"is_inline"`
	Archived       bool                        `json:"archived"`
	Icon           *api.Icon                   `json:"icon,omitempty"`
	Cover          *api.FileObject             `json:"cover,omitempty"`
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
		convertError       = converter.ConvertError{}
		relations          = make(map[string][]*converter.Relation, 0)
	)

	progress.SetProgressMessage("Start creating pages from notion databases")
	for _, d := range databases {
		if err := progress.TryStep(1); err != nil {
			ce := converter.NewFromError(d.ID, err)
			return nil, nil, nil, ce
		}

		id := uuid.New().String()

		snapshot, rel, err := ds.transformDatabase(d)
		if err != nil && mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, nil, nil, converter.NewFromError(d.ID, err)
		}

		if err != nil {
			continue
		}

		allSnapshots = append(allSnapshots, &converter.Snapshot{
			Id:       id,
			FileName: d.URL,
			Snapshot: snapshot,
			SbType:   sb.SmartBlockTypeCollection,
		})
		notionIdsToAnytype[d.ID] = id
		databaseNameToID[d.ID] = pbtypes.GetString(snapshot.Details, bundle.RelationKeyName.String())
		for key := range d.Properties {
			rel = append(rel, &converter.Relation{
				Relation: &model.Relation{
					Name: key,
				},
			})
		}
		relations[id] = rel
	}
	if convertError.IsEmpty() {
		return &converter.Response{Snapshots: allSnapshots, Relations: relations}, notionIdsToAnytype, databaseNameToID, nil
	}

	return &converter.Response{Snapshots: allSnapshots, Relations: relations}, notionIdsToAnytype, databaseNameToID, convertError
}

func (ds *Service) transformDatabase(d Database) (*model.SmartBlockSnapshotBase, []*converter.Relation, error) {
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
				Relation: &model.Relation{
					Name:   bundle.RelationKeyCoverId.String(),
					Format: model.RelationFormat_file,
				},
			}
		}

		if d.Cover.Type == api.File {
			details[bundle.RelationKeyCoverId.String()] = pbtypes.String(d.Cover.File.URL)
			details[bundle.RelationKeyCoverType.String()] = pbtypes.Float64(1)
			relation = &converter.Relation{
				Relation: &model.Relation{
					Name:   bundle.RelationKeyCoverId.String(),
					Format: model.RelationFormat_file,
				},
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
	details[bundle.RelationKeyIsFavorite.String()] = pbtypes.Bool(false)
	details[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_collection))

	detailsStruct := &types.Struct{Fields: details}
	_, _, st, err := ds.collectionService.CreateCollection(detailsStruct, nil)
	if err != nil {
		return nil, nil, err
	}
	detailsStruct = pbtypes.StructMerge(st.CombinedDetails(), detailsStruct, false)
	snapshot := &model.SmartBlockSnapshotBase{
		Blocks:        st.Blocks(),
		Details:       detailsStruct,
		ObjectTypes:   []string{bundle.TypeKeyCollection.URL()},
		Collections:   st.GetCollection(smartblock.CollectionStoreKey),
		RelationLinks: st.GetRelationLinks(),
	}

	return snapshot, relations, nil
}

func (ds *Service) AddPagesToCollections(databaseSnapshots *converter.Response,
	pages []page.Page,
	databases []Database,
	notionPageIdsToAnytype, notionDatabaseIdsToAnytype map[string]string) {
	snapshots := makeSnapshotMapFromArray(databaseSnapshots.Snapshots)

	databaseToObjects := make(map[string][]string, 0)
	for _, p := range pages {
		if p.Parent.DatabaseID != "" {
			if parentID, ok := notionDatabaseIdsToAnytype[p.Parent.DatabaseID]; ok {
				databaseToObjects[parentID] = append(databaseToObjects[parentID], notionPageIdsToAnytype[p.ID])
			}
		}
	}
	for _, d := range databases {
		if d.Parent.DatabaseID != "" {
			if parentID, ok := notionDatabaseIdsToAnytype[d.Parent.DatabaseID]; ok {
				databaseToObjects[parentID] = append(databaseToObjects[parentID], notionDatabaseIdsToAnytype[d.ID])
			}
		}
	}
	for db, objects := range databaseToObjects {
		ds.addObjectToCollection(snapshots[db], objects)
	}
}

func (ds *Service) AddPagesToRootCollections(databaseSnapshots *converter.Response, pagesSnapshots *converter.Response) error {
	details := make(map[string]*types.Value, 0)
	details[bundle.RelationKeySource.String()] = pbtypes.String(rootCollection)
	details[bundle.RelationKeyName.String()] = pbtypes.String(rootCollection)
	details[bundle.RelationKeyIsFavorite.String()] = pbtypes.Bool(true)
	details[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_collection))

	detailsStruct := &types.Struct{Fields: details}
	_, _, st, err := ds.collectionService.CreateCollection(detailsStruct, nil)
	if err != nil {
		return err
	}

	for _, relation := range []*model.Relation{
		{
			Key:    bundle.RelationKeyTag.String(),
			Format: model.RelationFormat_tag,
		},
		{
			Key:    bundle.RelationKeyCreatedDate.String(),
			Format: model.RelationFormat_date,
		},
	} {
		err = ds.addRelationsToCollectionDataView(st, relation)
		if err != nil {
			return err
		}
	}

	detailsStruct = pbtypes.StructMerge(st.CombinedDetails(), detailsStruct, false)
	rootCol := &converter.Snapshot{
		Id:       uuid.New().String(),
		FileName: rootCollection,
		SbType:   sb.SmartBlockTypeCollection,
		Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
			Blocks:        st.Blocks(),
			Details:       detailsStruct,
			ObjectTypes:   []string{bundle.TypeKeyCollection.URL()},
			RelationLinks: st.GetRelationLinks(),
			Collections:   st.GetCollection(smartblock.CollectionStoreKey),
		},
		},
	}
	allObjects := make([]string, 0, len(databaseSnapshots.Snapshots)+len(pagesSnapshots.Snapshots))

	for _, snapshot := range databaseSnapshots.Snapshots {
		allObjects = append(allObjects, snapshot.Id)
	}

	ds.addObjectToCollection(rootCol, allObjects)

	databaseSnapshots.Snapshots = append(databaseSnapshots.Snapshots, rootCol)

	return nil
}

func (ds *Service) addObjectToCollection(snapshots *converter.Snapshot, targetID []string) {
	snapshots.Snapshot.Data.Collections = &types.Struct{
		Fields: map[string]*types.Value{smartblock.CollectionStoreKey: pbtypes.StringList(targetID)},
	}
}

// MapProperties add properties from pages to related database, because if notion pages have the same properties
// as their database, need this method because database properties doesn't contain information about rollup and formula property format
// so we use pages relations, because they have this information
func (ds *Service) MapProperties(databaseSnapshots *converter.Response,
	relations map[string][]*converter.Relation,
	pages []page.Page,
	databases []Database,
	notionPageIdsToAnytype, notionDatabaseIdsToAnytype map[string]string) {
	for _, d := range databases {
		for _, p := range pages {
			if p.Parent.DatabaseID == d.ID {
				if parentID, ok := notionDatabaseIdsToAnytype[d.ID]; ok {
					anytypeID := notionPageIdsToAnytype[p.ID]
					if databaseSnapshots.Relations == nil {
						databaseSnapshots.Relations = make(map[string][]*converter.Relation, 0)
					}
					databaseSnapshots.Relations[parentID] = relations[anytypeID]
					break
				}
			}
		}
	}
}

func makeSnapshotMapFromArray(snapshots []*converter.Snapshot) map[string]*converter.Snapshot {
	snapshotsMap := make(map[string]*converter.Snapshot, len(snapshots))
	for _, s := range snapshots {
		snapshotsMap[s.Id] = s
	}
	return snapshotsMap
}

func (ds *Service) addRelationsToCollectionDataView(st *state.State, rel *model.Relation) error {
	return st.Iterate(func(bl simple.Block) (isContinue bool) {
		if dv, ok := bl.(simpleDataview.Block); ok {
			if len(bl.Model().GetDataview().GetViews()) == 0 {
				return false
			}
			err := dv.AddViewRelation(bl.Model().GetDataview().GetViews()[0].GetId(), &model.BlockContentDataviewRelation{
				Key:       rel.Key,
				IsVisible: true,
				Width:     192,
			})
			if err != nil {
				return false
			}
			err = dv.AddRelation(&model.RelationLink{
				Key:    rel.Key,
				Format: rel.Format,
			})
			if err != nil {
				return false
			}
		}
		return true
	})
}
