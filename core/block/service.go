package block

import (
	"errors"
	"fmt"
	"sync"
	"time"

	_import "github.com/anytypeio/go-anytype-middleware/core/block/editor/import"

	"github.com/anytypeio/go-anytype-library/files"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-library/logging"
	"github.com/anytypeio/go-anytype-library/pb/model"
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
	simpleFile "github.com/anytypeio/go-anytype-middleware/core/block/simple/file"
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
)

var log = logging.Logger("anytype-mw-service")

var (
	/*
		blockCacheTTL       = time.Minute
		blockCleanupTimeout = time.Second * 30
	*/
	blockCacheTTL       = time.Second // TODO: need to revert after changes testing
	blockCleanupTimeout = time.Second * 10
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
	CreatePage(ctx *state.Context, req pb.RpcBlockCreatePageRequest) (linkId string, pageId string, err error)
	CreateSmartBlock(req pb.RpcBlockCreatePageRequest) (pageId string, err error)
	DuplicateBlocks(ctx *state.Context, req pb.RpcBlockListDuplicateRequest) ([]string, error)
	UnlinkBlock(ctx *state.Context, req pb.RpcBlockUnlinkRequest) error
	ReplaceBlock(ctx *state.Context, req pb.RpcBlockReplaceRequest) (newId string, err error)

	MoveBlocks(ctx *state.Context, req pb.RpcBlockListMoveRequest) error
	MoveBlocksToNewPage(ctx *state.Context, req pb.RpcBlockListMoveToNewPageRequest) (linkId string, err error)
	ConvertChildrenToPages(req pb.RpcBlockListConvertChildrenToPagesRequest) (linkIds []string, err error)
	SetFields(ctx *state.Context, req pb.RpcBlockSetFieldsRequest) error
	SetFieldsList(ctx *state.Context, req pb.RpcBlockListSetFieldsRequest) error

	SetDetails(req pb.RpcBlockSetDetailsRequest) (err error)

	Paste(ctx *state.Context, req pb.RpcBlockPasteRequest) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, isSameBlockCaret bool, err error)

	Copy(req pb.RpcBlockCopyRequest, images map[string][]byte) (textSlot string, htmlSlot string, anySlot []*model.Block, err error)
	Cut(ctx *state.Context, req pb.RpcBlockCutRequest, images map[string][]byte) (textSlot string, htmlSlot string, anySlot []*model.Block, err error)
	Export(req pb.RpcBlockExportRequest, images map[string][]byte) (path string, err error)
	ImportMarkdown(ctx *state.Context, req pb.RpcBlockImportMarkdownRequest) (rootLinkIds []string, err error)

	SplitBlock(ctx *state.Context, req pb.RpcBlockSplitRequest) (blockId string, err error)
	MergeBlock(ctx *state.Context, req pb.RpcBlockMergeRequest) error
	SetTextText(req pb.RpcBlockSetTextTextRequest) error
	SetTextStyle(ctx *state.Context, contextId string, style model.BlockContentTextStyle, blockIds ...string) error
	SetTextChecked(ctx *state.Context, req pb.RpcBlockSetTextCheckedRequest) error
	SetTextColor(ctx *state.Context, contextId string, color string, blockIds ...string) error
	SetTextMark(ctx *state.Context, id string, mark *model.BlockContentTextMark, ids ...string) error
	SetBackgroundColor(ctx *state.Context, contextId string, color string, blockIds ...string) error
	SetAlign(ctx *state.Context, contextId string, align model.BlockAlign, blockIds ...string) (err error)

	SetDivStyle(ctx *state.Context, contextId string, style model.BlockContentDivStyle, ids ...string) (err error)

	UploadFile(req pb.RpcUploadFileRequest) (hash string, err error)
	UploadBlockFile(ctx *state.Context, req pb.RpcBlockUploadRequest) error
	CreateAndUploadFile(ctx *state.Context, req pb.RpcBlockFileCreateAndUploadRequest) (id string, err error)
	DropFiles(req pb.RpcExternalDropFilesRequest) (err error)

	Undo(ctx *state.Context, req pb.RpcBlockUndoRequest) error
	Redo(ctx *state.Context, req pb.RpcBlockRedoRequest) error

	SetPageIsArchived(req pb.RpcBlockSetPageIsArchivedRequest) error
	SetPagesIsArchived(req pb.RpcBlockListSetPageIsArchivedRequest) error
	DeletePages(req pb.RpcBlockListDeletePageRequest) error

	BookmarkFetch(ctx *state.Context, req pb.RpcBlockBookmarkFetchRequest) error
	BookmarkCreateAndFetch(ctx *state.Context, req pb.RpcBlockBookmarkCreateAndFetchRequest) (id string, err error)

	ProcessAdd(p process.Process) (err error)
	ProcessCancel(id string) error

	SimplePaste(contextId string, anySlot []*model.Block) (err error)

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
	s.meta = meta.NewService(a, s.metaFetcher)
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
	m            sync.RWMutex
}

func (s *service) init() {
	s.Do(s.anytype.PredefinedBlocks().Archive, func(b smartblock.SmartBlock) error {
		return nil
	})
}

func (s *service) metaFetcher(id string) (m meta.Meta, err error) {
	err = s.Do(id, func(b smartblock.SmartBlock) error {
		mt := b.Meta()
		if mt == nil {
			mt = &core.SmartBlockMeta{}
		}
		m = meta.Meta{
			BlockId:        id,
			SmartBlockMeta: *mt,
		}
		return nil
	})
	return
}

func (s *service) Anytype() anytype.Service {
	return s.anytype
}

func (s *service) OpenBlock(ctx *state.Context, id string) (err error) {
	s.m.Lock()
	defer s.m.Unlock()
	ob, ok := s.openedBlocks[id]
	if !ok {
		sb, e := s.createSmartBlock(id, false)
		if e != nil {
			return e
		}
		ob = &openedBlock{
			SmartBlock: sb,
			lastUsage:  time.Now(),
		}
		s.openedBlocks[id] = ob
	}
	ob.Lock()
	defer ob.Unlock()
	ob.locked = true
	ob.SetEventFunc(s.sendEvent)
	if err = ob.Show(ctx); err != nil {
		return
	}
	return nil
}

func (s *service) OpenBreadcrumbsBlock(ctx *state.Context) (blockId string, err error) {
	s.m.Lock()
	defer s.m.Unlock()
	bs := editor.NewBreadcrumbs()
	if err = bs.Init(source.NewVirtual(s.anytype, s.meta, pb.SmartBlockType_Breadcrumbs), true); err != nil {
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
		return b.SetDetails([]*pb.RpcBlockSetDetailsDetail{
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
		return nil
	})
}

func (s *service) CreateBlock(ctx *state.Context, req pb.RpcBlockCreateRequest) (id string, err error) {
	err = s.DoBasic(req.ContextId, func(b basic.Basic) error {
		id, err = b.Create(ctx, req)
		return err
	})
	return
}

func (s *service) CreateSmartBlock(req pb.RpcBlockCreatePageRequest) (pageId string, err error) {
	csm, err := s.anytype.CreateBlock(core.SmartBlockTypePage)
	if err != nil {
		err = fmt.Errorf("anytype.CreateBlock error: %v", err)
		return
	}
	pageId = csm.ID()

	if _, err = s.createSmartBlock(pageId, true); err != nil {
		return pageId, err
	}

	log.Infof("created new smartBlock: %v", pageId)
	if req.Details != nil && req.Details.Fields != nil {
		var details []*pb.RpcBlockSetDetailsDetail
		for k, v := range req.Details.Fields {
			details = append(details, &pb.RpcBlockSetDetailsDetail{
				Key:   k,
				Value: v,
			})
		}
		if err = s.SetDetails(pb.RpcBlockSetDetailsRequest{
			ContextId: pageId,
			Details:   details,
		}); err != nil {
			return pageId, fmt.Errorf("can't set details to page: %v", err)
		}
	}

	return pageId, nil
}

func (s *service) CreatePage(ctx *state.Context, req pb.RpcBlockCreatePageRequest) (linkId string, pageId string, err error) {
	pageId, err = s.CreateSmartBlock(req)
	if err != nil {
		err = fmt.Errorf("create smartblock error: %v", err)
	}

	err = s.DoBasic(req.ContextId, func(b basic.Basic) error {
		linkId, err = b.Create(ctx, pb.RpcBlockCreateRequest{
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
		blockId, err = b.RangeSplit(ctx, req.BlockId, req.Range.From, req.Range.To, req.Style)
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
	linkId, pageId, err := s.CreatePage(ctx, pb.RpcBlockCreatePageRequest{
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

		children := s.AllDescendantIds(blocks[blockId].ChildrenIds, blocks)
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

func (s *service) SetDetails(req pb.RpcBlockSetDetailsRequest) (err error) {
	return s.Do(req.ContextId, func(b smartblock.SmartBlock) error {
		return b.SetDetails(req.Details)
	})
}

func (s *service) SetFieldsList(ctx *state.Context, req pb.RpcBlockListSetFieldsRequest) (err error) {
	return s.DoBasic(req.ContextId, func(b basic.Basic) error {
		return b.SetFields(ctx, req.BlockFields...)
	})
}

func (s *service) Copy(req pb.RpcBlockCopyRequest, images map[string][]byte) (textSlot string, htmlSlot string, anySlot []*model.Block, err error) {
	err = s.DoClipboard(req.ContextId, func(cb clipboard.Clipboard) error {
		textSlot, htmlSlot, anySlot, err = cb.Copy(req, images)
		return err
	})

	return textSlot, htmlSlot, anySlot, err
}

func (s *service) Paste(ctx *state.Context, req pb.RpcBlockPasteRequest) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, isSameBlockCaret bool, err error) {
	err = s.DoClipboard(req.ContextId, func(cb clipboard.Clipboard) error {
		blockIds, uploadArr, caretPosition, isSameBlockCaret, err = cb.Paste(ctx, req)
		return err
	})

	return blockIds, uploadArr, caretPosition, isSameBlockCaret, err
}

func (s *service) Cut(ctx *state.Context, req pb.RpcBlockCutRequest, images map[string][]byte) (textSlot string, htmlSlot string, anySlot []*model.Block, err error) {
	err = s.DoClipboard(req.ContextId, func(cb clipboard.Clipboard) error {
		textSlot, htmlSlot, anySlot, err = cb.Cut(ctx, req, images)
		return err
	})
	return textSlot, htmlSlot, anySlot, err
}

func (s *service) Export(req pb.RpcBlockExportRequest, images map[string][]byte) (path string, err error) {
	err = s.DoClipboard(req.ContextId, func(cb clipboard.Clipboard) error {
		path, err = cb.Export(req, images)
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
		_, pageId, err := s.CreatePage(ctx, pb.RpcBlockCreatePageRequest{
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

func (s *service) SetTextText(req pb.RpcBlockSetTextTextRequest) error {
	return s.DoText(req.ContextId, func(b stext.Text) error {
		return b.UpdateTextBlocks(nil, []string{req.BlockId}, false, func(t text.Block) error {
			return t.SetText(req.Text, req.Marks)
		})
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
		return b.UpdateTextBlocks(ctx, blockIds, true, func(t text.Block) error {
			t.SetMarkForAllText(mark)
			return nil
		})
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

func (s *service) UploadBlockFile(ctx *state.Context, req pb.RpcBlockUploadRequest) (err error) {
	<-uploadFilesLimiter
	defer func() { uploadFilesLimiter <- struct{}{} }()
	return s.DoFile(req.ContextId, func(b file.File) error {
		err = b.Upload(ctx, req.BlockId, req.FilePath, req.Url)
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
	var tempFile = simpleFile.NewFile(&model.Block{Content: &model.BlockContentOfFile{File: &model.BlockContentFile{}}}).(simpleFile.Block)
	var opts []files.AddOption
	if req.DisableEncryption {
		opts = append(opts, files.WithPlaintext(true))
	}
	u := simpleFile.NewUploader(s.Anytype(), func(f func(file simpleFile.Block)) {
		f(tempFile)
	}, opts...)
	if err = u.DoType(req.LocalPath, req.Url, req.Type); err != nil {
		return
	}
	result := tempFile.Model().GetFile()
	if result.State != model.BlockContentFile_Done {
		return "", fmt.Errorf("unexpected upload error")
	}
	return result.Hash, nil
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
		return b.Fetch(ctx, req.BlockId, req.Url)
	})
}

func (s *service) BookmarkCreateAndFetch(ctx *state.Context, req pb.RpcBlockBookmarkCreateAndFetchRequest) (id string, err error) {
	err = s.DoBookmark(req.ContextId, func(b bookmark.Bookmark) error {
		id, err = b.CreateAndFetch(ctx, req)
		return err
	})
	return
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
		sb, err = s.createSmartBlock(id, false)
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

func (s *service) createSmartBlock(id string, initEmpty bool) (sb smartblock.SmartBlock, err error) {
	sc, err := source.NewSource(s.anytype, s.meta, id)
	if err != nil {
		return
	}
	switch sc.Type() {
	case pb.SmartBlockType_Page:
		sb = editor.NewPage(s, s, s, s.linkPreview)
	case pb.SmartBlockType_Home:
		sb = editor.NewDashboard(s)
	case pb.SmartBlockType_Archive:
		sb = editor.NewArchive(s)
	case pb.SmartBlockType_ProfilePage:
		sb = editor.NewProfile(s, s, s.linkPreview, s.sendEvent)
	default:
		return nil, fmt.Errorf("unexpected smartblock type: %v", sc.Type())
	}

	if err = sb.Init(sc, initEmpty); err != nil {
		return
	}

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
	return fmt.Errorf("history operation not available for this block type: %T", sb)
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

func (s *service) fillSlice(id string, ids []string, allBlocks map[string]*model.Block) []string {
	ids = append(ids, id)
	for _, chId := range allBlocks[id].ChildrenIds {
		ids = s.fillSlice(chId, ids, allBlocks)
	}
	return ids
}

func (s *service) AllDescendantIds(targetBlockIds []string, allBlocks map[string]*model.Block) (outputIds []string) {
	for _, tId := range targetBlockIds {
		outputIds = s.fillSlice(tId, outputIds, allBlocks)
	}

	return outputIds
}
