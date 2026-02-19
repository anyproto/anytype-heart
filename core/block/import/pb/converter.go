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

type Pb struct {
	service         *collection.Service
	accountService  account.Service
	tempDirProvider core.TempDirProvider

	progress  process.Progress
	errors    *common.ConvertError
	params    *pb.RpcObjectImportRequestPbParams
	pathCount int

	isMigration, isNewSpace, importWidgets bool
}

func New(service *collection.Service, accountService account.Service, tempDirProvider core.TempDirProvider) common.Converter {
	return &Pb{
		service:         service,
		accountService:  accountService,
		tempDirProvider: tempDirProvider,
	}
}

func (p *Pb) GetSnapshots(_ context.Context, req *pb.RpcObjectImportRequest, progress process.Progress) (*common.Response, *common.ConvertError) {
	if err := p.init(req, progress); err != nil {
		return nil, common.NewFromError(err, req.Mode)
	}
	snapshots := p.getSnapshots()
	if snapshots == nil {
		if p.errors.IsEmpty() {
			p.errors.Add(fmt.Errorf("PB: no snapshots are gathered"))
		}
		return nil, p.errors
	}
	oldToNewID := p.updateLinksToObjects(snapshots.List())
	p.updateDetails(snapshots.List())
	if p.errors.ShouldAbortImport(len(p.params.GetPath()), req.Type) {
		return nil, p.errors
	}
	collectionProvider := GetProvider(p.params.GetImportType(), p.service)
	var rootCollectionID string
	rootCollections, colErr := collectionProvider.ProvideCollection(snapshots, oldToNewID, p.params, req.IsNewSpace)
	if colErr != nil {
		p.errors.Add(colErr)
		if p.errors.ShouldAbortImport(len(p.params.GetPath()), req.Type) {
			return nil, p.errors
		}
	}
	if len(rootCollections) > 0 {
		snapshots.Add(rootCollections...)
		rootCollectionID = rootCollections[0].Id
	}
	progress.SetTotalPreservingRatio(int64(snapshots.Len()))
	return &common.Response{Snapshots: snapshots.List(), RootObjectID: rootCollectionID, RootObjectWidgetType: model.BlockContentWidget_CompactList}, p.errors.ErrorOrNil()
}

func (p *Pb) Name() string {
	return Name
}

func (p *Pb) init(req *pb.RpcObjectImportRequest, progress process.Progress) (err error) {
	p.params, err = p.getParams(req.Params)
	if err != nil || p.params == nil {
		return err
	}
	p.progress = progress
	p.errors = common.NewError(req.Mode)
	p.isMigration = req.IsMigration
	p.isNewSpace = req.IsNewSpace
	p.pathCount = len(p.params.GetPath())
	return nil
}

func (p *Pb) getParams(params pb.IsRpcObjectImportRequestParams) (*pb.RpcObjectImportRequestPbParams, error) {
	if p, ok := params.(*pb.RpcObjectImportRequestParamsOfPbParams); ok {
		return p.PbParams, nil
	}
	return nil, fmt.Errorf("PB: getParams wrong parameters format")
}

func (p *Pb) getSnapshots() (allSnapshots *common.SnapshotContext) {
	allSnapshots = common.NewSnapshotContext()
	for _, path := range p.params.GetPath() {
		if err := p.progress.TryStep(1); err != nil {
			p.errors.Add(common.ErrCancel)
			return nil
		}
		snapshots := p.handleImportPath(path)
		if p.errors.ShouldAbortImport(len(p.params.GetPath()), model.Import_Pb) {
			return nil
		}
		allSnapshots.Merge(snapshots)
	}
	return allSnapshots
}

func (p *Pb) handleImportPath(path string) *common.SnapshotContext {
	importSource := source.GetSource(path)
	defer importSource.Close()
	err := p.extractFiles(path, importSource)
	if err != nil {
		p.errors.Add(err)
		if p.errors.ShouldAbortImport(p.pathCount, model.Import_Pb) {
			return nil
		}
	}
	var (
		profileID string
	)
	profile, err := p.getProfileFromFiles(importSource)
	if err != nil {
		p.errors.Add(err)
		if p.errors.ShouldAbortImport(p.pathCount, model.Import_Pb) {
			return nil
		}
	}
	if profile != nil {
		pr, e := p.accountService.ProfileInfo()
		if e != nil {
			p.errors.Add(e)
			if p.errors.ShouldAbortImport(p.pathCount, model.Import_Pb) {
				return nil
			}
		}
		p.importWidgets = p.needToImportWidgets(profile.Address, pr.AccountId)
		profileID = profile.ProfileId
	}
	return p.getSnapshotsFromProvidedFiles(importSource, path, profileID)
}

func (p *Pb) extractFiles(importPath string, importSource source.Source) error {
	err := importSource.Initialize(importPath)
	if err != nil {
		return err
	}
	if importSource.CountFilesWithGivenExtensions([]string{".pb", ".json"}) == 0 {
		return common.ErrorBySourceType(importSource)
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

func (p *Pb) getSnapshotsFromProvidedFiles(pbFiles source.Source, path, profileID string) (snapshots *common.SnapshotContext) {
	snapshots = common.NewSnapshotContext()
	if iterateErr := pbFiles.Iterate(func(fileName string, fileReader io.ReadCloser) (isContinue bool) {
		// skip files from "files" directory
		if filepath.Dir(fileName) == fileDir {
			return true
		}
		snapshot, err := p.makeSnapshot(fileName, profileID, path, fileReader, pbFiles)
		if err != nil {
			p.errors.Add(err)
			if p.errors.ShouldAbortImport(p.pathCount, model.Import_Pb) {
				return false
			}
		}
		if snapshot != nil {
			if p.shouldImportSnapshot(snapshot) {
				snapshots.Add(snapshot)
			}
			switch snapshot.Snapshot.SbType {
			case smartblock.SmartBlockTypeWidget:
				snapshots.SetWidget(snapshot)
			case smartblock.SmartBlockTypeWorkspace:
				snapshots.SetWorkspace(snapshot)
			}
		}
		return true
	}); iterateErr != nil {
		p.errors.Add(iterateErr)
	}
	return snapshots
}

func (p *Pb) makeSnapshot(
	name, profileID, path string,
	file io.ReadCloser,
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
		return nil, fmt.Errorf("%w: %s", common.ErrPbNotAnyBlockFormat, errGS.Error())
	}
	id := uuid.New().String()
	id, err := p.normalizeSnapshot(snapshot, id, profileID, path, pbFiles)
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
		um := jsonpb.Unmarshaler{AllowUnknownFields: true}
		if uErr := um.Unmarshal(rd, snapshot); uErr != nil {
			return nil, fmt.Errorf("PB:GetSnapshot %w", uErr)
		}
		return common.NewSnapshotModelFromProto(snapshot)
	}
	if filepath.Ext(name) == ".pb" {
		snapshot := &pb.SnapshotWithType{}
		data, err := io.ReadAll(rd)
		if err != nil {
			return nil, err
		}
		if err = snapshot.Unmarshal(data); err != nil {
			return nil, fmt.Errorf("PB:GetSnapshot %w", err)
		}
		return common.NewSnapshotModelFromProto(snapshot)
	}
	return nil, ErrNotAnyBlockExtension
}

func (p *Pb) normalizeSnapshot(
	snapshot *common.SnapshotModel,
	id, profileID, path string,
	pbFiles source.Source,
) (string, error) {
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
		id, err = p.getIDForUserProfile(snapshot, profileID, id)
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

func (p *Pb) getIDForUserProfile(snapshot *common.SnapshotModel, profileID string, id string) (string, error) {
	objectID := snapshot.Data.Details.GetString(bundle.RelationKeyId)
	if objectID == profileID && p.isMigration {
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
	return int64(rand.Intn(16) + 1)
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

func (p *Pb) shouldImportSnapshot(snapshot *common.Snapshot) bool {
	return (snapshot.Snapshot.SbType == smartblock.SmartBlockTypeWorkspace && p.isNewSpace) ||
		(snapshot.Snapshot.SbType != smartblock.SmartBlockTypeWidget && snapshot.Snapshot.SbType != smartblock.SmartBlockTypeWorkspace) ||
		(snapshot.Snapshot.SbType == smartblock.SmartBlockTypeWidget && (p.importWidgets || p.params.GetImportType() == pb.RpcObjectImportRequestPbParams_EXPERIENCE)) // we import widget in case of experience import
}

func (p *Pb) updateLinksToObjects(snapshots []*common.Snapshot) map[string]string {
	oldToNewID := make(map[string]string, len(snapshots))
	relationKeysToFormat := make(map[domain.RelationKey]int32, len(snapshots))
	for _, snapshot := range snapshots {
		id := snapshot.Snapshot.Data.Details.GetString(bundle.RelationKeyId)
		oldToNewID[id] = snapshot.Id
		if snapshot.Snapshot.SbType == smartblock.SmartBlockTypeRelation {
			format := snapshot.Snapshot.Data.Details.GetInt64(bundle.RelationKeyRelationFormat)
			relationKeysToFormat[domain.RelationKey(snapshot.Snapshot.Data.Key)] = int32(format)
		}
	}
	for _, snapshot := range snapshots {
		st, err := state.NewDocFromSnapshot("", snapshot.Snapshot.ToProto())
		if err != nil {
			p.errors.Add(err)
			if p.errors.ShouldAbortImport(p.pathCount, model.Import_Pb) {
				return nil
			}
			continue
		}
		err = common.UpdateLinksToObjects(st, oldToNewID)
		if err != nil {
			p.errors.Add(err)
			if p.errors.ShouldAbortImport(p.pathCount, model.Import_Pb) {
				return nil
			}
			continue
		}
		common.UpdateObjectIDsInRelations(st, oldToNewID, relationKeysToFormat)
		// TODO Fix
		// converter.UpdateObjectType(oldToNewID, st.(*state.State))
		p.updateObjectsIDsInCollection(st, oldToNewID)
		p.updateSnapshot(snapshot, st)
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
			key != bundle.RelationKeyId &&
			key != bundle.RelationKeyResolvedLayout
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
