package block

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/anytypeio/any-sync/accountservice"
	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/app/ocache"
	"github.com/anytypeio/any-sync/commonspace/object/treegetter"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/hashicorp/go-multierror"
	"github.com/ipfs/go-datastore/query"

	bookmarksvc "github.com/anytypeio/go-anytype-middleware/core/block/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/clipboard"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/collection"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/file"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/stext"
	"github.com/anytypeio/go-anytype-middleware/core/block/history"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/core/block/restriction"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/core/event"
	"github.com/anytypeio/go-anytype-middleware/core/relation"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/core/status"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/filestore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/space"
	"github.com/anytypeio/go-anytype-middleware/util/internalflag"
	"github.com/anytypeio/go-anytype-middleware/util/linkpreview"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/uri"

	_ "github.com/anytypeio/go-anytype-middleware/core/block/editor/table"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/file"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/link"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/widget"
)

const (
	CName           = treegetter.CName
	linkObjectShare = "anytype://object/share?"
)

var (
	ErrBlockNotFound       = errors.New("block not found")
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

func New(tempDirProvider *core.TempDirService) *Service {
	return &Service{tempDirProvider: tempDirProvider}
}

type objectCreator interface {
	CreateSmartBlockFromState(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, createState *state.State) (id string, newDetails *types.Struct, err error)
	InjectWorkspaceID(details *types.Struct, objectID string)

	CreateObject(req DetailsGetter, forcedType bundle.TypeKey) (id string, details *types.Struct, err error)
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

type Service struct {
	anytype         core.Service
	status          status.Service
	sendEvent       func(event *pb.Event)
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

	objectCreator   objectCreator
	objectFactory   *editor.ObjectFactory
	clientService   space.Service
	commonAccount   accountservice.Service
	fileStore       filestore.FileStore
	tempDirProvider core.TempDirProvider

	spaceDashboardID string
}

func (s *Service) Name() string {
	return CName
}

func (s *Service) Init(a *app.App) (err error) {
	s.anytype = a.MustComponent(core.CName).(core.Service)
	s.status = a.MustComponent(status.CName).(status.Service)
	s.linkPreview = a.MustComponent(linkpreview.CName).(linkpreview.LinkPreview)
	s.process = a.MustComponent(process.CName).(process.Service)
	s.sendEvent = a.MustComponent(event.CName).(event.Sender).Send
	s.source = a.MustComponent(source.CName).(source.Service)
	s.objectStore = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	s.restriction = a.MustComponent(restriction.CName).(restriction.Service)
	s.bookmark = a.MustComponent("bookmark-importer").(bookmarksvc.Service)
	s.relationService = a.MustComponent(relation.CName).(relation.Service)
	s.objectCreator = a.MustComponent("objectCreator").(objectCreator)
	s.clientService = a.MustComponent(space.CName).(space.Service)
	s.objectFactory = app.MustComponent[*editor.ObjectFactory](a)
	s.commonAccount = a.MustComponent(accountservice.CName).(accountservice.Service)
	s.fileStore = app.MustComponent[filestore.FileStore](a)
	s.cache = s.createCache()
	s.app = a
	return
}

func (s *Service) Run(ctx context.Context) (err error) {
	return
}

func (s *Service) GetSpaceDashboardID(ctx context.Context) (string, error) {
	if s.spaceDashboardID == "" {
		obj, err := s.CreateTreeObject(ctx, coresb.SmartBlockTypePage, func(id string) *smartblock.InitContext {
			return &smartblock.InitContext{
				Ctx: ctx,
			}
		})
		if err != nil {
			return "", err
		}
		s.spaceDashboardID = obj.Id()
	}
	return s.spaceDashboardID, nil
}

func (s *Service) Anytype() core.Service {
	return s.anytype
}

func (s *Service) OpenBlock(
	ctx *session.Context, id string, includeRelationsAsDependentObjects bool,
) (obj *model.ObjectView, err error) {
	startTime := time.Now()
	ob, err := s.getSmartblock(context.WithValue(context.TODO(), metrics.CtxKeyRequest, "object_open"), id)
	if err != nil {
		return nil, err
	}
	if includeRelationsAsDependentObjects {
		ob.EnabledRelationAsDependentObjects()
	}
	afterSmartBlockTime := time.Now()

	ob.Lock()
	defer ob.Unlock()
	ob.SetEventFunc(s.sendEvent)
	if v, hasOpenListner := ob.(smartblock.SmartObjectOpenListner); hasOpenListner {
		v.SmartObjectOpened(ctx)
	}
	afterDataviewTime := time.Now()
	st := ob.NewState()

	st.SetLocalDetail(bundle.RelationKeyLastOpenedDate.String(), pbtypes.Int64(time.Now().Unix()))
	if err = ob.Apply(st, smartblock.NoHistory, smartblock.NoEvent); err != nil {
		log.Errorf("failed to update lastOpenedDate: %s", err.Error())
	}
	afterApplyTime := time.Now()
	if obj, err = ob.Show(ctx); err != nil {
		return
	}
	afterShowTime := time.Now()
	// TODO: [MR] add files to status logic
	if tp, err := coresb.SmartBlockTypeFromID(id); err == nil && tp == coresb.SmartBlockTypePage {
		s.status.Watch(id, func() []string {
			return nil
		})
		ob.AddHook(func(_ smartblock.ApplyInfo) error {
			s.status.Unwatch(id)
			return nil
		}, smartblock.HookOnClose)
	}
	// if tid := ob.threadId; tid != thread.Undef && s.status != nil {
	//	var (
	//		fList = func() []string {
	//			ob.Lock()
	//			defer ob.Unlock()
	//			bs := ob.NewState()
	//			return bs.GetAllFileHashes(ob.FileRelationKeys(bs))
	//		}
	//	)
	//
	//	if newWatcher := s.status.Watch(tid, fList); newWatcher {
	//		ob.AddHook(func(_ smartblock.ApplyInfo) error {
	//			s.status.Unwatch(tid)
	//			return nil
	//		}, smartblock.HookOnClose)
	//	}
	// }
	afterHashesTime := time.Now()
	tp, _ := coresb.SmartBlockTypeFromID(id)
	metrics.SharedClient.RecordEvent(metrics.OpenBlockEvent{
		ObjectId:       id,
		GetBlockMs:     afterSmartBlockTime.Sub(startTime).Milliseconds(),
		DataviewMs:     afterDataviewTime.Sub(afterSmartBlockTime).Milliseconds(),
		ApplyMs:        afterApplyTime.Sub(afterDataviewTime).Milliseconds(),
		ShowMs:         afterShowTime.Sub(afterApplyTime).Milliseconds(),
		FileWatcherMs:  afterHashesTime.Sub(afterShowTime).Milliseconds(),
		SmartblockType: int(tp),
	})
	return obj, nil
}

func (s *Service) ShowBlock(
	ctx *session.Context, id string, includeRelationsAsDependentObjects bool,
) (obj *model.ObjectView, err error) {
	cctx := context.WithValue(context.TODO(), metrics.CtxKeyRequest, "object_show")
	err2 := s.DoWithContext(cctx, id, func(b smartblock.SmartBlock) error {
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

func (s *Service) OpenBreadcrumbsBlock(ctx *session.Context) (obj *model.ObjectView, blockId string, err error) {
	bs := editor.NewBreadcrumbs()
	if err = bs.Init(&smartblock.InitContext{
		App:    s.app,
		Source: source.NewVirtual(s.anytype, model.SmartBlockType_Breadcrumbs),
	}); err != nil {
		return
	}
	_, err = s.PutObject(context.Background(), bs.Id(), bs)
	if err != nil {
		return
	}

	bs.Lock()
	defer bs.Unlock()
	bs.SetEventFunc(s.sendEvent)
	obj, err = bs.Show(ctx)
	if err != nil {
		return
	}
	return obj, bs.Id(), nil
}

func (s *Service) CloseBlock(id string) error {
	var isDraft bool
	err := s.Do(id, func(b smartblock.SmartBlock) error {
		b.ObjectClose()
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
	return nil
}

func (s *Service) CloseBlocks() {
	s.cache.ForEach(func(v ocache.Object) (isContinue bool) {
		ob := v.(smartblock.SmartBlock)
		ob.Lock()
		ob.ObjectClose()
		ob.Unlock()
		return true
	})
}

func (s *Service) AddSubObjectToWorkspace(
	sourceObjectId, workspaceId string,
) (id string, object *types.Struct, err error) {
	ids, details, err := s.AddSubObjectsToWorkspace([]string{sourceObjectId}, workspaceId)
	if err != nil {
		return "", nil, err
	}
	if len(ids) == 0 {
		return "", nil, fmt.Errorf("failed to add object")
	}

	return ids[0], details[0], nil
}

func (s *Service) AddSubObjectsToWorkspace(
	sourceObjectIds []string, workspaceId string,
) (ids []string, objects []*types.Struct, err error) {
	// todo: we should add route to object via workspace
	var details = make([]*types.Struct, 0, len(sourceObjectIds))

	for _, sourceObjectId := range sourceObjectIds {
		err = s.Do(sourceObjectId, func(b smartblock.SmartBlock) error {
			d := pbtypes.CopyStruct(b.Details())
			if pbtypes.GetString(d, bundle.RelationKeyWorkspaceId.String()) == workspaceId {
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

	err = s.Do(workspaceId, func(b smartblock.SmartBlock) error {
		ws, ok := b.(*editor.Workspaces)
		if !ok {
			return fmt.Errorf("incorrect workspace id")
		}
		ids, objects, err = ws.CreateSubObjects(details)
		return err
	})

	return
}

func (s *Service) RemoveSubObjectsInWorkspace(objectIds []string, workspaceId string, orphansGC bool) (err error) {
	for _, objectId := range objectIds {
		if err := s.restriction.CheckRestrictions(objectId, model.Restrictions_Delete); err != nil {
			return err
		}
	}
	err = s.Do(workspaceId, func(b smartblock.SmartBlock) error {
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
	workspaceID, err := s.objectStore.GetCurrentWorkspaceId()
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
	// return s.Do(workspaceId, func(b smartblock.SmartBlock) error {
	//	workspace, ok := b.(*editor.Workspaces)
	//	if !ok {
	//		return fmt.Errorf("incorrect object with workspace id")
	//	}
	//	return workspace.SetIsHighlighted(req.ObjectId, req.IsHighlighted)
	// })
}

func (s *Service) ObjectAddWithObjectId(req *pb.RpcObjectAddWithObjectIdRequest) error {
	if req.ObjectId == "" || req.Payload == "" {
		return fmt.Errorf("cannot add object without objectId or payload")
	}
	decodedPayload, err := base64.RawStdEncoding.DecodeString(req.Payload)
	if err != nil {
		return fmt.Errorf("error adding object: cannot decode base64 payload")
	}

	var protoPayload model.ThreadDeeplinkPayload
	err = proto.Unmarshal(decodedPayload, &protoPayload)
	if err != nil {
		return fmt.Errorf("failed unmarshalling the payload: %w", err)
	}
	// TODO: [MR] check the meaning of method and what should be the result
	return fmt.Errorf("not implemented")
	// return s.Do(s.Anytype().PredefinedBlocks().Account, func(b smartblock.SmartBlock) error {
	//	workspace, ok := b.(*editor.Workspaces)
	//	if !ok {
	//		return fmt.Errorf("incorrect object with workspace id")
	//	}
	//
	//	return workspace.AddObject(req.ObjectId, protoPayload.Key, protoPayload.Addrs)
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
	// err = s.Do(workspaceId, func(b smartblock.SmartBlock) error {
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
func (s *Service) SetPagesIsArchived(req pb.RpcObjectListSetIsArchivedRequest) error {
	return s.Do(s.anytype.PredefinedBlocks().Archive, func(b smartblock.SmartBlock) error {
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
			if restrErr := s.checkArchivedRestriction(req.IsArchived, id); restrErr != nil {
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
func (s *Service) SetPagesIsFavorite(req pb.RpcObjectListSetIsFavoriteRequest) error {
	return s.Do(s.anytype.PredefinedBlocks().Home, func(b smartblock.SmartBlock) error {
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

func (s *Service) objectLinksCollectionModify(collectionId string, objectId string, value bool) error {
	return s.Do(collectionId, func(b smartblock.SmartBlock) error {
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
	return s.objectLinksCollectionModify(s.anytype.PredefinedBlocks().Home, req.ContextId, req.IsFavorite)
}

func (s *Service) SetPageIsArchived(req pb.RpcObjectSetIsArchivedRequest) (err error) {
	if err := s.checkArchivedRestriction(req.IsArchived, req.ContextId); err != nil {
		return err
	}
	return s.objectLinksCollectionModify(s.anytype.PredefinedBlocks().Archive, req.ContextId, req.IsArchived)
}

func (s *Service) SetSource(ctx *session.Context, req pb.RpcObjectSetSourceRequest) (err error) {
	return s.Do(req.ContextId, func(b smartblock.SmartBlock) error {
		st := b.NewStateCtx(ctx)
		st.SetDetailAndBundledRelation(bundle.RelationKeySetOf, pbtypes.StringList(req.Source))
		return b.Apply(st, smartblock.NoRestrictions)
	})
}

func (s *Service) SetWorkspaceDashboardId(ctx *session.Context, workspaceId string, id string) (setId string, err error) {
	s.Do(workspaceId, func(ws smartblock.SmartBlock) error {
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
		s.spaceDashboardID = id
		return nil
	})
	return id, nil
}

func (s *Service) checkArchivedRestriction(isArchived bool, objectId string) error {
	if !isArchived {
		return nil
	}
	if err := s.restriction.CheckRestrictions(objectId, model.Restrictions_Delete); err != nil {
		return err
	}
	return nil
}

func (s *Service) DeleteArchivedObjects(req pb.RpcObjectListDeleteRequest) (err error) {
	return s.Do(s.anytype.PredefinedBlocks().Archive, func(b smartblock.SmartBlock) error {
		archive, ok := b.(collection.Collection)
		if !ok {
			return fmt.Errorf("unexpected archive block type: %T", b)
		}

		var merr multierror.Error
		var anySucceed bool
		for _, blockId := range req.ObjectIds {
			if exists, _ := archive.HasObject(blockId); exists {
				if err = s.DeleteObject(blockId); err != nil {
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

func (s *Service) ObjectsDuplicate(ids []string) (newIds []string, err error) {
	var newId string
	var merr multierror.Error
	var anySucceed bool
	for _, id := range ids {
		if newId, err = s.ObjectDuplicate(id); err != nil {
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
	return s.Do(s.anytype.PredefinedBlocks().Archive, func(b smartblock.SmartBlock) error {
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

func (s *Service) OnDelete(id string, workspaceRemove func() error) (err error) {
	var (
		fileHashes []string
		isFavorite bool
	)

	err = s.Do(id, func(b smartblock.SmartBlock) error {
		b.ObjectClose()
		st := b.NewState()
		fileHashes = st.GetAllFileHashes(b.FileRelationKeys(st))
		isFavorite = pbtypes.GetBool(st.LocalDetails(), bundle.RelationKeyIsFavorite.String())
		if isFavorite {
			_ = s.SetPageIsFavorite(pb.RpcObjectSetIsFavoriteRequest{IsFavorite: false, ContextId: id})
		}
		b.SetIsDeleted()
		if workspaceRemove != nil {
			return workspaceRemove()
		}
		return nil
	})
	if err != nil && err != ErrBlockNotFound {
		return err
	}

	for _, fileHash := range fileHashes {
		inboundLinks, err := s.objectStore.GetOutboundLinksById(fileHash)
		if err != nil {
			log.Errorf("failed to get inbound links for file %s: %s", fileHash, err.Error())
			continue
		}
		if len(inboundLinks) == 0 {
			if err = s.objectStore.DeleteObject(fileHash); err != nil {
				log.With("file", fileHash).Errorf("failed to delete file from objectstore: %s", err.Error())
			}
			if err = s.fileStore.DeleteByHash(fileHash); err != nil {
				log.With("file", fileHash).Errorf("failed to delete file from filestore: %s", err.Error())
			}
			// space will be reclaimed on the next GC cycle
			if _, err = s.Anytype().FileOffload(fileHash); err != nil {
				log.With("file", fileHash).Errorf("failed to offload file: %s", err.Error())
				continue
			}
			if err = s.fileStore.DeleteFileKeys(fileHash); err != nil {
				log.With("file", fileHash).Errorf("failed to delete file keys: %s", err.Error())
			}
		}
	}

	if err := s.objectStore.DeleteObject(id); err != nil {
		return fmt.Errorf("delete object from local store: %w", err)
	}

	return
}

func (s *Service) sendOnRemoveEvent(ids ...string) {
	if s.sendEvent != nil {
		s.sendEvent(&pb.Event{
			Messages: []*pb.EventMessage{
				&pb.EventMessage{
					Value: &pb.EventMessageValueOfObjectRemove{
						ObjectRemove: &pb.EventObjectRemove{
							Ids: ids,
						},
					},
				},
			},
		})
	}
}

func (s *Service) RemoveListOption(ctx *session.Context, optIds []string, checkInObjects bool) error {
	var workspace *editor.Workspaces
	if err := s.Do(s.anytype.PredefinedBlocks().Account, func(b smartblock.SmartBlock) error {
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
			f, err := database.NewFilters(q, nil, s.objectStore, nil)
			if err != nil {
				return nil
			}
			records, err := s.objectStore.QueryRaw(query.Query{
				Filters: []query.Filter{f},
			})

			if len(records) > 0 {
				return ErrOptionUsedByOtherObjects
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
	// return s.cache.Close()
	return
}

// PickBlock returns opened smartBlock or opens smartBlock in silent mode
func (s *Service) PickBlock(ctx context.Context, id string) (sb smartblock.SmartBlock, err error) {
	return s.getSmartblock(ctx, id)
}

func (s *Service) getSmartblock(ctx context.Context, id string) (sb smartblock.SmartBlock, err error) {
	return s.GetAccountObject(ctx, id)
}

func (s *Service) StateFromTemplate(templateID string, name string) (st *state.State, err error) {
	if err = s.Do(templateID, func(b smartblock.SmartBlock) error {
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

func (s *Service) DoLinksCollection(id string, apply func(b basic.AllOperations) error) error {
	sb, err := s.PickBlock(context.WithValue(context.TODO(), metrics.CtxKeyRequest, "do_links_collection"), id)
	if err != nil {
		return err
	}

	if bb, ok := sb.(basic.AllOperations); ok {
		sb.Lock()
		defer sb.Unlock()
		return apply(bb)
	}
	return fmt.Errorf("basic operation not available for this block type: %T", sb)
}

func (s *Service) DoClipboard(id string, apply func(b clipboard.Clipboard) error) error {
	sb, err := s.PickBlock(context.WithValue(context.TODO(), metrics.CtxKeyRequest, "do_clipboard"), id)
	if err != nil {
		return err
	}

	if bb, ok := sb.(clipboard.Clipboard); ok {
		sb.Lock()
		defer sb.Unlock()
		return apply(bb)
	}
	return fmt.Errorf("clipboard operation not available for this block type: %T", sb)
}

func (s *Service) DoText(id string, apply func(b stext.Text) error) error {
	sb, err := s.PickBlock(context.WithValue(context.TODO(), metrics.CtxKeyRequest, "do_text"), id)
	if err != nil {
		return err
	}

	if bb, ok := sb.(stext.Text); ok {
		sb.Lock()
		defer sb.Unlock()
		return apply(bb)
	}
	return fmt.Errorf("text operation not available for this block type: %T", sb)
}

func (s *Service) DoFile(id string, apply func(b file.File) error) error {
	sb, err := s.PickBlock(context.WithValue(context.TODO(), metrics.CtxKeyRequest, "do_file"), id)
	if err != nil {
		return err
	}

	if bb, ok := sb.(file.File); ok {
		sb.Lock()
		defer sb.Unlock()
		return apply(bb)
	}
	return fmt.Errorf("file operation not available for this block type: %T", sb)
}

func (s *Service) DoBookmark(id string, apply func(b bookmark.Bookmark) error) error {
	sb, err := s.PickBlock(context.WithValue(context.TODO(), metrics.CtxKeyRequest, "do_bookmark"), id)
	if err != nil {
		return err
	}

	if bb, ok := sb.(bookmark.Bookmark); ok {
		sb.Lock()
		defer sb.Unlock()
		return apply(bb)
	}
	return fmt.Errorf("bookmark operation not available for this block type: %T", sb)
}

func (s *Service) DoFileNonLock(id string, apply func(b file.File) error) error {
	sb, err := s.PickBlock(context.WithValue(context.TODO(), metrics.CtxKeyRequest, "do_filenonlock"), id)
	if err != nil {
		return err
	}

	if bb, ok := sb.(file.File); ok {
		return apply(bb)
	}
	return fmt.Errorf("file non lock operation not available for this block type: %T", sb)
}

func (s *Service) DoHistory(id string, apply func(b basic.IHistory) error) error {
	sb, err := s.PickBlock(context.WithValue(context.TODO(), metrics.CtxKeyRequest, "do_history"), id)
	if err != nil {
		return err
	}

	if bb, ok := sb.(basic.IHistory); ok {
		sb.Lock()
		defer sb.Unlock()
		return apply(bb)
	}
	return fmt.Errorf("undo operation not available for this block type: %T", sb)
}

func (s *Service) DoDataview(id string, apply func(b dataview.Dataview) error) error {
	sb, err := s.PickBlock(context.WithValue(context.TODO(), metrics.CtxKeyRequest, "do_dataview"), id)
	if err != nil {
		return err
	}

	if bb, ok := sb.(dataview.Dataview); ok {
		sb.Lock()
		defer sb.Unlock()
		return apply(bb)
	}
	return fmt.Errorf("text operation not available for this block type: %T", sb)
}

func (s *Service) Do(id string, apply func(b smartblock.SmartBlock) error) error {
	sb, err := s.PickBlock(context.WithValue(context.TODO(), metrics.CtxKeyRequest, "do"), id)
	if err != nil {
		return err
	}

	sb.Lock()
	defer sb.Unlock()
	return apply(sb)
}

type Picker interface {
	PickBlock(ctx context.Context, id string) (sb smartblock.SmartBlock, err error)
}

func Do[t any](p Picker, id string, apply func(sb t) error) error {
	sb, err := p.PickBlock(context.WithValue(context.TODO(), metrics.CtxKeyRequest, "do"), id)
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

func DoWithContext[t any](ctx context.Context, p Picker, id string, apply func(sb t) error) error {
	sb, err := p.PickBlock(ctx, id)
	if err != nil {
		return err
	}

	callerID, _ := ctx.Value(smartblock.CallerKey).(string)
	if callerID != id {
		sb.Lock()
		defer sb.Unlock()
	}

	bb, ok := sb.(t)
	if !ok {
		var dummy = new(t)
		return fmt.Errorf("the interface %T is not implemented in %T", dummy, sb)
	}
	return apply(bb)
}

func DoState[t any](p Picker, id string, apply func(s *state.State, sb t) error, flags ...smartblock.ApplyFlag) error {
	return DoStateCtx(p, nil, id, apply, flags...)
}

// DoState2 picks two blocks and perform an action on them. The order of locks is always the same for two ids.
// It correctly handles the case when two ids are the same.
func DoState2[t1, t2 any](s *Service, firstID, secondID string, f func(*state.State, *state.State, t1, t2) error) error {
	if firstID == secondID {
		return DoState(s, firstID, func(st *state.State, b t1) error {
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
		return DoState(s, firstID, func(firstState *state.State, firstBlock t1) error {
			return DoState(s, secondID, func(secondState *state.State, secondBlock t2) error {
				return f(firstState, secondState, firstBlock, secondBlock)
			})
		})
	}
	return DoState(s, secondID, func(secondState *state.State, secondBlock t2) error {
		return DoState(s, firstID, func(firstState *state.State, firstBlock t1) error {
			return f(firstState, secondState, firstBlock, secondBlock)
		})
	})
}

func DoStateCtx[t any](p Picker, ctx *session.Context, id string, apply func(s *state.State, sb t) error, flags ...smartblock.ApplyFlag) error {
	sb, err := p.PickBlock(context.WithValue(context.TODO(), metrics.CtxKeyRequest, "do"), id)
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

func (s *Service) DoWithContext(ctx context.Context, id string, apply func(b smartblock.SmartBlock) error) error {
	sb, err := s.PickBlock(ctx, id)
	if err != nil {
		return err
	}

	callerId, _ := ctx.Value(smartblock.CallerKey).(string)
	if callerId != id {
		sb.Lock()
		defer sb.Unlock()
	}
	return apply(sb)
}

func (s *Service) ObjectApplyTemplate(contextId, templateId string) error {
	return s.Do(contextId, func(b smartblock.SmartBlock) error {
		orig := b.NewState().ParentState()
		ts, err := s.StateFromTemplate(templateId, pbtypes.GetString(orig.Details(), bundle.RelationKeyName.String()))
		if err != nil {
			return err
		}
		ts.SetRootId(contextId)
		ts.SetParent(orig)

		if toLayout, ok := orig.Layout(); ok {
			if err := converter.ConvertLayout(orig, 0, toLayout); err != nil {
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

func (s *Service) ResetToState(pageID string, st *state.State) (err error) {
	return s.Do(pageID, func(sb smartblock.SmartBlock) error {
		return history.ResetToVersion(sb, st)
	})
}

func (s *Service) ObjectBookmarkFetch(req pb.RpcObjectBookmarkFetchRequest) (err error) {
	url, err := uri.NormalizeURI(req.Url)
	if err != nil {
		return fmt.Errorf("process uri: %w", err)
	}
	res := s.bookmark.FetchBookmarkContent(url)
	go func() {
		if err := s.bookmark.UpdateBookmarkObject(req.ContextId, res); err != nil {
			log.Errorf("update bookmark object %s: %s", req.ContextId, err)
		}
	}()
	return nil
}

func (s *Service) ObjectToBookmark(id string, url string) (objectId string, err error) {
	objectId, _, err = s.objectCreator.CreateObject(&pb.RpcObjectCreateBookmarkRequest{
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
	res, err := oStore.GetWithLinksInfoByID(id)
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
