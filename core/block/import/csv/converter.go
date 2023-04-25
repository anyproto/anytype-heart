package csv

import (
	"encoding/csv"
	"github.com/anytypeio/go-anytype-middleware/core/block/collection"
	sb "github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

const numberOfStages = 2 // 1 cycle to get snapshots and 1 cycle to create objects
const Name = "Csv"

type CSV struct {
	collectionService *collection.Service
}

func New(collectionService *collection.Service) converter.Converter {
	return &CSV{collectionService: collectionService}
}

func (c *CSV) Name() string {
	return Name
}

func (c *CSV) GetParams(req *pb.RpcObjectImportRequest) []string {
	if p := req.GetCsvParams(); p != nil {
		return p.Path
	}

	return nil
}

func (c *CSV) GetSnapshots(req *pb.RpcObjectImportRequest,
	progress *process.Progress) (*converter.Response, converter.ConvertError) {
	path := c.GetParams(req)
	if len(path) == 0 {
		return nil, nil
	}
	progress.SetTotal(int64(numberOfStages * len(path)))
	progress.SetProgressMessage("Start creating snapshots from files")
	snapshots := make([]*converter.Snapshot, 0)
	allRelations := make(map[string][]*converter.Relation, 0)
	cErr := converter.NewError()
	for _, p := range path {
		if err := progress.TryStep(1); err != nil {
			cancelError := converter.NewFromError(p, err)
			return nil, cancelError
		}
		if filepath.Ext(p) != ".csv" {
			continue
		}
		csvTable, err := readCsvFile(p)
		if err != nil {
			cErr.Add(p, err)
			if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return nil, cErr
			}
			continue
		}
		details := converter.GetDetails(p)
		details.GetFields()[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_collection))
		_, _, st, err := c.collectionService.CreateCollection(details, nil)
		relations := getDetailsFromCSVTable(csvTable)
		details = pbtypes.StructMerge(st.CombinedDetails(), details, false)
		objectsSnapshots, objectsRelations := getEmptyObjects(csvTable, relations)

		targetIDs := make([]string, 0, len(objectsSnapshots))
		for _, objectsSnapshot := range objectsSnapshots {
			targetIDs = append(targetIDs, objectsSnapshot.Id)
		}

		st.StoreSlice(sb.CollectionStoreKey, targetIDs)
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
			SbType:   smartblock.SmartBlockTypeCollection,
		}

		snapshots = append(snapshots, snapshot)
		snapshots = append(snapshots, objectsSnapshots...)
		allRelations[snapshot.Id] = relations

		allRelations = makeRelationsResultMap(allRelations, objectsRelations)
	}

	if cErr.IsEmpty() {
		return &converter.Response{
			Snapshots: snapshots,
			Relations: allRelations,
		}, nil
	}

	return &converter.Response{
		Snapshots: snapshots,
		Relations: allRelations,
	}, cErr
}

func readCsvFile(filePath string) ([][]string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	csvReader := csv.NewReader(f)
	records, err := csvReader.ReadAll()
	if err != nil {
		return nil, err
	}

	return records, nil
}

func getEmptyObjects(csvTable [][]string, relations []*converter.Relation) ([]*converter.Snapshot, map[string][]*converter.Relation) {
	snapshots := make([]*converter.Snapshot, 0, len(csvTable))
	objectsRelations := make(map[string][]*converter.Relation, len(csvTable))

	for i := 1; i < len(csvTable); i++ {
		details := &types.Struct{Fields: map[string]*types.Value{}}
		for j, value := range csvTable[i] {
			details.Fields[relations[j].Name] = pbtypes.String(value)
		}
		sn := &converter.Snapshot{
			Id:     uuid.New().String(),
			SbType: smartblock.SmartBlockTypePage,
			Snapshot: &pb.ChangeSnapshot{
				Data: &model.SmartBlockSnapshotBase{
					Details: details,
				},
			},
		}
		snapshots = append(snapshots, sn)

		objectsRelations[sn.Id] = relations
	}
	return snapshots, objectsRelations
}

func getDetailsFromCSVTable(csvTable [][]string) []*converter.Relation {
	if len(csvTable) == 0 {
		return nil
	}
	relations := make([]*converter.Relation, 0, len(csvTable[0]))
	for _, relation := range csvTable[0] {
		relations = append(relations, &converter.Relation{
			Relation: &model.Relation{
				Format: model.RelationFormat_longtext,
				Name:   relation,
			},
		})
	}
	return relations
}

func makeRelationsResultMap(rel1 map[string][]*converter.Relation, rel2 map[string][]*converter.Relation) map[string][]*converter.Relation {
	if len(rel1) != 0 {
		for id, relations := range rel2 {
			rel1[id] = relations
		}
		return rel1
	}
	if len(rel2) != 0 {
		for id, relations := range rel1 {
			rel2[id] = relations
		}
		return rel2
	}
	return map[string][]*converter.Relation{}
}
