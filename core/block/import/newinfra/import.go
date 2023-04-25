package newinfra

import (
	"archive/zip"
	"os"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/pb"
	sb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
)

type NewInfra struct{}

func NewImporter() *NewInfra {
	return &NewInfra{}
}

func (i *NewInfra) GetSnapshots(path string) *converter.Response {
	archive, err := zip.OpenReader(path)
	importError := converter.NewError()
	if err != nil {
		importError.Add(path, err)
		return &converter.Response{Error: importError}
	}
	defer archive.Close()
	res := &converter.Response{Snapshots: make([]*converter.Snapshot, 0)}
	for _, f := range archive.File {
		data, err := os.ReadFile(f.Name)
		if err != nil {
			importError.Add(f.Name, err)
			return &converter.Response{Error: importError}
		}

		var mo *pb.MigrationObject
		err = mo.Unmarshal(data)
		snapshot := &converter.Snapshot{
			SbType:   sb.SmartBlockType(mo.SbType),
			FileName: f.Name,
			Snapshot: mo.Snapshot,
		}
		res.Snapshots = append(res.Snapshots, snapshot)
	}
	return res
}
