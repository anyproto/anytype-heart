package csv

import (
	"encoding/csv"
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
	allErrors := converter.NewError(req.Mode)
	result := c.createObjectsFromCSVFiles(req, progress, params, allErrors)
	if allErrors.ShouldAbortImport(len(params.Path), req.Type) {
		return nil, allErrors
	}
	rootCollection := converter.NewRootCollection(c.collectionService)
	rootCol, err := rootCollection.MakeRootCollection(rootCollectionName, result.objectIDs)
	if err != nil {
		allErrors.Add(err)
		if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, allErrors
		}
	}
	if rootCol != nil {
		result.snapshots = append(result.snapshots, rootCol)
	}
	progress.SetTotal(int64(len(result.snapshots)))
	if allErrors.IsEmpty() {
		return &converter.Response{Snapshots: result.snapshots}, nil
	}

	return &converter.Response{Snapshots: result.snapshots}, allErrors
}

func (c *CSV) createObjectsFromCSVFiles(req *pb.RpcObjectImportRequest,
	progress process.Progress,
	params *pb.RpcObjectImportRequestCsvParams,
	allErrors *converter.ConvertError,
) *Result {
	csvMode := params.GetMode()
	str := c.chooseStrategy(csvMode)
	result := &Result{}
	for _, p := range params.GetPath() {
		pathResult := c.getSnapshotsFromFiles(req, p, allErrors, str, progress)
		if allErrors.ShouldAbortImport(len(params.GetPath()), req.Type) {
			return nil
		}
		result.Merge(pathResult)
	}
	return result
}

func (c *CSV) getSnapshotsFromFiles(req *pb.RpcObjectImportRequest,
	importPath string,
	allErrors *converter.ConvertError,
	str Strategy,
	progress process.Progress,
) *Result {
	params := req.GetCsvParams()
	importSource := source.GetSource(importPath)
	defer importSource.Close()
	err := importSource.Initialize(importPath)
	if err != nil {
		allErrors.Add(fmt.Errorf("failed to extract files: %s", err.Error()))
		if allErrors.ShouldAbortImport(len(params.GetPath()), req.Type) {
			return nil
		}
	}
	var numberOfFiles int
	if numberOfFiles = importSource.CountFilesWithGivenExtensions([]string{".csv"}); numberOfFiles == 0 {
		allErrors.Add(converter.ErrNoObjectsToImport)
		return nil
	}
	progress.SetProgressMessage("Start creating snapshots from files")
	progress.SetTotal(int64(numberOfFiles) * numberOfProgressSteps)
	return c.getSnapshotsAndObjectsIDs(importSource, params, str, allErrors, progress)
}

func (c *CSV) getSnapshotsAndObjectsIDs(importSource source.Source,
	params *pb.RpcObjectImportRequestCsvParams,
	str Strategy,
	allErrors *converter.ConvertError,
	progress process.Progress,
) *Result {
	allSnapshots := make([]*converter.Snapshot, 0)
	allObjectsIDs := make([]string, 0)
	if iterateErr := importSource.Iterate(func(fileName string, fileReader io.ReadCloser) (isContinue bool) {
		if err := progress.TryStep(1); err != nil {
			allErrors.Add(converter.ErrCancel)
			return false
		}
		csvTable, err := c.getCSVTable(fileReader, params.GetDelimiter())
		if err != nil {
			allErrors.Add(err)
			return !allErrors.ShouldAbortImport(len(params.GetPath()), pb.RpcObjectImportRequest_Csv)
		}
		if params.TransposeRowsAndColumns && len(csvTable) != 0 {
			csvTable = transpose(csvTable)
		}
		collectionID, snapshots, err := str.CreateObjects(fileName, csvTable, params, progress)
		if err != nil {
			allErrors.Add(err)
			return !allErrors.ShouldAbortImport(len(params.GetPath()), pb.RpcObjectImportRequest_Csv)
		}
		allObjectsIDs = append(allObjectsIDs, collectionID)
		allSnapshots = append(allSnapshots, snapshots...)
		return true
	}); iterateErr != nil {
		allErrors.Add(iterateErr)
	}
	return &Result{allObjectsIDs, allSnapshots}
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
