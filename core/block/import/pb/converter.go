package pb

import (
	"fmt"
	"io"
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
	"github.com/anytypeio/go-anytype-middleware/core/block/import/source"
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
		return nil, converter.NewFromError("", fmt.Errorf("wrong parameters"))
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
		if err := progress.TryStep(1); err != nil {
			ce := converter.NewFromError(path, err)
			return nil, nil, ce
		}
		snapshots, objects := p.handlePath(req, path, allErrors)
		allSnapshots = append(allSnapshots, snapshots...)
		targetObjects = append(targetObjects, objects...)
	}
	return allSnapshots, targetObjects, allErrors
}

func (p *Pb) handlePath(req *pb.RpcObjectImportRequest, path string, allErrors converter.ConvertError) ([]*converter.Snapshot, []string) {
	files, err := p.readFile(path)
	if err != nil {
		allErrors.Add(path, err)
		if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, nil
		}
	}
	if len(files) == 0 {
		return nil, nil
	}
	var (
		needToImportWidgets bool
		profileID           string
	)
	profile, err := p.getProfileFromFiles(files)
	if err != nil {
		allErrors.Add(constant.ProfileFile, err)
		if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, nil
		}
	}
	if profile != nil {
		pr, e := p.core.LocalProfile()
		if e != nil {
			allErrors.Add(constant.ProfileFile, e)
			if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return nil, nil
			}
		}
		needToImportWidgets = p.needToImportWidgets(profile.Address, pr.AccountAddr)
		profileID = profile.ProfileId
	}
	snapshots, objects := p.getSnapshotsFromFiles(req, files, allErrors, path, profileID, needToImportWidgets)
	if !allErrors.IsEmpty() && req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
		return nil, nil
	}
	p.setDashboardID(profile, snapshots)
	return snapshots, objects
}

func (p *Pb) getProfileFromFiles(files map[string]io.ReadCloser) (*pb.Profile, error) {
	var (
		profile *pb.Profile
		err     error
	)
	for name, f := range files {
		if filepath.Base(name) == constant.ProfileFile {
			profile, err = p.readProfileFile(f)
			if err != nil {
				return nil, err
			}
			break
		}
	}
	return profile, nil
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
	pbFiles map[string]io.ReadCloser,
	allErrors converter.ConvertError,
	path, profileID string,
	needToCreateWidgets bool) ([]*converter.Snapshot, []string) {
	targetObjects := make([]string, 0)
	allSnapshots := make([]*converter.Snapshot, 0)
	for name, file := range pbFiles {
		snapshot, err := p.getSnapshotForPbFile(name, file, needToCreateWidgets, profileID, path)
		if err != nil {
			allErrors.Add(name, err)
			if req.GetMode() == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return nil, nil
			}
			continue
		}
		if snapshot != nil {
			allSnapshots = append(allSnapshots, snapshot)
			// not add sub objects to root collection
			if snapshot.SbType == smartblock.SmartBlockTypeSubObject {
				continue
			}
			targetObjects = append(targetObjects, snapshot.Id)
		}
	}
	return allSnapshots, targetObjects
}

func (p *Pb) getSnapshotForPbFile(name string, file io.ReadCloser, needToCreateWidgets bool, profileID string, path string) (*converter.Snapshot, error) {
	if name == constant.ProfileFile || name == configFile {
		return nil, nil
	}
	id := uuid.New().String()
	mo, errGS := p.GetSnapshot(file, name, needToCreateWidgets)
	file.Close()
	if errGS != nil {
		return nil, errGS
	}
	if mo == nil {
		return nil, nil
	}
	if mo.SbType == model.SmartBlockType_ProfilePage {
		id = p.getIDForUserProfile(mo, profileID, id)
	}
	p.fillDetails(name, path, mo)
	return &converter.Snapshot{
		Id:       id,
		SbType:   smartblock.SmartBlockType(mo.SbType),
		FileName: name,
		Snapshot: mo.Snapshot,
	}, nil
}

func (p *Pb) getIDForUserProfile(mo *pb.SnapshotWithType, profileID string, id string) string {
	objectID := pbtypes.GetString(mo.Snapshot.Data.Details, bundle.RelationKeyId.String())
	if objectID == profileID {
		return p.core.ProfileID()
	}
	return id
}

func (p *Pb) fillDetails(name string, path string, mo *pb.SnapshotWithType) {
	sourceDetail := converter.GetSourceDetail(name, path)
	if mo.Snapshot.Data.Details == nil || mo.Snapshot.Data.Details.Fields == nil {
		mo.Snapshot.Data.Details = &types.Struct{Fields: map[string]*types.Value{}}
	}
	mo.Snapshot.Data.Details.Fields[bundle.RelationKeySource.String()] = pbtypes.String(sourceDetail)
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

func (p *Pb) readFile(importPath string) (map[string]io.ReadCloser, error) {
	s := source.GetSource(importPath)
	if s == nil {
		return nil, fmt.Errorf("failed to identify source")
	}
	readers, err := s.GetFileReaders(importPath, []string{".pb", ".json", ""})
	if err != nil {
		return nil, err
	}
	return readers, nil
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
		snapshot.Snapshot.Data.Blocks = st.Blocks()
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
	data, err := io.ReadAll(rd)
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
	defer f.Close()
	profile := &pb.Profile{}
	data, err := io.ReadAll(f)
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
