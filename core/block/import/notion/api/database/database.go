package database

import (
	"context"
	"time"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"github.com/google/uuid"

	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/block"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/page"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/property"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	sb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const ObjectType = "database"

const rootCollectionName = "Notion Import"

var logger = logging.Logger("notion-import-database")

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

// GetDatabase makes snapshots from notion Database objects
func (ds *Service) GetDatabase(ctx context.Context, mode pb.RpcObjectImportRequestMode, databases []Database, progress process.Progress) (
	*converter.Response, *block.MapRequest, *property.PropertiesStore, converter.ConvertError) {
	var (
		allSnapshots       = make([]*converter.Snapshot, 0)
		notionIdsToAnytype = make(map[string]string, 0)
		databaseNameToID   = make(map[string]string, 0)
		convertError       = converter.ConvertError{}
	)
	progress.SetProgressMessage("Start creating pages from notion databases")
	relations := &property.PropertiesStore{
		PropertyIdsToSnapshots: make(map[string]*model.SmartBlockSnapshotBase, 0),
		RelationsIdsToOptions:  make(map[string][]*model.SmartBlockSnapshotBase, 0),
	}
	for _, d := range databases {
		if err := progress.TryStep(1); err != nil {
			return nil, nil, nil, converter.NewCancelError(d.ID, err)
		}
		snapshot, err := ds.makeDatabaseSnapshot(d, notionIdsToAnytype, databaseNameToID, relations)
		if err != nil && mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, nil, nil, converter.NewFromError(d.ID, err)
		}
		if err != nil {
			continue
		}
		allSnapshots = append(allSnapshots, snapshot...)
	}
	mapRequest := &block.MapRequest{
		NotionDatabaseIdsToAnytype: notionIdsToAnytype,
		DatabaseNameToID:           databaseNameToID,
	}
	if convertError.IsEmpty() {
		return &converter.Response{Snapshots: allSnapshots}, mapRequest, relations, nil
	}
	return &converter.Response{Snapshots: allSnapshots}, mapRequest, relations, convertError
}

func (ds *Service) makeDatabaseSnapshot(d Database, notionIdsToAnytype, databaseNameToID map[string]string, relations *property.PropertiesStore) ([]*converter.Snapshot, error) {
	details := ds.getCollectionDetails(d)

	detailsStruct := &types.Struct{Fields: details}
	_, _, st, err := ds.collectionService.CreateCollection(detailsStruct, nil)
	if err != nil {
		return nil, err
	}
	detailsStruct = pbtypes.StructMerge(st.CombinedDetails(), detailsStruct, false)
	snapshots := make([]*converter.Snapshot, 0)
	for _, databaseProperty := range d.Properties {
		if _, ok := databaseProperty.(*property.DatabaseTitle); ok {
			ds.handleNameProperty(databaseProperty, st)
		}
	}
	for key, databaseProperty := range d.Properties {
		if snapshot := ds.makeRelationSnapshotFromDatabaseProperty(relations, databaseProperty, key, st); snapshot != nil {
			snapshots = append(snapshots, snapshot)
		}
	}
	id, converterSnapshot := ds.provideSnapshot(d, st, detailsStruct)
	notionIdsToAnytype[d.ID] = id
	databaseNameToID[d.ID] = pbtypes.GetString(converterSnapshot.Snapshot.GetData().GetDetails(), bundle.RelationKeyName.String())
	snapshots = append(snapshots, converterSnapshot)
	return snapshots, nil
}

func (ds *Service) handleNameProperty(databaseProperty property.DatabasePropertyHandler, st *state.State) *converter.Snapshot {
	databaseProperty.SetDetail(bundle.RelationKeyName.String(), st.Details().GetFields())
	relationLinks := &model.RelationLink{
		Key:    bundle.RelationKeyName.String(),
		Format: model.RelationFormat_shorttext,
	}
	st.AddRelationLinks(relationLinks)
	err := converter.AddRelationsToDataView(st, relationLinks)
	if err != nil {
		logger.Errorf("failed to add relation to notion database, %s", err.Error())
	}
	return nil
}

func (ds *Service) makeRelationSnapshotFromDatabaseProperty(relations *property.PropertiesStore, databaseProperty property.DatabasePropertyHandler, key string, st *state.State) *converter.Snapshot {
	var (
		rel *model.SmartBlockSnapshotBase
		sn  *converter.Snapshot
	)
	if rel = relations.ReadRelationsMap(databaseProperty.GetID()); rel == nil {
		rel, sn = ds.getRelationSnapshot(databaseProperty, key)
		relations.WriteToRelationsMap(databaseProperty.GetID(), rel)
	}
	relKey := pbtypes.GetString(rel.GetDetails(), bundle.RelationKeyRelationKey.String())
	databaseProperty.SetDetail(relKey, st.Details().GetFields())
	relationLinks := &model.RelationLink{
		Key:    relKey,
		Format: databaseProperty.GetFormat(),
	}
	st.AddRelationLinks(relationLinks)
	err := converter.AddRelationsToDataView(st, relationLinks)
	if err != nil {
		logger.Errorf("failed to add relation to notion database, %s", err.Error())
	}
	return sn
}

func (ds *Service) getRelationSnapshot(databaseProperty property.DatabasePropertyHandler, name string) (*model.SmartBlockSnapshotBase, *converter.Snapshot) {
	relationKey := bson.NewObjectId().Hex()
	relationDetails := ds.getRelationDetails(databaseProperty, name, relationKey)
	relationSnapshot := &model.SmartBlockSnapshotBase{
		Details:     relationDetails,
		ObjectTypes: []string{bundle.TypeKeyRelation.URL()},
	}
	snapshot := &converter.Snapshot{
		Id:       addr.RelationKeyToIdPrefix + relationKey,
		Snapshot: &pb.ChangeSnapshot{Data: relationSnapshot},
		SbType:   sb.SmartBlockTypeSubObject,
	}
	return relationSnapshot, snapshot
}

func (ds *Service) getRelationDetails(databaseProperty property.DatabasePropertyHandler, name, key string) *types.Struct {
	details := &types.Struct{Fields: map[string]*types.Value{}}
	details.Fields[bundle.RelationKeyRelationFormat.String()] = pbtypes.Float64(float64(databaseProperty.GetFormat()))
	details.Fields[bundle.RelationKeyName.String()] = pbtypes.String(name)
	details.Fields[bundle.RelationKeyId.String()] = pbtypes.String(addr.RelationKeyToIdPrefix + key)
	details.Fields[bundle.RelationKeyRelationKey.String()] = pbtypes.String(key)
	details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_relation))
	details.Fields[bundle.RelationKeySourceFilePath.String()] = pbtypes.String(databaseProperty.GetID())
	return details
}

func (ds *Service) getCollectionDetails(d Database) map[string]*types.Value {
	details := make(map[string]*types.Value, 0)
	details[bundle.RelationKeySourceFilePath.String()] = pbtypes.String(d.URL)
	if len(d.Title) > 0 {
		details[bundle.RelationKeyName.String()] = pbtypes.String(d.Title[0].PlainText)
	}
	if d.Icon != nil && d.Icon.Emoji != nil {
		details[bundle.RelationKeyIconEmoji.String()] = pbtypes.String(*d.Icon.Emoji)
	}

	if d.Cover != nil {

		if d.Cover.Type == api.External {
			details[bundle.RelationKeyCoverId.String()] = pbtypes.String(d.Cover.External.URL)
			details[bundle.RelationKeyCoverType.String()] = pbtypes.Float64(1)
		}

		if d.Cover.Type == api.File {
			details[bundle.RelationKeyCoverId.String()] = pbtypes.String(d.Cover.File.URL)
			details[bundle.RelationKeyCoverType.String()] = pbtypes.Float64(1)
		}
	}
	details[bundle.RelationKeyCreator.String()] = pbtypes.String(d.CreatedBy.Name)
	details[bundle.RelationKeyIsArchived.String()] = pbtypes.Bool(d.Archived)
	details[bundle.RelationKeyLastModifiedBy.String()] = pbtypes.String(d.LastEditedBy.Name)
	details[bundle.RelationKeyDescription.String()] = pbtypes.String(api.RichTextToDescription(d.Description))
	details[bundle.RelationKeyIsFavorite.String()] = pbtypes.Bool(false)
	details[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_collection))

	details[bundle.RelationKeyLastModifiedDate.String()] = pbtypes.Float64(float64(d.LastEditedTime.Unix()))
	details[bundle.RelationKeyCreatedDate.String()] = pbtypes.Float64(float64(d.CreatedTime.Unix()))
	return details
}

func (ds *Service) provideSnapshot(d Database, st *state.State, detailsStruct *types.Struct) (string, *converter.Snapshot) {
	snapshot := &model.SmartBlockSnapshotBase{
		Blocks:        st.Blocks(),
		Details:       detailsStruct,
		ObjectTypes:   []string{bundle.TypeKeyCollection.URL()},
		Collections:   st.Store(),
		RelationLinks: st.GetRelationLinks(),
	}

	id := uuid.New().String()
	converterSnapshot := &converter.Snapshot{
		Id:       id,
		FileName: d.URL,
		Snapshot: &pb.ChangeSnapshot{Data: snapshot},
		SbType:   sb.SmartBlockTypePage,
	}
	return id, converterSnapshot
}

func (ds *Service) AddPagesToCollections(databaseSnapshots []*converter.Snapshot, pages []page.Page, databases []Database, notionPageIdsToAnytype, notionDatabaseIdsToAnytype map[string]string) {
	snapshots := makeSnapshotMapFromArray(databaseSnapshots)

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
		addObjectToSnapshot(snapshots[db], objects)
	}
}

func (ds *Service) AddObjectsToNotionCollection(databaseSnapshots []*converter.Snapshot, pagesSnapshots []*converter.Snapshot) ([]*converter.Snapshot, error) {
	allObjects := make([]string, 0, len(databaseSnapshots)+len(pagesSnapshots))

	for _, snapshot := range databaseSnapshots {
		if snapshot.SbType == sb.SmartBlockTypeSubObject {
			continue
		}
		allObjects = append(allObjects, snapshot.Id)
	}

	for _, snapshot := range pagesSnapshots {
		if snapshot.SbType == sb.SmartBlockTypeSubObject {
			continue
		}
		allObjects = append(allObjects, snapshot.Id)
	}

	rootCollection := converter.NewRootCollection(ds.collectionService)
	rootCol, err := rootCollection.AddObjects(rootCollectionName, allObjects)
	if err != nil {
		return nil, err
	}

	databaseSnapshots = append(databaseSnapshots, rootCol)

	return databaseSnapshots, nil
}

func makeSnapshotMapFromArray(snapshots []*converter.Snapshot) map[string]*converter.Snapshot {
	snapshotsMap := make(map[string]*converter.Snapshot, len(snapshots))
	for _, s := range snapshots {
		snapshotsMap[s.Id] = s
	}
	return snapshotsMap
}

func addObjectToSnapshot(snapshots *converter.Snapshot, targetID []string) {
	snapshots.Snapshot.Data.Collections = &types.Struct{
		Fields: map[string]*types.Value{template.CollectionStoreKey: pbtypes.StringList(targetID)},
	}
}
