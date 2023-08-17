package block

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/ocache"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/commonspace/object/treemanager"
	"github.com/anyproto/anytype-heart/core/block/uniquekey"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/gogo/protobuf/types"
	"github.com/hashicorp/go-multierror"
	"github.com/samber/lo"
	"go.uber.org/zap"

	bookmarksvc "github.com/anyproto/anytype-heart/core/block/bookmark"
	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/collection"
	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/block/editor/file"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/history"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync"
	"github.com/anyproto/anytype-heart/core/relation"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/core/syncstatus"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/typeprovider"
	"github.com/anyproto/anytype-heart/util/internalflag"
	"github.com/anyproto/anytype-heart/util/linkpreview"
	"github.com/anyproto/anytype-heart/util/mutex"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/uri"

	_ "github.com/anyproto/anytype-heart/core/block/editor/table"
	_ "github.com/anyproto/anytype-heart/core/block/simple/file"
	_ "github.com/anyproto/anytype-heart/core/block/simple/link"
	_ "github.com/anyproto/anytype-heart/core/block/simple/widget"
)

const (
	CName           = treemanager.CName
	linkObjectShare = "anytype://object/share?"
)

var (
	ErrBlockNotFound                     = errors.New("block not found")
	ErrUnexpectedBlockType               = errors.New("unexpected block type")
	ErrUnknownObjectType                 = fmt.Errorf("unknown object type")
	ErrSubobjectAlreadyExistInCollection = errors.New("subobject already exist in collection")
)

var log = logging.Logger("anytype-mw-service")

var (
	// quick fix for limiting file upload goroutines
	uploadFilesLimiter = make(chan struct{}, 8)
)

func init() {
	for i := 0; i < cap(uploadFilesLimiter); i++ {
		uploadFilesLimiter <- struct{}{}
	}
}

type SmartblockOpener interface {
	Open(id string) (sb smartblock.SmartBlock, err error)
}

func New() *Service {
	return &Service{
		closing: make(chan struct{}),
		syncer:  map[string]*treeSyncer{},
		openedObjs: &openedObjects{
			objects: make(map[string]bool),
			lock:    &sync.Mutex{},
		},
	}
}

type objectCreator interface {
	CreateSmartBlockFromState(ctx context.Context, spaceID string, sbType coresb.SmartBlockType, details *types.Struct, createState *state.State) (id string, newDetails *types.Struct, err error)
	InjectWorkspaceID(details *types.Struct, spaceID string, objectID string)

	CreateObject(ctx context.Context, spaceID string, req DetailsGetter, forcedType bundle.TypeKey) (id string, details *types.Struct, err error)
}

type DetailsGetter interface {
	GetDetails() *types.Struct
}
type InternalFlagsGetter interface {
	GetInternalFlags() []*model.InternalFlag
}
type TemplateIDGetter interface {
	GetTemplateId() string
}

type indexer interface {
	ReindexSpace(spaceID string) error
}

type builtinObjects interface {
	CreateObjectsForUseCase(ctx context.Context, spaceID string, req pb.RpcObjectImportUseCaseRequestUseCase) (code pb.RpcObjectImportUseCaseResponseErrorCode, err error)
}

type Service struct {
	anytype         core.Service
	syncStatus      syncstatus.Service
	eventSender     event.Sender
	closed          bool
	linkPreview     linkpreview.LinkPreview
	process         process.Service
	app             *app.App
	source          source.Service
	objectStore     objectstore.ObjectStore
	restriction     restriction.Service
	bookmark        bookmarksvc.Service
	relationService relation.Service
	cache           ocache.OCache
	indexer         indexer

	objectCreator        objectCreator
	objectFactory        *editor.ObjectFactory
	spaceService         space.Service
	commonAccount        accountservice.Service
	fileStore            filestore.FileStore
	tempDirProvider      core.TempDirProvider
	sbtProvider          typeprovider.SmartBlockTypeProvider
	layoutConverter      converter.LayoutConverter
	builtinObjectService builtinObjects

	fileSync    filesync.FileSync
	fileService files.Service
	// TODO: move all this into separate treecache component or something like this
	syncer      map[string]*treeSyncer
	syncStarted bool
	syncerLock  sync.Mutex
	closing     chan struct{}

	predefinedObjectWasMissing bool
	openedObjs                 *openedObjects
}

type openedObjects struct {
	objects map[string]bool
	lock    *sync.Mutex
}

func (s *Service) Name() string {
	return CName
}

func (s *Service) Init(a *app.App) (err error) {
	s.anytype = a.MustComponent(core.CName).(core.Service)
	s.syncStatus = a.MustComponent(syncstatus.CName).(syncstatus.Service)
	s.linkPreview = a.MustComponent(linkpreview.CName).(linkpreview.LinkPreview)
	s.process = a.MustComponent(process.CName).(process.Service)
	s.eventSender = a.MustComponent(event.CName).(event.Sender)
	s.source = a.MustComponent(source.CName).(source.Service)
	s.objectStore = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	s.restriction = a.MustComponent(restriction.CName).(restriction.Service)
	s.bookmark = a.MustComponent("bookmark-importer").(bookmarksvc.Service)
	s.relationService = a.MustComponent(relation.CName).(relation.Service)
	s.objectCreator = a.MustComponent("objectCreator").(objectCreator)
	s.spaceService = a.MustComponent(space.CName).(space.Service)
	s.objectFactory = app.MustComponent[*editor.ObjectFactory](a)
	s.commonAccount = a.MustComponent(accountservice.CName).(accountservice.Service)
	s.fileStore = app.MustComponent[filestore.FileStore](a)
	s.fileSync = app.MustComponent[filesync.FileSync](a)
	s.fileService = app.MustComponent[files.Service](a)

	s.tempDirProvider = app.MustComponent[core.TempDirProvider](a)
	s.sbtProvider = app.MustComponent[typeprovider.SmartBlockTypeProvider](a)
	s.layoutConverter = app.MustComponent[converter.LayoutConverter](a)

	s.indexer = app.MustComponent[indexer](a)
	s.builtinObjectService = app.MustComponent[builtinObjects](a)
	s.cache = s.createCache()
	s.app = a
	return
}

func (s *Service) Run(ctx context.Context) (err error) {
	return
}

func (s *Service) Anytype() core.Service {
	return s.anytype
}

func (s *Service) OpenBlock(
	ctx context.Context,
	sctx session.Context, id string, includeRelationsAsDependentObjects bool,
) (obj *model.ObjectView, err error) {
	startTime := time.Now()
	spaceID, err := s.objectStore.ResolveSpaceID(id)
	if err != nil {
		return nil, fmt.Errorf("resolve spaceID: %w", err)
	}
	ob, err := s.getSmartblock(ctx, domain.FullID{
		SpaceID:  spaceID,
		ObjectID: id,
	})
	if err != nil {
		return nil, err
	}
	if includeRelationsAsDependentObjects {
		ob.EnabledRelationAsDependentObjects()
	}
	afterSmartBlockTime := time.Now()

	ob.Lock()
	defer ob.Unlock()
	ob.RegisterSession(sctx)

	afterDataviewTime := time.Now()
	st := ob.NewState()

	st.SetLocalDetail(bundle.RelationKeyLastOpenedDate.String(), pbtypes.Int64(time.Now().Unix()))
	if err = ob.Apply(st, smartblock.NoHistory, smartblock.NoEvent, smartblock.SkipIfNoChanges); err != nil {
		log.Errorf("failed to update lastOpenedDate: %s", err.Error())
	}
	afterApplyTime := time.Now()
	if obj, err = ob.Show(); err != nil {
		return
	}
	afterShowTime := time.Now()
	_, err = s.syncStatus.Watch(spaceID, id, func() []string {
		ob.Lock()
		defer ob.Unlock()
		bs := ob.NewState()

		return lo.Uniq(bs.GetAllFileHashes(ob.FileRelationKeys(bs)))
	})
	if err == nil {
		ob.AddHook(func(_ smartblock.ApplyInfo) error {
			s.syncStatus.Unwatch(spaceID, id)
			return nil
		}, smartblock.HookOnClose)
	}
	if err != nil && err != treestorage.ErrUnknownTreeId {
		log.Errorf("failed to watch status for object %s: %s", id, err.Error())
	}

	sbType, err := s.sbtProvider.Type(spaceID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get smartblock type: %w", err)
	}
	afterHashesTime := time.Now()
	metrics.SharedClient.RecordEvent(metrics.OpenBlockEvent{
		ObjectId:       id,
		GetBlockMs:     afterSmartBlockTime.Sub(startTime).Milliseconds(),
		DataviewMs:     afterDataviewTime.Sub(afterSmartBlockTime).Milliseconds(),
		ApplyMs:        afterApplyTime.Sub(afterDataviewTime).Milliseconds(),
		ShowMs:         afterShowTime.Sub(afterApplyTime).Milliseconds(),
		FileWatcherMs:  afterHashesTime.Sub(afterShowTime).Milliseconds(),
		SmartblockType: int(sbType),
	})
	mutex.WithLock(s.openedObjs.lock, func() any { s.openedObjs.objects[id] = true; return nil })
	return obj, nil
}

func (s *Service) ShowBlock(
	id string, includeRelationsAsDependentObjects bool,
) (obj *model.ObjectView, err error) {
	err2 := Do(s, id, func(b smartblock.SmartBlock) error {
		if includeRelationsAsDependentObjects {
			b.EnabledRelationAsDependentObjects()
		}
		obj, err = b.Show()
		return err
	})
	if err2 != nil {
		return nil, err2
	}
	return
}

func (s *Service) CloseBlock(ctx session.Context, id string) error {
	var isDraft bool
	err := Do(s, id, func(b smartblock.SmartBlock) error {
		b.ObjectClose(ctx)
		s := b.NewState()
		isDraft = internalflag.NewFromState(s).Has(model.InternalFlag_editorDeleteEmpty)
		// workspaceId = pbtypes.GetString(s.LocalDetails(), bundle.RelationKeyWorkspaceId.String())
		return nil
	})
	if err != nil {
		return err
	}

	if isDraft {
		if err = s.DeleteObject(id); err != nil {
			log.Errorf("error while block delete: %v", err)
		} else {
			s.sendOnRemoveEvent(id)
		}
	}
	mutex.WithLock(s.openedObjs.lock, func() any { delete(s.openedObjs.objects, id); return nil })
	return nil
}

func (s *Service) GetOpenedObjects() []string {
	return mutex.WithLock(s.openedObjs.lock, func() []string { return lo.Keys(s.openedObjs.objects) })
}

func (s *Service) CloseBlocks() {
	s.cache.ForEach(func(v ocache.Object) (isContinue bool) {
		ob := v.(smartblock.SmartBlock)
		ob.Lock()
		ob.ObjectCloseAllSessions()
		ob.Unlock()
		return true
	})
}

func (s *Service) AddSubObjectToWorkspace(
	ctx context.Context,
	spaceID string,
	sourceObjectId string,
) (id string, object *types.Struct, err error) {
	ids, details, err := s.AddBundledObjectToSpace(ctx, spaceID, []string{sourceObjectId})
	if err != nil {
		return "", nil, err
	}
	if len(ids) == 0 {
		return "", nil, fmt.Errorf("failed to add object")
	}

	return ids[0], details[0], nil
}

func (s *Service) prepareDetailsForInstallingObject(ctx context.Context, spaceID string, installedID string, bundledDetails *types.Struct) (*types.Struct, error) {
	newDetails := pbtypes.CopyStruct(bundledDetails)
	sourceId := pbtypes.GetString(newDetails, bundle.RelationKeyId.String())
	if pbtypes.GetString(newDetails, bundle.RelationKeySpaceId.String()) != addr.AnytypeMarketplaceWorkspace {
		return nil, errors.New("object is not bundled")
	}
	newDetails.Fields[bundle.RelationKeySpaceId.String()] = pbtypes.String(spaceID)
	newDetails.Fields[bundle.RelationKeyWorkspaceId.String()] = pbtypes.String(spaceID)

	newDetails.Fields[bundle.RelationKeySourceObject.String()] = pbtypes.String(sourceId)
	// newDetails.Fields[bundle.RelationKeyId.String()] = pbtypes.String(installedID)
	newDetails.Fields[bundle.RelationKeyIsReadonly.String()] = pbtypes.Bool(false)

	predefinedObjects := s.anytype.PredefinedObjects(spaceID)
	switch pbtypes.GetString(newDetails, bundle.RelationKeyType.String()) {
	case bundle.TypeKeyObjectType.BundledURL():
		newDetails.Fields[bundle.RelationKeyType.String()] = pbtypes.String(predefinedObjects.SystemTypes[bundle.TypeKeyObjectType])
	case bundle.TypeKeyRelation.BundledURL():
		newDetails.Fields[bundle.RelationKeyType.String()] = pbtypes.String(predefinedObjects.SystemTypes[bundle.TypeKeyRelation])
	default:
		return nil, fmt.Errorf("unknown object type: %s", pbtypes.GetString(newDetails, bundle.RelationKeyType.String()))
	}
	relations := pbtypes.GetStringList(newDetails, bundle.RelationKeyRecommendedRelations.String())

	if len(relations) > 0 {
		for i, r := range relations {
			// replace relation url with id
			uk, err := uniquekey.New(model.SmartBlockType_STRelation, strings.TrimPrefix(r, addr.BundledRelationURLPrefix))
			if err != nil {
				// should never happen
				return nil, err
			}
			id, err := s.anytype.DeriveObjectId(ctx, spaceID, uk)
			if err != nil {
				// should never happen
				return nil, err
			}
			relations[i] = id
		}
		newDetails.Fields[bundle.RelationKeyRecommendedRelations.String()] = pbtypes.StringList(relations)

	}
	return newDetails, nil
}

func (s *Service) AddBundledObjectToSpace(
	ctx context.Context,
	spaceID string,
	sourceObjectIds []string,
) (ids []string, objects []*types.Struct, err error) {
	// todo: replace this func to the universal space to space copy
	existingObjects, _, err := s.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeySourceObject.String(),
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       pbtypes.StringList(sourceObjectIds),
			},
		},
	})
	var existingObjectMap = make(map[string]struct{})
	for _, existingObject := range existingObjects {
		existingObjectMap[pbtypes.GetString(existingObject.Details, bundle.RelationKeySourceObject.String())] = struct{}{}
	}

	for _, sourceObjectId := range sourceObjectIds {
		if _, ok := existingObjectMap[sourceObjectId]; ok {
			continue
		}
		err = Do(s, sourceObjectId, func(b smartblock.SmartBlock) error {
			d, err := s.prepareDetailsForInstallingObject(ctx, spaceID, b.Id(), b.CombinedDetails())
			if err != nil {
				return err
			}

			uk, err := uniquekey.UnmarshalFromString(pbtypes.GetString(d, bundle.RelationKeyUniqueKey.String()))
			if err != nil {
				return err
			}

			// create via the state directly, because we have cyclic dependencies and we want to avoid typeId resolving from the details
			state := state.NewDocWithUniqueKey("", nil, uk).(*state.State)
			state.SetDetails(d)

			if uk.SmartblockType() == model.SmartBlockType_STRelation {
				state.SetObjectType(bundle.TypeKeyRelation.String())
			} else if uk.SmartblockType() == model.SmartBlockType_STType {
				state.SetObjectType(bundle.TypeKeyObjectType.String())
			} else {
				return fmt.Errorf("unsupported object type: %s", b.Type())
			}

			id, object, err := s.objectCreator.CreateSmartBlockFromState(ctx, spaceID, coresb.SmartBlockType(uk.SmartblockType()), nil, state)
			if err != nil {
				// todo: check alreadyexists error
				// we don't want to stop adding other objects
				log.Errorf("error while block create: %v", err)
				return nil
			}
			ids = append(ids, id)
			objects = append(objects, object)
			return nil
		})
		if err != nil {
			return
		}
	}

	return
}

func (s *Service) SelectWorkspace(req *pb.RpcWorkspaceSelectRequest) error {
	panic("should be removed")
}

func (s *Service) GetCurrentWorkspace(req *pb.RpcWorkspaceGetCurrentRequest) (string, error) {
	workspaceID, err := s.objectStore.GetCurrentWorkspaceID()
	if err != nil && strings.HasSuffix(err.Error(), "key not found") {
		return "", nil
	}
	return workspaceID, err
}

func (s *Service) GetAllWorkspaces(req *pb.RpcWorkspaceGetAllRequest) ([]string, error) {
	return s.anytype.GetAllWorkspaces()
}

func (s *Service) SetIsHighlighted(req *pb.RpcWorkspaceSetIsHighlightedRequest) error {
	panic("is not implemented")
	// workspaceId, _ := s.anytype.GetWorkspaceIdForObject(req.ObjectId)
	// return Do(s,ctx, workspaceId, func(b smartblock.SmartBlock) error {
	//	workspace, ok := b.(*editor.Workspaces)
	//	if !ok {
	//		return fmt.Errorf("incorrect object with workspace id")
	//	}
	//	return workspace.SetIsHighlighted(req.ObjectId, req.IsHighlighted)
	// })
}

func (s *Service) ObjectShareByLink(req *pb.RpcObjectShareByLinkRequest) (link string, err error) {
	return "", fmt.Errorf("not implemented")
	// workspaceId, err := s.anytype.GetWorkspaceIdForObject(req.ObjectId)
	// if err == core.ErrObjectDoesNotBelongToWorkspace {
	//	workspaceId = s.Anytype().AccountObjects().Account
	// }
	// var key string
	// var addrs []string
	// err = Do(s,ctx, workspaceId, func(b smartblock.SmartBlock) error {
	//	workspace, ok := b.(*editor.Workspaces)
	//	if !ok {
	//		return fmt.Errorf("incorrect object with workspace id")
	//	}
	//	key, addrs, err = workspace.GetObjectKeyAddrs(req.ObjectId)
	//	return err
	// })
	// if err != nil {
	//	return "", err
	// }
	// payload := &model.ThreadDeeplinkPayload{
	//	Key:   key,
	//	Addrs: addrs,
	// }
	// marshalledPayload, err := proto.Marshal(payload)
	// if err != nil {
	//	return "", fmt.Errorf("failed to marshal deeplink payload: %w", err)
	// }
	// encodedPayload := base64.RawStdEncoding.EncodeToString(marshalledPayload)
	//
	// params := url.Values{}
	// params.Add("id", req.ObjectId)
	// params.Add("payload", encodedPayload)
	// encoded := params.Encode()
	//
	// return fmt.Sprintf("%s%s", linkObjectShare, encoded), nil
}

func (s *Service) SetPagesIsArchived(ctx session.Context, req pb.RpcObjectListSetIsArchivedRequest) error {
	objectIDsPerSpace, err := s.partitionObjectIDsBySpaceID(req.ObjectIds)
	if err != nil {
		return fmt.Errorf("partition object ids by spaces: %w", err)
	}

	var (
		multiErr   multierror.Error
		anySucceed bool
	)
	for spaceID, objectIDs := range objectIDsPerSpace {
		err = s.setIsArchivedForObjects(spaceID, objectIDs, req.IsArchived)
		if err != nil {
			log.With("spaceID", spaceID, "objectIDs", objectIDs).Errorf("failed to set isArchived=%t objects in space: %s", req.IsArchived, err)
			multiErr.Errors = append(multiErr.Errors, err)
		} else {
			anySucceed = true
		}
	}
	if anySucceed {
		return nil
	}
	return multiErr.ErrorOrNil()
}

func (s *Service) setIsArchivedForObjects(spaceID string, objectIDs []string, isArchived bool) error {
	return Do(s, s.anytype.PredefinedObjects(spaceID).Archive, func(b smartblock.SmartBlock) error {
		archive, ok := b.(collection.Collection)
		if !ok {
			return fmt.Errorf("unexpected archive block type: %T", b)
		}

		var multiErr multierror.Error
		var anySucceed bool
		ids, err := s.objectStore.HasIDs(objectIDs...)
		if err != nil {
			return err
		}
		for _, id := range ids {
			var err error
			if restrErr := s.checkArchivedRestriction(isArchived, spaceID, id); restrErr != nil {
				err = restrErr
			} else {
				if isArchived {
					err = archive.AddObject(id)
				} else {
					err = archive.RemoveObject(id)
				}
			}
			if err != nil {
				log.With("objectID", id).Errorf("failed to set isArchived=%t for object: %s", isArchived, err)
				multiErr.Errors = append(multiErr.Errors, err)
				continue
			}
			anySucceed = true
		}

		if err := multiErr.ErrorOrNil(); err != nil {
			log.Warnf("failed to archive: %s", err)
		}
		if anySucceed {
			return nil
		}
		return multiErr.ErrorOrNil()
	})
}

func (s *Service) partitionObjectIDsBySpaceID(objectIDs []string) (map[string][]string, error) {
	res := map[string][]string{}
	for _, objectID := range objectIDs {
		spaceID, err := s.objectStore.ResolveSpaceID(objectID)
		if err != nil {
			return nil, fmt.Errorf("resolve spaceID: %w", err)
		}
		res[spaceID] = append(res[spaceID], objectID)
	}
	return res, nil
}

// SetPagesIsFavorite is deprecated
func (s *Service) SetPagesIsFavorite(ctx session.Context, req pb.RpcObjectListSetIsFavoriteRequest) error {
	// TODO Resolve spaceID for each ids
	return fmt.Errorf("have to be fixed")
	// return Do(s, s.anytype.PredefinedObjects(ctx.SpaceID()).Home, func(b smartblock.SmartBlock) error {
	// 	fav, ok := b.(collection.Collection)
	// 	if !ok {
	// 		return fmt.Errorf("unexpected home block type: %T", b)
	// 	}
	//
	// 	ids, err := s.objectStore.HasIDs(req.ObjectIds...)
	// 	if err != nil {
	// 		return err
	// 	}
	// 	var merr multierror.Error
	// 	var anySucceed bool
	// 	for _, id := range ids {
	// 		var err error
	// 		if req.IsFavorite {
	// 			err = fav.AddObject(id)
	// 		} else {
	// 			err = fav.RemoveObject(id)
	// 		}
	// 		if err != nil {
	// 			log.Errorf("failed to favorite object %s: %s", id, err.Error())
	// 			merr.Errors = append(merr.Errors, err)
	// 			continue
	// 		}
	// 		anySucceed = true
	// 	}
	// 	if err := merr.ErrorOrNil(); err != nil {
	// 		log.Warnf("failed to set objects as favorite: %s", err)
	// 	}
	// 	if anySucceed {
	// 		return nil
	// 	}
	// 	return merr.ErrorOrNil()
	// })
}

func (s *Service) objectLinksCollectionModify(collectionId string, objectId string, value bool) error {
	return Do(s, collectionId, func(b smartblock.SmartBlock) error {
		coll, ok := b.(collection.Collection)
		if !ok {
			return fmt.Errorf("unsupported sb block type: %T", b)
		}
		if value {
			return coll.AddObject(objectId)
		} else {
			return coll.RemoveObject(objectId)
		}
	})
}

func (s *Service) SetPageIsFavorite(req pb.RpcObjectSetIsFavoriteRequest) (err error) {
	spaceID, err := s.ResolveSpaceID(req.ContextId)
	if err != nil {
		return fmt.Errorf("resolve spaceID: %w", err)
	}
	return s.objectLinksCollectionModify(s.anytype.PredefinedObjects(spaceID).Home, req.ContextId, req.IsFavorite)
}

func (s *Service) SetPageIsArchived(req pb.RpcObjectSetIsArchivedRequest) (err error) {
	spaceID, err := s.ResolveSpaceID(req.ContextId)
	if err != nil {
		return fmt.Errorf("resolve spaceID: %w", err)
	}
	if err := s.checkArchivedRestriction(req.IsArchived, spaceID, req.ContextId); err != nil {
		return err
	}
	return s.objectLinksCollectionModify(s.anytype.PredefinedObjects(spaceID).Archive, req.ContextId, req.IsArchived)
}

func (s *Service) SetSource(ctx session.Context, req pb.RpcObjectSetSourceRequest) (err error) {
	return Do(s, req.ContextId, func(b smartblock.SmartBlock) error {
		st := b.NewStateCtx(ctx)
		st.SetDetailAndBundledRelation(bundle.RelationKeySetOf, pbtypes.StringList(req.Source))
		return b.Apply(st, smartblock.NoRestrictions)
	})
}

func (s *Service) SetWorkspaceDashboardId(ctx session.Context, workspaceId string, id string) (setId string, err error) {
	err = Do(s, workspaceId, func(ws *editor.Workspaces) error {
		if ws.Type() != model.SmartBlockType_Workspace {
			return ErrUnexpectedBlockType
		}
		if err = ws.SetDetails(ctx, []*pb.RpcObjectSetDetailsDetail{
			{
				Key:   bundle.RelationKeySpaceDashboardId.String(),
				Value: pbtypes.String(id),
			},
		}, false); err != nil {
			return err
		}
		return nil
	})
	return id, err
}

func (s *Service) checkArchivedRestriction(isArchived bool, spaceID string, objectId string) error {
	if !isArchived {
		return nil
	}
	if err := s.restriction.CheckRestrictions(spaceID, objectId, model.Restrictions_Delete); err != nil {
		return err
	}
	return nil
}

func (s *Service) DeleteArchivedObjects(ctx session.Context, req pb.RpcObjectListDeleteRequest) (err error) {
	// TODO Resolve spaces for each ID
	return fmt.Errorf("have to be fixed")
	// return Do(s, s.anytype.PredefinedObjects(ctx.SpaceID()).Archive, func(b smartblock.SmartBlock) error {
	// 	archive, ok := b.(collection.Collection)
	// 	if !ok {
	// 		return fmt.Errorf("unexpected archive block type: %T", b)
	// 	}
	//
	// 	var merr multierror.Error
	// 	var anySucceed bool
	// 	for _, blockId := range req.ObjectIds {
	// 		if exists, _ := archive.HasObject(blockId); exists {
	// 			if err = s.DeleteObject(ctx, blockId); err != nil {
	// 				merr.Errors = append(merr.Errors, err)
	// 				continue
	// 			}
	// 			archive.RemoveObject(blockId)
	// 			anySucceed = true
	// 		}
	// 	}
	// 	if err := merr.ErrorOrNil(); err != nil {
	// 		log.Warnf("failed to delete archived objects: %s", err)
	// 	}
	// 	if anySucceed {
	// 		return nil
	// 	}
	// 	return merr.ErrorOrNil()
	// })
}

func (s *Service) ObjectsDuplicate(ctx context.Context, ids []string) (newIds []string, err error) {
	var newId string
	var merr multierror.Error
	var anySucceed bool
	for _, id := range ids {
		if newId, err = s.ObjectDuplicate(ctx, id); err != nil {
			merr.Errors = append(merr.Errors, err)
			continue
		}
		newIds = append(newIds, newId)
		anySucceed = true
	}
	if err := merr.ErrorOrNil(); err != nil {
		log.Warnf("failed to duplicate objects: %s", err)
	}
	if anySucceed {
		return newIds, nil
	}
	return nil, merr.ErrorOrNil()
}

func (s *Service) DeleteArchivedObject(id string) (err error) {
	spaceID, err := s.objectStore.ResolveSpaceID(id)
	if err != nil {
		return fmt.Errorf("resolve spaceID: %w", err)
	}
	return Do(s, s.anytype.PredefinedObjects(spaceID).Archive, func(b smartblock.SmartBlock) error {
		archive, ok := b.(collection.Collection)
		if !ok {
			return fmt.Errorf("unexpected archive block type: %T", b)
		}

		if exists, _ := archive.HasObject(id); exists {
			if err = s.DeleteObject(id); err == nil {
				err = archive.RemoveObject(id)
				if err != nil {
					return err
				}
			}
		}

		return nil
	})
}

func (s *Service) OnDelete(id domain.FullID, workspaceRemove func() error) error {
	var (
		isFavorite bool
	)

	err := Do(s, id.ObjectID, func(b smartblock.SmartBlock) error {
		b.ObjectCloseAllSessions()
		st := b.NewState()
		isFavorite = pbtypes.GetBool(st.LocalDetails(), bundle.RelationKeyIsFavorite.String())
		if isFavorite {
			_ = s.SetPageIsFavorite(pb.RpcObjectSetIsFavoriteRequest{IsFavorite: false, ContextId: id.ObjectID})
		}
		b.SetIsDeleted()
		if workspaceRemove != nil {
			return workspaceRemove()
		}
		return nil
	})
	if err != nil {
		log.Error("failed to perform delete operation on object", zap.Error(err))
	}
	if err := s.objectStore.DeleteObject(id.ObjectID); err != nil {
		return fmt.Errorf("delete object from local store: %w", err)
	}

	return nil
}

func (s *Service) sendOnRemoveEvent(ids ...string) {
	s.eventSender.Broadcast(&pb.Event{
		Messages: []*pb.EventMessage{
			{
				Value: &pb.EventMessageValueOfObjectRemove{
					ObjectRemove: &pb.EventObjectRemove{
						Ids: ids,
					},
				},
			},
		},
	})
}

func (s *Service) RemoveListOption(ctx session.Context, optIds []string, checkInObjects bool) error {
	// TODO Resolve spaces for each ID
	return fmt.Errorf("have to be fixed")
	// var workspace *editor.Workspaces
	// if err := Do(s, s.anytype.PredefinedObjects(ctx.SpaceID()).Account, func(b smartblock.SmartBlock) error {
	// 	var ok bool
	// 	if workspace, ok = b.(*editor.Workspaces); !ok {
	// 		return fmt.Errorf("incorrect object with workspace id")
	// 	}
	// 	return nil
	// }); err != nil {
	// 	return err
	// }
	//
	// for _, id := range optIds {
	// 	if checkInObjects {
	// 		opt, err := workspace.Open(id)
	// 		if err != nil {
	// 			return fmt.Errorf("workspace open: %w", err)
	// 		}
	// 		relKey := pbtypes.GetString(opt.Details(), bundle.RelationKeyRelationKey.String())
	//
	// 		q := database.Query{
	// 			Filters: []*model.BlockContentDataviewFilter{
	// 				{
	// 					Condition:   model.BlockContentDataviewFilter_Equal,
	// 					RelationKey: relKey,
	// 					Value:       pbtypes.String(opt.Id()),
	// 				},
	// 			},
	// 		}
	// 		records, _, err := s.objectStore.Query(nil, q)
	// 		if err != nil {
	// 			return nil
	// 		}
	//
	// 		if len(records) > 0 {
	// 			return ErrOptionUsedByOtherObjects
	// 		}
	// 	}
	//
	// 	if err := s.DeleteObject(ctx, id); err != nil {
	// 		return err
	// 	}
	// }
	//
	// return nil
}

// TODO: remove proxy
func (s *Service) Process() process.Service {
	return s.process
}

// TODO: remove proxy
func (s *Service) ProcessAdd(p process.Process) (err error) {
	return s.process.Add(p)
}

// TODO: remove proxy
func (s *Service) ProcessCancel(id string) (err error) {
	return s.process.Cancel(id)
}

func (s *Service) Close(ctx context.Context) (err error) {
	close(s.closing)
	return s.cache.Close()
}

func (s *Service) ResolveSpaceID(objectID string) (spaceID string, err error) {
	return s.objectStore.ResolveSpaceID(objectID)
}

// PickBlock returns opened smartBlock or opens smartBlock in silent mode
func (s *Service) PickBlock(ctx context.Context, objectID string) (sb smartblock.SmartBlock, err error) {
	spaceID, err := s.objectStore.ResolveSpaceID(objectID)
	if err != nil {
		return nil, fmt.Errorf("resolve spaceID: %w", err)
	}
	if spaceID == "" {
		// todo: feels dirty
		if opts, ok := ctx.Value(optsKey).(cacheOpts); ok {
			if opts.spaceId != "" {
				spaceID = opts.spaceId
				s.objectStore.StoreSpaceID(objectID, opts.spaceId)
			}
		}

	}

	return s.getSmartblock(ctx, domain.FullID{
		SpaceID:  spaceID,
		ObjectID: objectID,
	})
}

// TODO Remove this wrapper
func (s *Service) getSmartblock(ctx context.Context, id domain.FullID) (sb smartblock.SmartBlock, err error) {
	return s.GetObjectWithTimeout(ctx, id)
}

func (s *Service) StateFromTemplate(templateID string, name string) (st *state.State, err error) {
	if err = Do(s, templateID, func(b smartblock.SmartBlock) error {
		if tmpl, ok := b.(*editor.Template); ok {
			st, err = tmpl.GetNewPageState(name)
		} else {
			return fmt.Errorf("not a template")
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("can't apply template: %v", err)
	}
	return
}

func (s *Service) DoFileNonLock(id string, apply func(b file.File) error) error {
	sb, err := s.PickBlock(context.Background(), id)
	if err != nil {
		return err
	}

	if bb, ok := sb.(file.File); ok {
		return apply(bb)
	}
	return fmt.Errorf("file non lock operation not available for this block type: %T", sb)
}

type Picker interface {
	PickBlock(ctx context.Context, objectID string) (sb smartblock.SmartBlock, err error)
}

func Do[t any](p Picker, objectID string, apply func(sb t) error) error {
	ctx := context.Background()
	sb, err := p.PickBlock(ctx, objectID)
	if err != nil {
		return err
	}

	bb, ok := sb.(t)
	if !ok {
		var dummy = new(t)
		return fmt.Errorf("the interface %T is not implemented in %T", dummy, sb)
	}

	sb.Lock()
	defer sb.Unlock()
	return apply(bb)
}

func DoContext[t any](p Picker, ctx context.Context, objectID string, apply func(sb t) error) error {
	sb, err := p.PickBlock(ctx, objectID)
	if err != nil {
		return err
	}

	bb, ok := sb.(t)
	if !ok {
		var dummy = new(t)
		return fmt.Errorf("the interface %T is not implemented in %T", dummy, sb)
	}

	sb.Lock()
	defer sb.Unlock()
	return apply(bb)
}

// DoState2 picks two blocks and perform an action on them. The order of locks is always the same for two ids.
// It correctly handles the case when two ids are the same.
func DoState2[t1, t2 any](s Picker, firstID, secondID string, f func(*state.State, *state.State, t1, t2) error) error {
	if firstID == secondID {
		return DoStateAsync(s, firstID, func(st *state.State, b t1) error {
			// Check that b satisfies t2
			b2, ok := any(b).(t2)
			if !ok {
				var dummy t2
				return fmt.Errorf("block %s is not of type %T", firstID, dummy)
			}
			return f(st, st, b, b2)
		})
	}
	if firstID < secondID {
		return DoStateAsync(s, firstID, func(firstState *state.State, firstBlock t1) error {
			return DoStateAsync(s, secondID, func(secondState *state.State, secondBlock t2) error {
				return f(firstState, secondState, firstBlock, secondBlock)
			})
		})
	}
	return DoStateAsync(s, secondID, func(secondState *state.State, secondBlock t2) error {
		return DoStateAsync(s, firstID, func(firstState *state.State, firstBlock t1) error {
			return f(firstState, secondState, firstBlock, secondBlock)
		})
	})
}

func DoStateAsync[t any](p Picker, id string, apply func(s *state.State, sb t) error, flags ...smartblock.ApplyFlag) error {
	ctx := context.Background()
	sb, err := p.PickBlock(ctx, id)
	if err != nil {
		return err
	}

	bb, ok := sb.(t)
	if !ok {
		var dummy = new(t)
		return fmt.Errorf("the interface %T is not implemented in %T", dummy, sb)
	}

	sb.Lock()
	defer sb.Unlock()

	st := sb.NewState()
	err = apply(st, bb)
	if err != nil {
		return fmt.Errorf("apply func: %w", err)
	}

	return sb.Apply(st, flags...)
}

// TODO rename to something more meaningful
func DoStateCtx[t any](p Picker, ctx session.Context, id string, apply func(s *state.State, sb t) error, flags ...smartblock.ApplyFlag) error {
	sb, err := p.PickBlock(context.Background(), id)
	if err != nil {
		return err
	}

	bb, ok := sb.(t)
	if !ok {
		var dummy = new(t)
		return fmt.Errorf("the interface %T is not implemented in %T", dummy, sb)
	}

	sb.Lock()
	defer sb.Unlock()

	st := sb.NewStateCtx(ctx)
	err = apply(st, bb)
	if err != nil {
		return fmt.Errorf("apply func: %w", err)
	}

	return sb.Apply(st, flags...)
}

func (s *Service) ObjectApplyTemplate(contextId, templateId string) error {
	return Do(s, contextId, func(b smartblock.SmartBlock) error {
		orig := b.NewState().ParentState()
		name := pbtypes.GetString(orig.Details(), bundle.RelationKeyName.String())
		ts, err := s.StateFromTemplate(templateId, name)
		if err != nil {
			return err
		}
		ts.SetRootId(contextId)
		ts.SetParent(orig)

		fromLayout, _ := orig.Layout()
		if toLayout, ok := orig.Layout(); ok {
			if err := s.layoutConverter.Convert(ts, fromLayout, toLayout); err != nil {
				return fmt.Errorf("convert layout: %w", err)
			}
		}

		ts.BlocksInit(ts)
		objType := ts.ObjectTypeKey()
		// StateFromTemplate returns state without the localdetails, so they will be taken from the orig state
		ts.SetObjectType(objType)

		flags := internalflag.NewFromState(ts)
		flags.Remove(model.InternalFlag_editorSelectType)
		flags.Remove(model.InternalFlag_editorSelectTemplate)
		flags.AddToState(ts)

		return b.Apply(ts, smartblock.NoRestrictions)
	})
}

func (s *Service) ResetToState(pageID string, st *state.State) (err error) {
	return Do(s, pageID, func(sb smartblock.SmartBlock) error {
		return history.ResetToVersion(sb, st)
	})
}

func (s *Service) ObjectBookmarkFetch(req pb.RpcObjectBookmarkFetchRequest) (err error) {
	spaceID, err := s.objectStore.ResolveSpaceID(req.ContextId)
	if err != nil {
		return fmt.Errorf("resolve spaceID: %w", err)
	}
	url, err := uri.NormalizeURI(req.Url)
	if err != nil {
		return fmt.Errorf("process uri: %w", err)
	}
	res := s.bookmark.FetchBookmarkContent(spaceID, url)
	go func() {
		if err := s.bookmark.UpdateBookmarkObject(req.ContextId, res); err != nil {
			log.Errorf("update bookmark object %s: %s", req.ContextId, err)
		}
	}()
	return nil
}

func (s *Service) ObjectToBookmark(ctx context.Context, id string, url string) (objectId string, err error) {
	spaceID, err := s.objectStore.ResolveSpaceID(id)
	if err != nil {
		return "", fmt.Errorf("resolve spaceID: %w", err)
	}
	objectId, _, err = s.objectCreator.CreateObject(ctx, spaceID, &pb.RpcObjectCreateBookmarkRequest{
		Details: &types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeySource.String(): pbtypes.String(url),
			},
		},
	}, bundle.TypeKeyBookmark)
	if err != nil {
		return
	}

	oStore := s.app.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	res, err := oStore.GetWithLinksInfoByID(spaceID, id)
	if err != nil {
		return
	}
	for _, il := range res.Links.Inbound {
		if err = s.replaceLink(il.Id, id, objectId); err != nil {
			return
		}
	}
	err = s.DeleteObject(id)
	if err != nil {
		// intentionally do not return error here
		log.Errorf("failed to delete object after conversion to bookmark: %s", err.Error())
		err = nil
	}

	return
}

func (s *Service) replaceLink(id, oldId, newId string) error {
	return Do(s, id, func(b basic.CommonOperations) error {
		return b.ReplaceLink(oldId, newId)
	})
}

func (s *Service) GetLogFields() []zap.Field {
	var fields []zap.Field
	if s.predefinedObjectWasMissing {
		fields = append(fields, zap.Bool("predefined_object_was_missing", true))
	}
	return fields
}
