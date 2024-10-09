package database

import (
	"context"
	"strings"
	"time"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"github.com/google/uuid"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/files"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/page"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/property"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	sb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const ObjectType = "database"

const rootCollectionName = "Notion Import"

var log = logging.Logger("notion-import-database")

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
func (ds *Service) GetDatabase(
	_ context.Context,
	mode pb.RpcObjectImportRequestMode,
	databases []Database,
	progress process.Progress,
	req *api.NotionImportContext,
	fileDownloader files.Downloader,
) (*common.Response, *property.PropertiesStore, *common.ConvertError) {
	var (
		allSnapshots = make([]*common.Snapshot, 0)
		convertError = common.NewError(mode)
	)
	progress.SetProgressMessage("Start creating pages from notion databases")
	relations := property.NewPropertiesStore()
	for _, d := range databases {
		if err := progress.TryStep(1); err != nil {
			convertError.Add(common.ErrCancel)
			return nil, nil, convertError
		}
		snapshot, err := ds.makeDatabaseSnapshot(d, req, relations, fileDownloader)
		if err != nil {
			convertError.Add(err)
			if convertError.ShouldAbortImport(0, model.Import_Notion) {
				return nil, nil, convertError
			}
			continue
		}
		allSnapshots = append(allSnapshots, snapshot...)
	}
	if convertError.IsEmpty() {
		return &common.Response{Snapshots: allSnapshots}, relations, nil
	}
	return &common.Response{Snapshots: allSnapshots}, relations, convertError
}

func (ds *Service) makeDatabaseSnapshot(
	d Database,
	importContext *api.NotionImportContext,
	relations *property.PropertiesStore,
	fileDownloader files.Downloader,
) ([]*common.Snapshot, error) {
	details, relationLinks := ds.getCollectionDetails(d)
	detailsStruct := &types.Struct{Fields: details}
	_, _, st, err := ds.collectionService.CreateCollection(detailsStruct, nil)
	if err != nil {
		return nil, err
	}
	api.UploadFileRelationLocally(fileDownloader, details, relationLinks)
	detailsStruct = pbtypes.StructMerge(st.CombinedDetails(), detailsStruct, false)
	snapshots := ds.makeRelationsSnapshots(d, st, relations)
	id, databaseSnapshot := ds.provideDatabaseSnapshot(d, st, detailsStruct, relationLinks)
	ds.fillImportContext(d, importContext, id, databaseSnapshot)
	snapshots = append(snapshots, databaseSnapshot)
	return snapshots, nil
}

func (ds *Service) fillImportContext(d Database, req *api.NotionImportContext, id string, databaseSnapshot *common.Snapshot) {
	req.NotionDatabaseIdsToAnytype[d.ID] = id
	req.DatabaseNameToID[d.ID] = pbtypes.GetString(databaseSnapshot.Snapshot.GetData().GetDetails(), bundle.RelationKeyName.String())
	if d.Parent.DatabaseID != "" {
		req.PageTree.ParentPageToChildIDs[d.Parent.DatabaseID] = append(req.PageTree.ParentPageToChildIDs[d.Parent.DatabaseID], d.ID)
	}
	if d.Parent.PageID != "" {
		req.PageTree.ParentPageToChildIDs[d.Parent.PageID] = append(req.PageTree.ParentPageToChildIDs[d.Parent.PageID], d.ID)
	}
	if d.Parent.BlockID != "" {
		req.PageTree.ParentPageToChildIDs[d.Parent.BlockID] = append(req.PageTree.ParentPageToChildIDs[d.Parent.BlockID], d.ID)
	}
}

func (ds *Service) makeRelationsSnapshots(d Database, st *state.State, relations *property.PropertiesStore) []*common.Snapshot {
	snapshots := make([]*common.Snapshot, 0)
	for _, databaseProperty := range d.Properties {
		if _, ok := databaseProperty.(*property.DatabaseTitle); ok {
			ds.handleNameProperty(databaseProperty, st)
		}
	}
	hasTag := isDbContainsTagProperty(d.Properties)
	var tagAlreadyExist bool
	for name, databaseProperty := range d.Properties {
		if _, ok := databaseProperty.(*property.DatabaseTitle); ok {
			continue
		}
		relationKey := bson.NewObjectId().Hex()
		if tagName, tagRelationKey := ds.getNameAndRelationKeyForTagProperty(databaseProperty, hasTag); tagName != "" && tagRelationKey != "" && !tagAlreadyExist {
			name = tagName
			relationKey = tagRelationKey
			tagAlreadyExist = true
		}
		if snapshot := ds.makeRelationSnapshotFromDatabaseProperty(relations, databaseProperty, name, relationKey, st); snapshot != nil {
			snapshots = append(snapshots, snapshot)
		}
	}
	return snapshots
}

func (ds *Service) getNameAndRelationKeyForTagProperty(databaseProperty property.DatabasePropertyHandler, hasTag bool) (string, string) {
	var name, relationKey string
	if tags, ok := databaseProperty.(*property.DatabaseMultiSelect); ok && property.IsPropertyMatchTagRelation(tags.Name, hasTag) {
		name = bundle.RelationKeyTag.String()
		relationKey = bundle.RelationKeyTag.String()
	} else if tags, ok := databaseProperty.(*property.DatabaseSelect); ok && property.IsPropertyMatchTagRelation(tags.Name, hasTag) {
		name = bundle.RelationKeyTag.String()
		relationKey = bundle.RelationKeyTag.String()
	}
	return name, relationKey
}

func (ds *Service) handleNameProperty(databaseProperty property.DatabasePropertyHandler, st *state.State) *common.Snapshot {
	databaseProperty.SetDetail(bundle.RelationKeyName.String(), st.Details().GetFields())
	relationLinks := &model.RelationLink{
		Key:    bundle.RelationKeyName.String(),
		Format: model.RelationFormat_shorttext,
	}
	err := common.ReplaceRelationsInDataView(st, relationLinks)
	if err != nil {
		log.Errorf("failed to add relation to notion database, %s", err)
	}
	return nil
}

func (ds *Service) makeRelationSnapshotFromDatabaseProperty(relations *property.PropertiesStore,
	databaseProperty property.DatabasePropertyHandler,
	name, relationKey string,
	st *state.State) *common.Snapshot {
	rel, sn := ds.provideRelationSnapshot(relations, databaseProperty, name, relationKey)
	relKey := pbtypes.GetString(rel.GetDetails(), bundle.RelationKeyRelationKey.String())
	databaseProperty.SetDetail(relKey, st.Details().GetFields())
	relationLinks := &model.RelationLink{
		Key:    relKey,
		Format: databaseProperty.GetFormat(),
	}
	if relationKey == bundle.RelationKeyTag.String() {
		err := common.ReplaceRelationsInDataView(st, relationLinks)
		if err != nil {
			log.Errorf("failed to make tag relation not hidden in notion database, %s", err)
		}
		return sn
	}
	st.AddRelationLinks(relationLinks)
	err := common.AddRelationsToDataView(st, relationLinks)
	if err != nil {
		log.Errorf("failed to add relation to notion database, %s", err)
	}
	return sn
}

func (ds *Service) provideRelationSnapshot(
	relations *property.PropertiesStore,
	databaseProperty property.DatabasePropertyHandler,
	name, relationKey string,
) (*model.SmartBlockSnapshotBase, *common.Snapshot) {
	var sn *common.Snapshot
	rel := relations.GetSnapshotByNameAndFormat(name, int64(databaseProperty.GetFormat()))
	if rel == nil {
		if rel = relations.ReadRelationsMap(databaseProperty.GetID()); rel == nil {
			rel, sn = ds.getRelationSnapshot(relationKey, databaseProperty, name)
			relations.WriteToRelationsMap(databaseProperty.GetID(), rel)
			relations.AddSnapshotByNameAndFormat(name, int64(databaseProperty.GetFormat()), rel)
		}
	}
	return rel, sn
}

func (ds *Service) getRelationSnapshot(relationKey string, databaseProperty property.DatabasePropertyHandler, name string) (*model.SmartBlockSnapshotBase, *common.Snapshot) {
	relationDetails := ds.getRelationDetails(databaseProperty, name, relationKey)
	relationSnapshot := &model.SmartBlockSnapshotBase{
		Details:     relationDetails,
		ObjectTypes: []string{bundle.TypeKeyRelation.String()},
		Key:         relationKey,
	}
	snapshot := &common.Snapshot{
		Id: pbtypes.GetString(relationDetails, bundle.RelationKeyId.String()),
		Snapshot: &pb.ChangeSnapshot{
			Data: relationSnapshot,
		},
		SbType: sb.SmartBlockTypeRelation,
	}
	return relationSnapshot, snapshot
}

func (ds *Service) getRelationDetails(databaseProperty property.DatabasePropertyHandler, name, key string) *types.Struct {
	if name == "" {
		name = property.UntitledProperty
	}
	details := &types.Struct{Fields: map[string]*types.Value{}}
	details.Fields[bundle.RelationKeyRelationFormat.String()] = pbtypes.Float64(float64(databaseProperty.GetFormat()))
	details.Fields[bundle.RelationKeyName.String()] = pbtypes.String(name)
	details.Fields[bundle.RelationKeyRelationKey.String()] = pbtypes.String(key)
	details.Fields[bundle.RelationKeyCreatedDate.String()] = pbtypes.Int64(time.Now().Unix())
	details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_relation))
	details.Fields[bundle.RelationKeySourceFilePath.String()] = pbtypes.String(databaseProperty.GetID())
	uniqueKey, err := domain.NewUniqueKey(sb.SmartBlockTypeRelation, key)
	if err != nil {
		log.Warnf("failed to create unique key for Notion relation: %v", err)
		return details
	}
	details.Fields[bundle.RelationKeyId.String()] = pbtypes.String(uniqueKey.Marshal())
	return details
}

func (ds *Service) getCollectionDetails(d Database) (map[string]*types.Value, []*model.RelationLink) {
	details := make(map[string]*types.Value, 0)
	details[bundle.RelationKeySourceFilePath.String()] = pbtypes.String(d.ID)
	if len(d.Title) > 0 {
		details[bundle.RelationKeyName.String()] = pbtypes.String(d.Title[0].PlainText)
	}
	var relationLinks []*model.RelationLink
	if d.Cover != nil {
		api.SetCover(details, d.Cover)
		relationLinks = append(relationLinks, &model.RelationLink{
			Key:    bundle.RelationKeyCoverId.String(),
			Format: model.RelationFormat_file,
		})
	}
	if d.Icon != nil {
		relationLink := api.SetIcon(details, d.Icon)
		if relationLink != nil {
			relationLinks = append(relationLinks, relationLink)
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
	return details, relationLinks
}

func (ds *Service) provideDatabaseSnapshot(d Database, st *state.State, detailsStruct *types.Struct, links []*model.RelationLink) (string, *common.Snapshot) {
	snapshot := &model.SmartBlockSnapshotBase{
		Blocks:        st.Blocks(),
		Details:       detailsStruct,
		ObjectTypes:   []string{bundle.TypeKeyCollection.String()},
		Collections:   st.Store(),
		RelationLinks: lo.Union(st.GetRelationLinks(), links),
	}

	id := uuid.New().String()
	databaseSnapshot := &common.Snapshot{
		Id:       id,
		FileName: d.URL,
		Snapshot: &pb.ChangeSnapshot{Data: snapshot},
		SbType:   sb.SmartBlockTypePage,
	}
	return id, databaseSnapshot
}

func (ds *Service) AddPagesToCollections(databaseSnapshots []*common.Snapshot, pages []page.Page, databases []Database, notionPageIdsToAnytype, notionDatabaseIdsToAnytype map[string]string) {
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

func (ds *Service) AddObjectsToNotionCollection(notionContext *api.NotionImportContext,
	notionDB []Database,
	notionPages []page.Page) (*common.Snapshot, error) {
	allObjects := ds.filterObjects(notionContext, notionDB, notionPages)

	rootCollection := common.NewImportCollection(ds.collectionService)
	settings := common.MakeImportCollectionSetting(rootCollectionName, allObjects, "", nil, true, true, true)
	rootCol, err := rootCollection.MakeImportCollection(settings)
	if err != nil {
		return nil, err
	}
	return rootCol, nil
}

func (ds *Service) filterObjects(notionContext *api.NotionImportContext,
	notionDB []Database,
	notionPages []page.Page) []string {
	allWorkspaceObjects := make([]string, 0)
	for _, database := range notionDB {
		if anytypeID := ds.getAnytypeIDForRootCollection(notionContext, notionContext.NotionDatabaseIdsToAnytype, database.Parent, database.ID); anytypeID != "" {
			allWorkspaceObjects = append(allWorkspaceObjects, anytypeID)
		}
	}
	for _, page := range notionPages {
		if anytypeID := ds.getAnytypeIDForRootCollection(notionContext, notionContext.NotionPageIdsToAnytype, page.Parent, page.ID); anytypeID != "" {
			allWorkspaceObjects = append(allWorkspaceObjects, anytypeID)
		}
	}
	return allWorkspaceObjects
}

func (ds *Service) getAnytypeIDForRootCollection(notionContext *api.NotionImportContext,
	notionIDToAnytypeID map[string]string,
	parent api.Parent,
	notionObjectID string) string {
	if parent.Workspace {
		if anytypeID, ok := notionIDToAnytypeID[notionObjectID]; ok {
			return anytypeID
		}
	}

	// if object is in database, but database wasn't added in integration, then we add object in root collection
	if parent.DatabaseID != "" {
		if _, ok := notionContext.NotionDatabaseIdsToAnytype[parent.DatabaseID]; !ok {
			if anytypeID, ok := notionIDToAnytypeID[notionObjectID]; ok {
				return anytypeID
			}
		}
	}

	// if object is a child in Page, but page wasn't added in integration, then we add object in root collection
	if parent.PageID != "" {
		if _, ok := notionContext.NotionPageIdsToAnytype[parent.PageID]; !ok {
			if anytypeID, ok := notionIDToAnytypeID[notionObjectID]; ok {
				return anytypeID
			}
		}
	}

	// If page with parent block is absent, we add child page to root collection
	if parent.BlockID != "" {
		if _, ok := notionContext.BlockToPage.ParentBlockToPage[parent.BlockID]; !ok {
			if anytypeID, ok := notionIDToAnytypeID[notionObjectID]; ok {
				return anytypeID
			}
		}
	}
	return ""
}

func isDbContainsTagProperty(databaseProperties property.DatabaseProperties) bool {
	for key, databaseProperty := range databaseProperties {
		if _, ok := databaseProperty.(*property.DatabaseMultiSelect); ok {
			if strings.TrimSpace(key) == property.TagNameProperty {
				return true
			}
		}
		if _, ok := databaseProperty.(*property.DatabaseSelect); ok {
			if strings.TrimSpace(key) == property.TagNameProperty {
				return true
			}
		}
	}
	return false
}

func makeSnapshotMapFromArray(snapshots []*common.Snapshot) map[string]*common.Snapshot {
	snapshotsMap := make(map[string]*common.Snapshot, len(snapshots))
	for _, s := range snapshots {
		snapshotsMap[s.Id] = s
	}
	return snapshotsMap
}

func addObjectToSnapshot(snapshots *common.Snapshot, targetID []string) {
	snapshots.Snapshot.Data.Collections = &types.Struct{
		Fields: map[string]*types.Value{template.CollectionStoreKey: pbtypes.StringList(targetID)},
	}
}
