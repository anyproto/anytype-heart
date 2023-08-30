package pb

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/anyproto/any-sync/util/slice"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/types"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"golang.org/x/exp/rand"

	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	widgets "github.com/anyproto/anytype-heart/core/block/editor/widget"
	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/import/source"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/typeprovider"
	"github.com/anyproto/anytype-heart/util/constant"
	"github.com/anyproto/anytype-heart/util/pbtypes"
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
	iconOption  int64
}

func New(service *collection.Service, sbtProvider typeprovider.SmartBlockTypeProvider, core core.Service) converter.Converter {
	return &Pb{
		service:     service,
		sbtProvider: sbtProvider,
		core:        core,
	}
}

func (p *Pb) GetSnapshots(req *pb.RpcObjectImportRequest, progress process.Progress) (*converter.Response, *converter.ConvertError) {
	params, e := p.getParams(req.Params)
	if e != nil || params == nil {
		return nil, converter.NewFromError(fmt.Errorf("wrong parameters"))
	}
	allSnapshots, widgetSnapshot, allErrors := p.getSnapshots(req, progress, params.GetPath(), req.IsMigration)
	oldToNewID := p.updateLinksToObjects(allSnapshots, allErrors, req.Mode)
	p.updateDetails(allSnapshots)
	if p.shouldReturnError(req, allErrors, params) {
		return nil, allErrors
	}
	if !params.GetNoCollection() {
		rootCol, colErr := p.provideRootCollection(allSnapshots, widgetSnapshot, oldToNewID)
		if colErr != nil {
			allErrors.Add(colErr)
			if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return nil, allErrors
			}
		}
		if rootCol != nil {
			allSnapshots = append(allSnapshots, rootCol)
		}
	}
	progress.SetTotal(int64(len(allSnapshots)))
	if allErrors.IsEmpty() {
		return &converter.Response{Snapshots: allSnapshots}, nil
	}
	return &converter.Response{Snapshots: allSnapshots}, allErrors
}

func (p *Pb) Name() string {
	return Name
}

func (p *Pb) getParams(params pb.IsRpcObjectImportRequestParams) (*pb.RpcObjectImportRequestPbParams, error) {
	if p, ok := params.(*pb.RpcObjectImportRequestParamsOfPbParams); ok {
		return p.PbParams, nil
	}
	return nil, errors.New("PB: getParams wrong parameters format")
}

func (p *Pb) getSnapshots(req *pb.RpcObjectImportRequest,
	progress process.Progress,
	allPaths []string,
	isMigration bool) ([]*converter.Snapshot, *converter.Snapshot, *converter.ConvertError) {
	allSnapshots := make([]*converter.Snapshot, 0)
	allErrors := converter.NewError()
	var widgetSnapshot *converter.Snapshot
	for _, path := range allPaths {
		if err := progress.TryStep(1); err != nil {
			return nil, nil, converter.NewCancelError(err)
		}
		snapshots, widget := p.handlePath(req, path, allErrors, isMigration)
		if !allErrors.IsEmpty() && req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, nil, allErrors
		}
		allSnapshots = append(allSnapshots, snapshots...)
		widgetSnapshot = widget
	}
	return allSnapshots, widgetSnapshot, allErrors
}

func (p *Pb) handlePath(req *pb.RpcObjectImportRequest,
	path string,
	allErrors *converter.ConvertError,
	isMigration bool) ([]*converter.Snapshot, *converter.Snapshot) {
	files, err := p.readFile(path)
	if err != nil {
		allErrors.Add(err)
		if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING || errors.Is(err, converter.ErrNoObjectsToImport) {
			return nil, nil
		}
	}
	var (
		profileID           string
		needToImportWidgets bool
	)
	profile, err := p.getProfileFromFiles(files)
	if err != nil {
		allErrors.Add(err)
		if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, nil
		}
	}
	if profile != nil {
		pr, e := p.core.LocalProfile()
		if e != nil {
			allErrors.Add(e)
			if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return nil, nil
			}
		}
		needToImportWidgets = p.needToImportWidgets(profile.Address, pr.AccountAddr)
		profileID = profile.ProfileId
	}
	snapshots, widget := p.getSnapshotsFromProvidedFiles(req, files, allErrors, path, profileID, needToImportWidgets, isMigration)
	if !allErrors.IsEmpty() && req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
		return nil, nil
	}
	return snapshots, widget
}

func (p *Pb) readFile(importPath string) (map[string]io.ReadCloser, error) {
	s := source.GetSource(importPath)
	if s == nil {
		return nil, fmt.Errorf("failed to identify source")
	}
	readers, err := s.GetFileReaders(importPath, []string{".pb", ".json"}, []string{constant.ProfileFile, configFile})
	if err != nil {
		return nil, err
	}
	if len(readers) == 0 {
		return nil, converter.ErrNoObjectsToImport
	}
	return readers, nil
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

func (p *Pb) needToImportWidgets(address, accountID string) bool {
	return address == accountID
}

func (p *Pb) getSnapshotsFromProvidedFiles(req *pb.RpcObjectImportRequest,
	pbFiles map[string]io.ReadCloser,
	allErrors *converter.ConvertError, path, profileID string,
	needToImportWidgets, isMigration bool) ([]*converter.Snapshot, *converter.Snapshot) {
	allSnapshots := make([]*converter.Snapshot, 0)
	var widgetSnapshot *converter.Snapshot
	for name, file := range pbFiles {
		snapshot, err := p.makeSnapshot(name, profileID, path, file, isMigration)
		if err != nil {
			allErrors.Add(err)
			if req.GetMode() == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return nil, nil
			}
			continue
		}
		if snapshot != nil {
			if p.shouldImportSnapshot(snapshot, needToImportWidgets) {
				allSnapshots = append(allSnapshots, snapshot)
			}
			if snapshot.SbType == smartblock.SmartBlockTypeWidget {
				widgetSnapshot = snapshot
			}
		}
	}
	return allSnapshots, widgetSnapshot
}

func (p *Pb) makeSnapshot(name, profileID, path string, file io.ReadCloser, isMigration bool) (*converter.Snapshot, error) {
	if name == constant.ProfileFile || name == configFile {
		return nil, nil
	}
	snapshot, errGS := p.getSnapshotFromFile(file, name)
	file.Close()
	if errGS != nil {
		return nil, errGS
	}
	if snapshot == nil {
		return nil, nil
	}
	id := uuid.New().String()
	id = p.normalizeSnapshot(snapshot, id, profileID, isMigration)
	p.injectImportDetails(name, path, snapshot)
	return &converter.Snapshot{
		Id:       id,
		SbType:   smartblock.SmartBlockType(snapshot.SbType),
		FileName: name,
		Snapshot: snapshot.Snapshot,
	}, nil
}

func (p *Pb) getSnapshotFromFile(rd io.ReadCloser, name string) (*pb.SnapshotWithType, error) {
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
	return snapshot, nil
}

func (p *Pb) normalizeSnapshot(snapshot *pb.SnapshotWithType, id string, profileID string, isMigration bool) string {
	if _, ok := model.SmartBlockType_name[int32(snapshot.SbType)]; !ok {
		newSbType := model.SmartBlockType_Page
		if int32(snapshot.SbType) == 96 { // fallback for objectType smartblocktype
			newSbType = model.SmartBlockType_SubObject
		}
		snapshot.SbType = newSbType
	}
	if snapshot.SbType == model.SmartBlockType_SubObject {
		id = p.getIDForSubObject(snapshot, id)
	}
	if snapshot.SbType == model.SmartBlockType_ProfilePage {
		id = p.getIDForUserProfile(snapshot, profileID, id, isMigration)
		p.setProfileIconOption(snapshot, profileID)
	}
	if snapshot.SbType == model.SmartBlockType_Page {
		p.cleanupEmptyBlock(snapshot)
	}
	return id
}

// getIDForSubObject preserves original id from snapshot for relations and object types
func (p *Pb) getIDForSubObject(sn *pb.SnapshotWithType, id string) string {
	originalID := pbtypes.GetString(sn.Snapshot.Data.Details, bundle.RelationKeyId.String())
	if strings.HasPrefix(originalID, addr.ObjectTypeKeyToIdPrefix) || strings.HasPrefix(originalID, addr.RelationKeyToIdPrefix) {
		return originalID
	}
	return id
}

func (p *Pb) getIDForUserProfile(mo *pb.SnapshotWithType, profileID string, id string, isMigration bool) string {
	objectID := pbtypes.GetString(mo.Snapshot.Data.Details, bundle.RelationKeyId.String())
	if objectID == profileID && isMigration {
		return p.core.ProfileID()
	}
	return id
}

func (p *Pb) setProfileIconOption(mo *pb.SnapshotWithType, profileID string) {
	objectID := pbtypes.GetString(mo.Snapshot.Data.Details, bundle.RelationKeyId.String())
	if objectID != profileID {
		return
	}
	mo.Snapshot.Data.Details.Fields[bundle.RelationKeyIconOption.String()] = pbtypes.Int64(p.getIconOption())
}

func (p *Pb) getIconOption() int64 {
	if p.iconOption == 0 {
		p.iconOption = int64(rand.Intn(16) + 1)
	}
	return p.iconOption
}

// cleanupEmptyBlockMigration is fixing existing pages, imported from Notion
func (p *Pb) cleanupEmptyBlock(snapshot *pb.SnapshotWithType) {
	var (
		emptyBlock *model.Block
	)

	for _, block := range snapshot.Snapshot.Data.Blocks {
		if block.Content == nil {
			emptyBlock = block
		} else if block.GetSmartblock() != nil {
			return
		}
	}
	if emptyBlock != nil {
		emptyBlock.Content = &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}
	}
}

func (p *Pb) injectImportDetails(name string, path string, mo *pb.SnapshotWithType) {
	if mo.Snapshot.Data.Details == nil || mo.Snapshot.Data.Details.Fields == nil {
		mo.Snapshot.Data.Details = &types.Struct{Fields: map[string]*types.Value{}}
	}
	if id := pbtypes.GetString(mo.Snapshot.Data.Details, bundle.RelationKeyId.String()); id != "" {
		mo.Snapshot.Data.Details.Fields[bundle.RelationKeyOldAnytypeID.String()] = pbtypes.String(id)
	}
	sourceDetail := converter.GetSourceDetail(name, path)
	mo.Snapshot.Data.Details.Fields[bundle.RelationKeySourceFilePath.String()] = pbtypes.String(sourceDetail)

	createdDate := pbtypes.GetInt64(mo.Snapshot.Data.Details, bundle.RelationKeyCreatedDate.String())
	if createdDate == 0 {
		mo.Snapshot.Data.Details.Fields[bundle.RelationKeyCreatedDate.String()] = pbtypes.Int64(time.Now().Unix())
	}
}

func (p *Pb) shouldImportSnapshot(snapshot *converter.Snapshot, needToImportWidgets bool) bool {
	return snapshot.SbType != smartblock.SmartBlockTypeWidget || (snapshot.SbType == smartblock.SmartBlockTypeWidget && needToImportWidgets)
}

func (p *Pb) updateLinksToObjects(snapshots []*converter.Snapshot, allErrors *converter.ConvertError, mode pb.RpcObjectImportRequestMode) map[string]string {
	oldToNewID := make(map[string]string, len(snapshots))
	fileIDs := make([]string, 0)
	for _, snapshot := range snapshots {
		id := pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeyId.String())
		oldToNewID[id] = snapshot.Id
		fileIDs = append(fileIDs, lo.Map(snapshot.Snapshot.GetFileKeys(), func(item *pb.ChangeFileKeys, index int) string {
			return item.Hash
		})...)
	}
	for _, snapshot := range snapshots {
		st := state.NewDocFromSnapshot("", snapshot.Snapshot)
		err := converter.UpdateLinksToObjects(st.(*state.State), oldToNewID, fileIDs)
		if err != nil {
			allErrors.Add(err)
			if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return nil
			}
			continue
		}
		converter.UpdateObjectIDsInRelations(st.(*state.State), oldToNewID, fileIDs)
		converter.UpdateObjectType(oldToNewID, st.(*state.State))
		p.updateObjectsIDsInCollection(st.(*state.State), oldToNewID)
		p.updateSnapshot(snapshot, st.(*state.State))
	}
	return oldToNewID
}

func (p *Pb) updateSnapshot(snapshot *converter.Snapshot, st *state.State) {
	snapshot.Snapshot.Data.Details = pbtypes.StructMerge(snapshot.Snapshot.Data.Details, st.CombinedDetails(), false)
	snapshot.Snapshot.Data.Blocks = st.Blocks()
	snapshot.Snapshot.Data.ObjectTypes = st.ObjectTypes()
	snapshot.Snapshot.Data.Collections = st.Store()
}

func (p *Pb) updateDetails(snapshots []*converter.Snapshot) {
	removeKeys := make([]string, 0, len(bundle.LocalRelationsKeys)+len(bundle.DerivedRelationsKeys))
	removeKeys = slice.Filter(removeKeys, func(key string) bool {
		// preserve some keys we have special cases for
		return key != bundle.RelationKeyIsFavorite.String() &&
			key != bundle.RelationKeyIsArchived.String() &&
			key != bundle.RelationKeyCreatedDate.String() &&
			key != bundle.RelationKeyLastModifiedDate.String()
	})

	for _, snapshot := range snapshots {
		details := pbtypes.StructCutKeys(snapshot.Snapshot.Data.Details, removeKeys)
		snapshot.Snapshot.Data.Details = details
	}
}

func (p *Pb) updateObjectsIDsInCollection(st *state.State, newToOldIDs map[string]string) {
	objectsInCollections := st.GetStoreSlice(template.CollectionStoreKey)
	for i, id := range objectsInCollections {
		if newID, ok := newToOldIDs[id]; ok {
			objectsInCollections[i] = newID
		}
	}
	if len(objectsInCollections) != 0 {
		st.UpdateStoreSlice(template.CollectionStoreKey, objectsInCollections)
	}
}

func (p *Pb) shouldReturnError(req *pb.RpcObjectImportRequest, allErrors *converter.ConvertError, params *pb.RpcObjectImportRequestPbParams) bool {
	return (!allErrors.IsEmpty() && req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING) ||
		allErrors.IsNoObjectToImportError(len(params.GetPath()))
}

func (p *Pb) provideRootCollection(allObjects []*converter.Snapshot, widget *converter.Snapshot, oldToNewID map[string]string) (*converter.Snapshot, error) {
	var (
		rootObjects         []string
		widgetFlags         widgets.ImportWidgetFlags
		objectsNotInWidgets []*converter.Snapshot
	)
	if widget != nil {
		widgetFlags, rootObjects = p.getObjectsFromWidgets(widget, oldToNewID)
		objectsNotInWidgets = lo.Filter(allObjects, func(item *converter.Snapshot, index int) bool {
			return !lo.Contains(rootObjects, item.Id)
		})
	}
	if !widgetFlags.IsEmpty() || len(rootObjects) > 0 {
		// add to root collection only objects from widgets, dashboard and favorites
		rootObjects = append(rootObjects, p.filterObjects(widgetFlags, objectsNotInWidgets)...)
	} else {
		// if we don't have any widget, we add everything (except sub objects and templates) to root collection
		rootObjects = lo.FilterMap(allObjects, func(item *converter.Snapshot, index int) (string, bool) {
			if item.SbType != smartblock.SmartBlockTypeSubObject && item.SbType != smartblock.SmartBlockTypeTemplate {
				return item.Id, true
			}
			return item.Id, false
		})
	}
	rootCollection := converter.NewRootCollection(p.service)
	rootCol, colErr := rootCollection.MakeRootCollection(rootCollectionName, rootObjects)
	return rootCol, colErr
}

func (p *Pb) getObjectsFromWidgets(widgetSnapshot *converter.Snapshot, oldToNewID map[string]string) (widgets.ImportWidgetFlags, []string) {
	widgetState := state.NewDocFromSnapshot("", widgetSnapshot.Snapshot).(*state.State)
	var (
		objectsInWidget     []string
		objectTypesToImport widgets.ImportWidgetFlags
	)
	err := widgetState.Iterate(func(b simple.Block) (isContinue bool) {
		if link := b.Model().GetLink(); link != nil && link.TargetBlockId != "" {
			if builtinWidget := widgets.FillImportFlags(link, &objectTypesToImport); builtinWidget {
				return true
			}
			if newID, objectExist := oldToNewID[link.TargetBlockId]; objectExist {
				objectsInWidget = append(objectsInWidget, newID)
			}
		}
		return true
	})
	if err != nil {
		return widgets.ImportWidgetFlags{}, nil
	}
	return objectTypesToImport, objectsInWidget
}

func (p *Pb) filterObjects(objectTypesToImport widgets.ImportWidgetFlags, objectsNotInWidget []*converter.Snapshot) []string {
	var rootObjects []string
	for _, snapshot := range objectsNotInWidget {
		if snapshot.SbType == smartblock.SmartBlockTypeSubObject || snapshot.SbType == smartblock.SmartBlockTypeTemplate {
			continue
		}
		if objectTypesToImport.ImportCollection && lo.Contains(snapshot.Snapshot.Data.ObjectTypes, bundle.TypeKeyCollection.URL()) {
			rootObjects = append(rootObjects, snapshot.Id)
			continue
		}
		if objectTypesToImport.ImportSet && lo.Contains(snapshot.Snapshot.Data.ObjectTypes, bundle.TypeKeySet.URL()) {
			rootObjects = append(rootObjects, snapshot.Id)
			continue
		}
		if pbtypes.GetBool(snapshot.Snapshot.Data.Details, bundle.RelationKeyIsFavorite.String()) {
			rootObjects = append(rootObjects, snapshot.Id)
			continue
		}
		if spaceDashboardID := pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeySpaceDashboardId.String()); spaceDashboardID != "" {
			rootObjects = append(rootObjects, spaceDashboardID)
			continue
		}
	}
	return rootObjects
}
