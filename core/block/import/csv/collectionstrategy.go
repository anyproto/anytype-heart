package csv

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"github.com/google/uuid"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var log = logging.Logger("import-csv")

const (
	defaultRelationName = "Field"
	rowSourceName       = "row"
	transposeSource     = "transpose"
	transposeName       = "Transpose"
)

type CollectionStrategy struct {
	collectionService *collection.Service
}

func NewCollectionStrategy(collectionService *collection.Service) *CollectionStrategy {
	return &CollectionStrategy{collectionService: collectionService}
}

func (c *CollectionStrategy) CreateObjects(path string, csvTable [][]string, params *pb.RpcObjectImportRequestCsvParams, progress process.Progress) (string, []*common.Snapshot, error) {
	snapshots := make([]*common.Snapshot, 0)
	allObjectsIDs := make([]string, 0)
	details := common.GetCommonDetails(path, "", "", model.ObjectType_collection)
	updateDetailsForTransposeCollection(details, params.TransposeRowsAndColumns)
	_, _, st, err := c.collectionService.CreateCollection(details, nil)
	if err != nil {
		return "", nil, err
	}
	relations, relationsSnapshots, errRelationLimit := getDetailsFromCSVTable(csvTable, params.UseFirstRowForRelations)
	objectsSnapshots, errRowLimit := getObjectsFromCSVRows(path, csvTable, relations, params)
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
		return "", nil, common.ErrLimitExceeded
	}
	return snapshot.Id, snapshots, nil
}

func updateDetailsForTransposeCollection(details *types.Struct, transpose bool) {
	if transpose {
		source := pbtypes.GetString(details, bundle.RelationKeySourceFilePath.String())
		source = source + string(filepath.Separator) + transposeSource
		details.Fields[bundle.RelationKeySourceFilePath.String()] = pbtypes.String(source)
		name := pbtypes.GetString(details, bundle.RelationKeyName.String())
		name = name + " " + transposeName
		details.Fields[bundle.RelationKeyName.String()] = pbtypes.String(name)
	}
}

func getDetailsFromCSVTable(csvTable [][]string, useFirstRowForRelations bool) ([]*model.Relation, []*common.Snapshot, error) {
	if len(csvTable) == 0 {
		return nil, nil, nil
	}
	relations := make([]*model.Relation, 0, len(csvTable[0]))
	// first column is always a name
	relations = append(relations, &model.Relation{
		Format: model.RelationFormat_shorttext,
		Key:    bundle.RelationKeyName.String(),
	})
	relationsSnapshots := make([]*common.Snapshot, 0, len(csvTable[0]))
	allRelations := lo.Map(csvTable[0], func(item string, index int) string { return strings.TrimSpace(item) })
	var err error
	numberOfRelationsLimit := len(allRelations)
	if numberOfRelationsLimit > limitForColumns {
		err = common.ErrLimitExceeded
		numberOfRelationsLimit = limitForColumns
	}
	allRelations = findUniqueRelationAndAddNumber(allRelations)
	for i := 1; i < numberOfRelationsLimit; i++ {
		if allRelations[i] == "" && useFirstRowForRelations {
			continue
		}
		relationName := allRelations[i]
		if !useFirstRowForRelations {
			relationName = getDefaultRelationName(i)
		}
		key := bson.NewObjectId().Hex()
		relations = append(relations, &model.Relation{
			Format: model.RelationFormat_longtext,
			Name:   relationName,
			Key:    key,
		})
		details := getRelationDetails(relationName, key, float64(model.RelationFormat_longtext))
		id := pbtypes.GetString(details, bundle.RelationKeyId.String())
		relationsSnapshots = append(relationsSnapshots, &common.Snapshot{
			Id:     id,
			SbType: smartblock.SmartBlockTypeRelation,
			Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
				Details:     details,
				ObjectTypes: []string{bundle.TypeKeyRelation.String()},
				Key:         key,
			}},
		})
	}
	return relations, relationsSnapshots, err
}

func findUniqueRelationAndAddNumber(relations []string) []string {
	countMap := make(map[string]int, 0)
	existedRelationMap := make(map[string]bool, 0)
	relationsName := make([]string, 0)
	for _, r := range relations {
		existedRelationMap[r] = true
	}
	for _, str := range relations {
		if number, ok := countMap[str]; ok || str == "" {
			if !ok && str == "" {
				number = 1
			}
			uniqueName := getUniqueName(str, number, existedRelationMap)
			existedRelationMap[uniqueName] = true
			relationsName = append(relationsName, uniqueName)
			countMap[str]++
			continue
		}
		countMap[str]++
		relationsName = append(relationsName, str)
	}
	return relationsName
}

func getUniqueName(str string, number int, existedRelationMap map[string]bool) string {
	uniqueName := strings.TrimSpace(fmt.Sprintf("%s %d", str, number))
	for {
		if _, ok := existedRelationMap[uniqueName]; ok {
			number++
			uniqueName = strings.TrimSpace(fmt.Sprintf("%s %d", str, number))
			continue
		}
		break
	}
	return uniqueName
}

func getDefaultRelationName(i int) string {
	return defaultRelationName + " " + strconv.FormatInt(int64(i), 10)
}

func getRelationDetails(name, key string, format float64) *types.Struct {
	details := &types.Struct{Fields: map[string]*types.Value{}}
	details.Fields[bundle.RelationKeyRelationFormat.String()] = pbtypes.Float64(format)
	details.Fields[bundle.RelationKeyName.String()] = pbtypes.String(name)
	details.Fields[bundle.RelationKeyRelationKey.String()] = pbtypes.String(key)
	details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_relation))
	uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, key)
	if err != nil {
		log.Warnf("failed to create unique key for Notion relation: %v", err)
		return details
	}
	details.Fields[bundle.RelationKeyId.String()] = pbtypes.String(uniqueKey.Marshal())
	return details
}

func getObjectsFromCSVRows(path string, csvTable [][]string, relations []*model.Relation, params *pb.RpcObjectImportRequestCsvParams) ([]*common.Snapshot, error) {
	snapshots := make([]*common.Snapshot, 0, len(csvTable))
	numberOfObjectsLimit := len(csvTable)
	var err error
	if numberOfObjectsLimit > limitForRows {
		err = common.ErrLimitExceeded
		numberOfObjectsLimit = limitForRows
		if params.UseFirstRowForRelations {
			numberOfObjectsLimit++ // because first row is relations, so we need to add plus 1 row
		}
	}
	for i := 0; i < numberOfObjectsLimit; i++ {
		// skip first row if option is turned on
		if i == 0 && params.UseFirstRowForRelations {
			continue
		}
		st := state.NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{
				Content: &model.BlockContentOfSmartblock{
					Smartblock: &model.BlockContentSmartblock{},
				},
			}),
		}).NewState()
		details, relationLinks := getDetailsForObject(csvTable[i], relations, path, i, params.TransposeRowsAndColumns)
		st.SetDetails(details)
		st.AddRelationLinks(relationLinks...)
		template.InitTemplate(st, template.WithTitle)
		sn := provideObjectSnapshot(st, details)
		snapshots = append(snapshots, sn)
	}
	return snapshots, err
}

func buildSourcePath(path string, i int, transpose bool) string {
	var transposePart string
	if transpose {
		transposePart = string(filepath.Separator) + transposeSource
	}
	return path +
		string(filepath.Separator) +
		rowSourceName +
		string(filepath.Separator) +
		strconv.FormatInt(int64(i), 10) +
		string(filepath.Separator) +
		transposePart
}

func getDetailsForObject(relationsValues []string, relations []*model.Relation, path string, objectOrderIndex int, transpose bool) (*types.Struct, []*model.RelationLink) {
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
	details.Fields[bundle.RelationKeySourceFilePath.String()] = pbtypes.String(buildSourcePath(path, objectOrderIndex, transpose))
	details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_basic))
	return details, relationLinks
}

func provideObjectSnapshot(st *state.State, details *types.Struct) *common.Snapshot {
	snapshot := &common.Snapshot{
		Id:     uuid.New().String(),
		SbType: smartblock.SmartBlockTypePage,
		Snapshot: &pb.ChangeSnapshot{
			Data: &model.SmartBlockSnapshotBase{
				Blocks:        st.Blocks(),
				Details:       details,
				RelationLinks: st.GetRelationLinks(),
				ObjectTypes:   []string{bundle.TypeKeyPage.String()},
			},
		},
	}
	return snapshot
}

func (c *CollectionStrategy) getCollectionSnapshot(details *types.Struct, st *state.State, p string, relations []*model.Relation) *common.Snapshot {
	details = pbtypes.StructMerge(st.CombinedDetails(), details, false)
	details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_collection))

	for _, relation := range relations {
		err := common.AddRelationsToDataView(st, &model.RelationLink{
			Key:    relation.Key,
			Format: relation.Format,
		})
		if err != nil {
			log.Errorf("failed to add relations to dataview, %s", err)
		}
	}
	return c.provideCollectionSnapshots(details, st, p)
}

func (c *CollectionStrategy) provideCollectionSnapshots(details *types.Struct, st *state.State, p string) *common.Snapshot {
	sn := &model.SmartBlockSnapshotBase{
		Blocks:        st.Blocks(),
		Details:       details,
		ObjectTypes:   []string{bundle.TypeKeyCollection.String()},
		Collections:   st.Store(),
		RelationLinks: st.GetRelationLinks(),
	}

	snapshot := &common.Snapshot{
		Id:       uuid.New().String(),
		FileName: p,
		Snapshot: &pb.ChangeSnapshot{Data: sn},
		SbType:   smartblock.SmartBlockTypePage,
	}
	return snapshot
}
