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
	"github.com/gogo/protobuf/types"
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
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/constant"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

const (
	Name               = "Pb"
	rootCollectionName = "Protobuf Import"
	configFile         = "config.json"
	fileDir            = "files"
)

var (
	ErrNotAnyBlockExtension = errors.New("not JSON or PB extension")
	ErrWrongFormat          = errors.New("wrong PB or JSON format")
)

type snapshotSet struct {
	List              []*common.Snapshot
	Widget, Workspace *common.Snapshot
}

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
	oldToNewID := p.updateLinksToObjects(snapshots.List)
	p.updateDetails(snapshots.List)
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
		snapshots.List = append(snapshots.List, rootCollections...)
		rootCollectionID = rootCollections[0].Id
	}
	progress.SetTotalPreservingRatio(int64(len(snapshots.List)))
	return &common.Response{Snapshots: snapshots.List, RootCollectionID: rootCollectionID}, p.errors.ErrorOrNil()
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

func (p *Pb) getSnapshots() (allSnapshots *snapshotSet) {
	allSnapshots = &snapshotSet{
		List: []*common.Snapshot{},
	}
	for _, path := range p.params.GetPath() {
		if err := p.progress.TryStep(1); err != nil {
			p.errors.Add(common.ErrCancel)
			return &snapshotSet{List: []*common.Snapshot{}}
		}
		snapshots := p.handleImportPath(path)
		if p.errors.ShouldAbortImport(len(p.params.GetPath()), model.Import_Pb) {
			return &snapshotSet{List: []*common.Snapshot{}}
		}
		allSnapshots.List = append(allSnapshots.List, snapshots.List...)
		if snapshots.Widget != nil {
			allSnapshots.Widget = snapshots.Widget
		}
		if snapshots.Workspace != nil {
			allSnapshots.Workspace = snapshots.Workspace
		}
	}
	return allSnapshots
}

func (p *Pb) handleImportPath(path string) *snapshotSet {
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

func (p *Pb) getSnapshotsFromProvidedFiles(pbFiles source.Source, path, profileID string) (snapshots *snapshotSet) {
	snapshots = &snapshotSet{
		List: []*common.Snapshot{},
	}
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
				snapshots.List = append(snapshots.List, snapshot)
			}
			if snapshot.SbType == smartblock.SmartBlockTypeWidget {
				snapshots.Widget = snapshot
			}
			if snapshot.SbType == smartblock.SmartBlockTypeWorkspace {
				snapshots.Workspace = snapshot
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
		return nil, errGS
	}
	if valid := p.isSnapshotValid(snapshot); !valid {
		return nil, fmt.Errorf("snapshot is not valid")
	}
	id := uuid.New().String()
	id, err := p.normalizeSnapshot(snapshot, id, profileID, path, pbFiles)
	if err != nil {
		return nil, fmt.Errorf("normalize snapshot: %w", err)
	}
	p.injectImportDetails(snapshot)
	return &common.Snapshot{
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
			return nil, ErrWrongFormat
		}
		return snapshot, nil
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
		return snapshot, nil
	}
	return nil, ErrNotAnyBlockExtension
}

func (p *Pb) normalizeSnapshot(
	snapshot *pb.SnapshotWithType,
	id, profileID, path string,
	pbFiles source.Source,
) (string, error) {
	if _, ok := model.SmartBlockType_name[int32(snapshot.SbType)]; !ok {
		newSbType := model.SmartBlockType_Page
		if int32(snapshot.SbType) == 96 { // fallback for objectType smartblocktype
			newSbType = model.SmartBlockType_SubObject
		}
		snapshot.SbType = newSbType
	}

	if snapshot.SbType == model.SmartBlockType_SubObject {
		details := snapshot.Snapshot.Data.Details
		originalId := pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeyId.String())
		var sourceObjectId string
		// migrate old sub objects into real objects
		if snapshot.Snapshot.Data.ObjectTypes[0] == bundle.TypeKeyObjectType.URL() {
			snapshot.SbType = model.SmartBlockType_STType
			typeKey, err := bundle.TypeKeyFromUrl(originalId)
			if err == nil {
				sourceObjectId = typeKey.BundledURL()
			}
		} else if snapshot.Snapshot.Data.ObjectTypes[0] == bundle.TypeKeyRelation.URL() {
			snapshot.SbType = model.SmartBlockType_STRelation
			relationKey, err := bundle.RelationKeyFromID(originalId)
			if err == nil {
				sourceObjectId = relationKey.BundledURL()
			}
		} else if snapshot.Snapshot.Data.ObjectTypes[0] == bundle.TypeKeyRelationOption.URL() {
			snapshot.SbType = model.SmartBlockType_STRelationOption
		} else {
			return "", fmt.Errorf("unknown sub object type %s", snapshot.Snapshot.Data.ObjectTypes[0])
		}
		if sourceObjectId != "" {
			if pbtypes.GetString(details, bundle.RelationKeySourceObject.String()) == "" {
				details.Fields[bundle.RelationKeySourceObject.String()] = pbtypes.String(sourceObjectId)
			}
		}
		id = originalId
	}

	if snapshot.SbType == model.SmartBlockType_ProfilePage {
		var err error
		id, err = p.getIDForUserProfile(snapshot, profileID, id)
		if err != nil {
			return "", fmt.Errorf("get user profile id: %w", err)
		}
		p.setProfileIconOption(snapshot, profileID)
	}
	if snapshot.SbType == model.SmartBlockType_Page {
		p.cleanupEmptyBlock(snapshot)
	}
	if snapshot.SbType == model.SmartBlockType_File {
		err := p.normalizeFilePath(snapshot, pbFiles, path)
		if err != nil {
			return "", fmt.Errorf("failed to update file path in file snapshot %w", err)
		}
	}
	if snapshot.SbType == model.SmartBlockType_FileObject {
		err := p.normalizeFilePath(snapshot, pbFiles, path)
		if err != nil {
			return "", fmt.Errorf("failed to update file path in file snapshot %w", err)
		}
	}
	return id, nil
}

func (p *Pb) normalizeFilePath(snapshot *pb.SnapshotWithType, pbFiles source.Source, path string) error {
	filePath := pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeySource.String())
	fileName, _, err := common.ProvideFileName(filePath, pbFiles, path, p.tempDirProvider)
	if err != nil {
		return err
	}
	if snapshot.Snapshot.Data.Details == nil || snapshot.Snapshot.Data.Details.Fields == nil {
		snapshot.Snapshot.Data.Details.Fields = map[string]*types.Value{}
	}
	snapshot.Snapshot.Data.Details.Fields[bundle.RelationKeySource.String()] = pbtypes.String(fileName)
	return nil
}

func (p *Pb) getIDForUserProfile(mo *pb.SnapshotWithType, profileID string, id string) (string, error) {
	objectID := pbtypes.GetString(mo.Snapshot.Data.Details, bundle.RelationKeyId.String())
	if objectID == profileID && p.isMigration {
		return p.accountService.ProfileObjectId()
	}
	return id, nil
}

func (p *Pb) setProfileIconOption(mo *pb.SnapshotWithType, profileID string) {
	objectID := pbtypes.GetString(mo.Snapshot.Data.Details, bundle.RelationKeyId.String())
	if objectID != profileID {
		return
	}
	mo.Snapshot.Data.Details.Fields[bundle.RelationKeyIconOption.String()] = pbtypes.Int64(p.getIconOption())
}

func (p *Pb) getIconOption() int64 {
	return int64(rand.Intn(16) + 1)
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

func (p *Pb) injectImportDetails(sn *pb.SnapshotWithType) {
	if sn.Snapshot.Data.Details == nil || sn.Snapshot.Data.Details.Fields == nil {
		sn.Snapshot.Data.Details = &types.Struct{Fields: map[string]*types.Value{}}
	}
	if id := pbtypes.GetString(sn.Snapshot.Data.Details, bundle.RelationKeyId.String()); id != "" {
		sn.Snapshot.Data.Details.Fields[bundle.RelationKeyOldAnytypeID.String()] = pbtypes.String(id)
	}
	p.setSourceFilePath(sn)
	createdDate := pbtypes.GetInt64(sn.Snapshot.Data.Details, bundle.RelationKeyCreatedDate.String())
	if createdDate == 0 {
		sn.Snapshot.Data.Details.Fields[bundle.RelationKeyCreatedDate.String()] = pbtypes.Int64(time.Now().Unix())
	}
}

func (p *Pb) setSourceFilePath(sn *pb.SnapshotWithType) {
	spaceId := pbtypes.GetString(sn.Snapshot.Data.Details, bundle.RelationKeySpaceId.String())
	id := pbtypes.GetString(sn.Snapshot.Data.Details, bundle.RelationKeyId.String())
	sourceFilePath := filepath.Join(spaceId, id)
	sn.Snapshot.Data.Details.Fields[bundle.RelationKeySourceFilePath.String()] = pbtypes.String(sourceFilePath)
}

func (p *Pb) shouldImportSnapshot(snapshot *common.Snapshot) bool {
	return (snapshot.SbType == smartblock.SmartBlockTypeWorkspace && p.isNewSpace) ||
		(snapshot.SbType != smartblock.SmartBlockTypeWidget && snapshot.SbType != smartblock.SmartBlockTypeWorkspace) ||
		(snapshot.SbType == smartblock.SmartBlockTypeWidget && (p.importWidgets || p.params.GetImportType() == pb.RpcObjectImportRequestPbParams_EXPERIENCE)) // we import widget in case of experience import
}

func (p *Pb) updateLinksToObjects(snapshots []*common.Snapshot) map[string]string {
	oldToNewID := make(map[string]string, len(snapshots))
	for _, snapshot := range snapshots {
		id := pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeyId.String())
		oldToNewID[id] = snapshot.Id
	}
	for _, snapshot := range snapshots {
		st := state.NewDocFromSnapshot("", snapshot.Snapshot, state.WithUniqueKeyMigration(snapshot.SbType))
		err := common.UpdateLinksToObjects(st.(*state.State), oldToNewID)
		if err != nil {
			p.errors.Add(err)
			if p.errors.ShouldAbortImport(p.pathCount, model.Import_Pb) {
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
	snapshot.Snapshot.Data.Details = pbtypes.StructMerge(snapshot.Snapshot.Data.Details, st.CombinedDetails(), false)
	snapshot.Snapshot.Data.Blocks = st.Blocks()
	snapshot.Snapshot.Data.ObjectTypes = domain.MarshalTypeKeys(st.ObjectTypeKeys())
	snapshot.Snapshot.Data.Collections = st.Store()
}

func (p *Pb) updateDetails(snapshots []*common.Snapshot) {
	removeKeys := slice.Filter(bundle.LocalAndDerivedRelationKeys, func(key string) bool {
		// preserve some keys we have special cases for
		return key != bundle.RelationKeyIsFavorite.String() &&
			key != bundle.RelationKeyIsArchived.String() &&
			key != bundle.RelationKeyCreatedDate.String() &&
			key != bundle.RelationKeyLastModifiedDate.String() &&
			key != bundle.RelationKeyId.String()
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

func (p *Pb) isSnapshotValid(snapshot *pb.SnapshotWithType) bool {
	return !(snapshot == nil || snapshot.Snapshot == nil || snapshot.Snapshot.Data == nil)
}
