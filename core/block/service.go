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
	"github.com/hashicorp/go-multierror"
	"github.com/samber/lo"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	bookmarksvc "github.com/anyproto/anytype-heart/core/block/bookmark"
	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/detailservice"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/blockcollection"
	"github.com/anyproto/anytype-heart/core/block/editor/file"
	"github.com/anyproto/anytype-heart/core/block/editor/layout"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/history"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/block/simple/bookmark"
	"github.com/anyproto/anytype-heart/core/block/template"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/core/files/fileoffloader"
	"github.com/anyproto/anytype-heart/core/files/fileuploader"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/util/internalflag"
	"github.com/anyproto/anytype-heart/util/mutex"
	"github.com/anyproto/anytype-heart/util/slice"
	"github.com/anyproto/anytype-heart/util/uri"

	_ "github.com/anyproto/anytype-heart/core/block/editor/table"
	_ "github.com/anyproto/anytype-heart/core/block/simple/file"
	_ "github.com/anyproto/anytype-heart/core/block/simple/link"
	_ "github.com/anyproto/anytype-heart/core/block/simple/widget"
)

const CName = "block-service"

type withVirtualBlocks interface {
	InjectVirtualBlocks(objectId string, view *model.ObjectView)
}

var ErrUnknownObjectType = fmt.Errorf("unknown object type")

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

func New() *Service {
	s := &Service{
		openedObjs: &openedObjects{
			objects: make(map[string]string),
			lock:    &sync.Mutex{},
		},
	}
	return s
}

type Service struct {
	accountService       account.Service
	eventSender          event.Sender
	process              process.Service
	objectStore          objectstore.ObjectStore
	bookmark             bookmarksvc.Service
	objectCreator        objectcreator.Service
	templateService      template.Service
	resolver             idresolver.Resolver
	spaceService         space.Service
	tempDirProvider      core.TempDirProvider
	builtinObjectService builtinObjects
	fileObjectService    fileobject.Service
	detailsService       detailservice.Service

	fileUploaderService fileuploader.Service
	fileOffloader       fileoffloader.Service

	predefinedObjectWasMissing bool
	openedObjs                 *openedObjects

	componentCtx       context.Context
	componentCtxCancel context.CancelFunc
}

type builtinObjects interface {
	CreateObjectsForUseCase(ctx session.Context, spaceID string, req pb.RpcObjectImportUseCaseRequestUseCase) (startingPageId string, code pb.RpcObjectImportUseCaseResponseErrorCode, err error)
}

type openedObjects struct {
	objects map[string]string
	lock    *sync.Mutex
}

func (s *Service) Name() string {
	return CName
}

func (s *Service) Init(a *app.App) (err error) {
	s.componentCtx, s.componentCtxCancel = context.WithCancel(context.Background())

	s.process = a.MustComponent(process.CName).(process.Service)
	s.eventSender = a.MustComponent(event.CName).(event.Sender)
	s.objectStore = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	s.bookmark = a.MustComponent("bookmark-importer").(bookmarksvc.Service)
	s.objectCreator = app.MustComponent[objectcreator.Service](a)
	s.templateService = app.MustComponent[template.Service](a)
	s.spaceService = a.MustComponent(space.CName).(space.Service)
	s.resolver = a.MustComponent(idresolver.CName).(idresolver.Resolver)
	s.fileObjectService = app.MustComponent[fileobject.Service](a)
	s.fileUploaderService = app.MustComponent[fileuploader.Service](a)
	s.fileOffloader = app.MustComponent[fileoffloader.Service](a)
	s.tempDirProvider = app.MustComponent[core.TempDirProvider](a)
	s.builtinObjectService = app.MustComponent[builtinObjects](a)
	s.detailsService = app.MustComponent[detailservice.Service](a)
	s.accountService = app.MustComponent[account.Service](a)
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

func (s *Service) WaitAndGetObject(ctx context.Context, objectID string) (sb smartblock.SmartBlock, err error) {
	spaceID, err := s.resolver.ResolveSpaceIdWithRetry(ctx, objectID)
	if err != nil {
		return nil, err
	}
	return s.WaitAndGetObjectByFullID(ctx, domain.FullID{SpaceID: spaceID, ObjectID: objectID})
}

func (s *Service) TryRemoveFromCache(ctx context.Context, objectId string) (res bool, err error) {
	spaceId, err := s.resolver.ResolveSpaceID(objectId)
	if err != nil {
		return false, fmt.Errorf("resolve space: %w", err)
	}
	spc, err := s.spaceService.Get(ctx, spaceId)
	if err != nil {
		return false, fmt.Errorf("get space: %w", err)
	}
	mutex.WithLock(s.openedObjs.lock, func() any {
		_, contains := s.openedObjs.objects[objectId]
		if !contains {
			res, err = spc.TryRemove(objectId)
		}
		return nil
	})
	return
}

func (s *Service) GetObjectByFullID(ctx context.Context, id domain.FullID) (sb smartblock.SmartBlock, err error) {
	spc, err := s.spaceService.Get(ctx, id.SpaceID)
	if err != nil {
		return nil, fmt.Errorf("get space: %w", err)
	}
	return spc.GetObject(ctx, id.ObjectID)
}

func (s *Service) WaitAndGetObjectByFullID(ctx context.Context, id domain.FullID) (sb smartblock.SmartBlock, err error) {
	spc, err := s.spaceService.Wait(ctx, id.SpaceID)
	if err != nil {
		return nil, fmt.Errorf("wait space: %w", err)
	}
	return spc.GetObject(ctx, id.ObjectID)
}

func (s *Service) ObjectRefresh(ctx context.Context, id domain.FullID) (err error) {
	id = s.resolveFullId(id)
	sp, err := s.spaceService.Get(ctx, id.SpaceID)
	if err != nil {
		return fmt.Errorf("get space: %w", err)
	}
	return sp.RefreshObjects([]string{id.ObjectID})
}

func (s *Service) OpenBlock(sctx session.Context, id domain.FullID, includeRelationsAsDependentObjects bool) (obj *model.ObjectView, err error) {
	id = s.resolveFullId(id)

	spc, err := s.spaceService.Wait(s.componentCtx, id.SpaceID)
	if err != nil {
		return nil, fmt.Errorf("wait space: %w", err)
	}

	err = spc.Do(id.ObjectID, func(ob smartblock.SmartBlock) error {
		if includeRelationsAsDependentObjects {
			ob.EnabledRelationAsDependentObjects()
		}

		ob.RegisterSession(sctx)

		st := ob.NewState()

		st.SetLocalDetail(bundle.RelationKeyLastOpenedDate, domain.Int64(time.Now().Unix()))
		if err = ob.Apply(st, smartblock.NoHistory, smartblock.NoEvent, smartblock.SkipIfNoChanges, smartblock.KeepInternalFlags, smartblock.IgnoreNoPermissions); err != nil {
			log.Errorf("failed to update lastOpenedDate: %s", err)
		}
		if err = ob.Space().RefreshObjects([]string{ob.Id()}); err != nil {
			log.Debug("failed to sync object", zap.String("objectId", id.ObjectID), zap.Error(err))
		}
		if obj, err = ob.Show(); err != nil {
			return fmt.Errorf("show: %w", err)
		}

		if err != nil && !errors.Is(err, treestorage.ErrUnknownTreeId) {
			log.Errorf("failed to watch status for object %s: %s", id, err)
		}

		if v, ok := ob.(withVirtualBlocks); ok {
			v.InjectVirtualBlocks(id.ObjectID, obj)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	mutex.WithLock(s.openedObjs.lock, func() any { s.openedObjs.objects[id.ObjectID] = id.SpaceID; return nil })
	return obj, nil
}

func (s *Service) RefreshOpenedObjects(ctx context.Context) {
	openedObjects := s.GetOpenedObjects()
	if len(openedObjects) == 0 {
		return
	}
	objsPerSpace := make(map[string][]string)
	for _, entry := range openedObjects {
		curVal := objsPerSpace[entry.Value]
		curVal = append(curVal, entry.Key)
		objsPerSpace[entry.Value] = curVal
	}
	for spaceId, objectIds := range objsPerSpace {
		sp, err := s.spaceService.Get(ctx, spaceId)
		if err != nil {
			log.Debug("failed to refresh: get space", zap.Error(err), zap.String("spaceId", spaceId))
			continue
		}
		err = sp.RefreshObjects(objectIds)
		if err != nil {
			log.Debug("failed to refresh: refresh objects", zap.Error(err), zap.String("spaceId", spaceId), zap.Strings("objectIds", objectIds))
		}
	}
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
	if id.SpaceID != "" {
		return id
	}
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

	spc, err := s.spaceService.Wait(s.componentCtx, id.SpaceID)
	if err != nil {
		return nil, fmt.Errorf("wait space: %w", err)
	}

	err = spc.Do(id.ObjectID, func(b smartblock.SmartBlock) error {
		if includeRelationsAsDependentObjects {
			b.EnabledRelationAsDependentObjects()
		}
		obj, err = b.Show()
		if err != nil {
			return err
		}

		if v, ok := b.(withVirtualBlocks); ok {
			v.InjectVirtualBlocks(id.ObjectID, obj)
		}

		return nil
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
			s.sendOnRemoveEvent(id.SpaceID, id.ObjectID)
		}
	}
	mutex.WithLock(s.openedObjs.lock, func() any { delete(s.openedObjs.objects, id.ObjectID); return nil })
	return nil
}

func (s *Service) GetOpenedObjects() []lo.Entry[string, string] {
	return mutex.WithLock(s.openedObjs.lock, func() []lo.Entry[string, string] { return lo.Entries[string, string](s.openedObjs.objects) })
}

func (s *Service) SpaceInstallBundledObject(
	ctx context.Context,
	spaceId string,
	sourceObjectId string,
) (id string, object *domain.Details, err error) {
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
) (ids []string, objects []*domain.Details, err error) {
	spc, err := s.spaceService.Get(ctx, spaceId)
	if err != nil {
		return nil, nil, fmt.Errorf("get space: %w", err)
	}
	return s.objectCreator.InstallBundledObjects(ctx, spc, sourceObjectIds, false)
}

func (s *Service) SpaceInitChat(ctx context.Context, spaceId string) error {
	spc, err := s.spaceService.Get(ctx, spaceId)
	if err != nil {
		return fmt.Errorf("get space: %w", err)
	}
	if spc.IsReadOnly() {
		return nil
	}
	if spc.IsPersonal() {
		return nil
	}

	workspaceId := spc.DerivedIDs().Workspace
	chatUk, err := domain.NewUniqueKey(coresb.SmartBlockTypeChatDerivedObject, workspaceId)
	if err != nil {
		return err
	}

	chatId, err := spc.DeriveObjectID(context.Background(), chatUk)
	if err != nil {
		return err
	}

	// cheap check if chat already exists
	if spaceChatExists, err := spc.Storage().HasTree(ctx, chatId); err != nil {
		return err
	} else if spaceChatExists {
		return nil
	}

	_, err = s.objectCreator.AddChatDerivedObject(ctx, spc, workspaceId)
	if err != nil {
		if !errors.Is(err, treestorage.ErrTreeExists) {
			return fmt.Errorf("add chat derived object: %w", err)
		}
	}

	err = spc.DoCtx(ctx, workspaceId, func(b smartblock.SmartBlock) error {
		st := b.NewState()
		st.SetLocalDetail(bundle.RelationKeyChatId, domain.String(chatId))
		st.SetDetail(bundle.RelationKeyHasChat, domain.Bool(true))

		return b.Apply(st, smartblock.NoHistory, smartblock.NoEvent, smartblock.SkipIfNoChanges, smartblock.KeepInternalFlags, smartblock.IgnoreNoPermissions)
	})
	if err != nil {
		return fmt.Errorf("apply chatId to workspace: %w", err)
	}

	err = s.autoInstallSpaceChatWidget(ctx, spc)
	if err != nil {
		return fmt.Errorf("install chat widget: %w", err)
	}

	return nil
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

func (s *Service) ObjectShareByLink(req *pb.RpcObjectShareByLinkRequest) (link string, err error) {
	panic("should be removed")
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
	if id == spc.DerivedIDs().Archive {
		return fmt.Errorf("cannot delete archive object")
	}
	return cache.Do(s, spc.DerivedIDs().Archive, func(b smartblock.SmartBlock) error {
		archive, ok := b.(blockcollection.Collection)
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
				relKey := domain.RelationKey(st.Details().GetString(bundle.RelationKeyRelationKey))

				records, err := s.objectStore.SpaceIndex(b.SpaceID()).Query(database.Query{
					Filters: []database.FilterRequest{
						{
							Condition:   model.BlockContentDataviewFilter_Equal,
							RelationKey: relKey,
							Value:       domain.String(id),
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

func (s *Service) Close(_ context.Context) (err error) {
	if s.componentCtxCancel != nil {
		s.componentCtxCancel()
	}
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

func (s *Service) CreateObjectFromUrl(
	ctx context.Context, req *pb.RpcObjectCreateFromUrlRequest,
) (id string, objectDetails *domain.Details, err error) {
	url, err := uri.NormalizeURI(req.Url)
	if err != nil {
		return "", nil, err
	}
	objectTypeKey, err := domain.GetTypeKeyFromRawUniqueKey(req.ObjectTypeUniqueKey)
	if err != nil {
		return "", nil, err
	}
	details := domain.NewDetailsFromProto(req.Details)
	details = s.enrichDetailsWithOrigin(details, model.ObjectOrigin_webclipper)
	createReq := objectcreator.CreateObjectRequest{
		ObjectTypeKey: objectTypeKey,
		Details:       details,
		TemplateId:    req.TemplateId,
	}
	id, objectDetails, err = s.objectCreator.CreateObject(ctx, req.SpaceId, createReq)
	if err != nil {
		return "", nil, err
	}

	res := s.bookmark.FetchBookmarkContent(req.SpaceId, url, req.AddPageContent)
	content := res()
	shouldUpdateDetails := s.updateBookmarkContentWithUserDetails(details, objectDetails, content)
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
			ImageKind:             model.ImageKind_AutomaticallyAdded,
		}
		if _, err = s.UploadBlockFile(nil, uploadReq, groupID, false); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) updateBookmarkContentWithUserDetails(userDetails, objectDetails *domain.Details, content *bookmark.ObjectContent) bool {
	shouldUpdate := false
	bookmarkRelationToValue := map[domain.RelationKey]*string{
		bundle.RelationKeyName:        &content.BookmarkContent.Title,
		bundle.RelationKeyDescription: &content.BookmarkContent.Description,
		bundle.RelationKeySource:      &content.BookmarkContent.Url,
		bundle.RelationKeyPicture:     &content.BookmarkContent.ImageHash,
		bundle.RelationKeyIconImage:   &content.BookmarkContent.FaviconHash,
	}

	for relation, valueFromBookmark := range bookmarkRelationToValue {
		// Don't change details of the object, if they are provided by client in request
		if userValue := userDetails.GetString(relation); userValue != "" {
			*valueFromBookmark = userValue
		} else {
			// if detail wasn't provided in request, we get it from bookmark and set it later in bookmark.UpdateObject
			// and add to response details
			shouldUpdate = true
			objectDetails.SetString(relation, *valueFromBookmark)
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

func (s *Service) enrichDetailsWithOrigin(details *domain.Details, origin model.ObjectOrigin) *domain.Details {
	if details == nil {
		details = domain.NewDetails()
	}
	details.SetInt64(bundle.RelationKeyOrigin, int64(origin))
	return details
}

func (s *Service) SyncObjectsWithType(typeId string) error {
	spaceId, err := s.resolver.ResolveSpaceID(typeId)
	if err != nil {
		return fmt.Errorf("failed to resolve spaceId for type object: %w", err)
	}

	spc, err := s.spaceService.Get(s.componentCtx, spaceId)
	if err != nil {
		return fmt.Errorf("failed to get space: %w", err)
	}

	index := s.objectStore.SpaceIndex(spaceId)
	details, err := index.GetDetails(typeId)
	if err != nil {
		return fmt.Errorf("failed to get details of type object: %w", err)
	}

	removeDescriptionFromRecommended(typeId, details, spc)

	syncer := layout.NewSyncer(typeId, spc, index)
	newLayout := layout.NewLayoutStateFromDetails(details)
	oldLayout := newLayout.Copy()
	return syncer.SyncLayoutWithType(oldLayout, newLayout, true, true, false)
}

// removeDescriptionFromRecommended removes description relation id from recommended relations lists of type if it was added accidentally (see GO-5826)
func removeDescriptionFromRecommended(typeId string, details *domain.Details, spc clientspace.Space) {
	descriptionId, err := spc.DeriveObjectID(nil, domain.MustUniqueKey(coresb.SmartBlockTypeRelation, bundle.RelationKeyDescription.String()))
	if err != nil {
		return
	}

	detailsToSet := make([]domain.Detail, 0)
	for _, key := range []domain.RelationKey{
		bundle.RelationKeyRecommendedRelations,
		bundle.RelationKeyRecommendedFeaturedRelations,
		bundle.RelationKeyRecommendedFileRelations,
		bundle.RelationKeyRecommendedHiddenRelations,
	} {
		list := details.GetStringList(key)
		i := slice.FindPos(list, descriptionId)
		if i == -1 {
			continue
		}

		detailsToSet = append(detailsToSet, domain.Detail{
			Key:   key,
			Value: domain.StringList(slice.RemoveIndex(list, i)),
		})
	}

	if len(detailsToSet) == 0 {
		return
	}

	// nolint:errcheck
	spc.Do(typeId, func(sb smartblock.SmartBlock) error {
		if ds, ok := sb.(basic.DetailsSettable); ok {
			return ds.SetDetails(nil, detailsToSet, false)
		}
		return nil
	})
}
