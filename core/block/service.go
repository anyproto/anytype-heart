package block

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/hashicorp/go-multierror"
	"github.com/ipfs/go-datastore/query"
	"github.com/textileio/go-threads/core/thread"

	"github.com/anytypeio/go-anytype-middleware/app"
	bookmarksvc "github.com/anytypeio/go-anytype-middleware/core/block/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/block/doc"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/clipboard"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/collection"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/file"
	_import "github.com/anytypeio/go-anytype-middleware/core/block/editor/import"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/stext"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/core/block/restriction"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/core/event"
	"github.com/anytypeio/go-anytype-middleware/core/relation"
	"github.com/anytypeio/go-anytype-middleware/core/relation/relationutils"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/core/status"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"
	"github.com/anytypeio/go-anytype-middleware/util/internalflag"
	"github.com/anytypeio/go-anytype-middleware/util/linkpreview"
	"github.com/anytypeio/go-anytype-middleware/util/ocache"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/uri"

	_ "github.com/anytypeio/go-anytype-middleware/core/block/editor/table"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/file"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/link"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/widget"
)

const (
	CName           = "blockService"
	linkObjectShare = "anytype://object/share?"
)

var (
	ErrBlockNotFound       = errors.New("block not found")
	ErrBlockAlreadyOpen    = errors.New("block already open")
	ErrUnexpectedBlockType = errors.New("unexpected block type")
	ErrUnknownObjectType   = fmt.Errorf("unknown object type")
)

var log = logging.Logger("anytype-mw-service")

var (
	blockCacheTTL       = time.Minute
	blockCleanupTimeout = time.Second * 30
)

var (
	// quick fix for limiting file upload goroutines
	uploadFilesLimiter = make(chan struct{}, 8)
)

func init() {
	for i := 0; i < cap(uploadFilesLimiter); i++ {
		uploadFilesLimiter <- struct{}{}
	}
}

type EventKey int

const ObjectCreateEvent EventKey = 0

type SmartblockOpener interface {
	Open(id string) (sb smartblock.SmartBlock, err error)
}

func newOpenedBlock(sb smartblock.SmartBlock) *openedBlock {
	var ob = openedBlock{SmartBlock: sb}
	if sb.Type() != model.SmartBlockType_Breadcrumbs {
		// decode and store corresponding threadID for appropriate block
		if tid, err := thread.Decode(sb.Id()); err != nil {
			log.With("thread", sb.Id()).Warnf("can't restore thread ID: %v", err)
		} else {
			ob.threadId = tid
		}
	}
	return &ob
}

type openedBlock struct {
	smartblock.SmartBlock
	threadId thread.ID
}

func New() *Service {
	return &Service{}
}

type Service struct {
	anytype         core.Service
	status          status.Service
	sendEvent       func(event *pb.Event)
	closed          bool
	linkPreview     linkpreview.LinkPreview
	process         process.Service
	doc             doc.Service
	app             *app.App
	source          source.Service
	cache           ocache.OCache
	objectStore     objectstore.ObjectStore
	restriction     restriction.Service
	bookmark        bookmarksvc.Service
	relationService relation.Service
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
	s.doc = a.MustComponent(doc.CName).(doc.Service)
	s.objectStore = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	s.restriction = a.MustComponent(restriction.CName).(restriction.Service)
	s.bookmark = a.MustComponent("bookmark-importer").(bookmarksvc.Service)
	s.relationService = a.MustComponent(relation.CName).(relation.Service)
	s.app = a
	s.cache = ocache.New(s.loadSmartblock)
	return
}

func (s *Service) Run(ctx context.Context) (err error) {
	s.initPredefinedBlocks(ctx)
	return
}

func (s *Service) initPredefinedBlocks(ctx context.Context) {
	ids := []string{
		s.anytype.PredefinedBlocks().Account,
		s.anytype.PredefinedBlocks().AccountOld,
		s.anytype.PredefinedBlocks().Profile,
		s.anytype.PredefinedBlocks().Archive,
		s.anytype.PredefinedBlocks().Home,
		s.anytype.PredefinedBlocks().MarketplaceType,
		s.anytype.PredefinedBlocks().MarketplaceRelation,
		s.anytype.PredefinedBlocks().MarketplaceTemplate,
		s.anytype.PredefinedBlocks().Widgets,
	}
	startTime := time.Now()
	for _, id := range ids {
		headsHash, _ := s.anytype.ObjectStore().GetLastIndexedHeadsHash(id)
		if headsHash != "" {
			// skip object that has been already indexed before
			continue
		}
		ctx := &smartblock.InitContext{Ctx: ctx, State: state.NewDoc(id, nil).(*state.State)}
		// this is needed so that old account will create its state successfully on first launch
		if id == s.anytype.PredefinedBlocks().AccountOld {
			ctx = nil
		}
		initTime := time.Now()
		sb, err := s.newSmartBlock(id, ctx)
		if err != nil {
			if err != smartblock.ErrCantInitExistingSmartblockWithNonEmptyState {
				if id == s.anytype.PredefinedBlocks().Account {
					log.With("thread", id).Errorf("can't init predefined account thread: %v", err)
				}
				if id == s.anytype.PredefinedBlocks().AccountOld {
					log.With("thread", id).Errorf("can't init predefined old account thread: %v", err)
				}
				log.With("thread", id).Errorf("can't init predefined block: %v", err)
			}
		} else {
			sb.Close()
		}
		sbType, _ := coresb.SmartBlockTypeFromID(id)
		metrics.SharedClient.RecordEvent(metrics.InitPredefinedBlock{
			SbType:   int(sbType),
			TimeMs:   time.Now().Sub(initTime).Milliseconds(),
			ObjectId: id,
		})
	}
	spent := time.Now().Sub(startTime).Milliseconds()
	if spent > 100 {
		metrics.SharedClient.RecordEvent(metrics.InitPredefinedBlocks{
			TimeMs: spent,
		})
	}
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
	defer s.cache.Release(id)
	ob.Lock()
	defer ob.Unlock()
	ob.SetEventFunc(s.sendEvent)
	if v, hasOpenListner := ob.SmartBlock.(smartblock.SmartObjectOpenListner); hasOpenListner {
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
	if tid := ob.threadId; tid != thread.Undef && s.status != nil {
		var (
			fList = func() []string {
				ob.Lock()
				defer ob.Unlock()
				bs := ob.NewState()
				return bs.GetAllFileHashes(ob.FileRelationKeys(bs))
			}
		)

		if newWatcher := s.status.Watch(tid, fList); newWatcher {
			ob.AddHook(func(_ smartblock.ApplyInfo) error {
				s.status.Unwatch(tid)
				return nil
			}, smartblock.HookOnClose)
		}
	}
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
	bs.Lock()
	defer bs.Unlock()
	bs.SetEventFunc(s.sendEvent)
	ob := newOpenedBlock(bs)
	s.cache.Add(bs.Id(), ob)

	// workaround to increase ref counter
	if _, err = s.cache.Get(context.Background(), bs.Id()); err != nil {
		return
	}

	obj, err = bs.Show(ctx)
	if err != nil {
		return
	}
	return obj, bs.Id(), nil
}

func (s *Service) CloseBlock(id string) error {
	var (
		isDraft     bool
		workspaceId string
	)
	err := s.Do(id, func(b smartblock.SmartBlock) error {
		b.ObjectClose()
		s := b.NewState()
		isDraft = internalflag.NewFromState(s).Has(model.InternalFlag_editorDeleteEmpty)
		workspaceId = pbtypes.GetString(s.LocalDetails(), bundle.RelationKeyWorkspaceId.String())
		return nil
	})
	if err != nil {
		return err
	}

	if isDraft {
		_, _ = s.cache.Remove(id)
		if err = s.DeleteObjectFromWorkspace(workspaceId, id); err != nil {
			log.Errorf("error while block delete: %v", err)
		} else {
			s.sendOnRemoveEvent(id)
		}
	}
	return nil
}

func (s *Service) CloseBlocks() {
	s.cache.ForEach(func(v ocache.Object) (isContinue bool) {
		ob := v.(*openedBlock)
		ob.Lock()
		ob.ObjectClose()
		ob.Unlock()
		s.cache.Reset(ob.Id())
		return true
	})
}

func (s *Service) CreateWorkspace(req *pb.RpcWorkspaceCreateRequest) (workspaceId string, err error) {
	id, _, err := s.CreateSmartBlock(context.TODO(), coresb.SmartBlockTypeWorkspace,
		&types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyName.String():      pbtypes.String(req.Name),
			bundle.RelationKeyType.String():      pbtypes.String(bundle.TypeKeySpace.URL()),
			bundle.RelationKeyIconEmoji.String(): pbtypes.String("🌎"),
			bundle.RelationKeyLayout.String():    pbtypes.Float64(float64(model.ObjectType_space)),
		}}, nil)
	return id, err
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

func (s *Service) RemoveSubObjectsInWorkspace(objectIds []string, workspaceId string) (err error) {
	err = s.Do(workspaceId, func(b smartblock.SmartBlock) error {
		ws, ok := b.(*editor.Workspaces)
		if !ok {
			return fmt.Errorf("incorrect workspace id")
		}
		err = ws.RemoveSubObjects(objectIds)
		return err
	})

	return
}

func (s *Service) SelectWorkspace(req *pb.RpcWorkspaceSelectRequest) error {
	panic("should be removed")
}

func (s *Service) GetCurrentWorkspace(req *pb.RpcWorkspaceGetCurrentRequest) (string, error) {
	workspaceId, err := s.anytype.ObjectStore().GetCurrentWorkspaceId()
	if err != nil && strings.HasSuffix(err.Error(), "key not found") {
		return "", nil
	}
	return workspaceId, err
}

func (s *Service) GetAllWorkspaces(req *pb.RpcWorkspaceGetAllRequest) ([]string, error) {
	return s.anytype.GetAllWorkspaces()
}

func (s *Service) SetIsHighlighted(req *pb.RpcWorkspaceSetIsHighlightedRequest) error {
	workspaceId, _ := s.anytype.GetWorkspaceIdForObject(req.ObjectId)
	return s.Do(workspaceId, func(b smartblock.SmartBlock) error {
		workspace, ok := b.(*editor.Workspaces)
		if !ok {
			return fmt.Errorf("incorrect object with workspace id")
		}
		return workspace.SetIsHighlighted(req.ObjectId, req.IsHighlighted)
	})
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
	return s.Do(s.Anytype().PredefinedBlocks().Account, func(b smartblock.SmartBlock) error {
		workspace, ok := b.(*editor.Workspaces)
		if !ok {
			return fmt.Errorf("incorrect object with workspace id")
		}

		return workspace.AddObject(req.ObjectId, protoPayload.Key, protoPayload.Addrs)
	})
}

func (s *Service) ObjectShareByLink(req *pb.RpcObjectShareByLinkRequest) (link string, err error) {
	workspaceId, err := s.anytype.GetWorkspaceIdForObject(req.ObjectId)
	if err == core.ErrObjectDoesNotBelongToWorkspace {
		workspaceId = s.Anytype().PredefinedBlocks().Account
	}
	var key string
	var addrs []string
	err = s.Do(workspaceId, func(b smartblock.SmartBlock) error {
		workspace, ok := b.(*editor.Workspaces)
		if !ok {
			return fmt.Errorf("incorrect object with workspace id")
		}
		key, addrs, err = workspace.GetObjectKeyAddrs(req.ObjectId)
		return err
	})
	if err != nil {
		return "", err
	}
	payload := &model.ThreadDeeplinkPayload{
		Key:   key,
		Addrs: addrs,
	}
	marshalledPayload, err := proto.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal deeplink payload: %w", err)
	}
	encodedPayload := base64.RawStdEncoding.EncodeToString(marshalledPayload)

	params := url.Values{}
	params.Add("id", req.ObjectId)
	params.Add("payload", encodedPayload)
	encoded := params.Encode()

	return fmt.Sprintf("%s%s", linkObjectShare, encoded), nil
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

func (s *Service) AddCreatorInfoIfNeeded(workspaceId string) error {
	if s.Anytype().PredefinedBlocks().IsAccount(workspaceId) {
		return nil
	}
	return s.Do(workspaceId, func(b smartblock.SmartBlock) error {
		workspace, ok := b.(*editor.Workspaces)
		if !ok {
			return fmt.Errorf("incorrect object with workspace id")
		}
		return workspace.AddCreatorInfoIfNeeded()
	})
}

func (s *Service) DeleteObject(id string) (err error) {
	var (
		fileHashes  []string
		workspaceId string
		isFavorite  bool
	)

	err = s.Do(id, func(b smartblock.SmartBlock) error {
		if err = b.Restrictions().Object.Check(model.Restrictions_Delete); err != nil {
			return err
		}
		b.ObjectClose()
		st := b.NewState()
		fileHashes = st.GetAllFileHashes(b.FileRelationKeys(st))
		workspaceId, err = s.anytype.GetWorkspaceIdForObject(id)
		if workspaceId == "" {
			workspaceId = s.anytype.PredefinedBlocks().Account
		}
		isFavorite = pbtypes.GetBool(st.LocalDetails(), bundle.RelationKeyIsFavorite.String())
		if isFavorite {
			_ = s.SetPageIsFavorite(pb.RpcObjectSetIsFavoriteRequest{IsFavorite: false, ContextId: id})
		}
		if err = s.DeleteObjectFromWorkspace(workspaceId, id); err != nil {
			return err
		}
		b.SetIsDeleted()
		return nil
	})

	if err != nil && err != ErrBlockNotFound {
		return err
	}
	_, _ = s.cache.Remove(id)

	for _, fileHash := range fileHashes {
		inboundLinks, err := s.Anytype().ObjectStore().GetOutboundLinksById(fileHash)
		if err != nil {
			log.Errorf("failed to get inbound links for file %s: %s", fileHash, err.Error())
			continue
		}
		if len(inboundLinks) == 0 {
			if err = s.Anytype().ObjectStore().DeleteObject(fileHash); err != nil {
				log.With("file", fileHash).Errorf("failed to delete file from objectstore: %s", err.Error())
			}
			if err = s.Anytype().FileStore().DeleteByHash(fileHash); err != nil {
				log.With("file", fileHash).Errorf("failed to delete file from filestore: %s", err.Error())
			}
			// space will be reclaimed on the next GC cycle
			if _, err = s.Anytype().FileOffload(fileHash); err != nil {
				log.With("file", fileHash).Errorf("failed to offload file: %s", err.Error())
				continue
			}
			if err = s.Anytype().FileStore().DeleteFileKeys(fileHash); err != nil {
				log.With("file", fileHash).Errorf("failed to delete file keys: %s", err.Error())
			}

		}
	}

	s.sendOnRemoveEvent(id)
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

func (s *Service) CreateSmartBlock(
	ctx context.Context,
	sbType coresb.SmartBlockType,
	details *types.Struct,
	relationIds []string,
) (id string, newDetails *types.Struct, err error) {
	return s.CreateSmartBlockFromState(ctx, sbType, details, relationIds, state.NewDoc("", nil).NewState())
}

func (s *Service) CreateSmartBlockFromTemplate(
	ctx context.Context,
	sbType coresb.SmartBlockType,
	details *types.Struct,
	relationIds []string,
	templateId string,
) (id string, newDetails *types.Struct, err error) {
	var createState *state.State
	if templateId != "" {
		if createState, err = s.stateFromTemplate(
			templateId,
			pbtypes.GetString(details, bundle.RelationKeyName.String()),
		); err != nil {
			return
		}
	} else {
		createState = state.NewDoc("", nil).NewState()
	}
	return s.CreateSmartBlockFromState(ctx, sbType, details, relationIds, createState)
}

func (s *Service) CreateSmartBlockFromState(
	ctx context.Context,
	sbType coresb.SmartBlockType,
	details *types.Struct,
	relationIds []string,
	createState *state.State,
) (id string, newDetails *types.Struct, err error) {
	startTime := time.Now()
	objectTypes := pbtypes.GetStringList(details, bundle.RelationKeyType.String())
	if objectTypes == nil {
		objectTypes = createState.ObjectTypes()
		if objectTypes == nil {
			objectTypes = pbtypes.GetStringList(createState.Details(), bundle.RelationKeyType.String())
		}
	}
	if len(objectTypes) == 0 {
		if ot, exists := bundle.DefaultObjectTypePerSmartblockType[sbType]; exists {
			objectTypes = []string{ot.URL()}
		} else {
			objectTypes = []string{bundle.TypeKeyPage.URL()}
		}
	}

	var workspaceId string
	if details != nil && details.Fields != nil {
		for k, v := range details.Fields {
			createState.SetDetail(k, v)
			// TODO: add relations to relationIds
		}

		detailsWorkspaceId := details.Fields[bundle.RelationKeyWorkspaceId.String()]
		if detailsWorkspaceId != nil && detailsWorkspaceId.GetStringValue() != "" {
			workspaceId = detailsWorkspaceId.GetStringValue()
		}
	}

	// if we don't have anything in details then check the object store
	if workspaceId == "" {
		workspaceId = s.anytype.PredefinedBlocks().Account
	}

	if workspaceId != "" {
		createState.SetDetailAndBundledRelation(bundle.RelationKeyWorkspaceId, pbtypes.String(workspaceId))
	}
	createState.SetDetailAndBundledRelation(bundle.RelationKeyCreatedDate, pbtypes.Int64(time.Now().Unix()))
	createState.SetDetailAndBundledRelation(bundle.RelationKeyCreator, pbtypes.String(s.anytype.ProfileID()))

	ev := &metrics.CreateObjectEvent{
		SetDetailsMs: time.Now().Sub(startTime).Milliseconds(),
	}
	ctx = context.WithValue(ctx, ObjectCreateEvent, ev)
	var tid = thread.Undef
	if id := pbtypes.GetString(createState.CombinedDetails(), bundle.RelationKeyId.String()); id != "" {
		tid, err = thread.Decode(id)
		if err != nil {
			log.Errorf("failed to decode thread id from the state: %s", err.Error())
		}
	}

	csm, err := s.CreateObjectInWorkspace(ctx, workspaceId, tid, sbType)
	if err != nil {
		err = fmt.Errorf("anytype.CreateBlock error: %v", err)
		return
	}
	id = csm.ID()
	createState.SetRootId(id)
	createState.SetObjectTypes(objectTypes)
	createState.InjectDerivedDetails()

	initCtx := &smartblock.InitContext{
		ObjectTypeUrls: objectTypes,
		State:          createState,
		RelationIds:    relationIds,
	}
	var sb smartblock.SmartBlock
	if sb, err = s.newSmartBlock(id, initCtx); err != nil {
		return id, nil, err
	}
	ev.SmartblockCreateMs = time.Now().Sub(startTime).Milliseconds() - ev.SetDetailsMs - ev.WorkspaceCreateMs - ev.GetWorkspaceBlockWaitMs
	ev.SmartblockType = int(sbType)
	ev.ObjectId = id
	metrics.SharedClient.RecordEvent(*ev)
	defer sb.Close()
	return id, sb.CombinedDetails(), nil
}

// CreateLinkToTheNewObject creates an object and stores the link to it in the context block
func (s *Service) CreateLinkToTheNewObject(
	ctx *session.Context,
	groupId string,
	req pb.RpcBlockLinkCreateWithObjectRequest,
) (linkId string, objectId string, err error) {
	if req.ContextId == req.TemplateId && req.ContextId != "" {
		err = fmt.Errorf("unable to create link to template from this template")
		return
	}
	req.Details = internalflag.PutToDetails(req.Details, req.InternalFlags)

	var creator func(ctx context.Context) (string, error)

	if pbtypes.GetString(req.Details, bundle.RelationKeyType.String()) == bundle.TypeKeySet.URL() {
		creator = func(ctx context.Context) (string, error) {
			objectId, _, err = s.CreateSet(pb.RpcObjectCreateSetRequest{
				Details: req.Details,
			})
			if err != nil {
				return objectId, fmt.Errorf("create smartblock error: %v", err)
			}
			return objectId, nil
		}
	} else {
		creator = func(ctx context.Context) (string, error) {
			objectId, _, err = s.CreateSmartBlockFromTemplate(ctx, coresb.SmartBlockTypePage, req.Details, nil, req.TemplateId)
			if err != nil {
				return objectId, fmt.Errorf("create smartblock error: %v", err)
			}
			return objectId, nil
		}
	}

	if req.ContextId != "" {
		err = s.Do(req.ContextId, func(sb smartblock.SmartBlock) error {

			linkId, objectId, err = s.createObject(ctx, sb, groupId, req, true, creator)
			return err
		})
		return
	}

	return s.createObject(ctx, nil, groupId, req, true, creator)
}

func (s *Service) CreateObjectFromState(
	ctx *session.Context,
	contextBlock smartblock.SmartBlock,
	groupId string,
	req pb.RpcBlockLinkCreateWithObjectRequest,
	state *state.State,
) (linkId string, objectId string, err error) {
	return s.createObject(ctx, contextBlock, groupId, req, false, func(ctx context.Context) (string, error) {
		objectId, _, err = s.CreateSmartBlockFromState(ctx, coresb.SmartBlockTypePage, req.Details, nil, state)
		if err != nil {
			return objectId, fmt.Errorf("create smartblock error: %v", err)
		}
		return objectId, nil
	})
}

func (s *Service) createObject(
	ctx *session.Context,
	contextBlock smartblock.SmartBlock,
	groupId string,
	req pb.RpcBlockLinkCreateWithObjectRequest,
	storeLink bool,
	create func(context.Context) (objectId string, err error),
) (linkId string, objectId string, err error) {
	if contextBlock != nil {
		if contextBlock.Type() == model.SmartBlockType_Set {
			return "", "", basic.ErrNotSupported
		}
	}
	workspaceId, err := s.anytype.GetWorkspaceIdForObject(req.ContextId)
	if err != nil {
		workspaceId = ""
	}
	if workspaceId != "" && req.Details != nil {
		threads.WorkspaceLogger.
			With("workspace id", workspaceId).
			Debug("adding workspace id to new object")
		if req.Details.Fields == nil {
			req.Details.Fields = make(map[string]*types.Value)
		}
		req.Details.Fields[bundle.RelationKeyWorkspaceId.String()] = pbtypes.String(workspaceId)
	}

	objectId, err = create(context.TODO())
	if err != nil {
		err = fmt.Errorf("create smartblock error: %v", err)
	}

	// do not create a link
	if (!storeLink) || contextBlock == nil {
		return "", objectId, err
	}

	st := contextBlock.NewStateCtx(ctx).SetGroupId(groupId)
	b, ok := contextBlock.(basic.Creatable)
	if !ok {
		err = fmt.Errorf("%T doesn't implement basic.Basic", contextBlock)
		return
	}
	linkId, err = b.CreateBlock(st, pb.RpcBlockCreateRequest{
		TargetId: req.TargetId,
		Block: &model.Block{
			Content: &model.BlockContentOfLink{
				Link: &model.BlockContentLink{
					TargetBlockId: objectId,
					Style:         model.BlockContentLink_Page,
				},
			},
			Fields: req.Fields,
		},
		Position: req.Position,
	})
	if err != nil {
		err = fmt.Errorf("link create error: %v", err)
	}
	err = contextBlock.Apply(st)
	return
}

func (s *Service) CreateSubObjectInWorkspace(
	details *types.Struct, workspaceId string,
) (id string, newDetails *types.Struct, err error) {
	// todo: rewrite to the current workspace id
	err = s.Do(workspaceId, func(b smartblock.SmartBlock) error {
		workspace, ok := b.(*editor.Workspaces)
		if !ok {
			return fmt.Errorf("object is not a workspace")
		}

		id, newDetails, err = workspace.CreateSubObject(details)
		return err
	})
	return
}

func (s *Service) CreateSubObjectsInWorkspace(
	details []*types.Struct,
) (ids []string, objects []*types.Struct, err error) {
	// todo: rewrite to the current workspace id
	err = s.Do(s.anytype.PredefinedBlocks().Account, func(b smartblock.SmartBlock) error {
		workspace, ok := b.(*editor.Workspaces)
		if !ok {
			return fmt.Errorf("incorrect object with workspace id")
		}
		ids, objects, err = workspace.CreateSubObjects(details)
		return err
	})
	return
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
			f, err := database.NewFilters(q, nil, nil)
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

func (s *Service) Process() process.Service {
	return s.process
}

func (s *Service) ProcessAdd(p process.Process) (err error) {
	return s.process.Add(p)
}

func (s *Service) ProcessCancel(id string) (err error) {
	return s.process.Cancel(id)
}

func (s *Service) Close() error {
	return s.cache.Close()
}

// pickBlock returns opened smartBlock or opens smartBlock in silent mode
func (s *Service) pickBlock(ctx context.Context, id string) (sb smartblock.SmartBlock, release func(), err error) {
	ob, err := s.getSmartblock(ctx, id)
	if err != nil {
		return
	}
	return ob.SmartBlock, func() {
		s.cache.Release(id)
	}, nil
}

func (s *Service) newSmartBlock(id string, initCtx *smartblock.InitContext) (sb smartblock.SmartBlock, err error) {
	sc, err := s.source.NewSource(id, false)
	if err != nil {
		return
	}
	switch sc.Type() {
	case model.SmartBlockType_Page, model.SmartBlockType_Date:
		sb = editor.NewPage(s, s, s, s.bookmark)
	case model.SmartBlockType_Archive:
		sb = editor.NewArchive(s)
	case model.SmartBlockType_Home:
		sb = editor.NewDashboard(s, s)
	case model.SmartBlockType_Set:
		sb = editor.NewSet()
	case model.SmartBlockType_ProfilePage, model.SmartBlockType_AnytypeProfile:
		sb = editor.NewProfile(s, s, s.bookmark, s.sendEvent)
	case model.SmartBlockType_STObjectType,
		model.SmartBlockType_BundledObjectType:
		sb = editor.NewObjectType()
	case model.SmartBlockType_BundledRelation:
		sb = editor.NewSet()
	case model.SmartBlockType_SubObject:
		sb = editor.NewSubObject()
	case model.SmartBlockType_File:
		sb = editor.NewFiles()
	case model.SmartBlockType_MarketplaceType:
		sb = editor.NewMarketplaceType()
	case model.SmartBlockType_MarketplaceRelation:
		sb = editor.NewMarketplaceRelation()
	case model.SmartBlockType_MarketplaceTemplate:
		sb = editor.NewMarketplaceTemplate()
	case model.SmartBlockType_Template:
		sb = editor.NewTemplate(s, s, s, s.bookmark)
	case model.SmartBlockType_BundledTemplate:
		sb = editor.NewTemplate(s, s, s, s.bookmark)
	case model.SmartBlockType_Breadcrumbs:
		sb = editor.NewBreadcrumbs()
	case model.SmartBlockType_Workspace:
		sb = editor.NewWorkspace(s)
	case model.SmartBlockType_AccountOld:
		sb = editor.NewThreadDB(s)
	case model.SmartBlockType_Widget:
		sb = editor.NewWidgetObject()
	default:
		return nil, fmt.Errorf("unexpected smartblock type: %v", sc.Type())
	}

	sb.Lock()
	defer sb.Unlock()
	if initCtx == nil {
		initCtx = &smartblock.InitContext{}
	}
	initCtx.App = s.app
	initCtx.Source = sc
	err = sb.Init(initCtx)
	return
}

func (s *Service) stateFromTemplate(templateId, name string) (st *state.State, err error) {
	if err = s.Do(templateId, func(b smartblock.SmartBlock) error {
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

func (s *Service) MigrateMany(objects []threads.ThreadInfo) (migrated int, err error) {
	err = s.DoWithContext(context.WithValue(context.TODO(), metrics.CtxKeyRequest, "migrate_many"), s.anytype.PredefinedBlocks().Account, func(b smartblock.SmartBlock) error {
		workspace, ok := b.(*editor.Workspaces)
		if !ok {
			return fmt.Errorf("incorrect object with workspace id")
		}

		migrated, err = workspace.MigrateMany(objects)
		if err != nil {
			return err
		}
		return err
	})
	return
}

func (s *Service) DoLinksCollection(id string, apply func(b basic.AllOperations) error) error {
	sb, release, err := s.pickBlock(context.WithValue(context.TODO(), metrics.CtxKeyRequest, "do_links_collection"), id)
	if err != nil {
		return err
	}
	defer release()
	if bb, ok := sb.(basic.AllOperations); ok {
		sb.Lock()
		defer sb.Unlock()
		return apply(bb)
	}
	return fmt.Errorf("basic operation not available for this block type: %T", sb)
}

func (s *Service) DoClipboard(id string, apply func(b clipboard.Clipboard) error) error {
	sb, release, err := s.pickBlock(context.WithValue(context.TODO(), metrics.CtxKeyRequest, "do_clipboard"), id)
	if err != nil {
		return err
	}
	defer release()
	if bb, ok := sb.(clipboard.Clipboard); ok {
		sb.Lock()
		defer sb.Unlock()
		return apply(bb)
	}
	return fmt.Errorf("clipboard operation not available for this block type: %T", sb)
}

func (s *Service) DoText(id string, apply func(b stext.Text) error) error {
	sb, release, err := s.pickBlock(context.WithValue(context.TODO(), metrics.CtxKeyRequest, "do_text"), id)
	if err != nil {
		return err
	}
	defer release()
	if bb, ok := sb.(stext.Text); ok {
		sb.Lock()
		defer sb.Unlock()
		return apply(bb)
	}
	return fmt.Errorf("text operation not available for this block type: %T", sb)
}

func (s *Service) DoFile(id string, apply func(b file.File) error) error {
	sb, release, err := s.pickBlock(context.WithValue(context.TODO(), metrics.CtxKeyRequest, "do_file"), id)
	if err != nil {
		return err
	}
	defer release()
	if bb, ok := sb.(file.File); ok {
		sb.Lock()
		defer sb.Unlock()
		return apply(bb)
	}
	return fmt.Errorf("file operation not available for this block type: %T", sb)
}

func (s *Service) DoBookmark(id string, apply func(b bookmark.Bookmark) error) error {
	sb, release, err := s.pickBlock(context.WithValue(context.TODO(), metrics.CtxKeyRequest, "do_bookmark"), id)
	if err != nil {
		return err
	}
	defer release()
	if bb, ok := sb.(bookmark.Bookmark); ok {
		sb.Lock()
		defer sb.Unlock()
		return apply(bb)
	}
	return fmt.Errorf("bookmark operation not available for this block type: %T", sb)
}

func (s *Service) DoFileNonLock(id string, apply func(b file.File) error) error {
	sb, release, err := s.pickBlock(context.WithValue(context.TODO(), metrics.CtxKeyRequest, "do_filenonlock"), id)
	if err != nil {
		return err
	}
	defer release()
	if bb, ok := sb.(file.File); ok {
		return apply(bb)
	}
	return fmt.Errorf("file non lock operation not available for this block type: %T", sb)
}

func (s *Service) DoHistory(id string, apply func(b basic.IHistory) error) error {
	sb, release, err := s.pickBlock(context.WithValue(context.TODO(), metrics.CtxKeyRequest, "do_history"), id)
	if err != nil {
		return err
	}
	defer release()
	if bb, ok := sb.(basic.IHistory); ok {
		sb.Lock()
		defer sb.Unlock()
		return apply(bb)
	}
	return fmt.Errorf("undo operation not available for this block type: %T", sb)
}

func (s *Service) DoImport(id string, apply func(b _import.Import) error) error {
	sb, release, err := s.pickBlock(context.WithValue(context.TODO(), metrics.CtxKeyRequest, "do_import"), id)
	if err != nil {
		return err
	}
	defer release()
	if bb, ok := sb.(_import.Import); ok {
		sb.Lock()
		defer sb.Unlock()
		return apply(bb)
	}

	return fmt.Errorf("import operation not available for this block type: %T", sb)
}

func (s *Service) DoDataview(id string, apply func(b dataview.Dataview) error) error {
	sb, release, err := s.pickBlock(context.WithValue(context.TODO(), metrics.CtxKeyRequest, "do_dataview"), id)
	if err != nil {
		return err
	}
	defer release()
	if bb, ok := sb.(dataview.Dataview); ok {
		sb.Lock()
		defer sb.Unlock()
		return apply(bb)
	}
	return fmt.Errorf("text operation not available for this block type: %T", sb)
}

func (s *Service) Do(id string, apply func(b smartblock.SmartBlock) error) error {
	sb, release, err := s.pickBlock(context.WithValue(context.TODO(), metrics.CtxKeyRequest, "do"), id)
	if err != nil {
		return err
	}
	defer release()
	sb.Lock()
	defer sb.Unlock()
	return apply(sb)
}

func Do[t any](s *Service, id string, apply func(sb t) error) error {
	sb, release, err := s.pickBlock(context.WithValue(context.TODO(), metrics.CtxKeyRequest, "do"), id)
	if err != nil {
		return err
	}
	defer release()

	bb, ok := sb.(t)
	if !ok {
		var dummy = new(t)
		return fmt.Errorf("the interface %T is not implemented in %T", dummy, sb)
	}

	sb.Lock()
	defer sb.Unlock()
	return apply(bb)
}

func DoState[t any](
	s *Service, id string, apply func(s *state.State, sb t) error, flags ...smartblock.ApplyFlag,
) error {
	return DoStateCtx(s, nil, id, apply, flags...)
}

func DoStateCtx[t any](
	s *Service, ctx *session.Context, id string, apply func(s *state.State, sb t) error, flags ...smartblock.ApplyFlag,
) error {
	sb, release, err := s.pickBlock(context.WithValue(context.TODO(), metrics.CtxKeyRequest, "do"), id)
	if err != nil {
		return err
	}
	defer release()

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
	sb, release, err := s.pickBlock(ctx, id)
	if err != nil {
		return err
	}
	defer release()
	callerId, _ := ctx.Value(smartblock.CallerKey).(string)
	if callerId != id {
		sb.Lock()
		defer sb.Unlock()
	}
	return apply(sb)
}

func (s *Service) TemplateCreateFromObject(id string) (templateId string, err error) {
	var st *state.State
	if err = s.Do(id, func(b smartblock.SmartBlock) error {
		if b.Type() != model.SmartBlockType_Page {
			return fmt.Errorf("can't make template from this obect type")
		}
		st, err = b.TemplateCreateFromObjectState()
		return err
	}); err != nil {
		return
	}

	templateId, _, err = s.CreateSmartBlockFromState(context.TODO(), coresb.SmartBlockTypeTemplate, nil, nil, st)
	if err != nil {
		return
	}
	return
}

func (s *Service) TemplateCreateFromObjectByObjectType(otId string) (templateId string, err error) {
	if err = s.Do(otId, func(_ smartblock.SmartBlock) error { return nil }); err != nil {
		return "", fmt.Errorf("can't open objectType: %v", err)
	}
	var st = state.NewDoc("", nil).(*state.State)
	st.SetDetail(bundle.RelationKeyTargetObjectType.String(), pbtypes.String(otId))
	st.SetObjectTypes([]string{bundle.TypeKeyTemplate.URL(), otId})
	templateId, _, err = s.CreateSmartBlockFromState(context.TODO(), coresb.SmartBlockTypeTemplate, nil, nil, st)
	if err != nil {
		return
	}
	return
}

func (s *Service) TemplateClone(id string) (templateId string, err error) {
	var st *state.State
	if err = s.Do(id, func(b smartblock.SmartBlock) error {
		if b.Type() != model.SmartBlockType_BundledTemplate {
			return fmt.Errorf("can clone bundled templates only")
		}
		st = b.NewState().Copy()
		st.RemoveDetail(bundle.RelationKeyTemplateIsBundled.String())
		st.SetLocalDetails(nil)
		t := st.ObjectTypes()
		t, _ = relationutils.MigrateObjectTypeIds(t)
		st.SetObjectTypes(t)
		targetObjectType, _ := relationutils.MigrateObjectTypeId(pbtypes.GetString(st.Details(), bundle.RelationKeyTargetObjectType.String()))
		st.SetDetail(bundle.RelationKeyTargetObjectType.String(), pbtypes.String(targetObjectType))
		return nil
	}); err != nil {
		return
	}
	templateId, _, err = s.CreateSmartBlockFromState(context.TODO(), coresb.SmartBlockTypeTemplate, nil, nil, st)
	if err != nil {
		return
	}
	return
}

func (s *Service) ObjectDuplicate(id string) (objectId string, err error) {
	var st *state.State
	if err = s.Do(id, func(b smartblock.SmartBlock) error {
		if err = b.Restrictions().Object.Check(model.Restrictions_Duplicate); err != nil {
			return err
		}
		st = b.NewState().Copy()
		st.SetLocalDetails(nil)
		return nil
	}); err != nil {
		return
	}

	sbt, err := coresb.SmartBlockTypeFromID(id)
	if err != nil {
		return
	}

	objectId, _, err = s.CreateSmartBlockFromState(context.TODO(), sbt, nil, nil, st)
	if err != nil {
		return
	}
	return
}

func (s *Service) ObjectApplyTemplate(contextId, templateId string) error {
	return s.Do(contextId, func(b smartblock.SmartBlock) error {
		orig := b.NewState().ParentState()
		ts, err := s.stateFromTemplate(templateId, pbtypes.GetString(orig.Details(), bundle.RelationKeyName.String()))
		if err != nil {
			return err
		}
		ts.SetRootId(contextId)
		ts.SetParent(orig)

		if layout, ok := orig.Layout(); ok && layout == model.ObjectType_note {
			textBlock, err := orig.GetFirstTextBlock()
			if err != nil {
				return err
			}
			if textBlock != nil {
				orig.SetDetail(bundle.RelationKeyName.String(), pbtypes.String(textBlock.Text.Text))
			}
		}

		ts.BlocksInit(orig)
		objType := ts.ObjectType()
		// stateFromTemplate returns state without the localdetails, so they will be taken from the orig state
		ts.SetObjectType(objType)

		flags := internalflag.NewFromState(ts)
		flags.Remove(model.InternalFlag_editorSelectType)
		flags.Remove(model.InternalFlag_editorSelectTemplate)
		flags.AddToState(ts)

		return b.Apply(ts, smartblock.NoRestrictions)
	})
}

func (s *Service) ResetToState(pageId string, state *state.State) (err error) {
	return s.Do(pageId, func(sb smartblock.SmartBlock) error {
		return sb.ResetToVersion(state)
	})
}

func (s *Service) fetchBookmarkContent(url string) bookmarksvc.ContentFuture {
	contentCh := make(chan *model.BlockContentBookmark, 1)
	go func() {
		defer close(contentCh)

		content := &model.BlockContentBookmark{
			Url: url,
		}
		updaters, err := s.bookmark.ContentUpdaters(url)
		if err != nil {
			log.Error("fetch bookmark content %s: %s", url, err)
		}
		for upd := range updaters {
			upd(content)
		}
		contentCh <- content
	}()

	return func() *model.BlockContentBookmark {
		return <-contentCh
	}
}

// ObjectCreateBookmark creates a new Bookmark object for provided URL or returns id of existing one
func (s *Service) ObjectCreateBookmark(
	req pb.RpcObjectCreateBookmarkRequest,
) (objectId string, newDetails *types.Struct, err error) {
	u, err := uri.NormalizeURI(pbtypes.GetString(req.Details, bundle.RelationKeySource.String()))
	if err != nil {
		return "", nil, fmt.Errorf("process uri: %w", err)
	}
	res := s.fetchBookmarkContent(u)
	return s.bookmark.CreateBookmarkObject(req.Details, res)
}

func (s *Service) ObjectBookmarkFetch(req pb.RpcObjectBookmarkFetchRequest) (err error) {
	url, err := uri.NormalizeURI(req.Url)
	if err != nil {
		return fmt.Errorf("process uri: %w", err)
	}
	res := s.fetchBookmarkContent(url)
	go func() {
		if err := s.bookmark.UpdateBookmarkObject(req.ContextId, res); err != nil {
			log.Errorf("update bookmark object %s: %s", req.ContextId, err)
		}
	}()
	return nil
}

func (s *Service) ObjectToBookmark(id string, url string) (objectId string, err error) {
	objectId, _, err = s.ObjectCreateBookmark(pb.RpcObjectCreateBookmarkRequest{
		Details: &types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeySource.String(): pbtypes.String(url),
			},
		},
	})
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

func (s *Service) loadSmartblock(ctx context.Context, id string) (value ocache.Object, err error) {
	sbt, _ := coresb.SmartBlockTypeFromID(id)
	if sbt == coresb.SmartBlockTypeSubObject {
		workspaceId := s.anytype.PredefinedBlocks().Account
		if value, err = s.cache.Get(ctx, workspaceId); err != nil {
			return
		}

		var ok bool
		var ob *openedBlock
		if ob, ok = value.(*openedBlock); !ok {
			return nil, fmt.Errorf("invalid id path '%s': '%s' not implement openedBlock", id, workspaceId)
		}

		var sbOpener SmartblockOpener
		if sbOpener, ok = ob.SmartBlock.(SmartblockOpener); !ok {
			return nil, fmt.Errorf("invalid id path '%s': '%s' not implement SmartblockOpener", id, workspaceId)
		}

		var sb smartblock.SmartBlock
		if sb, err = sbOpener.Open(id); err != nil {
			return
		}
		return newOpenedBlock(sb), nil
	}

	sb, err := s.newSmartBlock(id, &smartblock.InitContext{
		Ctx: ctx,
	})
	if err != nil {
		return
	}
	value = newOpenedBlock(sb)
	return
}

func (s *Service) getSmartblock(ctx context.Context, id string) (ob *openedBlock, err error) {
	val, err := s.cache.Get(ctx, id)
	if err != nil {
		return
	}
	var ok bool
	ob, ok = val.(*openedBlock)
	if !ok {
		return nil, fmt.Errorf("got unexpected object from cache: %t", val)
	} else if ob == nil {
		return nil, fmt.Errorf("got nil object from cache")
	}
	return ob, nil
}

func (s *Service) replaceLink(id, oldId, newId string) error {
	return Do(s, id, func(b basic.CommonOperations) error {
		return b.ReplaceLink(oldId, newId)
	})
}
