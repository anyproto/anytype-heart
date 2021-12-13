package block

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/gogo/protobuf/proto"
	"net/url"
	"strings"
	"time"

	"github.com/anytypeio/go-anytype-middleware/app"
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
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/bookmark"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/file"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/link"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/core/event"
	"github.com/anytypeio/go-anytype-middleware/core/status"
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
	"github.com/gogo/protobuf/types"
	"github.com/textileio/go-threads/core/thread"
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

	OpenBlock(ctx *state.Context, id string) error
	ShowBlock(ctx *state.Context, id string) error
	OpenBreadcrumbsBlock(ctx *state.Context) (blockId string, err error)
	SetBreadcrumbs(ctx *state.Context, req pb.RpcBlockSetBreadcrumbsRequest) (err error)
	CloseBlock(id string) error
	CloseBlocks()
	CreateBlock(ctx *state.Context, req pb.RpcBlockCreateRequest) (string, error)
	CreatePage(ctx *state.Context, groupId string, req pb.RpcBlockCreatePageRequest) (linkId string, pageId string, err error)
	CreateSmartBlock(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, relations []*model.Relation) (id string, newDetails *types.Struct, err error)
	CreateSmartBlockFromTemplate(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, relations []*model.Relation, templateId string) (id string, newDetails *types.Struct, err error)
	CreateSmartBlockFromState(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, relations []*model.Relation, createState *state.State) (id string, newDetails *types.Struct, err error)
	DuplicateBlocks(ctx *state.Context, req pb.RpcBlockListDuplicateRequest) ([]string, error)
	UnlinkBlock(ctx *state.Context, req pb.RpcBlockUnlinkRequest) error
	ReplaceBlock(ctx *state.Context, req pb.RpcBlockReplaceRequest) (newId string, err error)
	ObjectToSet(id string, source []string) (newId string, err error)
	UpdateBlockContent(ctx *state.Context, req pb.RpcBlockUpdateContentRequest) (err error)

	MoveBlocks(ctx *state.Context, req pb.RpcBlockListMoveRequest) error
	MoveBlocksToNewPage(ctx *state.Context, req pb.RpcBlockListMoveToNewPageRequest) (linkId string, err error)
	ConvertChildrenToPages(req pb.RpcBlockListConvertChildrenToPagesRequest) (linkIds []string, err error)
	SetFields(ctx *state.Context, req pb.RpcBlockSetFieldsRequest) error
	SetFieldsList(ctx *state.Context, req pb.RpcBlockListSetFieldsRequest) error

	SetDetails(ctx *state.Context, req pb.RpcBlockSetDetailsRequest) (err error)
	ModifyDetails(objectId string, modifier func(current *types.Struct) (*types.Struct, error)) (err error) // you must copy original struct within the modifier in order to modify it

	GetRelations(objectId string) (relations []*model.Relation, err error)
	UpdateExtraRelations(ctx *state.Context, id string, relations []*model.Relation, createIfMissing bool) (err error)
	ModifyExtraRelations(ctx *state.Context, objectId string, modifier func(current []*model.Relation) ([]*model.Relation, error)) (err error)
	AddExtraRelations(ctx *state.Context, id string, relations []*model.Relation) (relationsWithKeys []*model.Relation, err error)
	RemoveExtraRelations(ctx *state.Context, id string, relationKeys []string) (err error)
	CreateSet(ctx *state.Context, req pb.RpcBlockCreateSetRequest) (linkId string, setId string, err error)
	SetDataviewSource(ctx *state.Context, contextId, blockId string, source []string) error

	ListAvailableRelations(objectId string) (aggregatedRelations []*model.Relation, err error)
	SetObjectTypes(ctx *state.Context, objectId string, objectTypes []string) (err error)
	AddExtraRelationOption(ctx *state.Context, req pb.RpcObjectRelationOptionAddRequest) (opt *model.RelationOption, err error)
	UpdateExtraRelationOption(ctx *state.Context, req pb.RpcObjectRelationOptionUpdateRequest) (err error)
	DeleteExtraRelationOption(ctx *state.Context, req pb.RpcObjectRelationOptionDeleteRequest) (err error)

	Paste(ctx *state.Context, req pb.RpcBlockPasteRequest, groupId string) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, isSameBlockCaret bool, err error)

	Copy(req pb.RpcBlockCopyRequest) (textSlot string, htmlSlot string, anySlot []*model.Block, err error)
	Cut(ctx *state.Context, req pb.RpcBlockCutRequest) (textSlot string, htmlSlot string, anySlot []*model.Block, err error)
	Export(req pb.RpcBlockExportRequest) (path string, err error)
	ImportMarkdown(ctx *state.Context, req pb.RpcBlockImportMarkdownRequest) (rootLinkIds []string, err error)

	SplitBlock(ctx *state.Context, req pb.RpcBlockSplitRequest) (blockId string, err error)
	MergeBlock(ctx *state.Context, req pb.RpcBlockMergeRequest) error
	SetLatexText(ctx *state.Context, req pb.RpcBlockSetLatexTextRequest) error
	SetTextText(ctx *state.Context, req pb.RpcBlockSetTextTextRequest) error
	SetTextStyle(ctx *state.Context, contextId string, style model.BlockContentTextStyle, blockIds ...string) error
	SetTextChecked(ctx *state.Context, req pb.RpcBlockSetTextCheckedRequest) error
	SetTextColor(ctx *state.Context, contextId string, color string, blockIds ...string) error
	SetTextMark(ctx *state.Context, id string, mark *model.BlockContentTextMark, ids ...string) error
	SetBackgroundColor(ctx *state.Context, contextId string, color string, blockIds ...string) error
	SetAlign(ctx *state.Context, contextId string, align model.BlockAlign, blockIds ...string) (err error)
	SetLayout(ctx *state.Context, id string, layout model.ObjectTypeLayout) error

	FeaturedRelationAdd(ctx *state.Context, contextId string, relations ...string) error
	FeaturedRelationRemove(ctx *state.Context, contextId string, relations ...string) error

	TurnInto(ctx *state.Context, id string, style model.BlockContentTextStyle, ids ...string) error

	SetDivStyle(ctx *state.Context, contextId string, style model.BlockContentDivStyle, ids ...string) (err error)

	SetFileStyle(ctx *state.Context, contextId string, style model.BlockContentFileStyle, blockIds ...string) error
	UploadFile(req pb.RpcUploadFileRequest) (hash string, err error)
	UploadBlockFile(ctx *state.Context, req pb.RpcBlockUploadRequest, groupId string) error
	UploadBlockFileSync(ctx *state.Context, req pb.RpcBlockUploadRequest) (err error)
	CreateAndUploadFile(ctx *state.Context, req pb.RpcBlockFileCreateAndUploadRequest) (id string, err error)
	DropFiles(req pb.RpcExternalDropFilesRequest) (err error)

	Undo(ctx *state.Context, req pb.RpcBlockUndoRequest) (pb.RpcBlockUndoRedoCounter, error)
	Redo(ctx *state.Context, req pb.RpcBlockRedoRequest) (pb.RpcBlockUndoRedoCounter, error)

	SetPagesIsArchived(req pb.RpcObjectListSetIsArchivedRequest) error
	SetPagesIsFavorite(req pb.RpcObjectListSetIsFavoriteRequest) error
	SetPageIsArchived(req pb.RpcObjectSetIsArchivedRequest) error
	SetPageIsFavorite(req pb.RpcObjectSetIsFavoriteRequest) error

	DeleteArchivedObjects(req pb.RpcObjectListDeleteRequest) error
	DeleteObject(id string) error

	GetAggregatedRelations(req pb.RpcBlockDataviewRelationListAvailableRequest) (relations []*model.Relation, err error)
	DeleteDataviewView(ctx *state.Context, req pb.RpcBlockDataviewViewDeleteRequest) error
	UpdateDataviewView(ctx *state.Context, req pb.RpcBlockDataviewViewUpdateRequest) error
	SetDataviewActiveView(ctx *state.Context, req pb.RpcBlockDataviewViewSetActiveRequest) error
	SetDataviewViewPosition(ctx *state.Context, request pb.RpcBlockDataviewViewSetPositionRequest) error
	CreateDataviewView(ctx *state.Context, req pb.RpcBlockDataviewViewCreateRequest) (id string, err error)
	AddDataviewRelation(ctx *state.Context, req pb.RpcBlockDataviewRelationAddRequest) (relation *model.Relation, err error)
	UpdateDataviewRelation(ctx *state.Context, req pb.RpcBlockDataviewRelationUpdateRequest) error
	DeleteDataviewRelation(ctx *state.Context, req pb.RpcBlockDataviewRelationDeleteRequest) error
	AddDataviewRecordRelationOption(ctx *state.Context, req pb.RpcBlockDataviewRecordRelationOptionAddRequest) (opt *model.RelationOption, err error)
	UpdateDataviewRecordRelationOption(ctx *state.Context, req pb.RpcBlockDataviewRecordRelationOptionUpdateRequest) error
	DeleteDataviewRecordRelationOption(ctx *state.Context, req pb.RpcBlockDataviewRecordRelationOptionDeleteRequest) error

	CreateDataviewRecord(ctx *state.Context, req pb.RpcBlockDataviewRecordCreateRequest) (*types.Struct, error)
	UpdateDataviewRecord(ctx *state.Context, req pb.RpcBlockDataviewRecordUpdateRequest) error
	DeleteDataviewRecord(ctx *state.Context, req pb.RpcBlockDataviewRecordDeleteRequest) error

	BookmarkFetch(ctx *state.Context, req pb.RpcBlockBookmarkFetchRequest) error
	BookmarkFetchSync(ctx *state.Context, req pb.RpcBlockBookmarkFetchRequest) (err error)
	BookmarkCreateAndFetch(ctx *state.Context, req pb.RpcBlockBookmarkCreateAndFetchRequest) (id string, err error)

	SetRelationKey(ctx *state.Context, request pb.RpcBlockRelationSetKeyRequest) error
	AddRelationBlock(ctx *state.Context, request pb.RpcBlockRelationAddRequest) error

	Process() process.Service
	ProcessAdd(p process.Process) (err error)
	ProcessCancel(id string) error

	SimplePaste(contextId string, anySlot []*model.Block) (err error)

	GetDocInfo(ctx context.Context, id string) (info doc.DocInfo, err error)
	Wakeup(id string) (err error)

	MakeTemplate(id string) (templateId string, err error)
	MakeTemplateByObjectType(otId string) (templateId string, err error)
	CloneTemplate(id string) (templateId string, err error)
	ApplyTemplate(contextId, templateId string) error

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
	s.app = a
	s.cache = ocache.New(s.loadSmartblock)
	return
}

func (s *service) Run() (err error) {
	s.initPredefinedBlocks()
	return
}

func (s *service) initPredefinedBlocks() {
	ids := []string{
		s.anytype.PredefinedBlocks().Account,
		s.anytype.PredefinedBlocks().AccountOld,
		s.anytype.PredefinedBlocks().Profile,
		s.anytype.PredefinedBlocks().Archive,
		s.anytype.PredefinedBlocks().Home,
		s.anytype.PredefinedBlocks().SetPages,
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
		ctx := &smartblock.InitContext{State: state.NewDoc(id, nil).(*state.State)}
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
	metrics.SharedClient.RecordEvent(metrics.InitPredefinedBlocks{
		TimeMs: time.Now().Sub(startTime).Milliseconds()})
}

func (s *service) Anytype() core.Service {
	return s.anytype
}

func (s *service) OpenBlock(ctx *state.Context, id string) (err error) {
	startTime := time.Now()
	ob, err := s.getSmartblock(context.TODO(), id)
	if err != nil {
		return
	}
	afterSmartBlockTime := time.Now()
	defer s.cache.Release(id)
	ob.Lock()
	defer ob.Unlock()
	ob.SetEventFunc(s.sendEvent)
	if v, hasOpenListner := ob.SmartBlock.(smartblock.SmartblockOpenListner); hasOpenListner {
		v.SmartblockOpened(ctx)
	}
	afterDataviewTime := time.Now()
	st := ob.NewState()
	st.SetLocalDetail(bundle.RelationKeyLastOpenedDate.String(), pbtypes.Int64(time.Now().Unix()))
	if err = ob.Apply(st, smartblock.NoHistory); err != nil {
		log.Errorf("failed to update lastOpenedDate: %s", err.Error())
	}
	afterApplyTime := time.Now()
	if err = ob.Show(ctx); err != nil {
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
	return nil
}

func (s *service) ShowBlock(ctx *state.Context, id string) (err error) {
	return s.Do(id, func(b smartblock.SmartBlock) error {
		return b.Show(ctx)
	})
}

func (s *service) OpenBreadcrumbsBlock(ctx *state.Context) (blockId string, err error) {
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
	if err = bs.Show(ctx); err != nil {
		return
	}
	return bs.Id(), nil
}

func (s *service) CloseBlock(id string) error {
	var (
		isDraft     bool
		workspaceId string
	)
	err := s.Do(id, func(b smartblock.SmartBlock) error {
		b.BlockClose()
		s := b.NewState()
		isDraft = pbtypes.GetBool(s.LocalDetails(), bundle.RelationKeyIsDraft.String())
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
		ob.BlockClose()
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
		for _, blockId := range req.ObjectIds {
			if restrErr := s.checkArchivedRestriction(req.IsArchived, blockId); restrErr != nil {
				err = restrErr
			} else {
				if req.IsArchived {
					err = archive.AddObject(blockId)
				} else {
					err = archive.RemoveObject(blockId)
				}
			}
			if err != nil {
				log.Errorf("failed to archive %s: %s", blockId, err.Error())
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
		for _, blockId := range req.ObjectIds {
			if req.IsFavorite {
				err = fav.AddObject(blockId)
			} else {
				err = fav.RemoveObject(blockId)
			}
			if err != nil {
				log.Errorf("failed to favorite object %s: %s", blockId, err.Error())
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
		b.BlockClose()
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

	objType, err := objectstore.GetObjectType(s.anytype.ObjectStore(), objectTypes[0])
	if err != nil {
		return "", nil, fmt.Errorf("object type not found")
	}

	var workspaceId string
	if details != nil && details.Fields != nil {
		var isDraft = pbtypes.GetBool(details, bundle.RelationKeyIsDraft.String())
		delete(details.Fields, bundle.RelationKeyIsDraft.String())

		for k, v := range details.Fields {
			createState.SetDetail(k, v)
			if !createState.HasRelation(k) && !pbtypes.HasRelation(relations, k) {
				rel := pbtypes.GetRelation(objType.Relations, k)
				if rel == nil {
					return "", nil, fmt.Errorf("relation for detail %s not found", k)
				}
				relCopy := pbtypes.CopyRelation(rel)
				relCopy.Scope = model.Relation_object
				relations = append(relations, relCopy)
			}
		}

		if isDraft {
			createState.SetDetailAndBundledRelation(bundle.RelationKeyIsDraft, pbtypes.Bool(true))
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
	csm, err := s.CreateObjectInWorkspace(ctx, workspaceId, sbType)
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

func (s *service) CreatePage(ctx *state.Context, groupId string, req pb.RpcBlockCreatePageRequest) (linkId string, pageId string, err error) {
	if req.ContextId != "" {
		var contextBlockType model.SmartBlockType
		if err = s.Do(req.ContextId, func(b smartblock.SmartBlock) error {
			contextBlockType = b.Type()
			return nil
		}); err != nil {
			return
		}

		if contextBlockType == model.SmartBlockType_Set {
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

	pageId, _, err = s.CreateSmartBlockFromTemplate(context.TODO(), coresb.SmartBlockTypePage, req.Details, nil, req.TemplateId)
	if err != nil {
		err = fmt.Errorf("create smartblock error: %v", err)
	}

	if req.ContextId == "" && req.TargetId == "" {
		// do not create a link
		return "", pageId, err
	}

	err = s.DoBasic(req.ContextId, func(b basic.Basic) error {
		linkId, err = b.Create(ctx, groupId, pb.RpcBlockCreateRequest{
			TargetId: req.TargetId,
			Block: &model.Block{
				Content: &model.BlockContentOfLink{
					Link: &model.BlockContentLink{
						TargetBlockId: pageId,
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
		return err
	})
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
		sb = editor.NewPage(s, s, s, s.linkPreview)
	case model.SmartBlockType_Archive:
		sb = editor.NewArchive(s)
	case model.SmartBlockType_Home:
		sb = editor.NewDashboard(s, s)
	case model.SmartBlockType_Set:
		sb = editor.NewSet(s)
	case model.SmartBlockType_ProfilePage, model.SmartBlockType_AnytypeProfile:
		sb = editor.NewProfile(s, s, s.linkPreview, s.sendEvent)
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
		sb = editor.NewTemplate(s, s, s, s.linkPreview)
	case model.SmartBlockType_BundledTemplate:
		sb = editor.NewTemplate(s, s, s, s.linkPreview)
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
	err = s.Do(s.anytype.PredefinedBlocks().Account, func(b smartblock.SmartBlock) error {
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
	sb, release, err := s.pickBlock(context.TODO(), id)
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

func (s *service) DoLinksCollection(id string, apply func(b basic.Basic) error) error {
	sb, release, err := s.pickBlock(context.TODO(), id)
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
	sb, release, err := s.pickBlock(context.TODO(), id)
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
	sb, release, err := s.pickBlock(context.TODO(), id)
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
	sb, release, err := s.pickBlock(context.TODO(), id)
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
	sb, release, err := s.pickBlock(context.TODO(), id)
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
	sb, release, err := s.pickBlock(context.TODO(), id)
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
	sb, release, err := s.pickBlock(context.TODO(), id)
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
	sb, release, err := s.pickBlock(context.TODO(), id)
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
	sb, release, err := s.pickBlock(context.TODO(), id)
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
	sb, release, err := s.pickBlock(context.TODO(), id)
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

func (s *service) MakeTemplate(id string) (templateId string, err error) {
	var st *state.State
	if err = s.Do(id, func(b smartblock.SmartBlock) error {
		if b.Type() != model.SmartBlockType_Page {
			return fmt.Errorf("can't make template from this obect type")
		}
		st, err = b.MakeTemplateState()
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

func (s *service) MakeTemplateByObjectType(otId string) (templateId string, err error) {
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

func (s *service) CloneTemplate(id string) (templateId string, err error) {
	var st *state.State
	if err = s.Do(id, func(b smartblock.SmartBlock) error {
		if b.Type() != model.SmartBlockType_BundledTemplate {
			return fmt.Errorf("can clone bundled templates only")
		}
		st = b.NewState().Copy()
		st.RemoveDetail(bundle.RelationKeyTemplateIsBundled.String())
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

func (s *service) ApplyTemplate(contextId, templateId string) error {
	return s.Do(contextId, func(b smartblock.SmartBlock) error {
		orig := b.NewState().ParentState()
		ts, err := s.stateFromTemplate(templateId, pbtypes.GetString(orig.Details(), bundle.RelationKeyName.String()))
		if err != nil {
			return err
		}
		ts.SetRootId(contextId)
		ts.SetParent(orig)
		ts.BlocksInit(orig)
		ts.InjectDerivedDetails()
		return b.Apply(ts, smartblock.NoRestrictions)
	})
}

func (s *service) AllDescendantIds(rootBlockId string, allBlocks map[string]*model.Block) []string {
	var (
		// traversal queue
		queue = []string{rootBlockId}
		// traversed IDs collected (including root)
		traversed = []string{rootBlockId}
	)

	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]

		chIDs := allBlocks[next].ChildrenIds
		traversed = append(traversed, chIDs...)
		queue = append(queue, chIDs...)
	}

	return traversed
}

func (s *service) ResetToState(pageId string, state *state.State) (err error) {
	return s.Do(pageId, func(sb smartblock.SmartBlock) error {
		return sb.ResetToVersion(state)
	})
}

func (s *service) loadSmartblock(ctx context.Context, id string) (value ocache.Object, err error) {
	sb, err := s.newSmartBlock(id, nil)
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
	ob = val.(*openedBlock)
	if err != nil {
		return nil, err
	}
	return ob, nil
}

func (s *service) replaceLink(id, oldId, newId string) error {
	return s.DoBasic(id, func(b basic.Basic) error {
		return b.ReplaceLink(oldId, newId)
	})
}
