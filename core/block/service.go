package block

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
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
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/history"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/core/syncstatus"
	"github.com/anyproto/anytype-heart/core/system_object"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
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
	CName           = "block-service"
	linkObjectShare = "anytype://object/share?"
	BlankTemplateID = "blank"
)

var (
	ErrUnexpectedBlockType   = errors.New("unexpected block type")
	ErrUnknownObjectType     = fmt.Errorf("unknown object type")
	ErrObjectNotFoundByOldID = fmt.Errorf("failed to find template by Source Object id")
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

type objectCreator interface {
	CreateSmartBlockFromState(ctx context.Context, spaceID string, objectTypeKeys []domain.TypeKey, createState *state.State) (id string, newDetails *types.Struct, err error)
	CreateObject(ctx context.Context, spaceID string, req DetailsGetter, objectTypeKey domain.TypeKey) (id string, details *types.Struct, err error)
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
type builtinObjects interface {
	CreateObjectsForUseCase(ctx session.Context, spaceID string, req pb.RpcObjectImportUseCaseRequestUseCase) (code pb.RpcObjectImportUseCaseResponseErrorCode, err error)
}

type Service struct {
	anytype              core.Service
	syncStatus           syncstatus.Service
	eventSender          event.Sender
	linkPreview          linkpreview.LinkPreview
	process              process.Service
	app                  *app.App
	source               source.Service
	objectStore          objectstore.ObjectStore
	restriction          restriction.Service
	bookmark             bookmarksvc.Service
	systemObjectService  system_object.Service
	objectCache          objectcache.Cache
	objectCreator        objectCreator
	resolver             idresolver.Resolver
	spaceService         space.SpaceService
	spaceCore            spacecore.SpaceCoreService
	commonAccount        accountservice.Service
	fileStore            filestore.FileStore
	tempDirProvider      core.TempDirProvider
	sbtProvider          typeprovider.SmartBlockTypeProvider
	layoutConverter      converter.LayoutConverter
	builtinObjectService builtinObjects

	fileSync    filesync.FileSync
	fileService files.Service

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
	s.systemObjectService = a.MustComponent(system_object.CName).(system_object.Service)
	s.objectCreator = a.MustComponent("objectCreator").(objectCreator)
	s.spaceService = a.MustComponent(space.CName).(space.SpaceService)
	s.commonAccount = a.MustComponent(accountservice.CName).(accountservice.Service)
	s.fileStore = app.MustComponent[filestore.FileStore](a)
	s.fileSync = app.MustComponent[filesync.FileSync](a)
	s.fileService = app.MustComponent[files.Service](a)
	s.objectCache = app.MustComponent[objectcache.Cache](a)
	s.resolver = a.MustComponent(idresolver.CName).(idresolver.Resolver)
	s.spaceCore = app.MustComponent[spacecore.SpaceCoreService](a)

	s.tempDirProvider = app.MustComponent[core.TempDirProvider](a)
	s.sbtProvider = app.MustComponent[typeprovider.SmartBlockTypeProvider](a)
	s.layoutConverter = app.MustComponent[converter.LayoutConverter](a)

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
	return s.objectCache.GetObject(ctx, domain.FullID{
		ObjectID: objectID,
		SpaceID:  spaceID,
	})
}

func (s *Service) GetObjectByFullID(ctx context.Context, id domain.FullID) (sb smartblock.SmartBlock, err error) {
	return s.objectCache.GetObject(ctx, id)
}

func (s *Service) OpenBlock(sctx session.Context, id string, includeRelationsAsDependentObjects bool) (obj *model.ObjectView, err error) {
	spaceID, err := s.resolver.ResolveSpaceID(id)
	if err != nil {
		return nil, fmt.Errorf("resolve space id: %w", err)
	}
	startTime := time.Now()
	err = Do(s, id, func(ob smartblock.SmartBlock) error {
		if includeRelationsAsDependentObjects {
			ob.EnabledRelationAsDependentObjects()
		}
		afterSmartBlockTime := time.Now()

		ob.RegisterSession(sctx)

		afterDataviewTime := time.Now()
		st := ob.NewState()

		st.SetLocalDetail(bundle.RelationKeyLastOpenedDate.String(), pbtypes.Int64(time.Now().Unix()))
		if err = ob.Apply(st, smartblock.NoHistory, smartblock.NoEvent, smartblock.SkipIfNoChanges, smartblock.KeepInternalFlags); err != nil {
			log.Errorf("failed to update lastOpenedDate: %s", err.Error())
		}
		afterApplyTime := time.Now()
		if obj, err = ob.Show(); err != nil {
			return fmt.Errorf("show: %w", err)
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
			return fmt.Errorf("failed to get smartblock type: %w", err)
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
		return nil
	})
	if err != nil {
		return nil, err
	}
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
		return nil
	})
	if err != nil {
		return err
	}

	if isDraft {
		if err = s.DeleteObject(id); err != nil {
			log.Errorf("error while block delete: %v", err)
		} else {
			sendOnRemoveEvent(s.eventSender, id)
		}
	}
	mutex.WithLock(s.openedObjs.lock, func() any { delete(s.openedObjs.objects, id); return nil })
	return nil
}

func (s *Service) GetOpenedObjects() []string {
	return mutex.WithLock(s.openedObjs.lock, func() []string { return lo.Keys(s.openedObjs.objects) })
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

func (s *Service) SetIsHighlighted(req *pb.RpcWorkspaceSetIsHighlightedRequest) error {
	panic("should be removed")
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
	spaceID, err := s.resolver.ResolveSpaceID(req.ContextId)
	if err != nil {
		return fmt.Errorf("resolve spaceID: %w", err)
	}
	return s.objectLinksCollectionModify(s.anytype.PredefinedObjects(spaceID).Home, req.ContextId, req.IsFavorite)
}

func (s *Service) SetPageIsArchived(req pb.RpcObjectSetIsArchivedRequest) (err error) {
	spaceID, err := s.resolver.ResolveSpaceID(req.ContextId)
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
		if ws.Type() != coresb.SmartBlockTypeWorkspace {
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
	return Do(s, s.anytype.PredefinedObjects(spaceID).Archive, func(b smartblock.SmartBlock) error {
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
			err := Do(s, id, func(b smartblock.SmartBlock) error {
				st := b.NewState()
				relKey := pbtypes.GetString(st.Details(), bundle.RelationKeyRelationKey.String())

				records, _, err := s.objectStore.Query(database.Query{
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

func (s *Service) StateFromTemplate(templateID, name string) (st *state.State, err error) {
	if templateID == BlankTemplateID || templateID == "" {
		return s.BlankTemplateState(), nil
	}
	if err = Do(s, templateID, func(b smartblock.SmartBlock) error {
		if tmpl, ok := b.(*editor.Template); ok {
			st, err = tmpl.GetNewPageState(name)
		} else {
			return fmt.Errorf("object '%s' is not a template", templateID)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("can't apply template: %v", err)
	}
	return
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

func (s *Service) ObjectApplyTemplate(contextID, templateID string) error {
	return Do(s, contextID, func(b smartblock.SmartBlock) error {
		orig := b.NewState().ParentState()
		ts, err := s.StateFromTemplate(templateID, "")
		if err != nil {
			return err
		}
		ts.SetRootId(contextID)
		ts.SetParent(orig)

		layout, found := orig.Layout()
		if found {
			if commonOperations, ok := b.(basic.CommonOperations); ok {
				if err = commonOperations.SetLayoutInStateAndIgnoreRestriction(ts, layout); err != nil {
					return fmt.Errorf("convert layout: %w", err)
				}
			}
		}

		ts.BlocksInit(ts)

		objType := orig.ObjectTypeKey()
		ts.SetObjectTypeKey(objType)

		flags := internalflag.NewFromState(orig)
		flags.AddToState(ts)

		// we provide KeepInternalFlags to allow further template applying and object type change
		return b.Apply(ts, smartblock.NoRestrictions, smartblock.KeepInternalFlags)
	})
}

func (s *Service) ResetToState(pageID string, st *state.State) (err error) {
	return Do(s, pageID, func(sb smartblock.SmartBlock) error {
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
	res := s.bookmark.FetchBookmarkContent(spaceID, url)
	go func() {
		if err := s.bookmark.UpdateBookmarkObject(req.ContextId, res); err != nil {
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

func (s *Service) BlankTemplateState() (st *state.State) {
	st = state.NewDoc(BlankTemplateID, nil).NewState()
	template.InitTemplate(st, template.WithEmpty,
		template.WithDefaultFeaturedRelations,
		template.WithFeaturedRelations,
		template.WithRequiredRelations(),
		template.WithTitle,
		template.WithDescription,
	)
	return
}
