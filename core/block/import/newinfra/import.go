package newinfra

import (
	"archive/zip"
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

type NewInfra struct{}

func NewImporter() *NewInfra {
	return &NewInfra{}
}

func (i *NewInfra) GetUserProfile(req *pb.RpcUserDataImportRequest, progress *process.Progress) (*pb.Profile, error) {
	archive, err := zip.OpenReader(req.Path)
	if err != nil {
		return nil, err
	}
	defer archive.Close()
	progress.SetTotal(1)

	f, err := archive.Open(profileFile)
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	var profile pb.Profile

	err = profile.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	progress.SetDone(1)
	return &profile, nil
}

func (i *NewInfra) GetSnapshots(req *pb.RpcUserDataImportRequest, progress *process.Progress, address string) *converter.Response {
	archive, err := zip.OpenReader(req.Path)
	importError := converter.NewError()
	if err != nil {
		importError.Add(req.Path, err)
		return &converter.Response{Error: importError}
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
			return &converter.Response{Error: importError}
		}

		data, err := io.ReadAll(reader)
		if err != nil {
			importError.Add(f.Name, err)
			return &converter.Response{Error: importError}
		}

		var mo pb.MigrationObject
		err = mo.Unmarshal(data)
		if err != nil {
			importError.Add(f.Name, err)
			return &converter.Response{Error: importError}
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
	return res
}
