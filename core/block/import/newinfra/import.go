package newinfra

import (
	"archive/zip"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"os"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/pb"
	sb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
)

const profileFile = "profile"

type NewInfra struct {
}

func NewImporter() *NewInfra {
	return &NewInfra{}
}

func (i *NewInfra) GetUserProfile(req *pb.RpcUserDataImportRequest, progress *process.Progress) (*pb.Profile, error) {
	archive, err := zip.OpenReader(req.Path)
	importError := converter.NewError()
	if err != nil {
		importError.Add(req.Path, err)
		return nil, err
	}
	defer archive.Close()
	progress.SetTotal(1)
	data, err := os.ReadFile(profileFile)
	if err != nil {
		importError.Add(profileFile, err)
		return nil, err
	}

	var profile *pb.Profile

	err = profile.Unmarshal(data)
	if err != nil {
		importError.Add(profileFile, err)
		return nil, err
	}
	progress.SetDone(1)
	return profile, nil
}

func (i *NewInfra) GetSnapshots(req *pb.RpcUserDataImportRequest, progress *process.Progress) *converter.Response {
	archive, err := zip.OpenReader(req.Path)
	importError := converter.NewError()
	if err != nil {
		importError.Add(req.Path, err)
		return &converter.Response{Error: importError}
	}
	defer archive.Close()
	res := &converter.Response{Snapshots: make([]*converter.Snapshot, 0)}
	progress.SetTotal(int64(len(archive.File)) * 2)
	for _, f := range archive.File {
		if f.Name == profileFile {
			continue
		}
		data, err := os.ReadFile(f.Name)
		if err != nil {
			importError.Add(f.Name, err)
			return &converter.Response{Error: importError}
		}

		var mo *pb.MigrationObject
		err = mo.Unmarshal(data)
		if err != nil {
			importError.Add(f.Name, err)
			return &converter.Response{Error: importError}
		}
		snapshot := &converter.Snapshot{
			SbType:   sb.SmartBlockType(mo.SbType),
			FileName: f.Name,
			Snapshot: mo.Snapshot,
		}
		res.Snapshots = append(res.Snapshots, snapshot)
		progress.AddDone(1)
	}
	return res
}
