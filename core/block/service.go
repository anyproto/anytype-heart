package block

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/core/block/database/objects"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/dataview"
	_import "github.com/anytypeio/go-anytype-middleware/core/block/editor/import"
	"github.com/anytypeio/go-anytype-middleware/core/history"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/relation"
	"github.com/globalsign/mgo/bson"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/files"
	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/clipboard"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/file"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/stext"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/bookmark"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/file"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"

	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/link"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/linkpreview"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
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

type Service interface {
	OpenBlock(ctx *state.Context, id string) error
	OpenBreadcrumbsBlock(ctx *state.Context) (blockId string, err error)
	SetBreadcrumbs(ctx *state.Context, req pb.RpcBlockSetBreadcrumbsRequest) (err error)
	CloseBlock(id string) error
	CreateBlock(ctx *state.Context, req pb.RpcBlockCreateRequest) (string, error)
	CreatePage(ctx *state.Context, groupId string, req pb.RpcBlockCreatePageRequest) (linkId string, pageId string, err error)
	CreateSmartBlock(sbType coresb.SmartBlockType, details *types.Struct, objectTypes []string, relations []*pbrelation.Relation) (id string, err error)
	DuplicateBlocks(ctx *state.Context, req pb.RpcBlockListDuplicateRequest) ([]string, error)
	UnlinkBlock(ctx *state.Context, req pb.RpcBlockUnlinkRequest) error
	ReplaceBlock(ctx *state.Context, req pb.RpcBlockReplaceRequest) (newId string, err error)

	MoveBlocks(ctx *state.Context, req pb.RpcBlockListMoveRequest) error
	MoveBlocksToNewPage(ctx *state.Context, req pb.RpcBlockListMoveToNewPageRequest) (linkId string, err error)
	ConvertChildrenToPages(req pb.RpcBlockListConvertChildrenToPagesRequest) (linkIds []string, err error)
	SetFields(ctx *state.Context, req pb.RpcBlockSetFieldsRequest) error
	SetFieldsList(ctx *state.Context, req pb.RpcBlockListSetFieldsRequest) error

	SetDetails(ctx *state.Context, req pb.RpcBlockSetDetailsRequest) (err error)

	GetObjectType(url string) (objectType *pbrelation.ObjectType, err error)
	UpdateRelations(id string, relations []*pbrelation.Relation) (err error)
	AddRelations(id string, relations []*pbrelation.Relation) (relationsWithKeys []*pbrelation.Relation, err error)
	RemoveRelations(id string, relationKeys []string) (err error)
	CreateSet(ctx *state.Context, req pb.RpcBlockCreateSetRequest) (linkId string, setId string, err error)

	AddObjectTypes(objectId string, objectTypes []string) (err error)
	RemoveObjectTypes(objectId string, objectTypes []string) (err error)

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

	SetDivStyle(ctx *state.Context, contextId string, style model.BlockContentDivStyle, ids ...string) (err error)

	UploadFile(req pb.RpcUploadFileRequest) (hash string, err error)
	UploadBlockFile(ctx *state.Context, req pb.RpcBlockUploadRequest, groupId string) error
	UploadBlockFileSync(ctx *state.Context, req pb.RpcBlockUploadRequest) (err error)
	CreateAndUploadFile(ctx *state.Context, req pb.RpcBlockFileCreateAndUploadRequest) (id string, err error)
	DropFiles(req pb.RpcExternalDropFilesRequest) (err error)

	Undo(ctx *state.Context, req pb.RpcBlockUndoRequest) error
	Redo(ctx *state.Context, req pb.RpcBlockRedoRequest) error

	SetPageIsArchived(req pb.RpcBlockSetPageIsArchivedRequest) error
	SetPagesIsArchived(req pb.RpcBlockListSetPageIsArchivedRequest) error
	DeletePages(req pb.RpcBlockListDeletePageRequest) error

	GetAggregatedRelations(ctx *state.Context, req pb.RpcBlockGetDataviewAvailableRelationsRequest) (relations []*pbrelation.Relation, err error)
	GetDataviewObjectType(ctx *state.Context, contextId string, blockId string) (string, error)
	DeleteDataviewView(ctx *state.Context, req pb.RpcBlockDeleteDataviewViewRequest) error
	SetDataviewView(ctx *state.Context, req pb.RpcBlockSetDataviewViewRequest) error
	SetDataviewActiveView(ctx *state.Context, req pb.RpcBlockSetDataviewActiveViewRequest) error
	CreateDataviewView(ctx *state.Context, req pb.RpcBlockCreateDataviewViewRequest) (id string, err error)
	AddDataviewRelation(ctx *state.Context, req pb.RpcBlockDataviewRelationAddRequest) (id string, err error)
	DeleteDataviewRelation(ctx *state.Context, req pb.RpcBlockDataviewRelationDeleteRequest) error

	CreateDataviewRecord(ctx *state.Context, req pb.RpcBlockCreateDataviewRecordRequest) (*types.Struct, error)
	UpdateDataviewRecord(ctx *state.Context, req pb.RpcBlockUpdateDataviewRecordRequest) error
	DeleteDataviewRecord(ctx *state.Context, req pb.RpcBlockDeleteDataviewRecordRequest) error

	BookmarkFetch(ctx *state.Context, req pb.RpcBlockBookmarkFetchRequest) error
	BookmarkFetchSync(ctx *state.Context, req pb.RpcBlockBookmarkFetchRequest) (err error)
	BookmarkCreateAndFetch(ctx *state.Context, req pb.RpcBlockBookmarkCreateAndFetchRequest) (id string, err error)

	SetRelationKey(ctx *state.Context, request pb.RpcBlockRelationSetKeyRequest) error
	AddRelationBlock(ctx *state.Context, request pb.RpcBlockRelationAddRequest) error

	ProcessAdd(p process.Process) (err error)
	ProcessCancel(id string) error

	SimplePaste(contextId string, anySlot []*model.Block) (err error)

	Reindex(id string) (err error)

	History() history.History

	Close() error
}

func NewService(accountId string, a anytype.Service, lp linkpreview.LinkPreview, sendEvent func(event *pb.Event)) Service {
	s := &service{
		accountId: accountId,
		anytype:   a,
		sendEvent: func(event *pb.Event) {
			sendEvent(event)
		},
		openedBlocks: make(map[string]*openedBlock),
		linkPreview:  lp,
		process:      process.NewService(sendEvent),
	}
	s.meta = meta.NewService(a)
	s.history = history.NewHistory(a, s, s.meta)
	go s.cleanupTicker()
	s.init()
	log.Info("block service started")
	return s
}

type openedBlock struct {
	smartblock.SmartBlock
	lastUsage time.Time
	locked    bool
	refs      int32
}

type service struct {
	anytype      anytype.Service
	meta         meta.Service
	accountId    string
	sendEvent    func(event *pb.Event)
	openedBlocks map[string]*openedBlock
	closed       bool
	linkPreview  linkpreview.LinkPreview
	process      process.Service
	history      history.History
	m            sync.RWMutex
}

func (s *service) init() {
	s.Do(s.anytype.PredefinedBlocks().Archive, func(b smartblock.SmartBlock) error {
		return nil
	})

	s.Do(s.anytype.PredefinedBlocks().SetPages, func(b smartblock.SmartBlock) error {
		return nil
	})
}

func (s *service) Anytype() anytype.Service {
	return s.anytype
}

func (s *service) OpenBlock(ctx *state.Context, id string) (err error) {
	s.m.Lock()
	ob, ok := s.openedBlocks[id]
	if !ok {
		sb, e := s.createSmartBlock(id, false, nil)
		if e != nil {
			s.m.Unlock()
			return e
		}
		ob = &openedBlock{
			SmartBlock: sb,
			lastUsage:  time.Now(),
		}
		s.openedBlocks[id] = ob
	}
	s.m.Unlock()

	ob.Lock()
	defer ob.Unlock()
	ob.locked = true
	ob.SetEventFunc(s.sendEvent)
	if err = ob.Show(ctx); err != nil {
		return
	}
	if e := s.anytype.ObjectUpdateLastOpened(id); e != nil {
		log.Warnf("can't update last opened id: %v", e)
	}

	if v, hasOpenListner := ob.SmartBlock.(smartblock.SmartblockOpenListner); hasOpenListner {
		v.SmartblockOpened(ctx)
	}

	return nil
}

func (s *service) OpenBreadcrumbsBlock(ctx *state.Context) (blockId string, err error) {
	s.m.Lock()
	defer s.m.Unlock()
	bs := editor.NewBreadcrumbs(s.meta)
	if err = bs.Init(source.NewVirtual(s.anytype, pb.SmartBlockType_Breadcrumbs), true, nil); err != nil {
		return
	}
	bs.Lock()
	defer bs.Unlock()
	bs.SetEventFunc(s.sendEvent)
	s.openedBlocks[bs.Id()] = &openedBlock{
		SmartBlock: bs,
		lastUsage:  time.Now(),
		refs:       1,
	}
	if err = bs.Show(ctx); err != nil {
		return
	}
	return bs.Id(), nil
}

func (s *service) CloseBlock(id string) (err error) {
	s.m.Lock()
	defer s.m.Unlock()
	if ob, ok := s.openedBlocks[id]; ok {
		ob.Lock()
		defer ob.Unlock()
		ob.SetEventFunc(nil)
		ob.locked = false
		return
	}
	return ErrBlockNotFound
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
		return nil
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
		})
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

func (s *service) CreateSmartBlock(sbType coresb.SmartBlockType, details *types.Struct, objectTypes []string, relations []*pbrelation.Relation) (id string, err error) {
	csm, err := s.anytype.CreateBlock(sbType)
	if err != nil {
		err = fmt.Errorf("anytype.CreateBlock error: %v", err)
		return
	}
	id = csm.ID()

	var sb smartblock.SmartBlock
	if sb, err = s.createSmartBlock(id, true, objectTypes); err != nil {
		return id, err
	}

	log.Debugf("created new smartBlock: %v, objectType: %v", id, sb.ObjectTypes())

	if details != nil && details.Fields != nil {
		var setDetails []*pb.RpcBlockSetDetailsDetail
		for k, v := range details.Fields {
			setDetails = append(setDetails, &pb.RpcBlockSetDetailsDetail{
				Key:   k,
				Value: v,
			})
		}
		if err = s.SetDetails(nil, pb.RpcBlockSetDetailsRequest{
			ContextId: id,
			Details:   setDetails,
		}); err != nil {
			return id, fmt.Errorf("can't set details to object: %v", err)
		}
	}

	if relations != nil {
		var setDetails []*pb.RpcBlockSetDetailsDetail
		for k, v := range details.Fields {
			setDetails = append(setDetails, &pb.RpcBlockSetDetailsDetail{
				Key:   k,
				Value: v,
			})
		}
		if _, err = s.AddRelations(id, relations); err != nil {
			return id, fmt.Errorf("can't add relations to object: %v", err)
		}
	}

	return id, nil
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

	pageId, err = s.CreateSmartBlock(coresb.SmartBlockTypePage, req.Details, []string{objects.BundledObjectTypeURLPrefix + "page"}, nil)
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

		children := s.AllDescendantIds(blockId, blocks)
		linkId, err := s.MoveBlocksToNewPage(nil, pb.RpcBlockListMoveToNewPageRequest{
			ContextId: req.ContextId,
			BlockIds:  children,
			Details: &types.Struct{
				Fields: map[string]*types.Value{
					"name": pbtypes.String(blocks[blockId].GetText().Text),
				},
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
		return b.SetDetails(ctx, req.Details)
	})
}

func (s *service) SetFieldsList(ctx *state.Context, req pb.RpcBlockListSetFieldsRequest) (err error) {
	return s.DoBasic(req.ContextId, func(b basic.Basic) error {
		return b.SetFields(ctx, req.BlockFields...)
	})
}

func (s *service) GetAggregatedRelations(ctx *state.Context, req pb.RpcBlockGetDataviewAvailableRelationsRequest) (relations []*pbrelation.Relation, err error) {
	err = s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		relations, err = b.GetAggregatedRelations(ctx, req.BlockId)
		return err
	})

	return
}

func (s *service) GetDataviewObjectType(ctx *state.Context, contextId string, blockId string) (objectType string, err error) {
	err = s.DoDataview(contextId, func(b dataview.Dataview) error {
		objectType, err = b.GetObjectTypeURL(ctx, blockId)
		return err
	})

	return
}

func (s *service) SetDataviewView(ctx *state.Context, req pb.RpcBlockSetDataviewViewRequest) error {
	return s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		return b.UpdateView(ctx, req.BlockId, req.ViewId, *req.View, true)
	})
}

func (s *service) DeleteDataviewView(ctx *state.Context, req pb.RpcBlockDeleteDataviewViewRequest) error {
	return s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		return b.DeleteView(ctx, req.BlockId, req.ViewId, true)
	})
}

func (s *service) SetDataviewActiveView(ctx *state.Context, req pb.RpcBlockSetDataviewActiveViewRequest) error {
	return s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		return b.SetActiveView(ctx, req.BlockId, req.ViewId, int(req.Limit), int(req.Offset))
	})
}

func (s *service) CreateDataviewView(ctx *state.Context, req pb.RpcBlockCreateDataviewViewRequest) (id string, err error) {
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

func (s *service) CreateDataviewRecord(ctx *state.Context, req pb.RpcBlockCreateDataviewRecordRequest) (rec *types.Struct, err error) {
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

func (s *service) UpdateDataviewRecord(ctx *state.Context, req pb.RpcBlockUpdateDataviewRecordRequest) (err error) {
	err = s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		return b.UpdateRecord(ctx, req.BlockId, req.RecordId, model.ObjectDetails{Details: req.Record})
	})

	return
}

func (s *service) DeleteDataviewRecord(ctx *state.Context, req pb.RpcBlockDeleteDataviewRecordRequest) (err error) {
	err = s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		return b.DeleteRecord(ctx, req.BlockId, req.RecordId)
	})

	return
}

func (s *service) AddDataviewRelation(ctx *state.Context, req pb.RpcBlockDataviewRelationAddRequest) (key string, err error) {
	err = s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		rel, err := b.AddRelation(ctx, req.BlockId, *req.Relation, true)
		if err != nil {
			return err
		}
		key = rel.Key
		return nil
	})

	return
}

func (s *service) DeleteDataviewRelation(ctx *state.Context, req pb.RpcBlockDataviewRelationDeleteRequest) error {
	return s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		return b.DeleteRelation(ctx, req.BlockId, req.RelationKey, true)
	})
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

func (s *service) Undo(ctx *state.Context, req pb.RpcBlockUndoRequest) (err error) {
	return s.DoHistory(req.ContextId, func(b basic.IHistory) error {
		return b.Undo(ctx)
	})
}

func (s *service) Redo(ctx *state.Context, req pb.RpcBlockRedoRequest) (err error) {
	return s.DoHistory(req.ContextId, func(b basic.IHistory) error {
		return b.Redo(ctx)
	})
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
	return s.DoBasic(req.ContextId, func(b basic.Basic) error {
		return b.SetRelationKey(ctx, req)
	})
}

func (s *service) AddRelationBlock(ctx *state.Context, req pb.RpcBlockRelationAddRequest) error {
	return s.DoBasic(req.ContextId, func(b basic.Basic) error {
		return b.AddRelationAndSet(ctx, req)
	})
}

func (s *service) Reindex(id string) (err error) {
	return s.Do(id, func(b smartblock.SmartBlock) error {
		return b.Reindex()
	})
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
	defer s.m.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true
	for _, sb := range s.openedBlocks {
		sb.Lock()
		if err := sb.Close(); err != nil {
			log.Errorf("block[%s] close error: %v", sb.Id(), err)
		}
		sb.Unlock()
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
		sb, err = s.createSmartBlock(id, false, nil)
		if err != nil {
			return
		}
		ob = &openedBlock{
			SmartBlock: sb,
		}
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

func (s *service) createSmartBlock(id string, initEmpty bool, initWithObjectTypeUrls []string) (sb smartblock.SmartBlock, err error) {
	sc, err := source.NewSource(s.anytype, id)
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
	default:
		return nil, fmt.Errorf("unexpected smartblock type: %v", sc.Type())
	}

	err = sb.Init(sc, initEmpty, initWithObjectTypeUrls)
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

func (s *service) GetObjectType(url string) (objectType *pbrelation.ObjectType, err error) {
	objectType = &pbrelation.ObjectType{}
	if strings.HasPrefix(url, objects.BundledObjectTypeURLPrefix) {
		var err error
		objectType, err = relation.GetObjectType(url)
		if err != nil {
			if err == relation.ErrNotFound {
				return nil, ErrUnknownObjectType
			}
			return nil, err
		}
		return objectType, nil
	} else if !strings.HasPrefix(url, objects.CustomObjectTypeURLPrefix) {
		return nil, fmt.Errorf("incorrect object type URL format")
	}

	sbid := strings.TrimPrefix(url, objects.CustomObjectTypeURLPrefix)
	sb, err := s.anytype.GetBlock(sbid)
	if err != nil {
		return nil, err
	}

	err = s.Do(sb.ID(), func(b smartblock.SmartBlock) error {
		details := b.Details()
		objectType.Relations = b.Relations()
		objectType.Url = url
		if details != nil && details.Fields != nil {
			if v, ok := details.Fields["name"]; ok {
				objectType.Name = v.GetStringValue()
			}
			if v, ok := details.Fields["layout"]; ok {
				objectType.Layout = pbrelation.ObjectTypeLayout(int(v.GetNumberValue()))
			}
			if v, ok := details.Fields["iconEmoji"]; ok {
				objectType.IconEmoji = v.GetStringValue()
			}
		}
		return nil
	})

	return objectType, err
}

func (s *service) UpdateRelations(objectId string, relations []*pbrelation.Relation) (err error) {
	return s.Do(objectId, func(b smartblock.SmartBlock) error {
		return b.UpdateRelations(relations)
	})
}

func (s *service) AddRelations(objectId string, relations []*pbrelation.Relation) (relationsWithKeys []*pbrelation.Relation, err error) {
	err = s.Do(objectId, func(b smartblock.SmartBlock) error {
		var err2 error
		relationsWithKeys, err2 = b.AddRelations(relations)
		if err2 != nil {
			return err2
		}
		return nil
	})

	return
}

func (s *service) AddObjectTypes(objectId string, objectTypes []string) (err error) {
	return s.Do(objectId, func(b smartblock.SmartBlock) error {
		return b.AddObjectTypes(objectTypes)
	})
}

func (s *service) RemoveObjectTypes(objectId string, objectTypes []string) (err error) {
	return s.Do(objectId, func(b smartblock.SmartBlock) error {
		return b.RemoveObjectTypes(objectTypes)
	})
}

func (s *service) CreateSet(ctx *state.Context, req pb.RpcBlockCreateSetRequest) (linkId string, setId string, err error) {
	objType, err := s.GetObjectType(req.ObjectTypeUrl)
	if err != nil {
		return "", "", err
	}

	csm, err := s.anytype.CreateBlock(coresb.SmartBlockTypeSet)
	if err != nil {
		err = fmt.Errorf("anytype.CreateBlock error: %v", err)
		return
	}
	setId = csm.ID()

	sb, err := s.createSmartBlock(setId, true, nil)
	if err != nil {
		return "", "", err
	}
	set, ok := sb.(*editor.Set)
	if !ok {
		return "", setId, fmt.Errorf("unexpected set block type: %T", sb)
	}

	var relations []*model.BlockContentDataviewRelation
	for _, rel := range objType.Relations {
		relations = append(relations, &model.BlockContentDataviewRelation{Key: rel.Key, IsVisible: !rel.Hidden, IsReadOnly: rel.ReadOnly})
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

	var name, icon string
	if req.Details != nil && req.Details.Fields != nil {
		if req.Details.Fields["name"] != nil {
			name = req.Details.Fields["name"].GetStringValue()
		}
		if req.Details.Fields["iconEmoji"] != nil {
			icon = req.Details.Fields["iconEmoji"].GetStringValue()
		}
	}
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

	return linkId, setId, nil
}

func (s *service) RemoveRelations(objectTypeId string, relationKeys []string) (err error) {
	return s.Do(objectTypeId, func(b smartblock.SmartBlock) error {
		return b.RemoveRelations(relationKeys)
	})
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

func (s *service) History() history.History {
	return s.history
}
