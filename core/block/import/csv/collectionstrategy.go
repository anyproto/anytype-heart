package csv

import (
	"path/filepath"
	"strconv"
	"strings"

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

const (
	columnPath          = "column"
	defaultRelationName = "Field"
)

type CollectionStrategy struct {
	collectionService *collection.Service
}

func NewCollectionStrategy(collectionService *collection.Service) *CollectionStrategy {
	return &CollectionStrategy{collectionService: collectionService}
}

func (c *CollectionStrategy) CreateObjects(path string, fileName string, csvTable [][]string, useFirstRowForRelations bool, progress process.Progress) (string, []*converter.Snapshot, error) {
	snapshots := make([]*converter.Snapshot, 0)
	allObjectsIDs := make([]string, 0)
	name := strings.TrimSuffix(filepath.Base(fileName), filepath.Ext(fileName))
	details := converter.GetCommonDetails(name, "", converter.GetSourceDetail(fileName))
	details.GetFields()[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_collection))
	_, _, st, err := c.collectionService.CreateCollection(details, nil)
	if err != nil {
		return "", nil, err
	}
	relations, relationsSnapshots, errRelationLimit := getDetailsFromCSVTable(csvTable, fileName, useFirstRowForRelations)
	objectsSnapshots, errRowLimit := getObjectsFromCSVRows(csvTable, relations, useFirstRowForRelations)
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
	if errRelationLimit != nil || errRowLimit != nil {
		return snapshot.Id, snapshots, converter.ErrLimitExceeded
	}
	return snapshot.Id, snapshots, nil
}

func getDetailsFromCSVTable(csvTable [][]string, fileName string, useFirstRowForRelations bool) ([]*model.Relation, []*converter.Snapshot, error) {
	if len(csvTable) == 0 {
		return nil, nil, nil
	}
	relations := make([]*model.Relation, 0, len(csvTable[0]))
	// first column is always a name
	relations = append(relations, &model.Relation{
		Format: model.RelationFormat_shorttext,
		Key:    bundle.RelationKeyName.String(),
	})
	relationsSnapshots := make([]*converter.Snapshot, 0, len(csvTable[0]))
	allRelations := csvTable[0]
	var err error
	numberOfRelationsLimit := len(allRelations)
	if numberOfRelationsLimit > limitForColumns {
		err = converter.ErrLimitExceeded
		numberOfRelationsLimit = limitForColumns
	}
	relationsSourceMap := make(map[string]bool, 0)
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
		source := getRelationSource(relationsSourceMap, fileName, relationName)
		relationsSnapshots = append(relationsSnapshots, &converter.Snapshot{
			Id:     addr.RelationKeyToIdPrefix + id,
			SbType: smartblock.SmartBlockTypeSubObject,
			Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
				Details:     getRelationDetails(relationName, id, source),
				ObjectTypes: []string{bundle.TypeKeyRelation.URL()},
			}},
		})
	}
	return relations, relationsSnapshots, err
}

func getDefaultRelationName(i int) string {
	return defaultRelationName + " " + strconv.FormatInt(int64(i), 10)
}

func getRelationDetails(name, id, source string) *types.Struct {
	details := &types.Struct{Fields: map[string]*types.Value{}}
	details.Fields[bundle.RelationKeyRelationFormat.String()] = pbtypes.Float64(float64(model.RelationFormat_longtext))
	details.Fields[bundle.RelationKeyName.String()] = pbtypes.String(name)
	details.Fields[bundle.RelationKeyId.String()] = pbtypes.String(addr.RelationKeyToIdPrefix + id)
	details.Fields[bundle.RelationKeyRelationKey.String()] = pbtypes.String(id)
	details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_relation))
	details.Fields[bundle.RelationKeySourceFilePath.String()] = pbtypes.String(source)
	return details
}

func getObjectsFromCSVRows(csvTable [][]string, relations []*model.Relation, useFirstRowForRelations bool) ([]*converter.Snapshot, error) {
	snapshots := make([]*converter.Snapshot, 0, len(csvTable))
	numberOfObjectsLimit := len(csvTable)
	var err error
	if numberOfObjectsLimit > limitForRows {
		err = converter.ErrLimitExceeded
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
	return snapshots, err
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

// getRelationSource return unique source for relations from columns.
// Need it, if csv file has identical columns, to prevent duplicated relations in case of repeated import
func getRelationSource(sourceMap map[string]bool, fileName string, relation string) string {
	source := buildSource(fileName, relation)
	if _, ok := sourceMap[source]; ok {
		var i int64
		for {
			source = buildSource(fileName, relation) + strconv.FormatInt(i, 10)
			if _, ok = sourceMap[source]; ok {
				i++
				continue
			}
			break
		}
	}
	sourceMap[source] = true
	return source
}

func buildSource(fileName string, relation string) string {
	return fileName + string(filepath.Separator) + columnPath + string(filepath.Separator) + relation
}
