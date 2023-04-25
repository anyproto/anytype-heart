package pb

import (
	"archive/zip"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/gogo/protobuf/types"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/google/uuid"
	"github.com/pkg/errors"

	"github.com/anytypeio/go-anytype-middleware/core/block/collection"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/space/typeprovider"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

const (
	Name               = "Pb"
	rootCollectionName = "Protobuf Import"
	profileFile        = "profile"
	configFile         = "config.json"
)

type ProtobufSnapshotGetter struct {
}

type Pb struct {
	service     *collection.Service
	sbtProvider typeprovider.SmartBlockTypeProvider
}

func New(service *collection.Service, sbtProvider typeprovider.SmartBlockTypeProvider) converter.Converter {
	return &Pb{
		service:     service,
		sbtProvider: sbtProvider,
	}
}

func (p *Pb) GetSnapshots(req *pb.RpcObjectImportRequest,
	progress *process.Progress) (*converter.Response, converter.ConvertError) {
	params, e := p.GetParams(req.Params)
	if e != nil || params == nil {
		errors := converter.NewError()
		errors.Add("", fmt.Errorf("wrong parameters"))
		return nil, errors
	}

	profile, err := p.readProfileFile(profileFile)
	if err != nil {
		errors := converter.NewFromError(profileFile, err)
		if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, errors
		}
	}

	allSnapshots, targetObjects, allErrors := p.getSnapshots(req, progress, params.GetPath(), profile, params.AccountId)
	if !allErrors.IsEmpty() && req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
		return nil, allErrors
	}

	if params.GetCreateObjectsCollection() {
		rootCollection := converter.NewRootCollection(p.service)
		rootCol, colErr := rootCollection.AddObjects(rootCollectionName, targetObjects)
		if colErr != nil {
			allErrors.Add(rootCollectionName, colErr)
			if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return nil, allErrors
			}
		}

		if rootCol != nil {
			allSnapshots = append(allSnapshots, rootCol)
		}
	}

	progress.SetTotal(int64(len(allSnapshots)) * 2)

	if allErrors.IsEmpty() {
		return &converter.Response{Snapshots: allSnapshots}, nil
	}

	return &converter.Response{Snapshots: allSnapshots}, allErrors
}

func (p *Pb) getSnapshots(req *pb.RpcObjectImportRequest, progress *process.Progress, allPaths []string, profile *pb.Profile, accountID string) ([]*converter.Snapshot, []string, converter.ConvertError) {
	targetObjects := make([]string, 0)
	allSnapshots := make([]*converter.Snapshot, 0)
	allErrors := converter.NewError()
	needToImportWidgets := p.needToImportWidgets(profile.Address, accountID)
	spaceDashboardID := profile.SpaceDashboardId
	for _, path := range allPaths {
		pbFiles, err := p.readFile(path, req.Mode.String())
		if err != nil && req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			allErrors.Merge(err)
			return nil, nil, allErrors
		}

		snapshots, objects, ce := p.getSnapshotsFromFiles(req, progress, pbFiles, allErrors, path, needToImportWidgets, spaceDashboardID)
		if !ce.IsEmpty() {
			return nil, nil, ce
		}
		if !allErrors.IsEmpty() && req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, nil, allErrors
		}
		allSnapshots = append(allSnapshots, snapshots...)
		targetObjects = append(targetObjects, objects...)
	}
	return allSnapshots, targetObjects, allErrors
}

func (p *Pb) getSnapshotsFromFiles(req *pb.RpcObjectImportRequest,
	progress *process.Progress,
	pbFiles map[string]*converter.IOReader,
	allErrors converter.ConvertError,
	path string,
	needToCreateWidgets bool,
	spaceDashboardID string) ([]*converter.Snapshot, []string, converter.ConvertError) {
	targetObjects := make([]string, 0)
	allSnapshots := make([]*converter.Snapshot, 0)
	for name, file := range pbFiles {
		if name == profileFile || name == configFile {
			continue
		}
		if err := progress.TryStep(1); err != nil {
			ce := converter.NewFromError(name, err)
			return nil, nil, ce
		}

		id := uuid.New().String()
		var (
			mo    *pb.SnapshotWithType
			errGS error
		)
		rc := file.Reader
		mo, errGS = p.GetSnapshot(rc, name, needToCreateWidgets, spaceDashboardID)
		rc.Close()
		if errGS != nil {
			allErrors.Add(name, errGS)
			if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return nil, nil, nil
			} else {
				continue
			}
		}
		source := converter.GetSourceDetail(name, path)
		mo.Snapshot.Data.Details.Fields[bundle.RelationKeySource.String()] = pbtypes.String(source)
		allSnapshots = append(allSnapshots, &converter.Snapshot{
			Id:       id,
			SbType:   smartblock.SmartBlockType(mo.SbType),
			FileName: name,
			Snapshot: mo.Snapshot,
		})
		targetObjects = append(targetObjects, id)
	}
	return allSnapshots, targetObjects, nil
}

func (p *Pb) Name() string {
	return Name
}

func (p *Pb) GetParams(params pb.IsRpcObjectImportRequestParams) (*pb.RpcObjectImportRequestPbParams, error) {
	if p, ok := params.(*pb.RpcObjectImportRequestParamsOfPbParams); ok {
		return p.PbParams, nil
	}
	return nil, errors.New("PB: GetParams wrong parameters format")
}

func (p *Pb) readFile(importPath string, mode string) (map[string]*converter.IOReader, converter.ConvertError) {
	files := make(map[string]*converter.IOReader)
	r, err := zip.OpenReader(importPath)
	if err != nil {
		return p.handleFile(importPath, files)
	}
	convertError := p.handleZipArchive(r, mode, files)
	return files, convertError
}

func (p *Pb) handleZipArchive(r *zip.ReadCloser, mode string, files map[string]*converter.IOReader) converter.ConvertError {
	errors := converter.NewError()
	for _, f := range r.File {
		if !(filepath.Ext(f.Name) == ".pb" || filepath.Ext(f.Name) == ".json") {
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
				return errors
			}

		}
		files[shortPath] = &converter.IOReader{
			Name:   f.FileInfo().Name(),
			Reader: rc,
		}
	}
	return nil
}

func (p *Pb) handleFile(importPath string, files map[string]*converter.IOReader) (map[string]*converter.IOReader, converter.ConvertError) {
	errors := converter.NewError()
	f, fErr := os.Open(importPath)
	if fErr != nil {
		errors.Add(importPath, fErr)
		return nil, errors
	}
	if !(filepath.Ext(f.Name()) == ".pb" || filepath.Ext(f.Name()) == ".json") {
		return nil, nil
	}
	name := filepath.Clean(f.Name())
	files[name] = &converter.IOReader{
		Name:   f.Name(),
		Reader: f,
	}
	return files, nil
}

func (p *Pb) GetSnapshot(rd io.ReadCloser, name string, needToCreateWidget bool, spaceDashboardID string) (*pb.SnapshotWithType, error) {
	defer rd.Close()
	snapshot := &pb.SnapshotWithType{}
	if filepath.Ext(name) == ".json" {
		um := jsonpb.Unmarshaler{}
		if uErr := um.Unmarshal(rd, snapshot); uErr != nil {
			return nil, fmt.Errorf("PB:GetSnapshot %s", uErr)
		}
		return snapshot, nil
	}
	data, err := ioutil.ReadAll(rd)
	if err != nil {
		return nil, fmt.Errorf("PB:GetSnapshot %s", err)
	}
	if err = snapshot.Unmarshal(data); err != nil {
		return nil, fmt.Errorf("PB:GetSnapshot %s", err)
	}
	if snapshot.SbType == model.SmartBlockType_Widget && !needToCreateWidget {
		return nil, nil
	}
	p.setSpaceDashboardID(spaceDashboardID, snapshot)
	return snapshot, nil
}

func (p *Pb) readProfileFile(file string) (*pb.Profile, error) {
	f, fErr := os.Open(file)
	if fErr != nil {
		return nil, fErr
	}
	profile := &pb.Profile{}
	data, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}
	if err = profile.Unmarshal(data); err != nil {
		return nil, err
	}
	return profile, nil
}

func (p *Pb) needToImportWidgets(address string, accountId string) bool {
	if address == accountId {
		return true
	}
	return false
}

func (p *Pb) setSpaceDashboardID(spaceDashboardID string, snapshot *pb.SnapshotWithType) {
	if snapshot.SbType == model.SmartBlockType_Workspace && spaceDashboardID != "" {
		id := pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeySpaceDashboardId.String())
		if id != "" {
			return
		}
		details := snapshot.Snapshot.Data.Details
		if details == nil || details.Fields == nil {
			snapshot.Snapshot.Data.Details = &types.Struct{Fields: map[string]*types.Value{}}
		}
		snapshot.Snapshot.Data.Details.Fields[bundle.RelationKeySpaceDashboardId.String()] = pbtypes.String(spaceDashboardID)
		snapshot.Snapshot.Data.RelationLinks = append(snapshot.Snapshot.Data.RelationLinks, &model.RelationLink{
			Key:    bundle.RelationKeySpaceDashboardId.String(),
			Format: model.RelationFormat_object,
		})
	}
}
