package pb

import (
	"archive/zip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/types"
	"github.com/google/uuid"
	"github.com/pkg/errors"

	"github.com/anytypeio/go-anytype-middleware/core/block/collection"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/markdown/anymark/whitespace"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/space/typeprovider"
	"github.com/anytypeio/go-anytype-middleware/util/constant"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

const (
	Name               = "Pb"
	rootCollectionName = "Protobuf Import"
	configFile         = "config.json"
)

type Pb struct {
	service     *collection.Service
	sbtProvider typeprovider.SmartBlockTypeProvider
	core        core.Service
}

func New(service *collection.Service, sbtProvider typeprovider.SmartBlockTypeProvider, core core.Service) converter.Converter {
	return &Pb{
		service:     service,
		sbtProvider: sbtProvider,
		core:        core,
	}
}

func (p *Pb) GetSnapshots(req *pb.RpcObjectImportRequest, progress process.Progress) (*converter.Response, converter.ConvertError) {
	params, e := p.GetParams(req.Params)
	if e != nil || params == nil {
		errors := converter.NewError()
		errors.Add("", fmt.Errorf("wrong parameters"))
		return nil, errors
	}
	allSnapshots, targetObjects, allErrors := p.getSnapshots(req, progress, params.GetPath())
	if !allErrors.IsEmpty() && req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
		return nil, allErrors
	}

	p.updateLinksToObjects(allSnapshots, allErrors, req.Mode)
	p.updateDetails(allSnapshots)
	if !allErrors.IsEmpty() && req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
		return nil, allErrors
	}

	if !params.GetNoCollection() {
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

func (p *Pb) getSnapshots(req *pb.RpcObjectImportRequest, progress process.Progress, allPaths []string) ([]*converter.Snapshot, []string, converter.ConvertError) {
	targetObjects := make([]string, 0)
	allSnapshots := make([]*converter.Snapshot, 0)
	allErrors := converter.NewError()
	for _, path := range allPaths {
		pbFiles, profile, err := p.readFile(path, req.Mode.String())
		if err != nil {
			allErrors.Merge(err)
			if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return nil, nil, allErrors
			}
		}
		var (
			needToImportWidgets bool
			profileId           string
		)
		if profile != nil {
			pr, e := p.core.LocalProfile()
			if e != nil {
				allErrors.Add(constant.ProfileFile, e)
				if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
					return nil, nil, allErrors
				}
			}
			needToImportWidgets = p.needToImportWidgets(profile.Address, pr.AccountAddr)
			profileId = profile.ProfileId
		}
		snapshots, objects, ce := p.getSnapshotsFromFiles(req, progress, pbFiles, allErrors, path, needToImportWidgets, profileId)
		if !ce.IsEmpty() {
			return nil, nil, ce
		}
		if !allErrors.IsEmpty() && req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, nil, allErrors
		}
		p.setDashboardID(profile, snapshots)
		allSnapshots = append(allSnapshots, snapshots...)
		targetObjects = append(targetObjects, objects...)
	}
	return allSnapshots, targetObjects, allErrors
}

func (p *Pb) setDashboardID(profile *pb.Profile, snapshots []*converter.Snapshot) {
	var (
		newSpaceDashBoardID string
		workspace           *converter.Snapshot
	)
	if profile == nil {
		return
	}
	for _, snapshot := range snapshots {
		if snapshot.SbType == smartblock.SmartBlockTypeWorkspace {
			workspace = snapshot
		}
		id := pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeyId.String())
		normalizedID := whitespace.WhitespaceNormalizeString(id)
		normalizedSpaceDashboardID := whitespace.WhitespaceNormalizeString(profile.SpaceDashboardId)
		if strings.EqualFold(normalizedID, normalizedSpaceDashboardID) {
			newSpaceDashBoardID = snapshot.Id
		}
	}

	if workspace != nil && newSpaceDashBoardID != "" {
		workspace.Snapshot.Data.Details.Fields[bundle.RelationKeySpaceDashboardId.String()] = pbtypes.String(newSpaceDashBoardID)
	}
}

func (p *Pb) getSnapshotsFromFiles(req *pb.RpcObjectImportRequest,
	progress process.Progress,
	pbFiles map[string]*converter.IOReader,
	allErrors converter.ConvertError, path string,
	needToCreateWidgets bool,
	profileID string) ([]*converter.Snapshot, []string, converter.ConvertError) {
	targetObjects := make([]string, 0)
	allSnapshots := make([]*converter.Snapshot, 0)
	for name, file := range pbFiles {
		if name == constant.ProfileFile || name == configFile {
			continue
		}
		if err := progress.TryStep(1); err != nil {
			ce := converter.NewFromError(name, err)
			return nil, nil, ce
		}
		id := uuid.New().String()
		rc := file.Reader
		mo, errGS := p.GetSnapshot(rc, name, needToCreateWidgets)
		rc.Close()
		if errGS != nil {
			allErrors.Add(name, errGS)
			if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return nil, nil, nil
			}
		}
		if mo == nil {
			continue
		}
		if mo.SbType == model.SmartBlockType_ProfilePage {
			id = p.getIDForUserProfile(mo, profileID, id)
		}
		p.fillDetails(name, path, mo, id)
		allSnapshots = append(allSnapshots, &converter.Snapshot{
			Id:       id,
			SbType:   smartblock.SmartBlockType(mo.SbType),
			FileName: name,
			Snapshot: mo.Snapshot,
		})
		// not add sub objects to root collection
		if mo.SbType == model.SmartBlockType_SubObject {
			continue
		}
		targetObjects = append(targetObjects, id)
	}
	return allSnapshots, targetObjects, nil
}

func (p *Pb) getIDForUserProfile(mo *pb.SnapshotWithType, profileID string, id string) string {
	objectID := pbtypes.GetString(mo.Snapshot.Data.Details, bundle.RelationKeyId.String())
	if objectID == profileID {
		return p.core.ProfileID()
	}
	return id
}

func (p *Pb) fillDetails(name string, path string, mo *pb.SnapshotWithType, id string) {
	source := converter.GetSourceDetail(name, path)
	if mo.Snapshot.Data.Details == nil || mo.Snapshot.Data.Details.Fields == nil {
		mo.Snapshot.Data.Details = &types.Struct{Fields: map[string]*types.Value{}}
	}
	mo.Snapshot.Data.Details.Fields[bundle.RelationKeySource.String()] = pbtypes.String(source)
	if id := pbtypes.GetString(mo.Snapshot.Data.Details, bundle.RelationKeyId.String()); id != "" {
		mo.Snapshot.Data.Details.Fields[bundle.RelationKeyOldAnytypeID.String()] = pbtypes.String(id)
	}
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

func (p *Pb) readFile(importPath string, mode string) (map[string]*converter.IOReader, *pb.Profile, converter.ConvertError) {
	files := make(map[string]*converter.IOReader)
	r, err := zip.OpenReader(importPath)
	if err != nil {
		return p.handleFile(importPath, files)
	}
	pr, convertError := p.handleZipArchive(r, mode, files)
	return files, pr, convertError
}

func (p *Pb) handleZipArchive(r *zip.ReadCloser, mode string, files map[string]*converter.IOReader) (*pb.Profile, converter.ConvertError) {
	errors := converter.NewError()
	var (
		pr  *pb.Profile
		err error
	)
	for _, f := range r.File {
		if filepath.Base(f.Name) == constant.ProfileFile {
			pr, err = p.getProfile(f)
			if err != nil {
				errors.Add(constant.ProfileFile, err)
				if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING.String() {
					return nil, errors
				}
			}
			continue
		}
		if !(filepath.Ext(f.Name) == ".pb" || filepath.Ext(f.Name) == ".json") {
			continue
		}
		shortPath := filepath.Clean(f.Name)
		rc, fErr := f.Open()
		if fErr != nil {
			errors.Add(constant.ProfileFile, err)
			if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING.String() {
				return nil, errors
			}
		}
		files[shortPath] = &converter.IOReader{
			Name:   f.FileInfo().Name(),
			Reader: rc,
		}
	}
	return pr, nil
}

func (p *Pb) getProfile(f *zip.File) (*pb.Profile, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	pr, err := p.readProfileFile(rc)
	if err != nil {
		return nil, err
	}

	return pr, nil
}

func (p *Pb) handleFile(importPath string, files map[string]*converter.IOReader) (map[string]*converter.IOReader, *pb.Profile, converter.ConvertError) {
	errors := converter.NewError()
	f, err := os.Open(importPath)
	if err != nil {
		errors.Add(importPath, err)
		return nil, nil, errors
	}
	var pr *pb.Profile
	if filepath.Base(f.Name()) == constant.ProfileFile {
		pr, err = p.readProfileFile(f)
		if err != nil {
			errors.Add(importPath, err)
			return nil, nil, errors
		}
	}
	if !(filepath.Ext(f.Name()) == ".pb" || filepath.Ext(f.Name()) == ".json") {
		return nil, nil, nil
	}
	name := filepath.Clean(f.Name())
	files[name] = &converter.IOReader{
		Name:   f.Name(),
		Reader: f,
	}
	return files, pr, nil
}

func (p *Pb) updateLinksToObjects(snapshots []*converter.Snapshot, allErrors converter.ConvertError, mode pb.RpcObjectImportRequestMode) {
	newIDToOld := make(map[string]string, len(snapshots))
	for _, snapshot := range snapshots {
		id := pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeyId.String())
		newIDToOld[id] = snapshot.Id
	}

	for _, snapshot := range snapshots {
		st := state.NewDocFromSnapshot("", snapshot.Snapshot)
		err := converter.UpdateLinksToObjects(st.(*state.State), newIDToOld, snapshot.Id)
		if err != nil {
			allErrors.Add(snapshot.FileName, err)
			if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return
			}
			continue
		}
		converter.UpdateRelationsIDs(st.(*state.State), snapshot.Id, newIDToOld)
		converter.UpdateObjectType(newIDToOld, st.(*state.State))
		snapshot.Snapshot.Data.Blocks = st.Blocks()
		snapshot.Snapshot.Data.ObjectTypes = st.ObjectTypes()
	}
}

func (p *Pb) updateDetails(snapshots []*converter.Snapshot) {
	localRelationsToAdd := make([]string, 0, len(bundle.LocalRelationsKeys))
	for _, key := range bundle.LocalRelationsKeys {
		if key == bundle.RelationKeyIsFavorite.String() || key == bundle.RelationKeyIsArchived.String() {
			continue
		}
		localRelationsToAdd = append(localRelationsToAdd, key)
	}
	for _, snapshot := range snapshots {
		details := pbtypes.StructCutKeys(snapshot.Snapshot.Data.Details, append(bundle.DerivedRelationsKeys, localRelationsToAdd...))
		snapshot.Snapshot.Data.Details = details
	}
}

func (p *Pb) GetSnapshot(rd io.ReadCloser, name string, needToCreateWidget bool) (*pb.SnapshotWithType, error) {
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
	return snapshot, nil
}

func (p *Pb) readProfileFile(f io.ReadCloser) (*pb.Profile, error) {
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

func (p *Pb) needToImportWidgets(address string, accountID string) bool {
	return address == accountID
}
