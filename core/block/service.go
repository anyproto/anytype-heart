package block

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/clipboard"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/file"
	_import "github.com/anytypeio/go-anytype-middleware/core/block/editor/import"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/stext"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/bookmark"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/file"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/link"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/core/event"
	"github.com/anytypeio/go-anytype-middleware/core/indexer"
	"github.com/anytypeio/go-anytype-middleware/core/status"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/files"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/anytypeio/go-anytype-middleware/util/linkpreview"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"github.com/textileio/go-threads/core/thread"
)

const CName = "blockService"

var (
	ErrBlockNotFound       = errors.New("block not found")
	ErrBlockAlreadyOpen    = errors.New("block already open")
	ErrUnexpectedBlockType = errors.New("unexpected block type")
	ErrUnknownObjectType   = fmt.Errorf("unknown object type")
)

var log = logging.Logger("anytype-mw-service")

var (
	blockCacheTTL                      = time.Minute
	blockCleanupTimeout                = time.Second * 30
	defaultObjectTypePerSmartblockType = map[coresb.SmartBlockType]bundle.TypeKey{
		coresb.SmartBlockTypePage:        bundle.TypeKeyPage,
		coresb.SmartBlockTypeProfilePage: bundle.TypeKeyPage,
		coresb.SmartBlockTypeSet:         bundle.TypeKeySet,
		coresb.SmartBlockTypeObjectType:  bundle.TypeKeyObjectType,
		coresb.SmartBlockTypeHome:        bundle.TypeKeyDashboard,
	}
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

type Service interface {
	Do(id string, apply func(b smartblock.SmartBlock) error) error

	OpenBlock(ctx *state.Context, id string) error
	OpenBreadcrumbsBlock(ctx *state.Context) (blockId string, err error)
	SetBreadcrumbs(ctx *state.Context, req pb.RpcBlockSetBreadcrumbsRequest) (err error)
	CloseBlock(id string) error
	CreateBlock(ctx *state.Context, req pb.RpcBlockCreateRequest) (string, error)
	CreatePage(ctx *state.Context, groupId string, req pb.RpcBlockCreatePageRequest) (linkId string, pageId string, err error)
	CreateSmartBlock(sbType coresb.SmartBlockType, details *types.Struct, relations []*pbrelation.Relation) (id string, newDetails *types.Struct, err error)
	DuplicateBlocks(ctx *state.Context, req pb.RpcBlockListDuplicateRequest) ([]string, error)
	UnlinkBlock(ctx *state.Context, req pb.RpcBlockUnlinkRequest) error
	ReplaceBlock(ctx *state.Context, req pb.RpcBlockReplaceRequest) (newId string, err error)

	MoveBlocks(ctx *state.Context, req pb.RpcBlockListMoveRequest) error
	MoveBlocksToNewPage(ctx *state.Context, req pb.RpcBlockListMoveToNewPageRequest) (linkId string, err error)
	ConvertChildrenToPages(req pb.RpcBlockListConvertChildrenToPagesRequest) (linkIds []string, err error)
	SetFields(ctx *state.Context, req pb.RpcBlockSetFieldsRequest) error
	SetFieldsList(ctx *state.Context, req pb.RpcBlockListSetFieldsRequest) error

	SetDetails(ctx *state.Context, req pb.RpcBlockSetDetailsRequest) (err error)
	ModifyDetails(objectId string, modifier func(current *types.Struct) (*types.Struct, error)) (err error)

	GetRelations(objectId string) (relations []*pbrelation.Relation, err error)
	UpdateExtraRelations(ctx *state.Context, id string, relations []*pbrelation.Relation, createIfMissing bool) (err error)
	ModifyExtraRelations(ctx *state.Context, objectId string, modifier func(current []*pbrelation.Relation) ([]*pbrelation.Relation, error)) (err error)
	AddExtraRelations(ctx *state.Context, id string, relations []*pbrelation.Relation) (relationsWithKeys []*pbrelation.Relation, err error)
	RemoveExtraRelations(ctx *state.Context, id string, relationKeys []string) (err error)
	CreateSet(ctx *state.Context, req pb.RpcBlockCreateSetRequest) (linkId string, setId string, err error)

	ListAvailableRelations(objectId string) (aggregatedRelations []*pbrelation.Relation, err error)
	SetObjectTypes(ctx *state.Context, objectId string, objectTypes []string) (err error)
	AddExtraRelationOption(ctx *state.Context, req pb.RpcObjectRelationOptionAddRequest) (opt *pbrelation.RelationOption, err error)
	UpdateExtraRelationOption(ctx *state.Context, req pb.RpcObjectRelationOptionUpdateRequest) (err error)
	DeleteExtraRelationOption(ctx *state.Context, req pb.RpcObjectRelationOptionDeleteRequest) (err error)

	Paste(ctx *state.Context, req pb.RpcBlockPasteRequest, groupId string) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, isSameBlockCaret bool, err error)

	Copy(req pb.RpcBlockCopyRequest) (textSlot string, htmlSlot string, anySlot []*model.Block, err error)
	Cut(ctx *state.Context, req pb.RpcBlockCutRequest) (textSlot string, htmlSlot string, anySlot []*model.Block, err error)
	Export(req pb.RpcBlockExportRequest) (path string, err error)
	ImportMarkdown(ctx *state.Context, req pb.RpcBlockImportMarkdownRequest) (rootLinkIds []string, err error)

	SplitBlock(ctx *state.Context, req pb.RpcBlockSplitRequest) (blockId string, err error)
	MergeBlock(ctx *state.Context, req pb.RpcBlockMergeRequest) error
	SetTextText(ctx *state.Context, req pb.RpcBlockSetTextTextRequest) error
	SetTextStyle(ctx *state.Context, contextId string, style model.BlockContentTextStyle, blockIds ...string) error
	SetTextChecked(ctx *state.Context, req pb.RpcBlockSetTextCheckedRequest) error
	SetTextColor(ctx *state.Context, contextId string, color string, blockIds ...string) error
	SetTextMark(ctx *state.Context, id string, mark *model.BlockContentTextMark, ids ...string) error
	SetBackgroundColor(ctx *state.Context, contextId string, color string, blockIds ...string) error
	SetAlign(ctx *state.Context, contextId string, align model.BlockAlign, blockIds ...string) (err error)
	TurnInto(ctx *state.Context, id string, style model.BlockContentTextStyle, ids ...string) error

	SetDivStyle(ctx *state.Context, contextId string, style model.BlockContentDivStyle, ids ...string) (err error)

	UploadFile(req pb.RpcUploadFileRequest) (hash string, err error)
	UploadBlockFile(ctx *state.Context, req pb.RpcBlockUploadRequest, groupId string) error
	UploadBlockFileSync(ctx *state.Context, req pb.RpcBlockUploadRequest) (err error)
	CreateAndUploadFile(ctx *state.Context, req pb.RpcBlockFileCreateAndUploadRequest) (id string, err error)
	DropFiles(req pb.RpcExternalDropFilesRequest) (err error)

	Undo(ctx *state.Context, req pb.RpcBlockUndoRequest) (pb.RpcBlockUndoRedoCounter, error)
	Redo(ctx *state.Context, req pb.RpcBlockRedoRequest) (pb.RpcBlockUndoRedoCounter, error)

	SetPageIsArchived(req pb.RpcBlockSetPageIsArchivedRequest) error
	SetPagesIsArchived(req pb.RpcBlockListSetPageIsArchivedRequest) error
	DeletePages(req pb.RpcBlockListDeletePageRequest) error

	GetAggregatedRelations(req pb.RpcBlockDataviewRelationListAvailableRequest) (relations []*pbrelation.Relation, err error)
	DeleteDataviewView(ctx *state.Context, req pb.RpcBlockDataviewViewDeleteRequest) error
	UpdateDataviewView(ctx *state.Context, req pb.RpcBlockDataviewViewUpdateRequest) error
	SetDataviewActiveView(ctx *state.Context, req pb.RpcBlockDataviewViewSetActiveRequest) error
	CreateDataviewView(ctx *state.Context, req pb.RpcBlockDataviewViewCreateRequest) (id string, err error)
	AddDataviewRelation(ctx *state.Context, req pb.RpcBlockDataviewRelationAddRequest) (relation *pbrelation.Relation, err error)
	UpdateDataviewRelation(ctx *state.Context, req pb.RpcBlockDataviewRelationUpdateRequest) error
	DeleteDataviewRelation(ctx *state.Context, req pb.RpcBlockDataviewRelationDeleteRequest) error
	AddDataviewRecordRelationOption(ctx *state.Context, req pb.RpcBlockDataviewRecordRelationOptionAddRequest) (opt *pbrelation.RelationOption, err error)
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

	GetSearchInfo(id string) (info indexer.SearchInfo, err error)

	app.ComponentRunnable
}

func newOpenedBlock(sb smartblock.SmartBlock, setLastUsage bool) *openedBlock {
	var ob = openedBlock{SmartBlock: sb}
	if setLastUsage {
		ob.lastUsage = time.Now()
	}
	if sb.Type() != pb.SmartBlockType_Breadcrumbs {
		// decode and store corresponding threadID for appropriate block
		if tid, err := thread.Decode(sb.Id()); err != nil {
			log.Warnf("can't restore thread ID: %v", err)
		} else {
			ob.threadId = tid
		}
	}
	return &ob
}

type openedBlock struct {
	smartblock.SmartBlock
	threadId  thread.ID
	lastUsage time.Time
	locked    bool
	refs      int32
}

func New() Service {
	return new(service)
}

type service struct {
	anytype      core.Service
	meta         meta.Service
	status       status.Service
	sendEvent    func(event *pb.Event)
	openedBlocks map[string]*openedBlock
	closed       bool
	linkPreview  linkpreview.LinkPreview
	process      process.Service
	m            sync.Mutex
}

func (s *service) Name() string {
	return CName
}

func (s *service) Init(a *app.App) (err error) {
	s.anytype = a.MustComponent(core.CName).(core.Service)
	s.meta = a.MustComponent(meta.CName).(meta.Service)
	s.status = a.MustComponent(status.CName).(status.Service)
	s.linkPreview = a.MustComponent(linkpreview.CName).(linkpreview.LinkPreview)
	s.process = a.MustComponent(process.CName).(process.Service)
	s.openedBlocks = make(map[string]*openedBlock)
	s.sendEvent = a.MustComponent(event.CName).(event.Sender).Send
	return
}

func (s *service) Run() (err error) {
	s.initPredefinedBlocks()
	go s.cleanupTicker()
	return
}

func (s *service) initPredefinedBlocks() {
	ids := []string{
		// skip account because it is not a smartblock, it's a threadsDB
		s.anytype.PredefinedBlocks().Profile,
		s.anytype.PredefinedBlocks().Archive,
		s.anytype.PredefinedBlocks().Home,
		s.anytype.PredefinedBlocks().SetPages,
		s.anytype.PredefinedBlocks().MarketplaceType,
		s.anytype.PredefinedBlocks().MarketplaceRelation,
	}
	for _, id := range ids {
		sb, err := s.createSmartBlock(id, &smartblock.InitContext{State: &state.State{}})
		if err != nil {
			log.Errorf("can't init predefined block: %v", err)
		} else {
			sb.Close()
		}
	}
}

func (s *service) Anytype() core.Service {
	return s.anytype
}

func (s *service) OpenBlock(ctx *state.Context, id string) (err error) {
	s.m.Lock()
	ob, ok := s.openedBlocks[id]
	if !ok {
		sb, e := s.createSmartBlock(id, nil)
		if e != nil {
			s.m.Unlock()
			return e
		}
		ob = newOpenedBlock(sb, true)
		s.openedBlocks[id] = ob
	}
	s.m.Unlock()

	ob.Lock()
	defer ob.Unlock()
	ob.locked = true
	ob.SetEventFunc(s.sendEvent)
	if v, hasOpenListner := ob.SmartBlock.(smartblock.SmartblockOpenListner); hasOpenListner {
		v.SmartblockOpened(ctx)
	}

	if err = ob.Show(ctx); err != nil {
		return
	}

	if tid := ob.threadId; tid != thread.Undef && s.status != nil {
		var (
			bs    = ob.NewState()
			fList = func() []string {
				ob.Lock()
				ob.Relations()
				fs := bs.GetAllFileHashes(ob.FileRelationKeys())
				ob.Unlock()
				return fs
			}
		)

		if newWatcher := s.status.Watch(tid, fList); newWatcher {
			ob.AddHook(func() { s.status.Unwatch(tid) }, smartblock.HookOnClose)
		}
	}
	return nil
}

func (s *service) OpenBreadcrumbsBlock(ctx *state.Context) (blockId string, err error) {
	s.m.Lock()
	defer s.m.Unlock()
	bs := editor.NewBreadcrumbs(s.meta)
	if err = bs.Init(&smartblock.InitContext{
		Source: source.NewVirtual(s.anytype, pb.SmartBlockType_Breadcrumbs),
	}); err != nil {
		return
	}
	bs.Lock()
	defer bs.Unlock()
	bs.SetEventFunc(s.sendEvent)
	ob := newOpenedBlock(bs, true)
	ob.refs = 1
	s.openedBlocks[bs.Id()] = ob
	if err = bs.Show(ctx); err != nil {
		return
	}
	return bs.Id(), nil
}

func (s *service) CloseBlock(id string) error {
	s.m.Lock()
	ob, ok := s.openedBlocks[id]
	if !ok {
		s.m.Unlock()
		return ErrBlockNotFound
	}
	ob.locked = false
	s.m.Unlock()

	ob.Lock()
	defer ob.Unlock()
	ob.BlockClose()
	return nil
}

func (s *service) SetPagesIsArchived(req pb.RpcBlockListSetPageIsArchivedRequest) (err error) {
	return s.Do(s.anytype.PredefinedBlocks().Archive, func(b smartblock.SmartBlock) error {
		archive, ok := b.(*editor.Archive)
		if !ok {
			return fmt.Errorf("unexpected archive block type: %T", b)
		}

		anySucceed := false
		for _, blockId := range req.BlockIds {
			if req.IsArchived {
				err = archive.Archive(blockId)
			} else {
				err = archive.UnArchive(blockId)
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

func (s *service) SetPageIsArchived(req pb.RpcBlockSetPageIsArchivedRequest) (err error) {
	return s.Do(s.anytype.PredefinedBlocks().Archive, func(b smartblock.SmartBlock) error {
		archive, ok := b.(*editor.Archive)
		if !ok {
			return fmt.Errorf("unexpected archive block type: %T", b)
		}
		if req.IsArchived {
			return archive.Archive(req.BlockId)
		} else {
			return archive.UnArchive(req.BlockId)
		}
	})
}

func (s *service) DeletePages(req pb.RpcBlockListDeletePageRequest) (err error) {
	return s.Do(s.anytype.PredefinedBlocks().Archive, func(b smartblock.SmartBlock) error {
		archive, ok := b.(*editor.Archive)
		if !ok {
			return fmt.Errorf("unexpected archive block type: %T", b)
		}

		anySucceed := false
		for _, blockId := range req.BlockIds {
			err = archive.Delete(blockId)
			if err != nil {
				log.Errorf("failed to delete page %s: %s", blockId, err.Error())
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

func (s *service) DeletePage(id string) (err error) {
	err = s.CloseBlock(id)
	if err != nil && err != ErrBlockNotFound {
		return err
	}

	return s.anytype.DeleteBlock(id)
}

func (s *service) MarkArchived(id string, archived bool) (err error) {
	return s.Do(id, func(b smartblock.SmartBlock) error {
		return b.SetDetails(nil, []*pb.RpcBlockSetDetailsDetail{
			{
				Key:   "isArchived",
				Value: pbtypes.Bool(archived),
			},
		}, true)
	})
}

func (s *service) SetBreadcrumbs(ctx *state.Context, req pb.RpcBlockSetBreadcrumbsRequest) (err error) {
	return s.Do(req.BreadcrumbsId, func(b smartblock.SmartBlock) error {
		if breadcrumbs, ok := b.(*editor.Breadcrumbs); ok {
			return breadcrumbs.SetCrumbs(req.Ids)
		} else {
			return ErrUnexpectedBlockType
		}
	})
}

func (s *service) CreateBlock(ctx *state.Context, req pb.RpcBlockCreateRequest) (id string, err error) {
	err = s.DoBasic(req.ContextId, func(b basic.Basic) error {
		id, err = b.Create(ctx, "", req)
		return err
	})
	return
}

func (s *service) CreateSmartBlock(sbType coresb.SmartBlockType, details *types.Struct, relations []*pbrelation.Relation) (id string, newDetails *types.Struct, err error) {
	csm, err := s.anytype.CreateBlock(sbType)
	if err != nil {
		err = fmt.Errorf("anytype.CreateBlock error: %v", err)
		return
	}
	id = csm.ID()

	createState := state.NewDoc(id, nil).NewState()
	typesInDetails := pbtypes.GetStringList(details, bundle.RelationKeyType.String())
	if len(typesInDetails) == 0 {
		if ot, exists := defaultObjectTypePerSmartblockType[sbType]; exists {
			typesInDetails = []string{ot.URL()}
		} else {
			typesInDetails = []string{bundle.TypeKeyPage.URL()}
		}
	}
	initCtx := &smartblock.InitContext{
		State:          createState,
		ObjectTypeUrls: typesInDetails,
	}
	var sb smartblock.SmartBlock
	if sb, err = s.createSmartBlock(id, initCtx); err != nil {
		return id, nil, err
	}
	defer sb.Close()
	log.Debugf("created new smartBlock: %v, objectType: %v", id, sb.ObjectTypes())

	// todo: move into createSmartblock params to make a single tx
	if relations != nil {
		var setDetails []*pb.RpcBlockSetDetailsDetail
		for k, v := range details.Fields {
			setDetails = append(setDetails, &pb.RpcBlockSetDetailsDetail{
				Key:   k,
				Value: v,
			})
		}
		sb.Lock()
		if _, err = sb.AddExtraRelations(nil, relations); err != nil {
			sb.Unlock()
			return id, nil, fmt.Errorf("can't add relations to object: %v", err)
		}
		sb.Unlock()
	}

	if details != nil && details.Fields != nil {
		var setDetails []*pb.RpcBlockSetDetailsDetail
		for k, v := range details.Fields {
			setDetails = append(setDetails, &pb.RpcBlockSetDetailsDetail{
				Key:   k,
				Value: v,
			})
		}
		sb.Lock()
		if err = sb.SetDetails(nil, setDetails, true); err != nil {
			sb.Unlock()
			return id, nil, fmt.Errorf("can't set details to object: %v", err)
		}
		sb.Unlock()

	}

	return id, sb.Details(), nil
}

func (s *service) CreatePage(ctx *state.Context, groupId string, req pb.RpcBlockCreatePageRequest) (linkId string, pageId string, err error) {
	var contextBlockType pb.SmartBlockType
	err = s.Do(req.ContextId, func(b smartblock.SmartBlock) error {
		contextBlockType = b.Type()
		return nil
	})

	if contextBlockType == pb.SmartBlockType_Set {
		return "", "", basic.ErrNotSupported
	}

	pageId, _, err = s.CreateSmartBlock(coresb.SmartBlockTypePage, req.Details, nil)
	if err != nil {
		err = fmt.Errorf("create smartblock error: %v", err)
	}

	if req.ContextId == "" && req.TargetId == "" {
		// do not create a link
		return "", pageId, nil
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

func (s *service) DuplicateBlocks(ctx *state.Context, req pb.RpcBlockListDuplicateRequest) (newIds []string, err error) {
	err = s.DoBasic(req.ContextId, func(b basic.Basic) error {
		newIds, err = b.Duplicate(ctx, req)
		return err
	})
	return
}

func (s *service) UnlinkBlock(ctx *state.Context, req pb.RpcBlockUnlinkRequest) (err error) {
	return s.DoBasic(req.ContextId, func(b basic.Basic) error {
		return b.Unlink(ctx, req.BlockIds...)
	})
}

func (s *service) SetDivStyle(ctx *state.Context, contextId string, style model.BlockContentDivStyle, ids ...string) (err error) {
	return s.DoBasic(contextId, func(b basic.Basic) error {
		return b.SetDivStyle(ctx, style, ids...)
	})
}

func (s *service) SplitBlock(ctx *state.Context, req pb.RpcBlockSplitRequest) (blockId string, err error) {
	err = s.DoText(req.ContextId, func(b stext.Text) error {
		blockId, err = b.Split(ctx, req)
		return err
	})
	return
}

func (s *service) MergeBlock(ctx *state.Context, req pb.RpcBlockMergeRequest) (err error) {
	return s.DoText(req.ContextId, func(b stext.Text) error {
		return b.Merge(ctx, req.FirstBlockId, req.SecondBlockId)
	})
}

func (s *service) MoveBlocks(ctx *state.Context, req pb.RpcBlockListMoveRequest) (err error) {
	if req.ContextId == req.TargetContextId {
		return s.DoBasic(req.ContextId, func(b basic.Basic) error {
			return b.Move(ctx, req)
		})
	}
	return s.DoBasic(req.ContextId, func(b basic.Basic) error {
		return s.DoBasic(req.TargetContextId, func(tb basic.Basic) error {
			blocks, err := b.InternalCut(ctx, req)
			if err != nil {
				return err
			}
			return tb.InternalPaste(blocks)
		})
	})
}

func (s *service) TurnInto(ctx *state.Context, contextId string, style model.BlockContentTextStyle, ids ...string) error {
	return s.DoText(contextId, func(b stext.Text) error {
		return b.TurnInto(ctx, style, ids...)
	})
}

func (s *service) SimplePaste(contextId string, anySlot []*model.Block) (err error) {
	var blocks []simple.Block

	for _, b := range anySlot {
		blocks = append(blocks, simple.New(b))
	}

	return s.DoBasic(contextId, func(b basic.Basic) error {
		return b.InternalPaste(blocks)
	})
}

func (s *service) MoveBlocksToNewPage(ctx *state.Context, req pb.RpcBlockListMoveToNewPageRequest) (linkId string, err error) {
	// 1. Create new page, link
	linkId, pageId, err := s.CreatePage(ctx, "", pb.RpcBlockCreatePageRequest{
		ContextId: req.ContextId,
		TargetId:  req.DropTargetId,
		Position:  req.Position,
		Details:   req.Details,
	})

	if err != nil {
		return linkId, err
	}

	// 2. Move blocks to new page
	err = s.MoveBlocks(nil, pb.RpcBlockListMoveRequest{
		ContextId:       req.ContextId,
		BlockIds:        req.BlockIds,
		TargetContextId: pageId,
		DropTargetId:    "",
		Position:        0,
	})

	if err != nil {
		return linkId, err
	}

	return linkId, err
}

func (s *service) ConvertChildrenToPages(req pb.RpcBlockListConvertChildrenToPagesRequest) (linkIds []string, err error) {
	blocks := make(map[string]*model.Block)

	err = s.Do(req.ContextId, func(contextBlock smartblock.SmartBlock) error {
		for _, b := range contextBlock.Blocks() {
			blocks[b.Id] = b
		}
		return nil
	})

	if err != nil {
		return linkIds, err
	}

	for _, blockId := range req.BlockIds {
		if blocks[blockId] == nil || blocks[blockId].GetText() == nil {
			continue
		}

		fields := map[string]*types.Value{
			"name": pbtypes.String(blocks[blockId].GetText().Text),
		}

		if req.ObjectType != "" {
			fields["type"] = pbtypes.String(req.ObjectType)
		}

		children := s.AllDescendantIds(blockId, blocks)
		linkId, err := s.MoveBlocksToNewPage(nil, pb.RpcBlockListMoveToNewPageRequest{
			ContextId: req.ContextId,
			BlockIds:  children,
			Details: &types.Struct{
				Fields: fields,
			},
			DropTargetId: blockId,
			Position:     model.Block_Replace,
		})
		linkIds = append(linkIds, linkId)
		if err != nil {
			return linkIds, err
		}
	}

	return linkIds, err
}

func (s *service) ReplaceBlock(ctx *state.Context, req pb.RpcBlockReplaceRequest) (newId string, err error) {
	err = s.DoBasic(req.ContextId, func(b basic.Basic) error {
		newId, err = b.Replace(ctx, req.BlockId, req.Block)
		return err
	})
	return
}

func (s *service) SetFields(ctx *state.Context, req pb.RpcBlockSetFieldsRequest) (err error) {
	return s.DoBasic(req.ContextId, func(b basic.Basic) error {
		return b.SetFields(ctx, &pb.RpcBlockListSetFieldsRequestBlockField{
			BlockId: req.BlockId,
			Fields:  req.Fields,
		})
	})
}

func (s *service) SetDetails(ctx *state.Context, req pb.RpcBlockSetDetailsRequest) (err error) {
	return s.Do(req.ContextId, func(b smartblock.SmartBlock) error {
		return b.SetDetails(ctx, req.Details, false)
	})
}

func (s *service) SetFieldsList(ctx *state.Context, req pb.RpcBlockListSetFieldsRequest) (err error) {
	return s.DoBasic(req.ContextId, func(b basic.Basic) error {
		return b.SetFields(ctx, req.BlockFields...)
	})
}

func (s *service) GetAggregatedRelations(req pb.RpcBlockDataviewRelationListAvailableRequest) (relations []*pbrelation.Relation, err error) {
	err = s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		relations, err = b.GetAggregatedRelations(req.BlockId)
		return err
	})

	return
}

func (s *service) UpdateDataviewView(ctx *state.Context, req pb.RpcBlockDataviewViewUpdateRequest) error {
	return s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		return b.UpdateView(ctx, req.BlockId, req.ViewId, *req.View, true)
	})
}

func (s *service) DeleteDataviewView(ctx *state.Context, req pb.RpcBlockDataviewViewDeleteRequest) error {
	return s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		return b.DeleteView(ctx, req.BlockId, req.ViewId, true)
	})
}

func (s *service) SetDataviewActiveView(ctx *state.Context, req pb.RpcBlockDataviewViewSetActiveRequest) error {
	return s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		return b.SetActiveView(ctx, req.BlockId, req.ViewId, int(req.Limit), int(req.Offset))
	})
}

func (s *service) CreateDataviewView(ctx *state.Context, req pb.RpcBlockDataviewViewCreateRequest) (id string, err error) {
	err = s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		if req.View == nil {
			req.View = &model.BlockContentDataviewView{}
		}
		view, err := b.CreateView(ctx, req.BlockId, *req.View)
		id = view.Id
		return err
	})

	return
}

func (s *service) CreateDataviewRecord(ctx *state.Context, req pb.RpcBlockDataviewRecordCreateRequest) (rec *types.Struct, err error) {
	err = s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		cr, err := b.CreateRecord(ctx, req.BlockId, model.ObjectDetails{Details: req.Record})
		if err != nil {
			return err
		}
		rec = cr.Details
		return nil
	})

	return
}

func (s *service) UpdateDataviewRecord(ctx *state.Context, req pb.RpcBlockDataviewRecordUpdateRequest) (err error) {
	err = s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		return b.UpdateRecord(ctx, req.BlockId, req.RecordId, model.ObjectDetails{Details: req.Record})
	})

	return
}

func (s *service) DeleteDataviewRecord(ctx *state.Context, req pb.RpcBlockDataviewRecordDeleteRequest) (err error) {
	err = s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		return b.DeleteRecord(ctx, req.BlockId, req.RecordId)
	})

	return
}

func (s *service) UpdateDataviewRelation(ctx *state.Context, req pb.RpcBlockDataviewRelationUpdateRequest) error {
	return s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		return b.UpdateRelation(ctx, req.BlockId, req.RelationKey, *req.Relation, true)
	})
}

func (s *service) AddDataviewRelation(ctx *state.Context, req pb.RpcBlockDataviewRelationAddRequest) (relation *pbrelation.Relation, err error) {
	err = s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		var err error
		relation, err = b.AddRelation(ctx, req.BlockId, *req.Relation, true)
		if err != nil {
			return err
		}
		rels, err := b.GetDataviewRelations(req.BlockId)
		if err != nil {
			return err
		}

		relation = pbtypes.GetRelation(rels, relation.Key)
		if relation.Format == pbrelation.RelationFormat_status || relation.Format == pbrelation.RelationFormat_tag {
			err = b.FillAggregatedOptions(nil)
			if err != nil {
				log.Errorf("FillAggregatedOptions failed: %s", err.Error())
			}
		}
		return nil
	})

	return
}

func (s *service) DeleteDataviewRelation(ctx *state.Context, req pb.RpcBlockDataviewRelationDeleteRequest) error {
	return s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		return b.DeleteRelation(ctx, req.BlockId, req.RelationKey, true)
	})
}

func (s *service) AddDataviewRecordRelationOption(ctx *state.Context, req pb.RpcBlockDataviewRecordRelationOptionAddRequest) (opt *pbrelation.RelationOption, err error) {
	err = s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		opt, err = b.AddRelationOption(ctx, req.BlockId, req.RecordId, req.RelationKey, *req.Option, true)
		if err != nil {
			return err
		}
		return nil
	})

	return
}

func (s *service) UpdateDataviewRecordRelationOption(ctx *state.Context, req pb.RpcBlockDataviewRecordRelationOptionUpdateRequest) error {
	err := s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		err := b.UpdateRelationOption(ctx, req.BlockId, req.RecordId, req.RelationKey, *req.Option, true)
		if err != nil {
			return err
		}
		return nil
	})

	return err
}

func (s *service) DeleteDataviewRecordRelationOption(ctx *state.Context, req pb.RpcBlockDataviewRecordRelationOptionDeleteRequest) error {
	err := s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		err := b.DeleteRelationOption(ctx, req.BlockId, req.RecordId, req.RelationKey, req.OptionId, true)
		if err != nil {
			return err
		}
		return nil
	})

	return err
}

func (s *service) Copy(req pb.RpcBlockCopyRequest) (textSlot string, htmlSlot string, anySlot []*model.Block, err error) {
	err = s.DoClipboard(req.ContextId, func(cb clipboard.Clipboard) error {
		textSlot, htmlSlot, anySlot, err = cb.Copy(req)
		return err
	})

	return textSlot, htmlSlot, anySlot, err
}

func (s *service) Paste(ctx *state.Context, req pb.RpcBlockPasteRequest, groupId string) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, isSameBlockCaret bool, err error) {
	err = s.DoClipboard(req.ContextId, func(cb clipboard.Clipboard) error {
		blockIds, uploadArr, caretPosition, isSameBlockCaret, err = cb.Paste(ctx, req, groupId)
		return err
	})

	return blockIds, uploadArr, caretPosition, isSameBlockCaret, err
}

func (s *service) Cut(ctx *state.Context, req pb.RpcBlockCutRequest) (textSlot string, htmlSlot string, anySlot []*model.Block, err error) {
	err = s.DoClipboard(req.ContextId, func(cb clipboard.Clipboard) error {
		textSlot, htmlSlot, anySlot, err = cb.Cut(ctx, req)
		return err
	})
	return textSlot, htmlSlot, anySlot, err
}

func (s *service) Export(req pb.RpcBlockExportRequest) (path string, err error) {
	err = s.DoClipboard(req.ContextId, func(cb clipboard.Clipboard) error {
		path, err = cb.Export(req)
		return err
	})
	return path, err
}

func (s *service) ImportMarkdown(ctx *state.Context, req pb.RpcBlockImportMarkdownRequest) (rootLinkIds []string, err error) {
	var rootLinks []*model.Block
	err = s.DoImport(req.ContextId, func(imp _import.Import) error {
		rootLinks, err = imp.ImportMarkdown(ctx, req)
		return err
	})
	if err != nil {
		return rootLinkIds, err
	}

	if len(rootLinks) == 1 {
		err = s.SimplePaste(req.ContextId, rootLinks)

		if err != nil {
			return rootLinkIds, err
		}
	} else {
		_, pageId, err := s.CreatePage(ctx, "", pb.RpcBlockCreatePageRequest{
			ContextId: req.ContextId,
			Details: &types.Struct{Fields: map[string]*types.Value{
				"name":      pbtypes.String("Import from Notion"),
				"iconEmoji": pbtypes.String("📁"),
			}},
		})

		if err != nil {
			return rootLinkIds, err
		}

		err = s.SimplePaste(pageId, rootLinks)
	}

	for _, r := range rootLinks {
		rootLinkIds = append(rootLinkIds, r.Id)
	}

	return rootLinkIds, err
}

func (s *service) SetTextText(ctx *state.Context, req pb.RpcBlockSetTextTextRequest) error {
	return s.DoText(req.ContextId, func(b stext.Text) error {
		return b.SetText(req)
	})
}

func (s *service) SetTextStyle(ctx *state.Context, contextId string, style model.BlockContentTextStyle, blockIds ...string) error {
	return s.DoText(contextId, func(b stext.Text) error {
		return b.UpdateTextBlocks(ctx, blockIds, true, func(t text.Block) error {
			t.SetStyle(style)
			return nil
		})
	})
}

func (s *service) SetTextChecked(ctx *state.Context, req pb.RpcBlockSetTextCheckedRequest) error {
	return s.DoText(req.ContextId, func(b stext.Text) error {
		return b.UpdateTextBlocks(ctx, []string{req.BlockId}, true, func(t text.Block) error {
			t.SetChecked(req.Checked)
			return nil
		})
	})
}

func (s *service) SetTextColor(ctx *state.Context, contextId string, color string, blockIds ...string) error {
	return s.DoText(contextId, func(b stext.Text) error {
		return b.UpdateTextBlocks(ctx, blockIds, true, func(t text.Block) error {
			t.SetTextColor(color)
			return nil
		})
	})
}

func (s *service) SetTextMark(ctx *state.Context, contextId string, mark *model.BlockContentTextMark, blockIds ...string) error {
	return s.DoText(contextId, func(b stext.Text) error {
		return b.SetMark(ctx, mark, blockIds...)
	})
}

func (s *service) SetBackgroundColor(ctx *state.Context, contextId string, color string, blockIds ...string) (err error) {
	return s.DoBasic(contextId, func(b basic.Basic) error {
		return b.Update(ctx, func(b simple.Block) error {
			b.Model().BackgroundColor = color
			return nil
		}, blockIds...)
	})
}

func (s *service) SetAlign(ctx *state.Context, contextId string, align model.BlockAlign, blockIds ...string) (err error) {
	return s.DoBasic(contextId, func(b basic.Basic) error {
		return b.Update(ctx, func(b simple.Block) error {
			b.Model().Align = align
			return nil
		}, blockIds...)
	})
}

func (s *service) UploadBlockFile(ctx *state.Context, req pb.RpcBlockUploadRequest, groupId string) (err error) {
	return s.DoFile(req.ContextId, func(b file.File) error {
		err = b.Upload(ctx, req.BlockId, file.FileSource{
			Path:    req.FilePath,
			Url:     req.Url,
			GroupId: groupId,
		}, false)
		return err
	})
}

func (s *service) UploadBlockFileSync(ctx *state.Context, req pb.RpcBlockUploadRequest) (err error) {
	return s.DoFile(req.ContextId, func(b file.File) error {
		err = b.Upload(ctx, req.BlockId, file.FileSource{
			Path: req.FilePath,
			Url:  req.Url,
		}, true)
		return err
	})
}

func (s *service) CreateAndUploadFile(ctx *state.Context, req pb.RpcBlockFileCreateAndUploadRequest) (id string, err error) {
	err = s.DoFile(req.ContextId, func(b file.File) error {
		id, err = b.CreateAndUpload(ctx, req)
		return err
	})
	return
}

func (s *service) UploadFile(req pb.RpcUploadFileRequest) (hash string, err error) {
	upl := file.NewUploader(s)
	if req.DisableEncryption {
		upl.AddOptions(files.WithPlaintext(true))
	}
	if req.Type != model.BlockContentFile_None {
		upl.SetType(req.Type)
	} else {
		upl.AutoType(true)
	}
	res := upl.SetFile(req.LocalPath).Upload(context.TODO())
	if res.Err != nil {
		return "", res.Err
	}
	return res.Hash, nil
}

func (s *service) DropFiles(req pb.RpcExternalDropFilesRequest) (err error) {
	return s.DoFileNonLock(req.ContextId, func(b file.File) error {
		return b.DropFiles(req)
	})
}

func (s *service) Undo(ctx *state.Context, req pb.RpcBlockUndoRequest) (counters pb.RpcBlockUndoRedoCounter, err error) {
	err = s.DoHistory(req.ContextId, func(b basic.IHistory) error {
		counters, err = b.Undo(ctx)
		return err
	})
	return
}

func (s *service) Redo(ctx *state.Context, req pb.RpcBlockRedoRequest) (counters pb.RpcBlockUndoRedoCounter, err error) {
	err = s.DoHistory(req.ContextId, func(b basic.IHistory) error {
		counters, err = b.Redo(ctx)
		return err
	})
	return
}

func (s *service) BookmarkFetch(ctx *state.Context, req pb.RpcBlockBookmarkFetchRequest) (err error) {
	return s.DoBookmark(req.ContextId, func(b bookmark.Bookmark) error {
		return b.Fetch(ctx, req.BlockId, req.Url, false)
	})
}

func (s *service) BookmarkFetchSync(ctx *state.Context, req pb.RpcBlockBookmarkFetchRequest) (err error) {
	return s.DoBookmark(req.ContextId, func(b bookmark.Bookmark) error {
		return b.Fetch(ctx, req.BlockId, req.Url, true)
	})
}

func (s *service) BookmarkCreateAndFetch(ctx *state.Context, req pb.RpcBlockBookmarkCreateAndFetchRequest) (id string, err error) {
	err = s.DoBookmark(req.ContextId, func(b bookmark.Bookmark) error {
		id, err = b.CreateAndFetch(ctx, req)
		return err
	})
	return
}

func (s *service) SetRelationKey(ctx *state.Context, req pb.RpcBlockRelationSetKeyRequest) error {
	return s.Do(req.ContextId, func(b smartblock.SmartBlock) error {
		rels := b.Relations()
		rel := pbtypes.GetRelation(rels, req.Key)
		if rel == nil {
			var err error
			rels, err = s.Anytype().ObjectStore().ListRelations("")
			if err != nil {
				return err
			}
			rel = pbtypes.GetRelation(rels, req.Key)
			if rel == nil {
				return fmt.Errorf("relation with provided key not found")
			}
		}

		return b.(basic.Basic).AddRelationAndSet(ctx, pb.RpcBlockRelationAddRequest{Relation: rel, BlockId: req.BlockId, ContextId: req.ContextId})
	})
}

func (s *service) AddRelationBlock(ctx *state.Context, req pb.RpcBlockRelationAddRequest) error {
	return s.DoBasic(req.ContextId, func(b basic.Basic) error {
		return b.AddRelationAndSet(ctx, req)
	})
}

func (s *service) GetSearchInfo(id string) (info indexer.SearchInfo, err error) {
	if err = s.Do(id, func(b smartblock.SmartBlock) error {
		info, err = b.GetSearchInfo()
		return err
	}); err != nil {
		return
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
	if err := s.process.Close(); err != nil {
		log.Errorf("close error: %v", err)
	}
	s.m.Lock()
	if s.closed {
		s.m.Unlock()
		return nil
	}
	s.closed = true
	var blocks []*openedBlock
	for _, sb := range s.openedBlocks {
		blocks = append(blocks, sb)
	}
	s.m.Unlock()

	for _, sb := range blocks {
		if err := sb.Close(); err != nil {
			log.Errorf("block[%s] close error: %v", sb.Id(), err)
		}
	}
	log.Infof("block service closed")
	return nil
}

// pickBlock returns opened smartBlock or opens smartBlock in silent mode
func (s *service) pickBlock(id string) (sb smartblock.SmartBlock, release func(), err error) {
	s.m.Lock()
	defer s.m.Unlock()
	if s.closed {
		err = fmt.Errorf("block service closed")
		return
	}
	ob, ok := s.openedBlocks[id]
	if !ok {
		sb, err = s.createSmartBlock(id, nil)
		if err != nil {
			return
		}
		ob = newOpenedBlock(sb, false)
		s.openedBlocks[id] = ob
	}
	ob.refs++
	ob.lastUsage = time.Now()
	return ob.SmartBlock, func() {
		s.m.Lock()
		defer s.m.Unlock()
		ob.refs--
	}, nil
}

func (s *service) createSmartBlock(id string, initCtx *smartblock.InitContext) (sb smartblock.SmartBlock, err error) {
	sc, err := source.NewSource(s.anytype, s.status, id)
	if err != nil {
		return
	}
	switch sc.Type() {
	case pb.SmartBlockType_Page:
		sb = editor.NewPage(s.meta, s, s, s, s.linkPreview)
	case pb.SmartBlockType_Home:
		sb = editor.NewDashboard(s.meta, s)
	case pb.SmartBlockType_Archive:
		sb = editor.NewArchive(s.meta, s)
	case pb.SmartBlockType_Set:
		sb = editor.NewSet(s.meta, s)
	case pb.SmartBlockType_ProfilePage:
		sb = editor.NewProfile(s.meta, s, s, s.linkPreview, s.sendEvent)
	case pb.SmartBlockType_ObjectType:
		sb = editor.NewObjectType(s.meta, s)
	case pb.SmartBlockType_File:
		sb = editor.NewFiles(s.meta)
	case pb.SmartBlockType_MarketplaceType:
		sb = editor.NewMarketplaceType(s.meta, s)
	case pb.SmartBlockType_MarketplaceRelation:
		sb = editor.NewMarketplaceRelation(s.meta, s)
	default:
		return nil, fmt.Errorf("unexpected smartblock type: %v", sc.Type())
	}

	sb.Lock()
	defer sb.Unlock()
	if initCtx == nil {
		initCtx = &smartblock.InitContext{}
	}
	initCtx.Source = sc
	err = sb.Init(initCtx)
	return
}

func (s *service) cleanupTicker() {
	ticker := time.NewTicker(blockCleanupTimeout)
	defer ticker.Stop()
	for _ = range ticker.C {
		if s.cleanupBlocks() {
			return
		}
	}
}

func (s *service) DoBasic(id string, apply func(b basic.Basic) error) error {
	sb, release, err := s.pickBlock(id)
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
	sb, release, err := s.pickBlock(id)
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
	sb, release, err := s.pickBlock(id)
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
	sb, release, err := s.pickBlock(id)
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
	sb, release, err := s.pickBlock(id)
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
	sb, release, err := s.pickBlock(id)
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
	sb, release, err := s.pickBlock(id)
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
	sb, release, err := s.pickBlock(id)
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
	sb, release, err := s.pickBlock(id)
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
	sb, release, err := s.pickBlock(id)
	if err != nil {
		return err
	}
	defer release()
	sb.Lock()
	defer sb.Unlock()
	return apply(sb)
}

func (s *service) GetRelations(objectId string) (relations []*pbrelation.Relation, err error) {
	err = s.Do(objectId, func(b smartblock.SmartBlock) error {
		relations = b.Relations()
		return nil
	})
	return
}

// ModifyExtraRelations gets and updates extra relations under the sb lock to make sure no modifications are done in the middle
func (s *service) ModifyExtraRelations(ctx *state.Context, objectId string, modifier func(current []*pbrelation.Relation) ([]*pbrelation.Relation, error)) (err error) {
	if modifier == nil {
		return fmt.Errorf("modifier is nil")
	}
	return s.Do(objectId, func(b smartblock.SmartBlock) error {
		rels, err := modifier(b.Relations())
		if err != nil {
			return err
		}

		return b.UpdateExtraRelations(ctx, rels, true)
	})
}

// ModifyDetails performs details get and update under the sb lock to make sure no modifications are done in the middle
func (s *service) ModifyDetails(objectId string, modifier func(current *types.Struct) (*types.Struct, error)) (err error) {
	if modifier == nil {
		return fmt.Errorf("modifier is nil")
	}
	return s.Do(objectId, func(b smartblock.SmartBlock) error {
		dets, err := modifier(b.Details())
		if err != nil {
			return err
		}

		return b.Apply(b.NewState().SetDetails(dets))
	})
}

func (s *service) UpdateExtraRelations(ctx *state.Context, objectId string, relations []*pbrelation.Relation, createIfMissing bool) (err error) {
	return s.Do(objectId, func(b smartblock.SmartBlock) error {
		return b.UpdateExtraRelations(ctx, relations, createIfMissing)
	})
}

func (s *service) AddExtraRelations(ctx *state.Context, objectId string, relations []*pbrelation.Relation) (relationsWithKeys []*pbrelation.Relation, err error) {
	err = s.Do(objectId, func(b smartblock.SmartBlock) error {
		var err2 error
		relationsWithKeys, err2 = b.AddExtraRelations(ctx, relations)
		if err2 != nil {
			return err2
		}
		return nil
	})

	return
}

func (s *service) AddExtraRelationOption(ctx *state.Context, req pb.RpcObjectRelationOptionAddRequest) (opt *pbrelation.RelationOption, err error) {
	err = s.Do(req.ContextId, func(b smartblock.SmartBlock) error {
		opt, err = b.AddExtraRelationOption(ctx, req.RelationKey, *req.Option, true)
		if err != nil {
			return err
		}
		return nil
	})

	return
}

func (s *service) UpdateExtraRelationOption(ctx *state.Context, req pb.RpcObjectRelationOptionUpdateRequest) error {
	return s.Do(req.ContextId, func(b smartblock.SmartBlock) error {
		err := b.UpdateExtraRelationOption(ctx, req.RelationKey, *req.Option, true)
		if err != nil {
			return err
		}
		return nil
	})
}

func (s *service) DeleteExtraRelationOption(ctx *state.Context, req pb.RpcObjectRelationOptionDeleteRequest) error {
	return s.Do(req.ContextId, func(b smartblock.SmartBlock) error {
		err := b.DeleteExtraRelationOption(ctx, req.RelationKey, req.OptionId, true)
		if err != nil {
			return err
		}
		return nil
	})
}

func (s *service) SetObjectTypes(ctx *state.Context, objectId string, objectTypes []string) (err error) {
	return s.Do(objectId, func(b smartblock.SmartBlock) error {
		return b.SetObjectTypes(ctx, objectTypes)
	})
}

func (s *service) CreateSet(ctx *state.Context, req pb.RpcBlockCreateSetRequest) (linkId string, setId string, err error) {
	objType, err := localstore.GetObjectType(s.anytype.ObjectStore(), req.ObjectTypeUrl)
	if err != nil {
		return "", "", err
	}

	csm, err := s.anytype.CreateBlock(coresb.SmartBlockTypeSet)
	if err != nil {
		err = fmt.Errorf("anytype.CreateBlock error: %v", err)
		return
	}
	setId = csm.ID()

	sb, err := s.createSmartBlock(setId, &smartblock.InitContext{
		State: state.NewDoc(req.ObjectTypeUrl, nil).NewState(),
	})
	if err != nil {
		return "", "", err
	}
	set, ok := sb.(*editor.Set)
	if !ok {
		return "", setId, fmt.Errorf("unexpected set block type: %T", sb)
	}

	var relations []*model.BlockContentDataviewRelation
	for _, rel := range objType.Relations {
		visible := !rel.Hidden
		if rel.Key == bundle.RelationKeyType.String() {
			visible = false
		}
		relations = append(relations, &model.BlockContentDataviewRelation{Key: rel.Key, IsVisible: visible})
	}

	dataview := model.BlockContentOfDataview{
		Dataview: &model.BlockContentDataview{
			Relations: objType.Relations,
			Source:    objType.Url,
			Views: []*model.BlockContentDataviewView{
				{
					Id:   bson.NewObjectId().Hex(),
					Type: model.BlockContentDataviewView_Table,
					Name: "All",
					Sorts: []*model.BlockContentDataviewSort{
						{
							RelationKey: "name",
							Type:        model.BlockContentDataviewSort_Asc,
						},
					},
					Relations: relations,
					Filters:   nil,
				},
			},
		},
	}
	name := pbtypes.GetString(req.Details, bundle.RelationKeyName.String())
	icon := pbtypes.GetString(req.Details, bundle.RelationKeyIconEmoji.String())

	if name == "" {
		name = objType.Name + " set"
	}
	if icon == "" {
		icon = "📒"
	}

	err = set.InitDataview(dataview, name, icon)
	if err != nil {
		return "", setId, err
	}

	if req.ContextId == "" && req.TargetId == "" {
		// do not create a link
		return "", setId, nil
	}

	err = s.DoBasic(req.ContextId, func(b basic.Basic) error {
		linkId, err = b.Create(ctx, "", pb.RpcBlockCreateRequest{
			TargetId: req.TargetId,
			Block: &model.Block{
				Content: &model.BlockContentOfLink{
					Link: &model.BlockContentLink{
						TargetBlockId: setId,
						Style:         model.BlockContentLink_Dataview,
					},
				},
			},
			Position: req.Position,
		})
		if err != nil {
			err = fmt.Errorf("link create error: %v", err)
		}
		return err
	})

	return linkId, setId, nil
}

func (s *service) RemoveExtraRelations(ctx *state.Context, objectTypeId string, relationKeys []string) (err error) {
	return s.Do(objectTypeId, func(b smartblock.SmartBlock) error {
		return b.RemoveExtraRelations(ctx, relationKeys)
	})
}

func (s *service) ListAvailableRelations(objectId string) (aggregatedRelations []*pbrelation.Relation, err error) {
	err = s.Do(objectId, func(b smartblock.SmartBlock) error {
		objType := b.ObjectType()
		aggregatedRelations = b.Relations()

		agRels, err := s.Anytype().ObjectStore().ListRelations(objType)
		if err != nil {
			return err
		}

		for _, rel := range agRels {
			if pbtypes.HasRelation(aggregatedRelations, rel.Key) {
				continue
			}
			aggregatedRelations = append(aggregatedRelations, pbtypes.CopyRelation(rel))
		}
		return nil
	})

	return
}

func (s *service) cleanupBlocks() (closed bool) {
	s.m.Lock()
	defer s.m.Unlock()
	var closedCount, total int
	for id, ob := range s.openedBlocks {
		if !ob.locked && ob.refs == 0 && time.Now().After(ob.lastUsage.Add(blockCacheTTL)) {
			if err := ob.Close(); err != nil {
				log.Warnf("error while close block[%s]: %v", id, err)
			}
			delete(s.openedBlocks, id)
			closedCount++
		}
		total++
	}
	log.Infof("cleanup: block closed %d (total %v)", closedCount, total)
	return s.closed
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
