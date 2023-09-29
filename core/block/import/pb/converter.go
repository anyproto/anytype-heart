package pb

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

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
	"github.com/anyproto/anytype-heart/util/slice"
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

func (p *Pb) GetSnapshots(ctx context.Context, req *pb.RpcObjectImportRequest, progress process.Progress) (*converter.Response, *converter.ConvertError) {
	params, e := p.getParams(req.Params)
	if e != nil || params == nil {
		return nil, converter.NewFromError(fmt.Errorf("wrong parameters"), req.Mode)
	}
	allErrors := converter.NewError(req.Mode)
	allSnapshots, widgetSnapshot := p.getSnapshots(req.SpaceId, progress, params.GetPath(), req.IsMigration, allErrors)
	oldToNewID := p.updateLinksToObjects(allSnapshots, allErrors, len(params.GetPath()))
	p.updateDetails(allSnapshots)
	if allErrors.ShouldAbortImport(len(params.GetPath()), req.Type) {
		return nil, allErrors
	}
	var rootCollectionID string
	if !params.GetNoCollection() {
		rootCollection, colErr := p.provideRootCollection(allSnapshots, widgetSnapshot, oldToNewID)
		if colErr != nil {
			allErrors.Add(colErr)
			if allErrors.ShouldAbortImport(len(params.GetPath()), req.Type) {
				return nil, allErrors
			}
		}
		if rootCollection != nil {
			allSnapshots = append(allSnapshots, rootCollection)
			rootCollectionID = rootCollection.Id
		}
	}
	progress.SetTotal(int64(len(allSnapshots)))
	if allErrors.IsEmpty() {
		return &converter.Response{Snapshots: allSnapshots, RootCollectionID: rootCollectionID}, nil
	}
	return &converter.Response{Snapshots: allSnapshots, RootCollectionID: rootCollectionID}, allErrors
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

func (p *Pb) getSnapshots(
	spaceID string,
	progress process.Progress,
	allPaths []string,
	isMigration bool,
	allErrors *converter.ConvertError,
) ([]*converter.Snapshot, *converter.Snapshot) {
	allSnapshots := make([]*converter.Snapshot, 0)
	var widgetSnapshot *converter.Snapshot
	for _, path := range allPaths {
		if err := progress.TryStep(1); err != nil {
			allErrors.Add(converter.ErrCancel)
			return nil, nil
		}
		snapshots, widget := p.handleImportPath(spaceID, len(path), path, allErrors, isMigration)
		if allErrors.ShouldAbortImport(len(allPaths), pb.RpcObjectImportRequest_Pb) {
			return nil, nil
		}
		allSnapshots = append(allSnapshots, snapshots...)
		widgetSnapshot = widget
	}
	return allSnapshots, widgetSnapshot
}

func (p *Pb) handleImportPath(
	spaceID string,
	pathCount int,
	path string,
	allErrors *converter.ConvertError,
	isMigration bool) ([]*converter.Snapshot, *converter.Snapshot) {
	importSource := source.GetSource(path)
	defer importSource.Close()
	err := p.extractFiles(path, importSource)
	if err != nil {
		allErrors.Add(err)
		if allErrors.ShouldAbortImport(pathCount, pb.RpcObjectImportRequest_Pb) {
			return nil, nil
		}
	}
	var (
		profileID           string
		needToImportWidgets bool
	)
	profile, err := p.getProfileFromFiles(importSource)
	if err != nil {
		allErrors.Add(err)
		if allErrors.ShouldAbortImport(pathCount, pb.RpcObjectImportRequest_Pb) {
			return nil, nil
		}
	}
	if profile != nil {
		pr, e := p.core.LocalProfile(spaceID)
		if e != nil {
			allErrors.Add(e)
			if allErrors.ShouldAbortImport(pathCount, pb.RpcObjectImportRequest_Pb) {
				return nil, nil
			}
		}
		needToImportWidgets = p.needToImportWidgets(profile.Address, pr.AccountAddr)
		profileID = profile.ProfileId
	}
	return p.getSnapshotsFromProvidedFiles(spaceID, pathCount, importSource, allErrors, path, profileID, needToImportWidgets, isMigration)
}

func (p *Pb) extractFiles(importPath string, importSource source.Source) error {
	err := importSource.Initialize(importPath)
	if err != nil {
		return err
	}
	if importSource.CountFilesWithGivenExtensions([]string{".pb", ".json"}) == 0 {
		return converter.ErrNoObjectsToImport
	}
	return nil
}

func (p *Pb) getProfileFromFiles(importSource source.Source) (*pb.Profile, error) {
	var (
		profile *pb.Profile
		err     error
	)
	iterateError := importSource.Iterate(func(fileName string, fileReader io.ReadCloser) (isContinue bool) {
		if filepath.Base(fileName) == constant.ProfileFile {
			profile, err = p.readProfileFile(fileReader)
			return false
		}
		return true
	})
	if iterateError != nil {
		return nil, iterateError
	}
	return profile, err
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

func (p *Pb) getSnapshotsFromProvidedFiles(
	spaceID string,
	pathCount int,
	pbFiles source.Source,
	allErrors *converter.ConvertError,
	path, profileID string,
	needToImportWidgets, isMigration bool,
) ([]*converter.Snapshot, *converter.Snapshot) {
	allSnapshots := make([]*converter.Snapshot, 0)
	var widgetSnapshot *converter.Snapshot
	if iterateErr := pbFiles.Iterate(func(fileName string, fileReader io.ReadCloser) (isContinue bool) {
		snapshot, err := p.makeSnapshot(spaceID, fileName, profileID, path, fileReader, isMigration)
		if err != nil {
			allErrors.Add(err)
			if allErrors.ShouldAbortImport(pathCount, pb.RpcObjectImportRequest_Pb) {
				return false
			}
		}
		if snapshot != nil {
			if p.shouldImportSnapshot(snapshot, needToImportWidgets) {
				allSnapshots = append(allSnapshots, snapshot)
			}
			if snapshot.SbType == smartblock.SmartBlockTypeWidget {
				widgetSnapshot = snapshot
			}
		}
		return true
	}); iterateErr != nil {
		allErrors.Add(iterateErr)
	}
	return allSnapshots, widgetSnapshot
}

func (p *Pb) makeSnapshot(spaceID string, name, profileID, path string, file io.ReadCloser, isMigration bool) (*converter.Snapshot, error) {
	if name == constant.ProfileFile || name == configFile {
		return nil, nil
	}
	snapshot, errGS := p.getSnapshotFromFile(file, name)
	if errGS != nil {
		return nil, errGS
	}
	if snapshot == nil {
		return nil, nil
	}
	id := uuid.New().String()
	id, err := p.normalizeSnapshot(spaceID, snapshot, id, profileID, isMigration)
	if err != nil {
		return nil, fmt.Errorf("normalize snapshot: %w", err)
	}
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
	if filepath.Ext(name) == ".json" {
		snapshot := &pb.SnapshotWithType{}
		um := jsonpb.Unmarshaler{}
		if uErr := um.Unmarshal(rd, snapshot); uErr != nil {
			return nil, fmt.Errorf("PB:GetSnapshot %s", uErr)
		}
		return snapshot, nil
	}
	if filepath.Ext(name) == ".pb" {
		snapshot := &pb.SnapshotWithType{}
		data, err := io.ReadAll(rd)
		if err != nil {
			return nil, fmt.Errorf("PB:GetSnapshot %s", err)
		}
		if err = snapshot.Unmarshal(data); err != nil {
			return nil, fmt.Errorf("PB:GetSnapshot %s", err)
		}
		return snapshot, nil
	}
	return nil, nil
}

func (p *Pb) normalizeSnapshot(spaceID string, snapshot *pb.SnapshotWithType, id string, profileID string, isMigration bool) (string, error) {
	if _, ok := model.SmartBlockType_name[int32(snapshot.SbType)]; !ok {
		newSbType := model.SmartBlockType_Page
		if int32(snapshot.SbType) == 96 { // fallback for objectType smartblocktype
			newSbType = model.SmartBlockType_SubObject
		}
		snapshot.SbType = newSbType
	}

	if snapshot.SbType == model.SmartBlockType_SubObject {
		// migrate old sub objects into real objects
		if snapshot.Snapshot.Data.ObjectTypes[0] == addr.ObjectTypeKeyToIdPrefix+model.ObjectType_objectType.String() {
			snapshot.SbType = model.SmartBlockType_STType
		} else if snapshot.Snapshot.Data.ObjectTypes[0] == addr.ObjectTypeKeyToIdPrefix+model.ObjectType_relation.String() {
			snapshot.SbType = model.SmartBlockType_STRelation
		} else if snapshot.Snapshot.Data.ObjectTypes[0] == addr.ObjectTypeKeyToIdPrefix+model.ObjectType_relationOption.String() {
			snapshot.SbType = model.SmartBlockType_Page
		} else {
			return "", fmt.Errorf("unknown sub object type %s", snapshot.Snapshot.Data.ObjectTypes[0])
		}
	}

	if snapshot.SbType == model.SmartBlockType_ProfilePage {
		id = p.getIDForUserProfile(spaceID, snapshot, profileID, id, isMigration)
		p.setProfileIconOption(snapshot, profileID)
	}
	if snapshot.SbType == model.SmartBlockType_Page {
		p.cleanupEmptyBlock(snapshot)
	}
	return id, nil
}

// getIDForSubObject preserves original id from snapshot for relations and object types
func (p *Pb) getIDForSubObject(sn *pb.SnapshotWithType, id string) string {
	originalID := pbtypes.GetString(sn.Snapshot.Data.Details, bundle.RelationKeyId.String())
	if strings.HasPrefix(originalID, addr.ObjectTypeKeyToIdPrefix) || strings.HasPrefix(originalID, addr.RelationKeyToIdPrefix) {
		return originalID
	}
	return id
}

func (p *Pb) getIDForUserProfile(spaceID string, mo *pb.SnapshotWithType, profileID string, id string, isMigration bool) string {
	objectID := pbtypes.GetString(mo.Snapshot.Data.Details, bundle.RelationKeyId.String())
	if objectID == profileID && isMigration {
		return p.core.ProfileID(spaceID)
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

func (p *Pb) updateLinksToObjects(snapshots []*converter.Snapshot, allErrors *converter.ConvertError, pathCount int) map[string]string {
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
		st := state.NewDocFromSnapshot("", snapshot.Snapshot, state.WithUniqueKeyMigration(snapshot.SbType))
		err := converter.UpdateLinksToObjects(st.(*state.State), oldToNewID, fileIDs)
		if err != nil {
			allErrors.Add(err)
			if allErrors.ShouldAbortImport(pathCount, pb.RpcObjectImportRequest_Pb) {
				return nil
			}
			continue
		}
		converter.UpdateObjectIDsInRelations(st.(*state.State), oldToNewID, fileIDs)
		// TODO Fix
		// converter.UpdateObjectType(oldToNewID, st.(*state.State))
		p.updateObjectsIDsInCollection(st.(*state.State), oldToNewID)
		p.updateSnapshot(snapshot, st.(*state.State))
	}
	return oldToNewID
}

func (p *Pb) updateSnapshot(snapshot *converter.Snapshot, st *state.State) {
	snapshot.Snapshot.Data.Details = pbtypes.StructMerge(snapshot.Snapshot.Data.Details, st.CombinedDetails(), false)
	snapshot.Snapshot.Data.Blocks = st.Blocks()
	snapshot.Snapshot.Data.ObjectTypes = slice.UnwrapStrings(st.ObjectTypeKeys())
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
			if item.SbType != smartblock.SmartBlockTypeSubObject && item.SbType != smartblock.SmartBlockTypeTemplate &&
				item.SbType != smartblock.SmartBlockTypeRelation && item.SbType != smartblock.SmartBlockTypeObjectType {
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
		if snapshot.SbType == smartblock.SmartBlockTypeSubObject || snapshot.SbType == smartblock.SmartBlockTypeTemplate ||
			snapshot.SbType == smartblock.SmartBlockTypeRelation || snapshot.SbType == smartblock.SmartBlockTypeObjectType {
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
