package block

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/util/internalflag"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/hashicorp/go-multierror"
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
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/table"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/editor/table"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/core/block/restriction"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/file"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/link"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/core/event"
	"github.com/anytypeio/go-anytype-middleware/core/status"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"
	"github.com/anytypeio/go-anytype-middleware/util/linkpreview"
	"github.com/anytypeio/go-anytype-middleware/util/ocache"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/uri"
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

type Service interface {
	Do(id string, apply func(b smartblock.SmartBlock) error) error
	DoWithContext(ctx context.Context, id string, apply func(b smartblock.SmartBlock) error) error

	OpenBlock(ctx *session.Context, id string) (*model.ObjectView, error)
	ShowBlock(ctx *session.Context, id string) (*model.ObjectView, error)
	OpenBreadcrumbsBlock(ctx *session.Context) (obj *model.ObjectView, blockId string, err error)
	SetBreadcrumbs(ctx *session.Context, req pb.RpcObjectSetBreadcrumbsRequest) (err error)
	CloseBlock(id string) error
	CloseBlocks()
	CreateBlock(ctx *session.Context, req pb.RpcBlockCreateRequest) (string, error)
	CreateLinkToTheNewObject(ctx *session.Context, groupId string, req pb.RpcBlockLinkCreateWithObjectRequest) (linkId string, pageId string, err error)
	CreateObjectFromState(ctx *session.Context, contextBlock smartblock.SmartBlock, groupId string, req pb.RpcBlockLinkCreateWithObjectRequest, st *state.State) (linkId string, pageId string, err error)
	CreateSmartBlock(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, relations []*model.Relation) (id string, newDetails *types.Struct, err error)
	CreateSmartBlockFromTemplate(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, relations []*model.Relation, templateId string) (id string, newDetails *types.Struct, err error)
	CreateSmartBlockFromState(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, relations []*model.Relation, createState *state.State) (id string, newDetails *types.Struct, err error)
	DuplicateBlocks(ctx *session.Context, req pb.RpcBlockListDuplicateRequest) ([]string, error)
	UnlinkBlock(ctx *session.Context, req pb.RpcBlockListDeleteRequest) error
	ReplaceBlock(ctx *session.Context, req pb.RpcBlockReplaceRequest) (newId string, err error)
	ObjectToSet(id string, source []string) (newId string, err error)

	MoveBlocks(ctx *session.Context, req pb.RpcBlockListMoveToExistingObjectRequest) error
	MoveBlocksToNewPage(ctx *session.Context, req pb.RpcBlockListMoveToNewObjectRequest) (linkId string, err error)
	ListConvertToObjects(ctx *session.Context, req pb.RpcBlockListConvertToObjectsRequest) (linkIds []string, err error)
	SetFields(ctx *session.Context, req pb.RpcBlockSetFieldsRequest) error
	SetFieldsList(ctx *session.Context, req pb.RpcBlockListSetFieldsRequest) error

	SetDetails(ctx *session.Context, req pb.RpcObjectSetDetailsRequest) (err error)
	ModifyDetails(objectId string, modifier func(current *types.Struct) (*types.Struct, error)) (err error) // you must copy original struct within the modifier in order to modify it

	GetRelations(objectId string) (relations []*model.Relation, err error)
	UpdateExtraRelations(ctx *session.Context, id string, relations []*model.Relation, createIfMissing bool) (err error)
	ModifyExtraRelations(ctx *session.Context, objectId string, modifier func(current []*model.Relation) ([]*model.Relation, error)) (err error)
	AddExtraRelations(ctx *session.Context, id string, relations []*model.Relation) (relationsWithKeys []*model.Relation, err error)
	RemoveExtraRelations(ctx *session.Context, id string, relationKeys []string) (err error)
	CreateSet(req pb.RpcObjectCreateSetRequest) (setId string, err error)
	SetDataviewSource(ctx *session.Context, contextId, blockId string, source []string) error

	ListAvailableRelations(objectId string) (aggregatedRelations []*model.Relation, err error)
	SetObjectTypes(ctx *session.Context, objectId string, objectTypes []string) (err error)
	AddExtraRelationOption(ctx *session.Context, req pb.RpcObjectRelationOptionAddRequest) (opt *model.RelationOption, err error)
	UpdateExtraRelationOption(ctx *session.Context, req pb.RpcObjectRelationOptionUpdateRequest) (err error)
	DeleteExtraRelationOption(ctx *session.Context, req pb.RpcObjectRelationOptionDeleteRequest) (err error)

	Paste(ctx *session.Context, req pb.RpcBlockPasteRequest, groupId string) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, isSameBlockCaret bool, err error)

	Copy(req pb.RpcBlockCopyRequest) (textSlot string, htmlSlot string, anySlot []*model.Block, err error)
	Cut(ctx *session.Context, req pb.RpcBlockCutRequest) (textSlot string, htmlSlot string, anySlot []*model.Block, err error)
	Export(req pb.RpcBlockExportRequest) (path string, err error)
	ImportMarkdown(ctx *session.Context, req pb.RpcObjectImportMarkdownRequest) (rootLinkIds []string, err error)

	SplitBlock(ctx *session.Context, req pb.RpcBlockSplitRequest) (blockId string, err error)
	MergeBlock(ctx *session.Context, req pb.RpcBlockMergeRequest) error
	SetLatexText(ctx *session.Context, req pb.RpcBlockLatexSetTextRequest) error
	SetTextText(ctx *session.Context, req pb.RpcBlockTextSetTextRequest) error
	SetTextStyle(ctx *session.Context, contextId string, style model.BlockContentTextStyle, blockIds ...string) error
	SetTextChecked(ctx *session.Context, req pb.RpcBlockTextSetCheckedRequest) error
	SetTextColor(ctx *session.Context, contextId string, color string, blockIds ...string) error
	SetTextMark(ctx *session.Context, id string, mark *model.BlockContentTextMark, ids ...string) error
	SetTextIcon(ctx *session.Context, contextId, image, emoji string, blockIds ...string) error
	ClearTextStyle(ctx *session.Context, contextId string, blockIds ...string) error
	ClearTextContent(ctx *session.Context, contextId string, blockIds ...string) error
	SetBackgroundColor(ctx *session.Context, contextId string, color string, blockIds ...string) error
	SetAlign(ctx *session.Context, contextId string, align model.BlockAlign, blockIds ...string) (err error)
	SetVerticalAlign(ctx *session.Context, contextId string, align model.BlockVerticalAlign, blockIds ...string) (err error)
	SetLayout(ctx *session.Context, id string, layout model.ObjectTypeLayout) error
	SetLinkAppearance(ctx *session.Context, req pb.RpcBlockLinkListSetAppearanceRequest) (err error)

	FeaturedRelationAdd(ctx *session.Context, contextId string, relations ...string) error
	FeaturedRelationRemove(ctx *session.Context, contextId string, relations ...string) error

	TurnInto(ctx *session.Context, id string, style model.BlockContentTextStyle, ids ...string) error

	SetDivStyle(ctx *session.Context, contextId string, style model.BlockContentDivStyle, ids ...string) (err error)

	SetFileStyle(ctx *session.Context, contextId string, style model.BlockContentFileStyle, blockIds ...string) error
	UploadFile(req pb.RpcFileUploadRequest) (hash string, err error)
	UploadBlockFile(ctx *session.Context, req pb.RpcBlockUploadRequest, groupId string) error
	UploadBlockFileSync(ctx *session.Context, req pb.RpcBlockUploadRequest) (err error)
	CreateAndUploadFile(ctx *session.Context, req pb.RpcBlockFileCreateAndUploadRequest) (id string, err error)
	DropFiles(req pb.RpcFileDropRequest) (err error)

	Undo(ctx *session.Context, req pb.RpcObjectUndoRequest) (pb.RpcObjectUndoRedoCounter, error)
	Redo(ctx *session.Context, req pb.RpcObjectRedoRequest) (pb.RpcObjectUndoRedoCounter, error)

	SetPagesIsArchived(req pb.RpcObjectListSetIsArchivedRequest) error
	SetPagesIsFavorite(req pb.RpcObjectListSetIsFavoriteRequest) error
	SetPageIsArchived(req pb.RpcObjectSetIsArchivedRequest) error
	SetPageIsFavorite(req pb.RpcObjectSetIsFavoriteRequest) error

	DeleteArchivedObjects(req pb.RpcObjectListDeleteRequest) error
	DeleteObject(id string) error

	GetAggregatedRelations(req pb.RpcBlockDataviewRelationListAvailableRequest) (relations []*model.Relation, err error)
	DeleteDataviewView(ctx *session.Context, req pb.RpcBlockDataviewViewDeleteRequest) error
	UpdateDataviewView(ctx *session.Context, req pb.RpcBlockDataviewViewUpdateRequest) error
	SetDataviewActiveView(ctx *session.Context, req pb.RpcBlockDataviewViewSetActiveRequest) error
	SetDataviewViewPosition(ctx *session.Context, request pb.RpcBlockDataviewViewSetPositionRequest) error
	CreateDataviewView(ctx *session.Context, req pb.RpcBlockDataviewViewCreateRequest) (id string, err error)
	AddDataviewRelation(ctx *session.Context, req pb.RpcBlockDataviewRelationAddRequest) (relation *model.Relation, err error)
	UpdateDataviewRelation(ctx *session.Context, req pb.RpcBlockDataviewRelationUpdateRequest) error
	DeleteDataviewRelation(ctx *session.Context, req pb.RpcBlockDataviewRelationDeleteRequest) error
	AddDataviewRecordRelationOption(ctx *session.Context, req pb.RpcBlockDataviewRecordRelationOptionAddRequest) (opt *model.RelationOption, err error)
	UpdateDataviewRecordRelationOption(ctx *session.Context, req pb.RpcBlockDataviewRecordRelationOptionUpdateRequest) error
	DeleteDataviewRecordRelationOption(ctx *session.Context, req pb.RpcBlockDataviewRecordRelationOptionDeleteRequest) error
	UpdateDataviewGroupOrder(ctx *session.Context, req pb.RpcBlockDataviewGroupOrderUpdateRequest) error
	UpdateDataviewObjectOrder(ctx *session.Context, req pb.RpcBlockDataviewObjectOrderUpdateRequest) error

	CreateDataviewRecord(ctx *session.Context, req pb.RpcBlockDataviewRecordCreateRequest) (*types.Struct, error)
	UpdateDataviewRecord(ctx *session.Context, req pb.RpcBlockDataviewRecordUpdateRequest) error
	DeleteDataviewRecord(ctx *session.Context, req pb.RpcBlockDataviewRecordDeleteRequest) error

	BookmarkFetch(ctx *session.Context, req pb.RpcBlockBookmarkFetchRequest) error
	BookmarkFetchSync(ctx *session.Context, req pb.RpcBlockBookmarkFetchRequest) (err error)
	BookmarkCreateAndFetch(ctx *session.Context, req pb.RpcBlockBookmarkCreateAndFetchRequest) (id string, err error)
	ObjectCreateBookmark(req pb.RpcObjectCreateBookmarkRequest) (id string, err error)
	ObjectBookmarkFetch(req pb.RpcObjectBookmarkFetchRequest) (err error)
	ObjectToBookmark(id string, url string) (newId string, err error)

	CreateTableBlock(ctx *session.Context, req pb.RpcBlockTableCreateRequest) (id string, err error)
	TableExpand(ctx *session.Context, req pb.RpcBlockTableExpandRequest) (err error)
	TableRowCreate(ctx *session.Context, req pb.RpcBlockTableRowCreateRequest) (err error)
	TableRowDuplicate(ctx *session.Context, req pb.RpcBlockTableRowDuplicateRequest) (err error)
	TableRowDelete(ctx *session.Context, req pb.RpcBlockTableRowDeleteRequest) (err error)
	TableRowListFill(ctx *session.Context, req pb.RpcBlockTableRowListFillRequest) (err error)
	TableRowListClean(ctx *session.Context, req pb.RpcBlockTableRowListCleanRequest) (err error)
	TableRowSetHeader(ctx *session.Context, req pb.RpcBlockTableRowSetHeaderRequest) (err error)
	TableColumnCreate(ctx *session.Context, req pb.RpcBlockTableColumnCreateRequest) (err error)
	TableColumnDuplicate(ctx *session.Context, req pb.RpcBlockTableColumnDuplicateRequest) (id string, err error)
	TableColumnMove(ctx *session.Context, req pb.RpcBlockTableColumnMoveRequest) (err error)
	TableColumnDelete(ctx *session.Context, req pb.RpcBlockTableColumnDeleteRequest) (err error)
	TableColumnListFill(ctx *session.Context, req pb.RpcBlockTableColumnListFillRequest) (err error)
	TableSort(ctx *session.Context, req pb.RpcBlockTableSortRequest) (err error)

	SetRelationKey(ctx *session.Context, request pb.RpcBlockRelationSetKeyRequest) error
	AddRelationBlock(ctx *session.Context, request pb.RpcBlockRelationAddRequest) error

	Process() process.Service
	ProcessAdd(p process.Process) (err error)
	ProcessCancel(id string) error

	SimplePaste(contextId string, anySlot []*model.Block) (err error)

	GetDocInfo(ctx context.Context, id string) (info doc.DocInfo, err error)
	Wakeup(id string) (err error)

	TemplateCreateFromObject(id string) (templateId string, err error)
	TemplateCreateFromObjectByObjectType(otId string) (templateId string, err error)
	TemplateClone(id string) (templateId string, err error)
	ObjectsDuplicate(ids []string) (newIds []string, err error)
	ObjectApplyTemplate(contextId, templateId string) error

	CreateWorkspace(req *pb.RpcWorkspaceCreateRequest) (string, error)
	SelectWorkspace(req *pb.RpcWorkspaceSelectRequest) error
	GetCurrentWorkspace(req *pb.RpcWorkspaceGetCurrentRequest) (string, error)
	GetAllWorkspaces(req *pb.RpcWorkspaceGetAllRequest) ([]string, error)
	SetIsHighlighted(req *pb.RpcWorkspaceSetIsHighlightedRequest) error

	ObjectAddWithObjectId(req *pb.RpcObjectAddWithObjectIdRequest) error
	ObjectShareByLink(req *pb.RpcObjectShareByLinkRequest) (string, error)

	AddCreatorInfoIfNeeded(workspaceId string) error

	app.ComponentRunnable
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

func New() Service {
	return new(service)
}

type service struct {
	anytype     core.Service
	status      status.Service
	sendEvent   func(event *pb.Event)
	closed      bool
	linkPreview linkpreview.LinkPreview
	process     process.Service
	doc         doc.Service
	app         *app.App
	source      source.Service
	cache       ocache.OCache
	objectStore objectstore.ObjectStore
	restriction restriction.Service
	bookmark    bookmarksvc.Service
}

func (s *service) Name() string {
	return CName
}

func (s *service) Init(a *app.App) (err error) {
	s.anytype = a.MustComponent(core.CName).(core.Service)
	s.status = a.MustComponent(status.CName).(status.Service)
	s.linkPreview = a.MustComponent(linkpreview.CName).(linkpreview.LinkPreview)
	s.process = a.MustComponent(process.CName).(process.Service)
	s.sendEvent = a.MustComponent(event.CName).(event.Sender).Send
	s.source = a.MustComponent(source.CName).(source.Service)
	s.doc = a.MustComponent(doc.CName).(doc.Service)
	s.objectStore = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	s.restriction = a.MustComponent(restriction.CName).(restriction.Service)
	s.bookmark = a.MustComponent(bookmarksvc.CName).(bookmarksvc.Service)
	s.app = a
	s.cache = ocache.New(s.loadSmartblock)
	return
}

func (s *service) Run(ctx context.Context) (err error) {
	s.initPredefinedBlocks(ctx)
	return
}

func (s *service) initPredefinedBlocks(ctx context.Context) {
	ids := []string{
		s.anytype.PredefinedBlocks().Account,
		s.anytype.PredefinedBlocks().AccountOld,
		s.anytype.PredefinedBlocks().Profile,
		s.anytype.PredefinedBlocks().Archive,
		s.anytype.PredefinedBlocks().Home,
		s.anytype.PredefinedBlocks().MarketplaceType,
		s.anytype.PredefinedBlocks().MarketplaceRelation,
		s.anytype.PredefinedBlocks().MarketplaceTemplate,
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

func (s *service) Anytype() core.Service {
	return s.anytype
}

func (s *service) OpenBlock(ctx *session.Context, id string) (obj *model.ObjectView, err error) {
	startTime := time.Now()
	ob, err := s.getSmartblock(context.WithValue(context.TODO(), metrics.CtxKeyRequest, "object_open"), id)
	if err != nil {
		return nil, err
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
	if err = ob.Apply(st, smartblock.NoHistory); err != nil {
		log.Errorf("failed to update lastOpenedDate: %s", err.Error())
	}
	afterApplyTime := time.Now()
	if obj, err = ob.Show(ctx); err != nil {
		return
	}
	afterShowTime := time.Now()
	s.cache.Lock(id)
	if tid := ob.threadId; tid != thread.Undef && s.status != nil {
		var (
			fList = func() []string {
				ob.Lock()
				defer ob.Unlock()
				bs := ob.NewState()
				return bs.GetAllFileHashes(bs.FileRelationKeys())
			}
		)

		if newWatcher := s.status.Watch(tid, fList); newWatcher {
			ob.AddHook(func() { s.status.Unwatch(tid) }, smartblock.HookOnClose)
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

func (s *service) ShowBlock(ctx *session.Context, id string) (obj *model.ObjectView, err error) {
	cctx := context.WithValue(context.TODO(), metrics.CtxKeyRequest, "object_show")
	err2 := s.DoWithContext(cctx, id, func(b smartblock.SmartBlock) error {
		obj, err = b.Show(ctx)
		return err
	})
	if err2 != nil {
		return nil, err2
	}
	return
}

func (s *service) OpenBreadcrumbsBlock(ctx *session.Context) (obj *model.ObjectView, blockId string, err error) {
	bs := editor.NewBreadcrumbs()
	if err = bs.Init(&smartblock.InitContext{
		Restriction: s.restriction,
		ObjectStore: s.objectStore,
		Doc:         s.doc,
		Source:      source.NewVirtual(s.anytype, model.SmartBlockType_Breadcrumbs),
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

func (s *service) CloseBlock(id string) error {
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
	s.cache.Unlock(id)
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

func (s *service) CloseBlocks() {
	s.cache.ForEach(func(v ocache.Object) (isContinue bool) {
		ob := v.(*openedBlock)
		ob.Lock()
		ob.ObjectClose()
		ob.Unlock()
		s.cache.Reset(ob.Id())
		return true
	})
}

func (s *service) CreateWorkspace(req *pb.RpcWorkspaceCreateRequest) (workspaceId string, err error) {
	id, _, err := s.CreateSmartBlock(context.TODO(), coresb.SmartBlockTypeWorkspace,
		&types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyName.String():      pbtypes.String(req.Name),
			bundle.RelationKeyType.String():      pbtypes.String(bundle.TypeKeySpace.URL()),
			bundle.RelationKeyIconEmoji.String(): pbtypes.String("🌎"),
			bundle.RelationKeyLayout.String():    pbtypes.Float64(float64(model.ObjectType_space)),
		}}, nil)
	return id, err
}

func (s *service) SelectWorkspace(req *pb.RpcWorkspaceSelectRequest) error {
	panic("should be removed")
}

func (s *service) GetCurrentWorkspace(req *pb.RpcWorkspaceGetCurrentRequest) (string, error) {
	workspaceId, err := s.anytype.ObjectStore().GetCurrentWorkspaceId()
	if err != nil && strings.HasSuffix(err.Error(), "key not found") {
		return "", nil
	}
	return workspaceId, err
}

func (s *service) GetAllWorkspaces(req *pb.RpcWorkspaceGetAllRequest) ([]string, error) {
	return s.anytype.GetAllWorkspaces()
}

func (s *service) SetIsHighlighted(req *pb.RpcWorkspaceSetIsHighlightedRequest) error {
	workspaceId, _ := s.anytype.GetWorkspaceIdForObject(req.ObjectId)
	return s.Do(workspaceId, func(b smartblock.SmartBlock) error {
		workspace, ok := b.(*editor.Workspaces)
		if !ok {
			return fmt.Errorf("incorrect object with workspace id")
		}
		return workspace.SetIsHighlighted(req.ObjectId, req.IsHighlighted)
	})
}

func (s *service) ObjectAddWithObjectId(req *pb.RpcObjectAddWithObjectIdRequest) error {
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

func (s *service) ObjectShareByLink(req *pb.RpcObjectShareByLinkRequest) (link string, err error) {
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
func (s *service) SetPagesIsArchived(req pb.RpcObjectListSetIsArchivedRequest) (err error) {
	return s.Do(s.anytype.PredefinedBlocks().Archive, func(b smartblock.SmartBlock) error {
		archive, ok := b.(collection.Collection)
		if !ok {
			return fmt.Errorf("unexpected archive block type: %T", b)
		}

		anySucceed := false
		ids, err := s.objectStore.HasIDs(req.ObjectIds...)
		if err != nil {
			return err
		}
		for _, id := range ids {
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
				log.Errorf("failed to archive %s: %s", id, err.Error())
			} else {
				anySucceed = true
			}
		}

		if !anySucceed {
			return err
		}

		return nil
	})
}

// SetPagesIsFavorite is deprecated
func (s *service) SetPagesIsFavorite(req pb.RpcObjectListSetIsFavoriteRequest) (err error) {
	return s.Do(s.anytype.PredefinedBlocks().Home, func(b smartblock.SmartBlock) error {
		fav, ok := b.(collection.Collection)
		if !ok {
			return fmt.Errorf("unexpected home block type: %T", b)
		}

		anySucceed := false
		ids, err := s.objectStore.HasIDs(req.ObjectIds...)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if req.IsFavorite {
				err = fav.AddObject(id)
			} else {
				err = fav.RemoveObject(id)
			}
			if err != nil {
				log.Errorf("failed to favorite object %s: %s", id, err.Error())
			} else {
				anySucceed = true
			}
		}

		if !anySucceed {
			return err
		}

		return nil
	})
}

func (s *service) objectLinksCollectionModify(collectionId string, objectId string, value bool) error {
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

func (s *service) SetPageIsFavorite(req pb.RpcObjectSetIsFavoriteRequest) (err error) {
	return s.objectLinksCollectionModify(s.anytype.PredefinedBlocks().Home, req.ContextId, req.IsFavorite)
}

func (s *service) SetPageIsArchived(req pb.RpcObjectSetIsArchivedRequest) (err error) {
	if err := s.checkArchivedRestriction(req.IsArchived, req.ContextId); err != nil {
		return err
	}
	return s.objectLinksCollectionModify(s.anytype.PredefinedBlocks().Archive, req.ContextId, req.IsArchived)
}

func (s *service) checkArchivedRestriction(isArchived bool, objectId string) error {
	if err := s.restriction.CheckRestrictions(objectId, model.Restrictions_Delete); isArchived && err != nil {
		return err
	}
	return nil
}

func (s *service) DeleteArchivedObjects(req pb.RpcObjectListDeleteRequest) (err error) {
	return s.Do(s.anytype.PredefinedBlocks().Archive, func(b smartblock.SmartBlock) error {
		archive, ok := b.(collection.Collection)
		if !ok {
			return fmt.Errorf("unexpected archive block type: %T", b)
		}

		anySucceed := false
		for _, blockId := range req.ObjectIds {
			if exists, _ := archive.HasObject(blockId); exists {
				if err = s.DeleteObject(blockId); err == nil {
					archive.RemoveObject(blockId)
					anySucceed = true
				}
			}
		}

		if !anySucceed {
			return err
		}

		return nil
	})
}

func (s *service) ObjectsDuplicate(ids []string) (newIds []string, err error) {
	var (
		newId      string
		anySucceed bool
	)
	var merr multierror.Error
	for _, id := range ids {
		if newId, err = s.ObjectDuplicate(id); err == nil {
			newIds = append(newIds, newId)
			anySucceed = true
		} else {
			merr.Errors = append(merr.Errors, err)
		}
	}
	if !anySucceed {
		err = merr.ErrorOrNil()
	} else {
		err = nil
	}
	return
}

func (s *service) DeleteArchivedObject(id string) (err error) {
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

func (s *service) AddCreatorInfoIfNeeded(workspaceId string) error {
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

func (s *service) DeleteObject(id string) (err error) {
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
		fileHashes = st.GetAllFileHashes(st.FileRelationKeys())
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
	s.cache.Unlock(id)
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

func (s *service) sendOnRemoveEvent(ids ...string) {
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

func (s *service) CreateSmartBlock(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, relations []*model.Relation) (id string, newDetails *types.Struct, err error) {
	return s.CreateSmartBlockFromState(ctx, sbType, details, relations, state.NewDoc("", nil).NewState())
}

func (s *service) CreateSmartBlockFromTemplate(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, relations []*model.Relation, templateId string) (id string, newDetails *types.Struct, err error) {
	var createState *state.State
	if templateId != "" {
		if createState, err = s.stateFromTemplate(templateId, pbtypes.GetString(details, bundle.RelationKeyName.String())); err != nil {
			return
		}
	} else {
		createState = state.NewDoc("", nil).NewState()
	}
	return s.CreateSmartBlockFromState(ctx, sbType, details, relations, createState)
}

func (s *service) CreateSmartBlockFromState(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, relations []*model.Relation, createState *state.State) (id string, newDetails *types.Struct, err error) {
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
			var rel *model.Relation
			if !createState.HasRelation(k) && !pbtypes.HasRelation(relations, k) {
				// in case we don't have a relation both in the state and relations slice, we need to find it other places and add it
				rel, _ = bundle.GetRelation(bundle.RelationKey(k))
				if rel == nil {
					rel, _ = s.objectStore.GetRelation(k)
					if err != nil {
						return "", nil, fmt.Errorf("relation for detail %s not found", k)
					}
				}
				relCopy := pbtypes.CopyRelation(rel)
				relCopy.Scope = model.Relation_object
				relations = append(relations, relCopy)
			}
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
	initCtx := &smartblock.InitContext{
		State:          createState,
		ObjectTypeUrls: objectTypes,
		Relations:      relations,
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
func (s *service) CreateLinkToTheNewObject(ctx *session.Context, groupId string, req pb.RpcBlockLinkCreateWithObjectRequest) (linkId string, objectId string, err error) {
	req.Details = internalflag.AddToDetails(req.Details, req.InternalFlags)

	var creator func(ctx context.Context) (string, error)

	if pbtypes.GetString(req.Details, bundle.RelationKeyType.String()) == bundle.TypeKeySet.URL() {
		creator = func(ctx context.Context) (string, error) {
			objectId, err = s.CreateSet(pb.RpcObjectCreateSetRequest{
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

func (s *service) CreateObjectFromState(ctx *session.Context, contextBlock smartblock.SmartBlock, groupId string, req pb.RpcBlockLinkCreateWithObjectRequest, state *state.State) (linkId string, objectId string, err error) {
	return s.createObject(ctx, contextBlock, groupId, req, false, func(ctx context.Context) (string, error) {
		objectId, _, err = s.CreateSmartBlockFromState(ctx, coresb.SmartBlockTypePage, req.Details, nil, state)
		if err != nil {
			return objectId, fmt.Errorf("create smartblock error: %v", err)
		}
		return objectId, nil
	})
}

func (s *service) createObject(ctx *session.Context, contextBlock smartblock.SmartBlock, groupId string, req pb.RpcBlockLinkCreateWithObjectRequest, storeLink bool, create func(context.Context) (objectId string, err error)) (linkId string, objectId string, err error) {
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

	b, ok := contextBlock.(basic.Basic)
	if !ok {
		err = fmt.Errorf("%T doesn't implement basic.Basic", contextBlock)
		return
	}
	linkId, err = b.Create(ctx, groupId, pb.RpcBlockCreateRequest{
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
	return
}

func (s *service) Process() process.Service {
	return s.process
}

func (s *service) ProcessAdd(p process.Process) (err error) {
	return s.process.Add(p)
}

func (s *service) ProcessCancel(id string) (err error) {
	return s.process.Cancel(id)
}

func (s *service) Close() error {
	return s.cache.Close()
}

// pickBlock returns opened smartBlock or opens smartBlock in silent mode
func (s *service) pickBlock(ctx context.Context, id string) (sb smartblock.SmartBlock, release func(), err error) {
	ob, err := s.getSmartblock(ctx, id)
	if err != nil {
		return
	}
	return ob.SmartBlock, func() {
		s.cache.Release(id)
	}, nil
}

func (s *service) newSmartBlock(id string, initCtx *smartblock.InitContext) (sb smartblock.SmartBlock, err error) {
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
		sb = editor.NewSet(s)
	case model.SmartBlockType_ProfilePage, model.SmartBlockType_AnytypeProfile:
		sb = editor.NewProfile(s, s, s.bookmark, s.sendEvent)
	case model.SmartBlockType_STObjectType,
		model.SmartBlockType_BundledObjectType:
		sb = editor.NewObjectType(s)
	case model.SmartBlockType_BundledRelation,
		model.SmartBlockType_IndexedRelation:
		sb = editor.NewRelation(s)
	case model.SmartBlockType_File:
		sb = editor.NewFiles()
	case model.SmartBlockType_MarketplaceType:
		sb = editor.NewMarketplaceType(s)
	case model.SmartBlockType_MarketplaceRelation:
		sb = editor.NewMarketplaceRelation(s)
	case model.SmartBlockType_MarketplaceTemplate:
		sb = editor.NewMarketplaceTemplate(s)
	case model.SmartBlockType_Template:
		sb = editor.NewTemplate(s, s, s, s.bookmark)
	case model.SmartBlockType_BundledTemplate:
		sb = editor.NewTemplate(s, s, s, s.bookmark)
	case model.SmartBlockType_Breadcrumbs:
		sb = editor.NewBreadcrumbs()
	case model.SmartBlockType_Workspace:
		sb = editor.NewWorkspace(s, s)
	case model.SmartBlockType_AccountOld:
		sb = editor.NewThreadDB(s)
	default:
		return nil, fmt.Errorf("unexpected smartblock type: %v", sc.Type())
	}

	sb.Lock()
	defer sb.Unlock()
	if initCtx == nil {
		initCtx = &smartblock.InitContext{}
	}
	if initCtx.Restriction == nil {
		initCtx.Restriction = s.restriction
	}
	if initCtx.ObjectStore == nil {
		initCtx.ObjectStore = s.objectStore
	}
	if initCtx.Doc == nil {
		initCtx.Doc = s.doc
	}
	initCtx.Source = sc
	err = sb.Init(initCtx)
	return
}

func (s *service) stateFromTemplate(templateId, name string) (st *state.State, err error) {
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

func (s *service) MigrateMany(objects []threads.ThreadInfo) (migrated int, err error) {
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

func (s *service) DoBasic(id string, apply func(b basic.Basic) error) error {
	sb, release, err := s.pickBlock(context.WithValue(context.TODO(), metrics.CtxKeyRequest, "do_basic"), id)
	if err != nil {
		return err
	}
	defer release()
	if bb, ok := sb.(basic.Basic); ok {
		sb.Lock()
		defer sb.Unlock()
		return apply(bb)
	}
	return fmt.Errorf("basic operation not available for this block type: %T", sb)
}

func (s *service) DoTable(id string, ctx *session.Context, apply func(st *state.State, b table.Editor) error) error {
	sb, release, err := s.pickBlock(context.TODO(), id)
	if err != nil {
		return err
	}
	defer release()
	if bb, ok := sb.(table.Editor); ok {
		sb.Lock()
		defer sb.Unlock()

		st := sb.NewStateCtx(ctx)
		if err := apply(st, bb); err != nil {
			return fmt.Errorf("apply function: %w", err)
		}
		return sb.Apply(st)
	}
	return fmt.Errorf("table operation not available for this block type: %T", sb)
}

func (s *service) DoLinksCollection(id string, apply func(b basic.Basic) error) error {
	sb, release, err := s.pickBlock(context.WithValue(context.TODO(), metrics.CtxKeyRequest, "do_links_collection"), id)
	if err != nil {
		return err
	}
	defer release()
	if bb, ok := sb.(basic.Basic); ok {
		sb.Lock()
		defer sb.Unlock()
		return apply(bb)
	}
	return fmt.Errorf("basic operation not available for this block type: %T", sb)
}

func (s *service) DoClipboard(id string, apply func(b clipboard.Clipboard) error) error {
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

func (s *service) DoText(id string, apply func(b stext.Text) error) error {
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

func (s *service) DoFile(id string, apply func(b file.File) error) error {
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

func (s *service) DoBookmark(id string, apply func(b bookmark.Bookmark) error) error {
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

func (s *service) DoFileNonLock(id string, apply func(b file.File) error) error {
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

func (s *service) DoHistory(id string, apply func(b basic.IHistory) error) error {
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

func (s *service) DoImport(id string, apply func(b _import.Import) error) error {
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

func (s *service) DoDataview(id string, apply func(b dataview.Dataview) error) error {
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

func (s *service) Do(id string, apply func(b smartblock.SmartBlock) error) error {
	sb, release, err := s.pickBlock(context.WithValue(context.TODO(), metrics.CtxKeyRequest, "do"), id)
	if err != nil {
		return err
	}
	defer release()
	sb.Lock()
	defer sb.Unlock()
	return apply(sb)
}

func (s *service) DoWithContext(ctx context.Context, id string, apply func(b smartblock.SmartBlock) error) error {
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

func (s *service) TemplateCreateFromObject(id string) (templateId string, err error) {
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

func (s *service) TemplateCreateFromObjectByObjectType(otId string) (templateId string, err error) {
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

func (s *service) TemplateClone(id string) (templateId string, err error) {
	var st *state.State
	if err = s.Do(id, func(b smartblock.SmartBlock) error {
		if b.Type() != model.SmartBlockType_BundledTemplate {
			return fmt.Errorf("can clone bundled templates only")
		}
		st = b.NewState().Copy()
		st.RemoveDetail(bundle.RelationKeyTemplateIsBundled.String())
		st.SetLocalDetails(nil)
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

func (s *service) ObjectDuplicate(id string) (objectId string, err error) {
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

func (s *service) ObjectApplyTemplate(contextId, templateId string) error {
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

func (s *service) ResetToState(pageId string, state *state.State) (err error) {
	return s.Do(pageId, func(sb smartblock.SmartBlock) error {
		return sb.ResetToVersion(state)
	})
}

func (s *service) fetchBookmarkContent(url string) bookmarksvc.ContentFuture {
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
func (s *service) ObjectCreateBookmark(req pb.RpcObjectCreateBookmarkRequest) (id string, err error) {
	url, err := uri.ProcessURI(req.Url)
	if err != nil {
		return "", fmt.Errorf("process uri: %w", err)
	}
	res := s.fetchBookmarkContent(url)
	return s.bookmark.CreateBookmarkObject(url, res)
}

func (s *service) ObjectBookmarkFetch(req pb.RpcObjectBookmarkFetchRequest) (err error) {
	url, err := uri.ProcessURI(req.Url)
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

func (s *service) ObjectToBookmark(id string, url string) (objectId string, err error) {
	objectId, err = s.ObjectCreateBookmark(pb.RpcObjectCreateBookmarkRequest{
		Url: url,
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

func (s *service) loadSmartblock(ctx context.Context, id string) (value ocache.Object, err error) {
	sb, err := s.newSmartBlock(id, &smartblock.InitContext{Ctx: ctx})
	if err != nil {
		return
	}
	value = newOpenedBlock(sb)
	return
}

func (s *service) getSmartblock(ctx context.Context, id string) (ob *openedBlock, err error) {
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

func (s *service) replaceLink(id, oldId, newId string) error {
	return s.DoBasic(id, func(b basic.Basic) error {
		return b.ReplaceLink(oldId, newId)
	})
}
