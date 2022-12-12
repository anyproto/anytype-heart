package pb

import (
	"archive/zip"
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/textileio/go-threads/core/thread"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

const Name = "PB"

type Pb struct{}

func init() {
	converter.RegisterFunc(New)
}

func New(core.Service) converter.Converter {
	return new(Pb)
}

func (p *Pb) GetSnapshots(req *pb.RpcObjectImportRequest, progress *process.Progress) (*converter.Response, converter.ConvertError) {
	path, e := p.GetParams(req.Params)
	allErrors := converter.NewError()
	if e != nil {
		allErrors.Add(path, e)
		return nil, allErrors
	}
	pbFiles, err := p.readFile(path, req.Mode.String())
	if err != nil && req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
		allErrors.Merge(err)
		return nil, allErrors
	}
	allSnapshots := make([]*converter.Snapshot, 0)

	progress.SetProgressMessage("Start creating snapshots from files")
	progress.SetTotal(int64(len(pbFiles) * 2))

	for name, file := range pbFiles {
		if err := progress.TryStep(1); err != nil {
			ce := converter.NewFromError(name, err)
			return nil, ce
		}

		id := strings.TrimSuffix(file.Name, filepath.Ext(file.Name))
		var (
			snapshot *model.SmartBlockSnapshotBase
			errGS    error
		)
		rc := file.Reader
		snapshot, errGS = p.GetSnapshot(rc)
		rc.Close()
		if errGS != nil {
			allErrors.Add(file.Name, errGS)
			if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return nil, allErrors
			} else {
				continue
			}
		}
		sbt, err := smartblock.SmartBlockTypeFromID(id)
		if err != nil {
			allErrors.Add(path, e)
			if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return nil, allErrors
			} else {
				continue
			}
		}
		tid, err := threads.ThreadCreateID(thread.AccessControlled, sbt)
		if err != nil {
			allErrors.Add(path, e)
			if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return nil, allErrors
			} else {
				continue
			}
		}
		source := converter.GetSourceDetail(name, path)
		snapshot.Details.Fields[bundle.RelationKeySource.String()] = pbtypes.String(source)
		allSnapshots = append(allSnapshots, &converter.Snapshot{
			Id:       tid.String(),
			FileName: name,
			Snapshot: snapshot,
		})
	}

	if allErrors.IsEmpty() {
		return &converter.Response{Snapshots: allSnapshots}, nil
	}

	return &converter.Response{Snapshots: allSnapshots}, allErrors
}

func (p *Pb) Name() string {
	return Name
}

func (p *Pb) GetImage() ([]byte, int64, int64, error) {
	return nil, 0, 0, nil
}

func (p *Pb) GetParams(params pb.IsRpcObjectImportRequestParams) (string, error) {
	if p, ok := params.(*pb.RpcObjectImportRequestParamsOfMarkdownParams); ok {
		return p.MarkdownParams.GetPath(), nil
	}
	return "", errors.New("PB: GetParams wrong parameters format")
}

func (p *Pb) readFile(importPath string, mode string) (map[string]*converter.IOReader, converter.ConvertError) {
	r, err := zip.OpenReader(importPath)
	errors := converter.NewError()
	if err != nil {
		errors.Add(importPath, err)
		return nil, errors
	}
	files := make(map[string]*converter.IOReader)
	for _, f := range r.File {
		if filepath.Ext(f.Name) != ".pb" {
			continue
		}
		shortPath := filepath.Clean(f.Name)

		rc, err := f.Open()
		if err != nil {
			errors.Add(f.FileInfo().Name(), err)
			switch mode {
			case pb.RpcObjectImportRequest_IGNORE_ERRORS.String():
				continue
			default:
				return nil, errors
			}

		}
		files[shortPath] = &converter.IOReader{
			Name:   f.FileInfo().Name(),
			Reader: rc,
		}
	}
	return files, nil
}

func (p *Pb) GetSnapshot(rd io.ReadCloser) (*model.SmartBlockSnapshotBase, error) {
	defer rd.Close()
	data, err := ioutil.ReadAll(rd)
	if err != nil {
		return nil, fmt.Errorf("PB:GetSnapshot %s", err)
	}
	snapshot := &pb.ChangeSnapshot{}
	if err = snapshot.Unmarshal(data); err != nil {
		return nil, fmt.Errorf("PB:GetSnapshot %s", err)
	}
	return snapshot.Data, nil
}
