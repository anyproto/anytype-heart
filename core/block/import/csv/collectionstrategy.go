package csv

import (
	"strconv"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"github.com/google/uuid"

	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var logger = logging.Logger("import-csv")

const defaultRelationName = "Field"

type CollectionStrategy struct {
	collectionService *collection.Service
}

func NewCollectionStrategy(collectionService *collection.Service) *CollectionStrategy {
	return &CollectionStrategy{collectionService: collectionService}
}

func (c *CollectionStrategy) CreateObjects(path string, csvTable [][]string, useFirstRowForRelations bool, progress process.Progress) (string, []*converter.Snapshot, error) {
	snapshots := make([]*converter.Snapshot, 0)
	allObjectsIDs := make([]string, 0)
	details := converter.GetCommonDetails(path, "", "")
	details.GetFields()[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_collection))
	_, _, st, err := c.collectionService.CreateCollection(details, nil)
	if err != nil {
		return "", nil, err
	}

	relations, relationsSnapshots := getDetailsFromCSVTable(csvTable, useFirstRowForRelations)
	objectsSnapshots := getObjectsFromCSVRows(csvTable, relations, useFirstRowForRelations)
	targetIDs := make([]string, 0, len(objectsSnapshots))
	for _, objectsSnapshot := range objectsSnapshots {
		targetIDs = append(targetIDs, objectsSnapshot.Id)
	}
	allObjectsIDs = append(allObjectsIDs, targetIDs...)

	st.UpdateStoreSlice(template.CollectionStoreKey, targetIDs)
	snapshot := c.getCollectionSnapshot(details, st, path, relations)

	snapshots = append(snapshots, snapshot)
	snapshots = append(snapshots, objectsSnapshots...)
	snapshots = append(snapshots, relationsSnapshots...)
	progress.AddDone(1)
	return snapshot.Id, snapshots, nil
}

func getDetailsFromCSVTable(csvTable [][]string, useFirstRowForRelations bool) ([]*model.Relation, []*converter.Snapshot) {
	if len(csvTable) == 0 {
		return nil, nil
	}
	relations := make([]*model.Relation, 0, len(csvTable[0]))
	// first column is always a name
	relations = append(relations, &model.Relation{
		Format: model.RelationFormat_shorttext,
		Key:    bundle.RelationKeyName.String(),
	})
	relationsSnapshots := make([]*converter.Snapshot, 0, len(csvTable[0]))
	allRelations := csvTable[0]
	numberOfRelationsLimit := len(allRelations)
	if numberOfRelationsLimit > limitForColumns {
		numberOfRelationsLimit = limitForColumns
	}
	for i := 1; i < numberOfRelationsLimit; i++ {
		if allRelations[i] == "" && useFirstRowForRelations {
			continue
		}
		relationName := allRelations[i]
		if !useFirstRowForRelations {
			relationName = getDefaultRelationName(i)
		}
		id := bson.NewObjectId().Hex()
		relations = append(relations, &model.Relation{
			Format: model.RelationFormat_longtext,
			Name:   relationName,
			Key:    id,
		})
		relationsSnapshots = append(relationsSnapshots, &converter.Snapshot{
			Id:     addr.RelationKeyToIdPrefix + id,
			SbType: smartblock.SmartBlockTypeSubObject,
			Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
				Details:     getRelationDetails(relationName, id, float64(model.RelationFormat_longtext)),
				ObjectTypes: []string{bundle.TypeKeyRelation.URL()},
			}},
		})
	}
	return relations, relationsSnapshots
}

func getDefaultRelationName(i int) string {
	return defaultRelationName + " " + strconv.FormatInt(int64(i), 10)
}

func getRelationDetails(name, key string, format float64) *types.Struct {
	details := &types.Struct{Fields: map[string]*types.Value{}}
	details.Fields[bundle.RelationKeyRelationFormat.String()] = pbtypes.Float64(format)
	details.Fields[bundle.RelationKeyName.String()] = pbtypes.String(name)
	details.Fields[bundle.RelationKeyId.String()] = pbtypes.String(addr.RelationKeyToIdPrefix + key)
	details.Fields[bundle.RelationKeyRelationKey.String()] = pbtypes.String(key)
	details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_relation))
	return details
}

func getObjectsFromCSVRows(csvTable [][]string, relations []*model.Relation, useFirstRowForRelations bool) []*converter.Snapshot {
	snapshots := make([]*converter.Snapshot, 0, len(csvTable))
	numberOfObjectsLimit := len(csvTable)
	if numberOfObjectsLimit >= limitForRows {
		numberOfObjectsLimit = limitForRows
		if useFirstRowForRelations {
			numberOfObjectsLimit++ // because first row is relations, so we need to add plus 1 row
		}
	}
	for i := 0; i < numberOfObjectsLimit; i++ {
		// skip first row if option is turned on
		if i == 0 && useFirstRowForRelations {
			continue
		}
		st := state.NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{
				Content: &model.BlockContentOfSmartblock{
					Smartblock: &model.BlockContentSmartblock{},
				},
			}),
		}).NewState()
		details, relationLinks := getDetailsForObject(csvTable[i], relations)
		st.SetDetails(details)
		st.AddRelationLinks(relationLinks...)
		template.InitTemplate(st, template.WithTitle)
		sn := provideObjectSnapshot(st, details)
		snapshots = append(snapshots, sn)
	}
	return snapshots
}

func getDetailsForObject(relationsValues []string, relations []*model.Relation) (*types.Struct, []*model.RelationLink) {
	details := &types.Struct{Fields: map[string]*types.Value{}}
	relationLinks := make([]*model.RelationLink, 0)
	for j, value := range relationsValues {
		if len(relations) <= j {
			break
		}
		relation := relations[j]
		details.Fields[relation.Key] = pbtypes.String(value)
		relationLinks = append(relationLinks, &model.RelationLink{
			Key:    relation.Key,
			Format: relation.Format,
		})
	}
	return details, relationLinks
}

func provideObjectSnapshot(st *state.State, details *types.Struct) *converter.Snapshot {
	snapshot := &converter.Snapshot{
		Id:     uuid.New().String(),
		SbType: smartblock.SmartBlockTypePage,
		Snapshot: &pb.ChangeSnapshot{
			Data: &model.SmartBlockSnapshotBase{
				Blocks:        st.Blocks(),
				Details:       details,
				RelationLinks: st.GetRelationLinks(),
				ObjectTypes:   []string{bundle.TypeKeyPage.URL()},
			},
		},
	}
	return snapshot
}

func (c *CollectionStrategy) getCollectionSnapshot(details *types.Struct, st *state.State, p string, relations []*model.Relation) *converter.Snapshot {
	details = pbtypes.StructMerge(st.CombinedDetails(), details, false)
	details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_collection))

	for _, relation := range relations {
		err := converter.AddRelationsToDataView(st, &model.RelationLink{
			Key:    relation.Key,
			Format: relation.Format,
		})
		if err != nil {
			logger.Errorf("failed to add relations to dataview, %s", err.Error())
		}
	}
	return c.provideCollectionSnapshots(details, st, p)
}

func (c *CollectionStrategy) provideCollectionSnapshots(details *types.Struct, st *state.State, p string) *converter.Snapshot {
	sn := &model.SmartBlockSnapshotBase{
		Blocks:        st.Blocks(),
		Details:       details,
		ObjectTypes:   []string{bundle.TypeKeyCollection.URL()},
		Collections:   st.Store(),
		RelationLinks: st.GetRelationLinks(),
	}

	snapshot := &converter.Snapshot{
		Id:       uuid.New().String(),
		FileName: p,
		Snapshot: &pb.ChangeSnapshot{Data: sn},
		SbType:   smartblock.SmartBlockTypePage,
	}
	return snapshot
}
