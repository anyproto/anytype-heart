package csv

import (
	"encoding/csv"
	"fmt"
	te "github.com/anyproto/anytype-heart/core/block/editor/table"
	"io"

	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/import/source"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
)

const (
	Name               = "Csv"
	rootCollectionName = "CSV Import"
)

type Result struct {
	objectIDs []string
	snapshots []*converter.Snapshot
}

func (r *Result) Merge(r2 *Result) {
	if r2 == nil {
		return
	}
	r.objectIDs = append(r.objectIDs, r2.objectIDs...)
	r.snapshots = append(r.snapshots, r2.snapshots...)
}

type CSV struct {
	collectionService *collection.Service
}

func New(collectionService *collection.Service) converter.Converter {
	return &CSV{collectionService: collectionService}
}

func (c *CSV) Name() string {
	return Name
}

func (c *CSV) GetParams(req *pb.RpcObjectImportRequest) *pb.RpcObjectImportRequestCsvParams {
	if p := req.GetCsvParams(); p != nil {
		return p
	}

	return nil
}

func (c *CSV) GetSnapshots(req *pb.RpcObjectImportRequest, progress process.Progress) (*converter.Response, converter.ConvertError) {
	params := c.GetParams(req)
	if params == nil {
		return nil, nil
	}
	progress.SetProgressMessage("Start creating snapshots from files")
	cErr := converter.NewError()
	result, cancelError := c.CreateObjectsFromCSVFiles(req, progress, params, cErr)
	if !cancelError.IsEmpty() {
		return nil, cancelError
	}
	if !cErr.IsEmpty() && req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
		return nil, cErr
	}
	rootCollection := converter.NewRootCollection(c.collectionService)
	rootCol, err := rootCollection.AddObjects(rootCollectionName, result.objectIDs)
	if err != nil {
		cErr.Add(rootCollectionName, err)
		if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, cErr
		}
	}
	if rootCol != nil {
		result.snapshots = append(result.snapshots, rootCol)
	}
	progress.SetTotal(int64(len(result.objectIDs)))
	if cErr.IsEmpty() {
		return &converter.Response{
			Snapshots: result.snapshots,
		}, nil
	}

	return &converter.Response{
		Snapshots: result.snapshots,
	}, cErr
}

func (c *CSV) CreateObjectsFromCSVFiles(req *pb.RpcObjectImportRequest,
	progress process.Progress,
	params *pb.RpcObjectImportRequestCsvParams,
	cErr converter.ConvertError) (*Result, converter.ConvertError) {
	csvMode := params.GetMode()
	str := c.chooseStrategy(csvMode)
	result := &Result{}
	for _, p := range params.GetPath() {
		if err := progress.TryStep(1); err != nil {
			cancelError := converter.NewFromError(p, err)
			return nil, cancelError
		}
		pathResult := c.handlePath(req, p, cErr, str)
		if !cErr.IsEmpty() && req.GetMode() == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, nil
		}
		result.Merge(pathResult)
	}
	return result, nil
}

func (c *CSV) handlePath(req *pb.RpcObjectImportRequest, p string, cErr converter.ConvertError, str Strategy) *Result {
	params := req.GetCsvParams()
	s := source.GetSource(p)
	if s == nil {
		cErr.Add(p, fmt.Errorf("failed to identify source: %s", p))
		return nil
	}
	readers, err := s.GetFileReaders(p, []string{".csv"})
	if err != nil {
		cErr.Add(p, fmt.Errorf("failed to get readers: %s", err.Error()))
		if req.GetMode() == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil
		}
	}
	if len(readers) == 0 {
		cErr.Add(p, converter.ErrNoObjectsToImport)
		return nil
	}
	return c.handleCSVTables(req.Mode, readers, params, str, p, cErr)
}

func (c *CSV) handleCSVTables(mode pb.RpcObjectImportRequestMode,
	readers map[string]io.ReadCloser,
	params *pb.RpcObjectImportRequestCsvParams,
	str Strategy,
	p string,
	cErr converter.ConvertError) *Result {
	allSnapshots := make([]*converter.Snapshot, 0)
	allObjectsIDs := make([]string, 0)
	for _, rc := range readers {
		csvTable, err := c.getCSVTable(rc, params.GetDelimiter())
		if err != nil {
			cErr.Add(p, err)
			if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return nil
			}
			continue
		}
		if c.needToTranspose(params) && len(csvTable) != 0 {
			csvTable = transpose(csvTable)
		}
		objectsIDs, snapshots, err := str.CreateObjects(p, csvTable)
		if err != nil {
			cErr.Add(p, err)
			if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return nil
			}
			continue
		}
		allObjectsIDs = append(allObjectsIDs, objectsIDs...)
		allSnapshots = append(allSnapshots, snapshots...)
	}
	return &Result{objectIDs: allObjectsIDs, snapshots: allSnapshots}
}

func (c *CSV) getCSVTable(rc io.ReadCloser, delimiter string) ([][]string, error) {
	defer rc.Close()
	csvReader := csv.NewReader(rc)
	if delimiter != "" {
		characters := []rune(delimiter)
		csvReader.Comma = characters[0]
	}
	csvTable, err := csvReader.ReadAll()
	if err != nil {
		return nil, err
	}
	return csvTable, nil
}

func (c *CSV) needToTranspose(params *pb.RpcObjectImportRequestCsvParams) bool {
	return (params.GetTransposeRowsAndColumns() && params.GetUseFirstRowForRelations()) ||
		(!params.GetUseFirstRowForRelations() && !params.GetTransposeRowsAndColumns())
}

func (c *CSV) chooseStrategy(mode pb.RpcObjectImportRequestCsvParamsMode) Strategy {
	if mode == pb.RpcObjectImportRequestCsvParams_COLLECTION {
		return NewCollectionStrategy(c.collectionService)
	}
	return NewTableStrategy(te.NewEditor(nil))
}

func transpose(csvTable [][]string) [][]string {
	x := len(csvTable[0])
	y := len(csvTable)
	result := make([][]string, x)
	for i := range result {
		result[i] = make([]string, y)
	}
	for i := 0; i < x; i++ {
		for j := 0; j < y; j++ {
			result[i][j] = csvTable[j][i]
		}
	}
	return result
}
