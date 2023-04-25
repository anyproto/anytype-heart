package newinfra

import (
	"archive/zip"
	"errors"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"io"
	"strings"

	"github.com/google/uuid"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	sb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

const profileFile = "profile"

const Name = "Migration"

func init() {
	converter.RegisterFunc(New)
}

type NewInfra struct{}

func New(core.Service) converter.Converter {
	return &NewInfra{}
}

func (i *NewInfra) GetParams(params pb.IsRpcObjectImportRequestParams) (string, string, error) {
	if p, ok := params.(*pb.RpcObjectImportRequestParamsOfMigrationParams); ok {
		return p.MigrationParams.GetPath(), p.MigrationParams.GetAddress(), nil
	}
	return "", "", errors.New("NewInfra: GetParams wrong parameters format")
}

func (i *NewInfra) GetSnapshots(req *pb.RpcObjectImportRequest, progress *process.Progress) (*converter.Response, converter.ConvertError) {
	importError := converter.NewError()
	path, address, e := i.GetParams(req.Params)
	if e != nil {
		importError.Add(path, e)
		return nil, importError
	}
	archive, err := zip.OpenReader(path)
	if err != nil {
		importError.Add(path, err)
		return nil, importError
	}
	defer archive.Close()
	res := &converter.Response{Snapshots: make([]*converter.Snapshot, 0)}
	progress.SetTotal(int64(len(archive.File)) * 2)
	oldIDToNew := make(map[string]string, 0)
	for _, f := range archive.File {
		if f.Name == profileFile {
			continue
		}
		if f.FileInfo().IsDir() {
			continue
		}
		// skip files from account directory
		if strings.Contains(f.FileHeader.Name, address) {
			continue
		}
		oldIDToNew[f.Name] = uuid.New().String()

	}
	for _, f := range archive.File {
		if f.Name == profileFile {
			continue
		}
		if f.FileInfo().IsDir() {
			continue
		}
		// skip files from account directory
		if strings.Contains(f.FileHeader.Name, address) {
			continue
		}
		reader, err := f.Open()
		if err != nil {
			importError.Add(f.Name, err)
			return nil, importError
		}

		data, err := io.ReadAll(reader)
		if err != nil {
			importError.Add(f.Name, err)
			return nil, importError
		}

		var mo pb.MigrationObject
		err = mo.Unmarshal(data)
		if err != nil {
			importError.Add(f.Name, err)
			return nil, importError
		}

		mo.Snapshot.Data.Details.Fields[bundle.RelationKeyOldAnytypeID.String()] = pbtypes.String(f.Name)
		snapshot := &converter.Snapshot{
			SbType:   sb.SmartBlockType(mo.SbType),
			FileName: f.Name,
			Snapshot: mo.Snapshot,
			Id:       f.Name,
		}

		res.Snapshots = append(res.Snapshots, snapshot)
		progress.AddDone(1)
	}
	return res, nil
}

func (i *NewInfra) Name() string {
	return Name
}
