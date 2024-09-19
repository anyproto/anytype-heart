package block

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"github.com/hashicorp/go-multierror"
	"github.com/samber/lo"
	"go.uber.org/zap"

	bookmarksvc "github.com/anyproto/anytype-heart/core/block/bookmark"
	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/collection"
	"github.com/anyproto/anytype-heart/core/block/editor/file"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/history"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/bookmark"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/core/files/fileuploader"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/internalflag"
	"github.com/anyproto/anytype-heart/util/mutex"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/uri"

	_ "github.com/anyproto/anytype-heart/core/block/editor/table"
	_ "github.com/anyproto/anytype-heart/core/block/simple/file"
	_ "github.com/anyproto/anytype-heart/core/block/simple/link"
	_ "github.com/anyproto/anytype-heart/core/block/simple/widget"
)

const (
	CName = "block-service"
)

var (
	ErrUnexpectedBlockType = errors.New("unexpected block type")
	ErrUnknownObjectType   = fmt.Errorf("unknown object type")
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
	s := &Service{
		openedObjs: &openedObjects{
			objects: make(map[string]bool),
			lock:    &sync.Mutex{},
		},
	}
	return s
}

type Service struct {
	eventSender          event.Sender
	process              process.Service
	app                  *app.App
	objectStore          objectstore.ObjectStore
	restriction          restriction.Service
	bookmark             bookmarksvc.Service
	objectCreator        objectcreator.Service
	templateService      templateService
	resolver             idresolver.Resolver
	spaceService         space.Service
	tempDirProvider      core.TempDirProvider
	builtinObjectService builtinObjects
	fileObjectService    fileobject.Service

	fileService         files.Service
	fileUploaderService fileuploader.Service

	predefinedObjectWasMissing bool
	openedObjs                 *openedObjects
}

type builtinObjects interface {
	CreateObjectsForUseCase(ctx session.Context, spaceID string, req pb.RpcObjectImportUseCaseRequestUseCase) (code pb.RpcObjectImportUseCaseResponseErrorCode, err error)
}

type templateService interface {
	CreateTemplateStateWithDetails(templateId string, details *types.Struct) (*state.State, error)
	CreateTemplateStateFromSmartBlock(sb smartblock.SmartBlock, details *types.Struct) *state.State
}

type openedObjects struct {
	objects map[string]bool
	lock    *sync.Mutex
}

func (s *Service) Name() string {
	return CName
}

func (s *Service) Init(a *app.App) (err error) {
	s.process = a.MustComponent(process.CName).(process.Service)
	s.eventSender = a.MustComponent(event.CName).(event.Sender)
	s.objectStore = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	s.restriction = a.MustComponent(restriction.CName).(restriction.Service)
	s.bookmark = a.MustComponent("bookmark-importer").(bookmarksvc.Service)
	s.objectCreator = app.MustComponent[objectcreator.Service](a)
	s.templateService = app.MustComponent[templateService](a)
	s.spaceService = a.MustComponent(space.CName).(space.Service)
	s.fileService = app.MustComponent[files.Service](a)
	s.resolver = a.MustComponent(idresolver.CName).(idresolver.Resolver)
	s.fileObjectService = app.MustComponent[fileobject.Service](a)
	s.fileUploaderService = app.MustComponent[fileuploader.Service](a)

	s.tempDirProvider = app.MustComponent[core.TempDirProvider](a)

	s.builtinObjectService = app.MustComponent[builtinObjects](a)
	s.app = a
	return
}

func (s *Service) Run(ctx context.Context) (err error) {
	return
}

func (s *Service) GetObject(ctx context.Context, objectID string) (sb smartblock.SmartBlock, err error) {
	spaceID, err := s.resolver.ResolveSpaceID(objectID)
	if err != nil {
		return nil, err
	}
	return s.GetObjectByFullID(ctx, domain.FullID{SpaceID: spaceID, ObjectID: objectID})
}

func (s *Service) GetObjectByFullID(ctx context.Context, id domain.FullID) (sb smartblock.SmartBlock, err error) {
	spc, err := s.spaceService.Get(ctx, id.SpaceID)
	if err != nil {
		return nil, fmt.Errorf("get space: %w", err)
	}
	return spc.GetObject(ctx, id.ObjectID)
}

func (s *Service) OpenBlock(sctx session.Context, id domain.FullID, includeRelationsAsDependentObjects bool) (obj *model.ObjectView, err error) {
	id = s.resolveFullId(id)
	startTime := time.Now()
	err = s.DoFullId(id, func(ob smartblock.SmartBlock) error {
		if includeRelationsAsDependentObjects {
			ob.EnabledRelationAsDependentObjects()
		}
		afterSmartBlockTime := time.Now()

		ob.RegisterSession(sctx)

		afterDataviewTime := time.Now()
		st := ob.NewState()

		st.SetLocalDetail(bundle.RelationKeyLastOpenedDate.String(), pbtypes.Int64(time.Now().Unix()))
		if err = ob.Apply(st, smartblock.NoHistory, smartblock.NoEvent, smartblock.SkipIfNoChanges, smartblock.KeepInternalFlags, smartblock.IgnoreNoPermissions); err != nil {
			log.Errorf("failed to update lastOpenedDate: %s", err)
		}
		afterApplyTime := time.Now()
		if obj, err = ob.Show(); err != nil {
			return fmt.Errorf("show: %w", err)
		}
		afterShowTime := time.Now()

		if err != nil && !errors.Is(err, treestorage.ErrUnknownTreeId) {
			log.Errorf("failed to watch status for object %s: %s", id, err)
		}

		afterHashesTime := time.Now()
		metrics.Service.Send(&metrics.OpenBlockEvent{
			ObjectId:       id.ObjectID,
			GetBlockMs:     afterSmartBlockTime.Sub(startTime).Milliseconds(),
			DataviewMs:     afterDataviewTime.Sub(afterSmartBlockTime).Milliseconds(),
			ApplyMs:        afterApplyTime.Sub(afterDataviewTime).Milliseconds(),
			ShowMs:         afterShowTime.Sub(afterApplyTime).Milliseconds(),
			FileWatcherMs:  afterHashesTime.Sub(afterShowTime).Milliseconds(),
			SmartblockType: int(ob.Type()),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	mutex.WithLock(s.openedObjs.lock, func() any { s.openedObjs.objects[id.ObjectID] = true; return nil })
	return obj, nil
}

func (s *Service) DoFullId(id domain.FullID, apply func(sb smartblock.SmartBlock) error) error {
	space, err := s.spaceService.Get(context.Background(), id.SpaceID)
	if err != nil {
		return fmt.Errorf("get space: %w", err)
	}
	return space.Do(id.ObjectID, apply)
}

// resolveFullId resolves missing spaceId
func (s *Service) resolveFullId(id domain.FullID) domain.FullID {
	// First try to resolve space. It's necessary if client accidentally passes wrong spaceId
	spaceId, err := s.resolver.ResolveSpaceID(id.ObjectID)
	if err == nil {
		return domain.FullID{SpaceID: spaceId, ObjectID: id.ObjectID}
	}
	// Or use spaceId from request
	return id
}

func (s *Service) ShowBlock(id domain.FullID, includeRelationsAsDependentObjects bool) (obj *model.ObjectView, err error) {
	id = s.resolveFullId(id)
	err = s.DoFullId(id, func(b smartblock.SmartBlock) error {
		if includeRelationsAsDependentObjects {
			b.EnabledRelationAsDependentObjects()
		}
		obj, err = b.Show()
		return err
	})
	return obj, err
}

func (s *Service) CloseBlock(ctx session.Context, id domain.FullID) error {
	id = s.resolveFullId(id)
	var isDraft bool
	err := s.DoFullId(id, func(b smartblock.SmartBlock) error {
		b.ObjectClose(ctx)
		s := b.NewState()
		isDraft = internalflag.NewFromState(s).Has(model.InternalFlag_editorDeleteEmpty)
		return nil
	})
	if err != nil {
		return err
	}

	if isDraft {
		if err = s.DeleteObjectByFullID(id); err != nil {
			log.Errorf("error while block delete: %v", err)
		} else {
			sendOnRemoveEvent(s.eventSender, id.ObjectID)
		}
	}
	mutex.WithLock(s.openedObjs.lock, func() any { delete(s.openedObjs.objects, id.ObjectID); return nil })
	return nil
}

func (s *Service) GetOpenedObjects() []string {
	return mutex.WithLock(s.openedObjs.lock, func() []string { return lo.Keys(s.openedObjs.objects) })
}

func (s *Service) SpaceInstallBundledObject(
	ctx context.Context,
	spaceId string,
	sourceObjectId string,
) (id string, object *types.Struct, err error) {
	spc, err := s.spaceService.Get(ctx, spaceId)
	if err != nil {
		return "", nil, fmt.Errorf("get space: %w", err)
	}
	ids, details, err := s.objectCreator.InstallBundledObjects(ctx, spc, []string{sourceObjectId}, false)
	if err != nil {
		return "", nil, err
	}
	if len(ids) == 0 {
		return "", nil, fmt.Errorf("failed to add object")
	}

	return ids[0], details[0], nil
}

func (s *Service) SpaceInstallBundledObjects(
	ctx context.Context,
	spaceId string,
	sourceObjectIds []string,
) (ids []string, objects []*types.Struct, err error) {
	spc, err := s.spaceService.Get(ctx, spaceId)
	if err != nil {
		return nil, nil, fmt.Errorf("get space: %w", err)
	}
	return s.objectCreator.InstallBundledObjects(ctx, spc, sourceObjectIds, false)
}

func (s *Service) SelectWorkspace(req *pb.RpcWorkspaceSelectRequest) error {
	panic("should be removed")
}

func (s *Service) GetCurrentWorkspace(req *pb.RpcWorkspaceGetCurrentRequest) (string, error) {
	panic("should be removed")
}

func (s *Service) GetAllWorkspaces(req *pb.RpcWorkspaceGetAllRequest) ([]string, error) {
	panic("should be removed")
}

func (s *Service) SetSpaceInfo(req *pb.RpcWorkspaceSetInfoRequest) error {
	ctx := context.TODO()
	spc, err := s.spaceService.Get(ctx, req.SpaceId)
	if err != nil {
		return err
	}
	workspaceId := spc.DerivedIDs().Workspace

	setDetails := make([]*model.Detail, 0, len(req.Details.GetFields()))
	for k, v := range req.Details.GetFields() {
		setDetails = append(setDetails, &model.Detail{
			Key:   k,
			Value: v,
		})
	}
	return s.SetDetails(nil, workspaceId, setDetails)
}

func (s *Service) ObjectShareByLink(req *pb.RpcObjectShareByLinkRequest) (link string, err error) {
	panic("should be removed")
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
	spc, err := s.spaceService.Get(context.Background(), spaceID)
	if err != nil {
		return fmt.Errorf("get space: %w", err)
	}
	return cache.Do(s, spc.DerivedIDs().Archive, func(b smartblock.SmartBlock) error {
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
		spaceID, err := s.resolver.ResolveSpaceID(objectID)
		if err != nil {
			return nil, fmt.Errorf("resolve spaceID: %w", err)
		}
		res[spaceID] = append(res[spaceID], objectID)
	}
	return res, nil
}

func (s *Service) SetPagesIsFavorite(req pb.RpcObjectListSetIsFavoriteRequest) error {
	ids, err := s.objectStore.HasIDs(req.ObjectIds...)
	if err != nil {
		return err
	}
	var (
		anySucceed  bool
		resultError error
	)
	for _, id := range ids {
		err := s.SetPageIsFavorite(pb.RpcObjectSetIsFavoriteRequest{
			ContextId:  id,
			IsFavorite: req.IsFavorite,
		})
		if err != nil {
			log.Errorf("failed to favorite object %s: %s", id, err)
			resultError = errors.Join(resultError, err)
		} else {
			anySucceed = true
		}
	}
	if resultError != nil {
		log.Warnf("failed to set objects as favorite: %s", resultError)
	}
	if anySucceed {
		return nil
	}
	return resultError
}

func (s *Service) objectLinksCollectionModify(collectionId string, objectId string, value bool) error {
	return cache.Do(s, collectionId, func(b smartblock.SmartBlock) error {
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
	spaceID, err := s.resolver.ResolveSpaceID(req.ContextId)
	if err != nil {
		return fmt.Errorf("resolve spaceID: %w", err)
	}
	spc, err := s.spaceService.Get(context.Background(), spaceID)
	if err != nil {
		return fmt.Errorf("get space: %w", err)
	}
	return s.objectLinksCollectionModify(spc.DerivedIDs().Home, req.ContextId, req.IsFavorite)
}

func (s *Service) SetPageIsArchived(req pb.RpcObjectSetIsArchivedRequest) (err error) {
	spaceID, err := s.resolver.ResolveSpaceID(req.ContextId)
	if err != nil {
		return fmt.Errorf("resolve spaceID: %w", err)
	}
	spc, err := s.spaceService.Get(context.Background(), spaceID)
	if err != nil {
		return fmt.Errorf("get space: %w", err)
	}
	if err := s.checkArchivedRestriction(req.IsArchived, spaceID, req.ContextId); err != nil {
		return err
	}
	return s.objectLinksCollectionModify(spc.DerivedIDs().Archive, req.ContextId, req.IsArchived)
}

func (s *Service) SetSource(ctx session.Context, req pb.RpcObjectSetSourceRequest) (err error) {
	return cache.Do(s, req.ContextId, func(sb smartblock.SmartBlock) error {
		st := sb.NewStateCtx(ctx)
		// nolint:errcheck
		_ = st.Iterate(func(b simple.Block) (isContinue bool) {
			if dv := b.Model().GetDataview(); dv != nil {
				for _, view := range dv.Views {
					view.DefaultTemplateId = ""
					view.DefaultObjectTypeId = ""
				}
				st.Set(b)
				return false
			}
			return true
		})
		st.SetDetailAndBundledRelation(bundle.RelationKeySetOf, pbtypes.StringList(req.Source))

		flags := internalflag.NewFromState(st)
		// set with source is no longer empty
		flags.Remove(model.InternalFlag_editorDeleteEmpty)
		flags.AddToState(st)

		return sb.Apply(st, smartblock.NoRestrictions, smartblock.KeepInternalFlags)
	})
}

func (s *Service) SetWorkspaceDashboardId(ctx session.Context, workspaceId string, id string) (setId string, err error) {
	err = cache.Do(s, workspaceId, func(ws *editor.Workspaces) error {
		if ws.Type() != coresb.SmartBlockTypeWorkspace {
			return ErrUnexpectedBlockType
		}
		if err = ws.SetDetails(ctx, []*model.Detail{
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
	return cache.Do(s, objectId, func(sb smartblock.SmartBlock) error {
		return s.restriction.CheckRestrictions(sb, model.Restrictions_Delete)
	})
}

func (s *Service) DeleteArchivedObjects(objectIDs []string) error {
	var (
		resultError error
		anySucceed  bool
	)
	for _, objectID := range objectIDs {
		err := s.DeleteArchivedObject(objectID)
		if err != nil {
			resultError = errors.Join(resultError, err)
		} else {
			anySucceed = true
		}
	}
	if resultError != nil {
		log.Warnf("failed to delete archived objects: %s", resultError)
	}
	if anySucceed {
		return nil
	}
	return resultError
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
	spaceID, err := s.resolver.ResolveSpaceID(id)
	if err != nil {
		return fmt.Errorf("resolve spaceID: %w", err)
	}
	spc, err := s.spaceService.Get(context.Background(), spaceID)
	if err != nil {
		return fmt.Errorf("get space: %w", err)
	}
	return cache.Do(s, spc.DerivedIDs().Archive, func(b smartblock.SmartBlock) error {
		archive, ok := b.(collection.Collection)
		if !ok {
			return fmt.Errorf("unexpected archive block type: %T", b)
		}

		err = s.DeleteObject(id)
		if err != nil {
			return fmt.Errorf("delete object: %w", err)
		}
		if exists, _ := archive.HasObject(id); exists {
			err = archive.RemoveObject(id)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) RemoveListOption(optionIds []string, checkInObjects bool) error {
	for _, id := range optionIds {
		if checkInObjects {
			err := cache.Do(s, id, func(b smartblock.SmartBlock) error {
				st := b.NewState()
				relKey := pbtypes.GetString(st.Details(), bundle.RelationKeyRelationKey.String())

				records, err := s.objectStore.Query(database.Query{
					Filters: []*model.BlockContentDataviewFilter{
						{
							Condition:   model.BlockContentDataviewFilter_Equal,
							RelationKey: relKey,
							Value:       pbtypes.String(id),
						},
					},
				})
				if err != nil {
					return fmt.Errorf("query dependent objects: %w", err)
				}

				if len(records) > 0 {
					return ErrOptionUsedByOtherObjects
				}

				return nil
			})
			if err != nil {
				return fmt.Errorf("check option usage: %w", err)
			}
		}

		if err := s.DeleteObject(id); err != nil {
			return err
		}
	}

	return nil
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
	return nil
}

func (s *Service) DoFileNonLock(id string, apply func(b file.File) error) error {
	sb, err := s.GetObject(context.Background(), id)
	if err != nil {
		return err
	}

	if bb, ok := sb.(file.File); ok {
		return apply(bb)
	}
	return fmt.Errorf("file non lock operation not available for this block type: %T", sb)
}

func (s *Service) ResetToState(pageID string, st *state.State) (err error) {
	return cache.Do(s, pageID, func(sb smartblock.SmartBlock) error {
		return history.ResetToVersion(sb, st)
	})
}

func (s *Service) ObjectBookmarkFetch(req pb.RpcObjectBookmarkFetchRequest) (err error) {
	spaceID, err := s.resolver.ResolveSpaceID(req.ContextId)
	if err != nil {
		return fmt.Errorf("resolve spaceID: %w", err)
	}
	url, err := uri.NormalizeURI(req.Url)
	if err != nil {
		return fmt.Errorf("process uri: %w", err)
	}
	res := s.bookmark.FetchBookmarkContent(spaceID, url, false)
	go func() {
		if err := s.bookmark.UpdateObject(req.ContextId, res()); err != nil {
			log.Errorf("update bookmark object %s: %s", req.ContextId, err)
		}
	}()
	return nil
}

func (s *Service) ObjectToBookmark(ctx context.Context, id string, url string) (objectId string, err error) {
	spaceID, err := s.resolver.ResolveSpaceID(id)
	if err != nil {
		return "", fmt.Errorf("resolve spaceID: %w", err)
	}
	req := objectcreator.CreateObjectRequest{
		ObjectTypeKey: bundle.TypeKeyBookmark,
		Details: &types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeySource.String(): pbtypes.String(url),
			},
		},
	}
	objectId, _, err = s.objectCreator.CreateObject(ctx, spaceID, req)
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
		log.Errorf("failed to delete object after conversion to bookmark: %s", err)
		err = nil
	}

	return
}

func (s *Service) CreateObjectFromUrl(ctx context.Context, req *pb.RpcObjectCreateFromUrlRequest,
) (id string, objectDetails *types.Struct, err error) {
	url, err := uri.NormalizeURI(req.Url)
	if err != nil {
		return "", nil, err
	}
	objectTypeKey, err := domain.GetTypeKeyFromRawUniqueKey(req.ObjectTypeUniqueKey)
	if err != nil {
		return "", nil, err
	}
	s.enrichDetailsWithOrigin(req.Details, model.ObjectOrigin_webclipper)
	createReq := objectcreator.CreateObjectRequest{
		ObjectTypeKey: objectTypeKey,
		Details:       req.Details,
	}
	id, objectDetails, err = s.objectCreator.CreateObject(ctx, req.SpaceId, createReq)
	if err != nil {
		return "", nil, err
	}

	res := s.bookmark.FetchBookmarkContent(req.SpaceId, url, req.AddPageContent)
	content := res()
	shouldUpdateDetails := s.updateBookmarkContentWithUserDetails(req.Details, objectDetails, content)
	if shouldUpdateDetails {
		err = s.bookmark.UpdateObject(id, content)
		if err != nil {
			return "", nil, err
		}
	}

	if content != nil && len(content.Blocks) > 0 {
		err := s.pasteBlocks(id, content)
		if err != nil {
			return "", nil, err
		}
	}
	return id, objectDetails, nil
}

func (s *Service) pasteBlocks(id string, content *bookmark.ObjectContent) error {
	groupID := bson.NewObjectId().Hex()
	_, uploadArr, _, _, err := s.Paste(nil, pb.RpcBlockPasteRequest{
		ContextId: id,
		AnySlot:   content.Blocks,
	}, groupID)
	if err != nil {
		return err
	}
	for _, r := range uploadArr {
		r.ContextId = id
		uploadReq := UploadRequest{
			RpcBlockUploadRequest: r,
			ObjectOrigin:          objectorigin.Webclipper(),
			ImageKind:             model.ImageKind_AutomaticallyCreated,
		}
		if _, err = s.UploadBlockFile(nil, uploadReq, groupID, false); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) updateBookmarkContentWithUserDetails(userDetails, objectDetails *types.Struct, content *bookmark.ObjectContent) bool {
	shouldUpdate := false
	bookmarkRelationToValue := map[string]*string{
		bundle.RelationKeyName.String():        &content.BookmarkContent.Title,
		bundle.RelationKeyDescription.String(): &content.BookmarkContent.Description,
		bundle.RelationKeySource.String():      &content.BookmarkContent.Url,
		bundle.RelationKeyPicture.String():     &content.BookmarkContent.ImageHash,
		bundle.RelationKeyIconImage.String():   &content.BookmarkContent.FaviconHash,
	}

	for relation, valueFromBookmark := range bookmarkRelationToValue {
		// Don't change details of the object, if they are provided by client in request
		if userValue := pbtypes.GetString(userDetails, relation); userValue != "" {
			*valueFromBookmark = userValue
		} else {
			// if detail wasn't provided in request, we get it from bookmark and set it later in bookmark.UpdateObject
			// and add to response details
			shouldUpdate = true
			objectDetails.Fields[relation] = pbtypes.String(*valueFromBookmark)
		}
	}
	return shouldUpdate
}

func (s *Service) replaceLink(id, oldId, newId string) error {
	return cache.Do(s, id, func(b basic.CommonOperations) error {
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

func (s *Service) enrichDetailsWithOrigin(details *types.Struct, origin model.ObjectOrigin) {
	if details == nil || details.Fields == nil {
		details = &types.Struct{Fields: map[string]*types.Value{}}
	}
	details.Fields[bundle.RelationKeyOrigin.String()] = pbtypes.Int64(int64(origin))
}
