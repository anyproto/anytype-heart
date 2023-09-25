package csv

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"

	"github.com/anyproto/anytype-heart/core/block/collection"
	te "github.com/anyproto/anytype-heart/core/block/editor/table"
	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/import/source"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
)

const (
	Name                  = "Csv"
	rootCollectionName    = "CSV Import"
	numberOfProgressSteps = 2
	limitForColumns       = 10
	limitForRows          = 1000
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

func (c *CSV) GetSnapshots(req *pb.RpcObjectImportRequest, progress process.Progress) (*converter.Response, *converter.ConvertError) {
	params := c.GetParams(req)
	if params == nil {
		return nil, nil
	}
	cErr := converter.NewError()
	result, cancelError := c.createObjectsFromCSVFiles(req, progress, params, cErr)
	if !cancelError.IsEmpty() {
		return nil, cancelError
	}
	if c.needToReturnError(req, cErr, params.Path) {
		return nil, cErr
	}
	rootCollection := converter.NewRootCollection(c.collectionService)
	rootCol, err := rootCollection.MakeRootCollection(rootCollectionName, result.objectIDs)
	if err != nil {
		cErr.Add(err)
		if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, cErr
		}
	}
	var rootCollectionID string
	if rootCol != nil {
		result.snapshots = append(result.snapshots, rootCol)
		rootCollectionID = rootCol.Id
	}
	progress.SetTotal(int64(len(result.snapshots)))
	if cErr.IsEmpty() {
		return &converter.Response{Snapshots: result.snapshots, RootCollectionID: rootCollectionID}, nil
	}

	return &converter.Response{Snapshots: result.snapshots, RootCollectionID: rootCollectionID}, cErr
}

func (c *CSV) needToReturnError(req *pb.RpcObjectImportRequest, cErr *converter.ConvertError, params []string) bool {
	return (!cErr.IsEmpty() && req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING) ||
		(cErr.IsNoObjectToImportError(len(params))) ||
		errors.Is(cErr.GetResultError(pb.RpcObjectImportRequest_Csv), converter.ErrLimitExceeded)
}

func (c *CSV) createObjectsFromCSVFiles(req *pb.RpcObjectImportRequest,
	progress process.Progress,
	params *pb.RpcObjectImportRequestCsvParams,
	cErr *converter.ConvertError) (*Result, *converter.ConvertError) {
	csvMode := params.GetMode()
	str := c.chooseStrategy(csvMode)
	result := &Result{}
	for _, p := range params.GetPath() {
		pathResult := c.getSnapshotsFromFiles(req, p, cErr, str, progress)
		if !cErr.IsEmpty() && req.GetMode() == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, nil
		}
		result.Merge(pathResult)
	}
	return result, nil
}

func (c *CSV) getSnapshotsFromFiles(req *pb.RpcObjectImportRequest, p string, cErr *converter.ConvertError, str Strategy, progress process.Progress) *Result {
	params := req.GetCsvParams()
	importSource := source.GetSource(p)
	if importSource == nil {
		cErr.Add(fmt.Errorf("failed to identify source: %s", p))
		return nil
	}
	defer importSource.Close()
	readers, err := importSource.GetFileReaders(p, []string{".csv"}, nil)
	if err != nil {
		cErr.Add(fmt.Errorf("failed to get readers: %s", err.Error()))
		if req.GetMode() == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil
		}
	}
	if len(readers) == 0 {
		cErr.Add(converter.ErrNoObjectsToImport)
		return nil
	}
	return c.getSnapshots(req.Mode, readers, params, str, cErr, progress)
}

func (c *CSV) getSnapshots(mode pb.RpcObjectImportRequestMode,
	readers map[string]io.ReadCloser,
	params *pb.RpcObjectImportRequestCsvParams,
	str Strategy,
	cErr *converter.ConvertError,
	progress process.Progress) *Result {
	allSnapshots := make([]*converter.Snapshot, 0)
	allObjectsIDs := make([]string, 0)
	progress.SetProgressMessage("Start creating snapshots from files")
	progress.SetTotal(int64(len(readers) * numberOfProgressSteps))
	for filePath, rc := range readers {
		if err := progress.TryStep(1); err != nil {
			cErr = converter.NewCancelError(err)
			return nil
		}
		csvTable, err := c.getCSVTable(rc, params.GetDelimiter())
		if err != nil {
			cErr.Add(err)
			if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return nil
			}
			continue
		}
		if params.TransposeRowsAndColumns && len(csvTable) != 0 {
			csvTable = transpose(csvTable)
		}
		collectionID, snapshots, err := str.CreateObjects(filePath, csvTable, params, progress)
		if err != nil {
			cErr.Add(err)
			if errors.Is(err, converter.ErrLimitExceeded) {
				return nil
			}
			if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return nil
			}
		}
		allObjectsIDs = append(allObjectsIDs, collectionID)
		allSnapshots = append(allSnapshots, snapshots...)
	}
	return &Result{objectIDs: allObjectsIDs, snapshots: allSnapshots}
}

func (c *CSV) getCSVTable(rc io.ReadCloser, delimiter string) ([][]string, error) {
	defer rc.Close()
	csvReader := csv.NewReader(rc)
	csvReader.LazyQuotes = true
	csvReader.ReuseRecord = true
	csvReader.FieldsPerRecord = -1
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
