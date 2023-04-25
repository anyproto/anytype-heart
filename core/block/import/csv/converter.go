package csv

import (
	"encoding/csv"
	"os"
	"path/filepath"

	"github.com/anytypeio/go-anytype-middleware/core/block/collection"
	te "github.com/anytypeio/go-anytype-middleware/core/block/editor/table"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

const (
	Name               = "Csv"
	rootCollectionName = "CSV Import"
)

var log = logging.Logger("csv-import")

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

func (c *CSV) GetMode(req *pb.RpcObjectImportRequest) pb.RpcObjectImportRequestCsvParamsMode {
	if p := req.GetCsvParams(); p != nil {
		return p.Mode
	}

	return pb.RpcObjectImportRequestCsvParams_COLLECTION
}

func (c *CSV) GetSnapshots(req *pb.RpcObjectImportRequest,
	progress *process.Progress) (*converter.Response, converter.ConvertError) {
	path := c.GetParams(req)
	if len(path) == 0 {
		return nil, nil
	}
	progress.SetProgressMessage("Start creating snapshots from files")

	allObjectsIDs, allSnapshots, allRelations, cErr := c.CreateObjectsFromCSVFiles(req, progress, path)

	rootCollection := converter.NewRootCollection(c.collectionService)
	rootCol, err := rootCollection.AddObjects(rootCollectionName, allObjectsIDs)
	if err != nil {
		cErr.Add(rootCollectionName, err)
		if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, cErr
		}
	}

	if rootCol != nil {
		allSnapshots = append(allSnapshots, rootCol)
	}
	progress.SetTotal(int64(len(allObjectsIDs)))

	if cErr.IsEmpty() {
		return &converter.Response{
			Snapshots: allSnapshots,
			Relations: allRelations,
		}, nil
	}

	return &converter.Response{
		Snapshots: allSnapshots,
		Relations: allRelations,
	}, cErr
}

func (c *CSV) CreateObjectsFromCSVFiles(req *pb.RpcObjectImportRequest,
	progress *process.Progress,
	path []string) ([]string, []*converter.Snapshot, map[string][]*converter.Relation, converter.ConvertError) {
	csvMode := c.GetMode(req)
	str := c.chooseStrategy(csvMode)
	allSnapshots := make([]*converter.Snapshot, 0)
	allRelations := make(map[string][]*converter.Relation, 0)
	allObjectsIDs := make([]string, 0)
	cErr := converter.NewError()
	for _, p := range path {
		if err := progress.TryStep(1); err != nil {
			cancelError := converter.NewFromError(p, err)
			return nil, nil, nil, cancelError
		}
		if filepath.Ext(p) != ".csv" {
			continue
		}
		csvTable, err := readCsvFile(p)
		if err != nil {
			cErr.Add(p, err)
			if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return nil, nil, nil, cErr
			}
			continue
		}

		objectsIDs, snapshots, relations, err := str.CreateObjects(p, csvTable)
		if err != nil {
			cErr.Add(p, err)
			if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return nil, nil, nil, cErr
			}
			continue
		}
		allObjectsIDs = append(allObjectsIDs, objectsIDs...)
		allSnapshots = append(allSnapshots, snapshots...)
		allRelations = mergeRelationsMaps(allRelations, relations)
	}
	return allObjectsIDs, allSnapshots, allRelations, cErr
}

func (c *CSV) chooseStrategy(mode pb.RpcObjectImportRequestCsvParamsMode) Strategy {
	if mode == pb.RpcObjectImportRequestCsvParams_COLLECTION {
		return NewCollectionStrategy(c.collectionService)
	}
	return NewTableStrategy(te.NewEditor(nil))
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

func mergeRelationsMaps(rel1 map[string][]*converter.Relation, rel2 map[string][]*converter.Relation) map[string][]*converter.Relation {
	if rel1 != nil {
		for id, relations := range rel2 {
			rel1[id] = relations
		}
		return rel1
	}
	if rel2 != nil {
		for id, relations := range rel1 {
			rel2[id] = relations
		}
		return rel2
	}
	return map[string][]*converter.Relation{}
}
