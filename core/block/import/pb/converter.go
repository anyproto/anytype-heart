package pb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"path/filepath"
	"time"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/google/uuid"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/import/common/source"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/constant"
	"github.com/anyproto/anytype-heart/util/slice"
)

const (
	Name               = "Pb"
	rootCollectionName = "Protobuf Import"
	configFile         = "config.json"
	fileDir            = "files"
)

var ErrNotAnyBlockExtension = errors.New("not JSON or PB extension")
var ErrWrongFormat = errors.New("wrong PB or JSON format")

type Pb struct {
	service         *collection.Service
	accountService  account.Service
	tempDirProvider core.TempDirProvider
	iconOption      int64
}

func New(service *collection.Service, accountService account.Service, tempDirProvider core.TempDirProvider) common.Converter {
	return &Pb{
		service:         service,
		accountService:  accountService,
		tempDirProvider: tempDirProvider,
	}
}

func (p *Pb) GetSnapshots(ctx context.Context, req *pb.RpcObjectImportRequest, progress process.Progress) (*common.Response, *common.ConvertError) {
	params, e := p.getParams(req.Params)
	if e != nil || params == nil {
		return nil, common.NewFromError(fmt.Errorf("wrong parameters"), req.Mode)
	}
	allErrors := common.NewError(req.Mode)
	allSnapshots, widgetSnapshot, workspaceSnapshot := p.getSnapshots(progress, params, req.IsMigration, allErrors)
	oldToNewID := p.updateLinksToObjects(allSnapshots, allErrors, len(params.GetPath()))
	p.updateDetails(allSnapshots)
	if allErrors.ShouldAbortImport(len(params.GetPath()), req.Type) {
		return nil, allErrors
	}
	collectionProvider := GetProvider(params.GetImportType(), p.service)
	var rootCollectionID string
	rootCollections, colErr := collectionProvider.ProvideCollection(allSnapshots, widgetSnapshot, oldToNewID, params, workspaceSnapshot, req.IsNewSpace)
	if colErr != nil {
		allErrors.Add(colErr)
		if allErrors.ShouldAbortImport(len(params.GetPath()), req.Type) {
			return nil, allErrors
		}
	}
	if len(rootCollections) > 0 {
		allSnapshots = append(allSnapshots, rootCollections...)
		rootCollectionID = rootCollections[0].Id
	}
	progress.SetTotalPreservingRatio(int64(len(allSnapshots)))
	if allErrors.IsEmpty() {
		return &common.Response{Snapshots: allSnapshots, RootCollectionID: rootCollectionID}, nil
	}
	return &common.Response{Snapshots: allSnapshots, RootCollectionID: rootCollectionID}, allErrors
}

func (p *Pb) Name() string {
	return Name
}

func (p *Pb) getParams(params pb.IsRpcObjectImportRequestParams) (*pb.RpcObjectImportRequestPbParams, error) {
	if p, ok := params.(*pb.RpcObjectImportRequestParamsOfPbParams); ok {
		return p.PbParams, nil
	}
	return nil, fmt.Errorf("PB: getParams wrong parameters format")
}

func (p *Pb) getSnapshots(
	progress process.Progress,
	params *pb.RpcObjectImportRequestPbParams,
	isMigration bool,
	allErrors *common.ConvertError,
) (
	allSnapshots []*common.Snapshot,
	widgetSnapshot *common.Snapshot,
	workspaceSnapshot *common.Snapshot,
) {
	for _, path := range params.GetPath() {
		if err := progress.TryStep(1); err != nil {
			allErrors.Add(common.ErrCancel)
			return nil, nil, nil
		}
		snapshots, widget, workspace := p.handleImportPath(len(path), path, allErrors, isMigration, params.GetImportType())
		if allErrors.ShouldAbortImport(len(params.GetPath()), model.Import_Pb) {
			return nil, nil, nil
		}
		allSnapshots = append(allSnapshots, snapshots...)
		widgetSnapshot = widget
		workspaceSnapshot = workspace
	}
	return allSnapshots, widgetSnapshot, workspaceSnapshot
}

func (p *Pb) handleImportPath(
	pathCount int,
	path string,
	allErrors *common.ConvertError,
	isMigration bool,
	importType pb.RpcObjectImportRequestPbParamsType,
) ([]*common.Snapshot, *common.Snapshot, *common.Snapshot) {
	importSource := source.GetSource(path)
	defer importSource.Close()
	err := p.extractFiles(path, importSource)
	if err != nil {
		allErrors.Add(err)
		if allErrors.ShouldAbortImport(pathCount, model.Import_Pb) {
			return nil, nil, nil
		}
	}
	var (
		profileID           string
		needToImportWidgets bool
	)
	profile, err := p.getProfileFromFiles(importSource)
	if err != nil {
		allErrors.Add(err)
		if allErrors.ShouldAbortImport(pathCount, model.Import_Pb) {
			return nil, nil, nil
		}
	}
	if profile != nil {
		pr, e := p.accountService.ProfileInfo()
		if e != nil {
			allErrors.Add(e)
			if allErrors.ShouldAbortImport(pathCount, model.Import_Pb) {
				return nil, nil, nil
			}
		}
		needToImportWidgets = p.needToImportWidgets(profile.Address, pr.AccountId)
		profileID = profile.ProfileId
	}
	return p.getSnapshotsFromProvidedFiles(pathCount, importSource, allErrors, path, profileID, needToImportWidgets, isMigration, importType)
}

func (p *Pb) extractFiles(importPath string, importSource source.Source) error {
	err := importSource.Initialize(importPath)
	if err != nil {
		return err
	}
	if importSource.CountFilesWithGivenExtensions([]string{".pb", ".json"}) == 0 {
		return common.ErrNoObjectsToImport
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
	pathCount int,
	pbFiles source.Source,
	allErrors *common.ConvertError,
	path, profileID string,
	needToImportWidgets, isMigration bool,
	importType pb.RpcObjectImportRequestPbParamsType,
) (
	allSnapshots []*common.Snapshot,
	widgetSnapshot *common.Snapshot,
	workspaceSnapshot *common.Snapshot,
) {
	if iterateErr := pbFiles.Iterate(func(fileName string, fileReader io.ReadCloser) (isContinue bool) {
		// skip files from "files" directory
		if filepath.Dir(fileName) == fileDir {
			return true
		}
		snapshot, err := p.makeSnapshot(fileName, profileID, path, fileReader, isMigration, pbFiles)
		if err != nil {
			allErrors.Add(err)
			if allErrors.ShouldAbortImport(pathCount, model.Import_Pb) {
				return false
			}
		}
		if snapshot != nil {
			if p.shouldImportSnapshot(snapshot, needToImportWidgets, importType) {
				allSnapshots = append(allSnapshots, snapshot)
			}
			if snapshot.Snapshot.SbType == smartblock.SmartBlockTypeWidget {
				widgetSnapshot = snapshot
			}
			if snapshot.Snapshot.SbType == smartblock.SmartBlockTypeWorkspace {
				workspaceSnapshot = snapshot
			}
		}
		return true
	}); iterateErr != nil {
		allErrors.Add(iterateErr)
	}
	return allSnapshots, widgetSnapshot, workspaceSnapshot
}

func (p *Pb) makeSnapshot(name, profileID, path string,
	file io.ReadCloser,
	isMigration bool,
	pbFiles source.Source,
) (*common.Snapshot, error) {
	if name == constant.ProfileFile || name == configFile {
		return nil, nil
	}

	snapshot, errGS := p.getSnapshotFromFile(file, name)
	if errGS != nil {
		if errors.Is(errGS, ErrNotAnyBlockExtension) {
			return nil, nil
		}
		return nil, errGS
	}
	if valid := p.isSnapshotValid(snapshot); !valid {
		return nil, fmt.Errorf("snapshot is not valid")
	}
	id := uuid.New().String()
	id, err := p.normalizeSnapshot(snapshot, id, profileID, path, isMigration, pbFiles)
	if err != nil {
		return nil, fmt.Errorf("normalize snapshot: %w", err)
	}
	p.injectImportDetails(snapshot)
	return &common.Snapshot{
		Id:       id,
		FileName: name,
		Snapshot: snapshot,
	}, nil
}

func (p *Pb) getSnapshotFromFile(rd io.ReadCloser, name string) (*common.SnapshotModel, error) {
	defer rd.Close()
	if filepath.Ext(name) == ".json" {
		snapshot := &pb.SnapshotWithType{}
		um := jsonpb.Unmarshaler{}
		if uErr := um.Unmarshal(rd, snapshot); uErr != nil {
			return nil, ErrWrongFormat
		}
		return common.NewSnapshotModelFromProto(snapshot), nil
	}
	if filepath.Ext(name) == ".pb" {
		snapshot := &pb.SnapshotWithType{}
		data, err := io.ReadAll(rd)
		if err != nil {
			return nil, err
		}
		if err = snapshot.Unmarshal(data); err != nil {
			return nil, ErrWrongFormat
		}
		return common.NewSnapshotModelFromProto(snapshot), nil
	}
	return nil, ErrNotAnyBlockExtension
}

func (p *Pb) normalizeSnapshot(snapshot *common.SnapshotModel,
	id, profileID, path string,
	isMigration bool,
	pbFiles source.Source) (string, error) {
	if _, ok := model.SmartBlockType_name[int32(snapshot.SbType)]; !ok {
		newSbType := model.SmartBlockType_Page
		if int32(snapshot.SbType) == 96 { // fallback for objectType smartblocktype
			newSbType = model.SmartBlockType_SubObject
		}
		snapshot.SbType = coresb.SmartBlockType(newSbType)
	}

	if snapshot.SbType == coresb.SmartBlockTypeSubObject {
		details := snapshot.Data.Details
		originalId := snapshot.Data.Details.GetString(bundle.RelationKeyId)
		var sourceObjectId string
		// migrate old sub objects into real objects
		if snapshot.Data.ObjectTypes[0] == bundle.TypeKeyObjectType.URL() {
			snapshot.SbType = coresb.SmartBlockTypeObjectType
			typeKey, err := bundle.TypeKeyFromUrl(originalId)
			if err == nil {
				sourceObjectId = typeKey.BundledURL()
			}
		} else if snapshot.Data.ObjectTypes[0] == bundle.TypeKeyRelation.URL() {
			snapshot.SbType = coresb.SmartBlockTypeRelation
			relationKey, err := bundle.RelationKeyFromID(originalId)
			if err == nil {
				sourceObjectId = relationKey.BundledURL()
			}
		} else if snapshot.Data.ObjectTypes[0] == bundle.TypeKeyRelationOption.URL() {
			snapshot.SbType = coresb.SmartBlockTypeRelationOption
		} else {
			return "", fmt.Errorf("unknown sub object type %s", snapshot.Data.ObjectTypes[0])
		}
		if sourceObjectId != "" {
			if details.GetString(bundle.RelationKeySourceObject) == "" {
				details.SetString(bundle.RelationKeySourceObject, sourceObjectId)
			}
		}
		id = originalId
	}

	if snapshot.SbType == coresb.SmartBlockTypeProfilePage {
		var err error
		id, err = p.getIDForUserProfile(snapshot, profileID, id, isMigration)
		if err != nil {
			return "", fmt.Errorf("get user profile id: %w", err)
		}
		p.setProfileIconOption(snapshot, profileID)
	}
	if snapshot.SbType == coresb.SmartBlockTypePage {
		p.cleanupEmptyBlock(snapshot)
	}
	if snapshot.SbType == coresb.SmartBlockTypeFile {
		err := p.normalizeFilePath(snapshot, pbFiles, path)
		if err != nil {
			return "", fmt.Errorf("failed to update file path in file snapshot %w", err)
		}
	}
	if snapshot.SbType == coresb.SmartBlockTypeFileObject {
		err := p.normalizeFilePath(snapshot, pbFiles, path)
		if err != nil {
			return "", fmt.Errorf("failed to update file path in file snapshot %w", err)
		}
	}
	return id, nil
}

func (p *Pb) normalizeFilePath(snapshot *common.SnapshotModel, pbFiles source.Source, path string) error {
	filePath := snapshot.Data.Details.GetString(bundle.RelationKeySource)
	fileName, _, err := common.ProvideFileName(filePath, pbFiles, path, p.tempDirProvider)
	if err != nil {
		return err
	}
	if snapshot.Data.Details == nil {
		snapshot.Data.Details = domain.NewDetails()
	}
	snapshot.Data.Details.SetString(bundle.RelationKeySource, fileName)
	return nil
}

func (p *Pb) getIDForUserProfile(snapshot *common.SnapshotModel, profileID string, id string, isMigration bool) (string, error) {
	objectID := snapshot.Data.Details.GetString(bundle.RelationKeyId)
	if objectID == profileID && isMigration {
		return p.accountService.ProfileObjectId()
	}
	return id, nil
}

func (p *Pb) setProfileIconOption(snapshot *common.SnapshotModel, profileID string) {
	objectID := snapshot.Data.Details.GetString(bundle.RelationKeyId)
	if objectID != profileID {
		return
	}
	snapshot.Data.Details.SetInt64(bundle.RelationKeyIconOption, p.getIconOption())
}

func (p *Pb) getIconOption() int64 {
	if p.iconOption == 0 {
		p.iconOption = int64(rand.Intn(16) + 1)
	}
	return p.iconOption
}

// cleanupEmptyBlockMigration is fixing existing pages, imported from Notion
func (p *Pb) cleanupEmptyBlock(snapshot *common.SnapshotModel) {
	var (
		emptyBlock *model.Block
	)

	for _, block := range snapshot.Data.Blocks {
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

func (p *Pb) injectImportDetails(snapshot *common.SnapshotModel) {
	if snapshot.Data.Details == nil {
		snapshot.Data.Details = domain.NewDetails()
	}
	if id := snapshot.Data.Details.GetString(bundle.RelationKeyId); id != "" {
		snapshot.Data.Details.SetString(bundle.RelationKeyOldAnytypeID, id)
	}
	p.setSourceFilePath(snapshot)
	createdDate := snapshot.Data.Details.GetInt64(bundle.RelationKeyCreatedDate)
	if createdDate == 0 {
		snapshot.Data.Details.SetInt64(bundle.RelationKeyCreatedDate, time.Now().Unix())
	}
}

func (p *Pb) setSourceFilePath(snapshot *common.SnapshotModel) {
	spaceId := snapshot.Data.Details.GetString(bundle.RelationKeySpaceId)
	id := snapshot.Data.Details.GetString(bundle.RelationKeyId)
	sourceFilePath := filepath.Join(spaceId, id)
	snapshot.Data.Details.SetString(bundle.RelationKeySourceFilePath, sourceFilePath)
}

func (p *Pb) shouldImportSnapshot(snapshot *common.Snapshot, needToImportWidgets bool, importType pb.RpcObjectImportRequestPbParamsType) bool {
	return (snapshot.Snapshot.SbType == smartblock.SmartBlockTypeWorkspace && importType == pb.RpcObjectImportRequestPbParams_SPACE) ||
		(snapshot.Snapshot.SbType != smartblock.SmartBlockTypeWidget && snapshot.Snapshot.SbType != smartblock.SmartBlockTypeWorkspace) ||
		(snapshot.Snapshot.SbType == smartblock.SmartBlockTypeWidget && (needToImportWidgets || importType == pb.RpcObjectImportRequestPbParams_EXPERIENCE)) // we import widget in case of experience import
}

func (p *Pb) updateLinksToObjects(snapshots []*common.Snapshot, allErrors *common.ConvertError, pathCount int) map[string]string {
	oldToNewID := make(map[string]string, len(snapshots))
	for _, snapshot := range snapshots {
		id := snapshot.Snapshot.Data.Details.GetString(bundle.RelationKeyId)
		oldToNewID[id] = snapshot.Id
	}
	for _, snapshot := range snapshots {
		st := state.NewDocFromSnapshot("", snapshot.Snapshot.ToProto())
		err := common.UpdateLinksToObjects(st.(*state.State), oldToNewID)
		if err != nil {
			allErrors.Add(err)
			if allErrors.ShouldAbortImport(pathCount, model.Import_Pb) {
				return nil
			}
			continue
		}
		common.UpdateObjectIDsInRelations(st.(*state.State), oldToNewID)
		// TODO Fix
		// converter.UpdateObjectType(oldToNewID, st.(*state.State))
		p.updateObjectsIDsInCollection(st.(*state.State), oldToNewID)
		p.updateSnapshot(snapshot, st.(*state.State))
	}
	return oldToNewID
}

func (p *Pb) updateSnapshot(snapshot *common.Snapshot, st *state.State) {
	snapshot.Snapshot.Data.Details = snapshot.Snapshot.Data.Details.Merge(st.CombinedDetails())
	snapshot.Snapshot.Data.Blocks = st.Blocks()
	snapshot.Snapshot.Data.ObjectTypes = domain.MarshalTypeKeys(st.ObjectTypeKeys())
	snapshot.Snapshot.Data.Collections = st.Store()
}

func (p *Pb) updateDetails(snapshots []*common.Snapshot) {
	removeKeys := slice.Filter(bundle.LocalAndDerivedRelationKeys, func(key domain.RelationKey) bool {
		// preserve some keys we have special cases for
		return key != bundle.RelationKeyIsFavorite &&
			key != bundle.RelationKeyIsArchived &&
			key != bundle.RelationKeyCreatedDate &&
			key != bundle.RelationKeyLastModifiedDate &&
			key != bundle.RelationKeyId
	})

	for _, snapshot := range snapshots {
		details := snapshot.Snapshot.Data.Details.CopyWithoutKeys(removeKeys...)
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

func (p *Pb) isSnapshotValid(snapshot *common.SnapshotModel) bool {
	return !(snapshot == nil || snapshot.Data == nil)
}
