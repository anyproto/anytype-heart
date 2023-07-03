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
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/typeprovider"
	"github.com/anyproto/anytype-heart/util/internalflag"
	"github.com/anyproto/anytype-heart/util/linkpreview"
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

func New(
	tempDirProvider *core.TempDirService,
	sbtProvider typeprovider.SmartBlockTypeProvider,
	layoutConverter converter.LayoutConverter,
) *Service {
	return &Service{
		tempDirProvider: tempDirProvider,
		sbtProvider:     sbtProvider,
		layoutConverter: layoutConverter,
		closing:         make(chan struct{}),
		syncer:          map[string]*treeSyncer{},
	}
}

type objectCreator interface {
	CreateSmartBlockFromState(ctx session.Context, sbType coresb.SmartBlockType, details *types.Struct, createState *state.State) (id string, newDetails *types.Struct, err error)
	InjectWorkspaceID(details *types.Struct, spaceID string, objectID string)

	CreateObject(ctx session.Context, req DetailsGetter, forcedType bundle.TypeKey) (id string, details *types.Struct, err error)
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
	ReindexSpace(ctx session.Context) error
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

	objectCreator   objectCreator
	objectFactory   *editor.ObjectFactory
	spaceService    space.Service
	commonAccount   accountservice.Service
	fileStore       filestore.FileStore
	tempDirProvider core.TempDirProvider
	sbtProvider     typeprovider.SmartBlockTypeProvider
	layoutConverter converter.LayoutConverter

	fileSync    filesync.FileSync
	fileService files.Service
	// TODO: move all this into separate treecache component or something like this
	syncer      map[string]*treeSyncer
	syncStarted bool
	syncerLock  sync.Mutex
	closing     chan struct{}

	predefinedObjectWasMissing bool
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
	s.indexer = app.MustComponent[indexer](a)
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
	ctx session.Context, id string, includeRelationsAsDependentObjects bool,
) (obj *model.ObjectView, err error) {
	startTime := time.Now()
	ob, err := s.getSmartblock(ctx, id)
	if err != nil {
		return nil, err
	}
	if includeRelationsAsDependentObjects {
		ob.EnabledRelationAsDependentObjects()
	}
	afterSmartBlockTime := time.Now()

	ob.Lock()
	defer ob.Unlock()
	ob.RegisterSession(ctx)
	if v, hasOpenListner := ob.(smartblock.SmartObjectOpenListner); hasOpenListner {
		v.SmartObjectOpened(ctx)
	}
	afterDataviewTime := time.Now()
	st := ob.NewState()

	st.SetLocalDetail(bundle.RelationKeyLastOpenedDate.String(), pbtypes.Int64(time.Now().Unix()))
	if err = ob.Apply(st, smartblock.NoHistory, smartblock.NoEvent, smartblock.SkipIfNoChanges); err != nil {
		log.Errorf("failed to update lastOpenedDate: %s", err.Error())
	}
	afterApplyTime := time.Now()
	if obj, err = ob.Show(ctx); err != nil {
		return
	}
	afterShowTime := time.Now()
	_, err = s.syncStatus.Watch(ctx.SpaceID(), id, func() []string {
		ob.Lock()
		defer ob.Unlock()
		bs := ob.NewState()

		return lo.Uniq(bs.GetAllFileHashes(ob.FileRelationKeys(bs)))
	})
	if err == nil {
		ob.AddHook(func(_ smartblock.ApplyInfo) error {
			s.syncStatus.Unwatch(ctx.SpaceID(), id)
			return nil
		}, smartblock.HookOnClose)
	}
	if err != nil && err != treestorage.ErrUnknownTreeId {
		log.Errorf("failed to watch status for object %s: %s", id, err.Error())
	}

	sbType, err := s.sbtProvider.Type(ctx.SpaceID(), id)
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
	return obj, nil
}

func (s *Service) ShowBlock(
	ctx session.Context, id string, includeRelationsAsDependentObjects bool,
) (obj *model.ObjectView, err error) {
	cctx := context.WithValue(ctx.Context(), metrics.CtxKeyEntrypoint, "object_show")
	ctx = ctx.WithContext(cctx)
	err2 := Do(s, ctx, id, func(b smartblock.SmartBlock) error {
		if includeRelationsAsDependentObjects {
			b.EnabledRelationAsDependentObjects()
		}
		obj, err = b.Show(ctx)
		return err
	})
	if err2 != nil {
		return nil, err2
	}
	return
}

func (s *Service) CloseBlock(ctx session.Context, id string) error {
	var isDraft bool
	err := Do(s, ctx, id, func(b smartblock.SmartBlock) error {
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
		if err = s.DeleteObject(ctx, id); err != nil {
			log.Errorf("error while block delete: %v", err)
		} else {
			s.sendOnRemoveEvent(ctx.SpaceID(), id)
		}
	}
	return nil
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
	ctx session.Context,
	sourceObjectId string,
) (id string, object *types.Struct, err error) {
	ids, details, err := s.AddSubObjectsToWorkspace(ctx, []string{sourceObjectId})
	if err != nil {
		return "", nil, err
	}
	if len(ids) == 0 {
		return "", nil, fmt.Errorf("failed to add object")
	}

	return ids[0], details[0], nil
}

func (s *Service) AddSubObjectsToWorkspace(
	ctx session.Context,
	sourceObjectIds []string,
) (ids []string, objects []*types.Struct, err error) {
	workspaceID := s.anytype.PredefinedObjects(ctx.SpaceID()).Account

	// todo: we should add route to object via workspace
	var details = make([]*types.Struct, 0, len(sourceObjectIds))

	for _, sourceObjectId := range sourceObjectIds {
		err = Do(s, ctx, sourceObjectId, func(b smartblock.SmartBlock) error {
			d := pbtypes.CopyStruct(b.Details())
			if pbtypes.GetString(d, bundle.RelationKeyWorkspaceId.String()) == workspaceID {
				return errors.New("object already in collection")
			}
			d.Fields[bundle.RelationKeySourceObject.String()] = pbtypes.String(sourceObjectId)
			u, err := addr.ConvertBundledObjectIdToInstalledId(b.ObjectType())
			if err != nil {
				u = b.ObjectType()
			}
			d.Fields[bundle.RelationKeyType.String()] = pbtypes.String(u)
			d.Fields[bundle.RelationKeyIsReadonly.String()] = pbtypes.Bool(false)
			d.Fields[bundle.RelationKeyId.String()] = pbtypes.String(b.Id())

			details = append(details, d)
			return nil
		})
		if err != nil {
			return
		}
	}

	err = Do(s, ctx, workspaceID, func(b smartblock.SmartBlock) error {
		ws, ok := b.(*editor.Workspaces)
		if !ok {
			return fmt.Errorf("incorrect workspace id")
		}
		ids, objects, err = ws.CreateSubObjects(ctx, details)
		return err
	})

	return
}

func (s *Service) RemoveSubObjectsInWorkspace(ctx session.Context, objectIds []string, orphansGC bool) (err error) {
	workspaceID := s.anytype.PredefinedObjects(ctx.SpaceID()).Account
	for _, objectID := range objectIds {
		if err = s.restriction.CheckRestrictions(ctx.SpaceID(), objectID, model.Restrictions_Delete); err != nil {
			return err
		}
	}
	err = Do(s, ctx, workspaceID, func(b smartblock.SmartBlock) error {
		ws, ok := b.(*editor.Workspaces)
		if !ok {
			return fmt.Errorf("incorrect workspace id")
		}
		err = ws.RemoveSubObjects(objectIds, orphansGC)
		return err
	})

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
	//	workspaceId = s.Anytype().PredefinedBlocks().Account
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

// SetPagesIsArchived is deprecated
func (s *Service) SetPagesIsArchived(ctx session.Context, req pb.RpcObjectListSetIsArchivedRequest) error {
	return Do(s, ctx, s.anytype.PredefinedObjects(ctx.SpaceID()).Archive, func(b smartblock.SmartBlock) error {
		archive, ok := b.(collection.Collection)
		if !ok {
			return fmt.Errorf("unexpected archive block type: %T", b)
		}

		var merr multierror.Error
		var anySucceed bool
		ids, err := s.objectStore.HasIDs(req.ObjectIds...)
		if err != nil {
			return err
		}
		for _, id := range ids {
			var err error
			if restrErr := s.checkArchivedRestriction(req.IsArchived, ctx.SpaceID(), id); restrErr != nil {
				err = restrErr
			} else {
				if req.IsArchived {
					err = archive.AddObject(id)
				} else {
					err = archive.RemoveObject(id)
				}
			}
			if err != nil {
				log.Warnf("failed to archive %s: %s", id, err.Error())
				merr.Errors = append(merr.Errors, err)
				continue
			}
			anySucceed = true
		}

		if err := merr.ErrorOrNil(); err != nil {
			log.Warnf("failed to archive: %s", err)
		}
		if anySucceed {
			return nil
		}
		return merr.ErrorOrNil()
	})
}

// SetPagesIsFavorite is deprecated
func (s *Service) SetPagesIsFavorite(ctx session.Context, req pb.RpcObjectListSetIsFavoriteRequest) error {
	return Do(s, ctx, s.anytype.PredefinedObjects(ctx.SpaceID()).Home, func(b smartblock.SmartBlock) error {
		fav, ok := b.(collection.Collection)
		if !ok {
			return fmt.Errorf("unexpected home block type: %T", b)
		}

		ids, err := s.objectStore.HasIDs(req.ObjectIds...)
		if err != nil {
			return err
		}
		var merr multierror.Error
		var anySucceed bool
		for _, id := range ids {
			var err error
			if req.IsFavorite {
				err = fav.AddObject(id)
			} else {
				err = fav.RemoveObject(id)
			}
			if err != nil {
				log.Errorf("failed to favorite object %s: %s", id, err.Error())
				merr.Errors = append(merr.Errors, err)
				continue
			}
			anySucceed = true
		}
		if err := merr.ErrorOrNil(); err != nil {
			log.Warnf("failed to set objects as favorite: %s", err)
		}
		if anySucceed {
			return nil
		}
		return merr.ErrorOrNil()
	})
}

func (s *Service) objectLinksCollectionModify(ctx session.Context, collectionId string, objectId string, value bool) error {
	return Do(s, ctx, collectionId, func(b smartblock.SmartBlock) error {
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

func (s *Service) SetPageIsFavorite(ctx session.Context, req pb.RpcObjectSetIsFavoriteRequest) (err error) {
	return s.objectLinksCollectionModify(ctx, s.anytype.PredefinedObjects(ctx.SpaceID()).Home, req.ContextId, req.IsFavorite)
}

func (s *Service) SetPageIsArchived(ctx session.Context, req pb.RpcObjectSetIsArchivedRequest) (err error) {
	if err := s.checkArchivedRestriction(req.IsArchived, ctx.SpaceID(), req.ContextId); err != nil {
		return err
	}
	return s.objectLinksCollectionModify(ctx, s.anytype.PredefinedObjects(ctx.SpaceID()).Archive, req.ContextId, req.IsArchived)
}

func (s *Service) SetSource(ctx session.Context, req pb.RpcObjectSetSourceRequest) (err error) {
	return Do(s, ctx, req.ContextId, func(b smartblock.SmartBlock) error {
		st := b.NewStateCtx(ctx)
		st.SetDetailAndBundledRelation(bundle.RelationKeySetOf, pbtypes.StringList(req.Source))
		return b.Apply(st, smartblock.NoRestrictions)
	})
}

func (s *Service) SetWorkspaceDashboardId(ctx session.Context, workspaceId string, id string) (setId string, err error) {
	err = Do(s, ctx, workspaceId, func(ws *editor.Workspaces) error {
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
	return Do(s, ctx, s.anytype.PredefinedObjects(ctx.SpaceID()).Archive, func(b smartblock.SmartBlock) error {
		archive, ok := b.(collection.Collection)
		if !ok {
			return fmt.Errorf("unexpected archive block type: %T", b)
		}

		var merr multierror.Error
		var anySucceed bool
		for _, blockId := range req.ObjectIds {
			if exists, _ := archive.HasObject(blockId); exists {
				if err = s.DeleteObject(ctx, blockId); err != nil {
					merr.Errors = append(merr.Errors, err)
					continue
				}
				archive.RemoveObject(blockId)
				anySucceed = true
			}
		}
		if err := merr.ErrorOrNil(); err != nil {
			log.Warnf("failed to delete archived objects: %s", err)
		}
		if anySucceed {
			return nil
		}
		return merr.ErrorOrNil()
	})
}

func (s *Service) ObjectsDuplicate(ctx session.Context, ids []string) (newIds []string, err error) {
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

func (s *Service) DeleteArchivedObject(ctx session.Context, id string) (err error) {
	return Do(s, ctx, s.anytype.PredefinedObjects(ctx.SpaceID()).Archive, func(b smartblock.SmartBlock) error {
		archive, ok := b.(collection.Collection)
		if !ok {
			return fmt.Errorf("unexpected archive block type: %T", b)
		}

		if exists, _ := archive.HasObject(id); exists {
			if err = s.DeleteObject(ctx, id); err == nil {
				err = archive.RemoveObject(id)
				if err != nil {
					return err
				}
			}
		}

		return nil
	})
}

func (s *Service) OnDelete(ctx session.Context, id string, workspaceRemove func() error) error {
	var (
		isFavorite bool
	)

	err := Do(s, ctx, id, func(b smartblock.SmartBlock) error {
		b.ObjectCloseAllSessions()
		st := b.NewState()
		isFavorite = pbtypes.GetBool(st.LocalDetails(), bundle.RelationKeyIsFavorite.String())
		if isFavorite {
			_ = s.SetPageIsFavorite(ctx, pb.RpcObjectSetIsFavoriteRequest{IsFavorite: false, ContextId: id})
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
	if err := s.objectStore.DeleteObject(id); err != nil {
		return fmt.Errorf("delete object from local store: %w", err)
	}

	return nil
}

func (s *Service) sendOnRemoveEvent(spaceID string, ids ...string) {
	s.eventSender.BroadcastForSpace(spaceID, &pb.Event{
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
	var workspace *editor.Workspaces
	if err := Do(s, ctx, s.anytype.PredefinedObjects(ctx.SpaceID()).Account, func(b smartblock.SmartBlock) error {
		var ok bool
		if workspace, ok = b.(*editor.Workspaces); !ok {
			return fmt.Errorf("incorrect object with workspace id")
		}
		return nil
	}); err != nil {
		return err
	}

	for _, id := range optIds {
		if checkInObjects {
			opt, err := workspace.Open(id)
			if err != nil {
				return fmt.Errorf("workspace open: %w", err)
			}
			relKey := pbtypes.GetString(opt.Details(), bundle.RelationKeyRelationKey.String())

			q := database.Query{
				Filters: []*model.BlockContentDataviewFilter{
					{
						Condition:   model.BlockContentDataviewFilter_Equal,
						RelationKey: relKey,
						Value:       pbtypes.String(opt.Id()),
					},
				},
			}
			records, _, err := s.objectStore.Query(nil, q)
			if err != nil {
				return nil
			}

			if len(records) > 0 {
				return ErrOptionUsedByOtherObjects
			}
		}

		if err := s.DeleteObject(ctx, id); err != nil {
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
	close(s.closing)
	return s.cache.Close()
}

// PickBlock returns opened smartBlock or opens smartBlock in silent mode
func (s *Service) PickBlock(ctx session.Context, id string) (sb smartblock.SmartBlock, err error) {
	return s.getSmartblock(ctx, id)
}

func (s *Service) getSmartblock(ctx session.Context, id string) (sb smartblock.SmartBlock, err error) {
	return s.GetObjectWithTimeout(ctx, id)
}

func (s *Service) StateFromTemplate(ctx session.Context, templateID string, name string) (st *state.State, err error) {
	if err = Do(s, ctx, templateID, func(b smartblock.SmartBlock) error {
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

func (s *Service) DoFileNonLock(ctx session.Context, id string, apply func(b file.File) error) error {
	sb, err := s.PickBlock(ctx, id)
	if err != nil {
		return err
	}

	if bb, ok := sb.(file.File); ok {
		return apply(bb)
	}
	return fmt.Errorf("file non lock operation not available for this block type: %T", sb)
}

type Picker interface {
	PickBlock(ctx session.Context, id string) (sb smartblock.SmartBlock, err error)
}

func Do[t any](p Picker, ctx session.Context, id string, apply func(sb t) error) error {
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
	return apply(bb)
}

// DoState2 picks two blocks and perform an action on them. The order of locks is always the same for two ids.
// It correctly handles the case when two ids are the same.
func DoState2[t1, t2 any](s *Service, ctx session.Context, firstID, secondID string, f func(*state.State, *state.State, t1, t2) error) error {
	if firstID == secondID {
		return DoStateAsync(s, ctx, firstID, func(st *state.State, b t1) error {
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
		return DoStateAsync(s, ctx, firstID, func(firstState *state.State, firstBlock t1) error {
			return DoStateAsync(s, ctx, secondID, func(secondState *state.State, secondBlock t2) error {
				return f(firstState, secondState, firstBlock, secondBlock)
			})
		})
	}
	return DoStateAsync(s, ctx, secondID, func(secondState *state.State, secondBlock t2) error {
		return DoStateAsync(s, ctx, firstID, func(firstState *state.State, firstBlock t1) error {
			return f(firstState, secondState, firstBlock, secondBlock)
		})
	})
}

func DoStateAsync[t any](p Picker, ctx session.Context, id string, apply func(s *state.State, sb t) error, flags ...smartblock.ApplyFlag) error {
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

func DoStateCtx[t any](p Picker, ctx session.Context, id string, apply func(s *state.State, sb t) error, flags ...smartblock.ApplyFlag) error {
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

	st := sb.NewStateCtx(ctx)
	err = apply(st, bb)
	if err != nil {
		return fmt.Errorf("apply func: %w", err)
	}

	return sb.Apply(st, flags...)
}

func (s *Service) ObjectApplyTemplate(ctx session.Context, contextId, templateId string) error {
	return Do(s, ctx, contextId, func(b smartblock.SmartBlock) error {
		orig := b.NewState().ParentState()
		ts, err := s.StateFromTemplate(ctx, templateId, pbtypes.GetString(orig.Details(), bundle.RelationKeyName.String()))
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

		ts.BlocksInit(orig)
		objType := ts.ObjectType()
		// StateFromTemplate returns state without the localdetails, so they will be taken from the orig state
		ts.SetObjectType(objType)

		flags := internalflag.NewFromState(ts)
		flags.Remove(model.InternalFlag_editorSelectType)
		flags.Remove(model.InternalFlag_editorSelectTemplate)
		flags.AddToState(ts)

		return b.Apply(ts, smartblock.NoRestrictions)
	})
}

func (s *Service) ResetToState(ctx session.Context, pageID string, st *state.State) (err error) {
	return Do(s, ctx, pageID, func(sb smartblock.SmartBlock) error {
		return history.ResetToVersion(ctx, sb, st)
	})
}

func (s *Service) ObjectBookmarkFetch(ctx session.Context, req pb.RpcObjectBookmarkFetchRequest) (err error) {
	url, err := uri.NormalizeURI(req.Url)
	if err != nil {
		return fmt.Errorf("process uri: %w", err)
	}
	res := s.bookmark.FetchBookmarkContent(ctx, url)
	go func() {
		if err := s.bookmark.UpdateBookmarkObject(req.ContextId, res); err != nil {
			log.Errorf("update bookmark object %s: %s", req.ContextId, err)
		}
	}()
	return nil
}

func (s *Service) ObjectToBookmark(ctx session.Context, id string, url string) (objectId string, err error) {
	objectId, _, err = s.objectCreator.CreateObject(ctx, &pb.RpcObjectCreateBookmarkRequest{
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
	res, err := oStore.GetWithLinksInfoByID(ctx.SpaceID(), id)
	if err != nil {
		return
	}
	for _, il := range res.Links.Inbound {
		if err = s.replaceLink(ctx, il.Id, id, objectId); err != nil {
			return
		}
	}
	err = s.DeleteObject(ctx, id)
	if err != nil {
		// intentionally do not return error here
		log.Errorf("failed to delete object after conversion to bookmark: %s", err.Error())
		err = nil
	}

	return
}

func (s *Service) replaceLink(ctx session.Context, id, oldId, newId string) error {
	return Do(s, ctx, id, func(b basic.CommonOperations) error {
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
