package block

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-library/logging"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/clipboard"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/file"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/stext"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/bookmark"
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
	blockCacheTTL       = time.Minute
	blockCleanupTimeout = time.Second * 30
)

type Service interface {
	OpenBlock(id string, breadcrumbsIds ...string) error
	OpenBreadcrumbsBlock() (blockId string, err error)
	CutBreadcrumbs(req pb.RpcBlockCutBreadcrumbsRequest) (err error)
	CloseBlock(id string) error
	CreateBlock(req pb.RpcBlockCreateRequest) (string, error)
	CreatePage(req pb.RpcBlockCreatePageRequest) (linkId string, pageId string, err error)
	DuplicateBlocks(req pb.RpcBlockListDuplicateRequest) ([]string, error)
	UnlinkBlock(req pb.RpcBlockUnlinkRequest) error
	ReplaceBlock(req pb.RpcBlockReplaceRequest) (newId string, err error)

	MoveBlocks(req pb.RpcBlockListMoveRequest) error
	MoveBlocksToNewPage(req pb.RpcBlockListMoveToNewPageRequest) (linkId string, err error)
	ConvertChildrenToPages(req pb.RpcBlockListConvertChildrenToPagesRequest) (linkIds []string, err error)

	SetFields(req pb.RpcBlockSetFieldsRequest) error
	SetFieldsList(req pb.RpcBlockListSetFieldsRequest) error

	SetDetails(req pb.RpcBlockSetDetailsRequest) (err error)

	Paste(req pb.RpcBlockPasteRequest) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, err error)

	Copy(req pb.RpcBlockCopyRequest, images map[string][]byte) (html string, err error)
	Cut(req pb.RpcBlockCutRequest, images map[string][]byte) (textSlot string, htmlSlot string, anySlot []*model.Block, err error)
	Export(req pb.RpcBlockExportRequest, images map[string][]byte) (path string, err error)

	SplitBlock(req pb.RpcBlockSplitRequest) (blockId string, err error)
	MergeBlock(req pb.RpcBlockMergeRequest) error
	SetTextText(req pb.RpcBlockSetTextTextRequest) error
	SetTextStyle(contextId string, style model.BlockContentTextStyle, blockIds ...string) error
	SetTextChecked(req pb.RpcBlockSetTextCheckedRequest) error
	SetTextColor(contextId string, color string, blockIds ...string) error
	SetBackgroundColor(contextId string, color string, blockIds ...string) error
	SetAlign(contextId string, align model.BlockAlign, blockIds ...string) (err error)

	UploadFile(req pb.RpcUploadFileRequest) (hash string, err error)
	UploadBlockFile(req pb.RpcBlockUploadRequest) error
	DropFiles(req pb.RpcExternalDropFilesRequest) (err error)

	Undo(req pb.RpcBlockUndoRequest) error
	Redo(req pb.RpcBlockRedoRequest) error

	SetPageIsArchived(req pb.RpcBlockSetPageIsArchivedRequest) error

	BookmarkFetch(req pb.RpcBlockBookmarkFetchRequest) error

	ProcessAdd(p process.Process) (err error)
	ProcessCancel(id string) error

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
		meta:         meta.NewService(a),
	}
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

func (s *service) Anytype() anytype.Service {
	return s.anytype
}

func (s *service) OpenBlock(id string, breadcrumbsIds ...string) (err error) {
	s.m.Lock()
	defer s.m.Unlock()
	ob, ok := s.openedBlocks[id]
	if !ok {
		sb, e := s.createSmartBlock(id)
		if e != nil {
			return e
		}
		ob = &openedBlock{
			SmartBlock: sb,
			lastUsage:  time.Now(),
		}
		s.openedBlocks[id] = ob
	}
	ob.locked = true
	ob.SetEventFunc(s.sendEvent)
	if err = ob.Show(); err != nil {
		return
	}

	for _, bid := range breadcrumbsIds {
		if b, ok := s.openedBlocks[bid]; ok {
			if bs, ok := b.SmartBlock.(*editor.Breadcrumbs); ok {
				bs.OnSmartOpen(id)
			} else {
				log.Warnf("unexpected smart block type %T; wand breadcrumbs", b)
			}
		} else {
			log.Warnf("breadcrumbs block not found")
		}
	}
	return nil
}

func (s *service) OpenBreadcrumbsBlock() (blockId string, err error) {
	s.m.Lock()
	defer s.m.Unlock()
	bs := editor.NewBreadcrumbs()
	if err = bs.Init(source.NewVirtual(s.anytype, s.meta)); err != nil {
		return
	}
	bs.SetEventFunc(s.sendEvent)
	s.openedBlocks[bs.Id()] = &openedBlock{
		SmartBlock: bs,
		lastUsage:  time.Now(),
		refs:       1,
	}
	if err = bs.Show(); err != nil {
		return
	}
	return bs.Id(), nil
}

func (s *service) CloseBlock(id string) (err error) {
	s.m.Lock()
	defer s.m.Unlock()
	if ob, ok := s.openedBlocks[id]; ok {
		ob.SetEventFunc(nil)
		ob.locked = false
		return
	}
	return ErrBlockNotFound
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

func (s *service) CutBreadcrumbs(req pb.RpcBlockCutBreadcrumbsRequest) (err error) {
	return s.Do(req.BreadcrumbsId, func(b smartblock.SmartBlock) error {
		if breadcrumbs, ok := b.(*editor.Breadcrumbs); ok {
			breadcrumbs.ChainCut(int(req.Index))
		} else {
			return ErrUnexpectedBlockType
		}
		return nil
	})
}

func (s *service) CreateBlock(req pb.RpcBlockCreateRequest) (id string, err error) {
	err = s.DoBasic(req.ContextId, func(b basic.Basic) error {
		id, err = b.Create(req)
		return err
	})
	return
}

func (s *service) CreatePage(req pb.RpcBlockCreatePageRequest) (linkId string, pageId string, err error) {
	csm, err := s.anytype.CreateBlock(core.SmartBlockTypePage)
	if err != nil {
		err = fmt.Errorf("anytype.CreateBlock error: %v", err)
		return
	}
	pageId = csm.ID()
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
			err = fmt.Errorf("can't set details to page: %v", err)
			return
		}
	}
	err = s.DoBasic(req.ContextId, func(b basic.Basic) error {
		linkId, err = b.Create(pb.RpcBlockCreateRequest{
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

func (s *service) DuplicateBlocks(req pb.RpcBlockListDuplicateRequest) (newIds []string, err error) {
	err = s.DoBasic(req.ContextId, func(b basic.Basic) error {
		newIds, err = b.Duplicate(req)
		return err
	})
	return
}

func (s *service) UnlinkBlock(req pb.RpcBlockUnlinkRequest) (err error) {
	return s.DoBasic(req.ContextId, func(b basic.Basic) error {
		return b.Unlink(req.BlockIds...)
	})
}

func (s *service) SplitBlock(req pb.RpcBlockSplitRequest) (blockId string, err error) {
	err = s.DoText(req.ContextId, func(b stext.Text) error {
		blockId, err = b.RangeSplit(req.BlockId, req.Range.From, req.Range.To, req.Style)
		return err
	})
	return
}

func (s *service) MergeBlock(req pb.RpcBlockMergeRequest) (err error) {
	return s.DoText(req.ContextId, func(b stext.Text) error {
		return b.Merge(req.FirstBlockId, req.SecondBlockId)
	})
}

func (s *service) MoveBlocks(req pb.RpcBlockListMoveRequest) (err error) {
	if req.ContextId == req.TargetContextId {
		return s.DoBasic(req.ContextId, func(b basic.Basic) error {
			return b.Move(req)
		})
	}

	return s.Do(req.ContextId, func(contextBlock smartblock.SmartBlock) error {
		return s.Do(req.TargetContextId, func(targetBlock smartblock.SmartBlock) error {
			contextState := contextBlock.NewState()
			targetState := targetBlock.NewState()

			for _, bId := range req.BlockIds {
				b := contextBlock.Pick(bId)
				if b != nil {
					targetState.Add(b)
					err = targetState.InsertTo("", model.Block_Inner, b.Model().Id)
					if err != nil {
						return err
					}

					contextState.Remove(b.Model().Id)
				}
			}

			err = targetBlock.Apply(targetState)
			if err != nil {
				return err
			}
			err = contextBlock.Apply(contextState)
			if err != nil {
				return err
			}

			return err
		})
	})
}

func (s *service) MoveBlocksToNewPage(req pb.RpcBlockListMoveToNewPageRequest) (linkId string, err error) {
	// 1. Create new page, link
	linkId, pageId, err := s.CreatePage(pb.RpcBlockCreatePageRequest{
		ContextId: req.ContextId,
		TargetId:  req.DropTargetId,
		Position:  req.Position,
		Details:   req.Details,
	})

	if err != nil {
		return linkId, err
	}

	// 2. Move blocks to new page
	err = s.MoveBlocks(pb.RpcBlockListMoveRequest{
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
	err = s.Do(req.ContextId, func(contextBlock smartblock.SmartBlock) error {
		contextState := contextBlock.NewState()

		for _, blockId := range req.BlockIds {
			// 1. Get blocks
			linkId := ""
			convertedBlock := contextState.Pick(blockId)
			if convertedBlock == nil {
				return fmt.Errorf("block not found")
			}

			blocks := []simple.Block{}
			cIds := convertedBlock.Model().ChildrenIds

			for _, cId := range cIds {
				b := contextState.Pick(cId)
				if b != nil {
					blocks = append(blocks, b)
				}
			}

			// 2. Get name

			text := convertedBlock.Model().GetText()
			name := ""

			if text != nil {
				name = convertedBlock.Model().GetText().Text
			}

			// 3. Create new page, link
			pageId := ""
			pageId, linkId, err = s.CreatePage(pb.RpcBlockCreatePageRequest{
				ContextId: req.ContextId,
				TargetId:  blockId,
				Details: &types.Struct{
					Fields: map[string]*types.Value{
						"name": pbtypes.String(name),
						"icon": pbtypes.String(":file_folder:"),
					},
				},
				Position: model.Block_Top,
			})

			if err != nil {
				return err
			}

			// 4. Move blocks to new page
			err = s.MoveBlocks(pb.RpcBlockListMoveRequest{
				ContextId:       req.ContextId,
				BlockIds:        cIds,
				TargetContextId: pageId,
				DropTargetId:    "",
				Position:        0,
			})

			if err != nil {
				return err
			}

			// 5. Remove original block
			contextState.Remove(blockId)
			linkIds = append(linkIds, linkId)
		}
		return contextBlock.Apply(contextState)
	})

	return linkIds, err
}

func (s *service) ReplaceBlock(req pb.RpcBlockReplaceRequest) (newId string, err error) {
	err = s.DoBasic(req.ContextId, func(b basic.Basic) error {
		newId, err = b.Replace(req.BlockId, req.Block)
		return err
	})
	return
}

func (s *service) SetFields(req pb.RpcBlockSetFieldsRequest) (err error) {
	return s.DoBasic(req.ContextId, func(b basic.Basic) error {
		return b.SetFields(&pb.RpcBlockListSetFieldsRequestBlockField{
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

func (s *service) SetFieldsList(req pb.RpcBlockListSetFieldsRequest) (err error) {
	return s.DoBasic(req.ContextId, func(b basic.Basic) error {
		return b.SetFields(req.BlockFields...)
	})
}

func (s *service) Copy(req pb.RpcBlockCopyRequest, images map[string][]byte) (html string, err error) {
	err = s.DoClipboard(req.ContextId, func(cb clipboard.Clipboard) error {
		html, err = cb.Copy(req, images)
		return err
	})
	return html, err
}

func (s *service) Paste(req pb.RpcBlockPasteRequest) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, err error) {
	err = s.DoClipboard(req.ContextId, func(cb clipboard.Clipboard) error {
		blockIds, uploadArr, caretPosition, err = cb.Paste(req)
		return err
	})

	return blockIds, uploadArr, caretPosition, err
}

func (s *service) Cut(req pb.RpcBlockCutRequest, images map[string][]byte) (textSlot string, htmlSlot string, anySlot []*model.Block, err error) {
	err = s.DoClipboard(req.ContextId, func(cb clipboard.Clipboard) error {
		textSlot, htmlSlot, anySlot, err = cb.Cut(req, images)
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

func (s *service) SetTextText(req pb.RpcBlockSetTextTextRequest) error {
	return s.DoText(req.ContextId, func(b stext.Text) error {
		return b.UpdateTextBlocks([]string{req.BlockId}, false, func(t text.Block) error {
			return t.SetText(req.Text, req.Marks)
		})
	})
}

func (s *service) SetTextStyle(contextId string, style model.BlockContentTextStyle, blockIds ...string) error {
	return s.DoText(contextId, func(b stext.Text) error {
		return b.UpdateTextBlocks(blockIds, true, func(t text.Block) error {
			t.SetStyle(style)
			return nil
		})
	})
}

func (s *service) SetTextChecked(req pb.RpcBlockSetTextCheckedRequest) error {
	return s.DoText(req.ContextId, func(b stext.Text) error {
		return b.UpdateTextBlocks([]string{req.BlockId}, true, func(t text.Block) error {
			t.SetChecked(req.Checked)
			return nil
		})
	})
}

func (s *service) SetTextColor(contextId, color string, blockIds ...string) error {
	return s.DoText(contextId, func(b stext.Text) error {
		return b.UpdateTextBlocks(blockIds, true, func(t text.Block) error {
			t.SetTextColor(color)
			return nil
		})
	})
}

func (s *service) SetBackgroundColor(contextId, color string, blockIds ...string) (err error) {
	return s.DoBasic(contextId, func(b basic.Basic) error {
		return b.Update(func(b simple.Block) error {
			b.Model().BackgroundColor = color
			return nil
		}, blockIds...)
	})
}

func (s *service) SetAlign(contextId string, align model.BlockAlign, blockIds ...string) (err error) {
	return s.DoBasic(contextId, func(b basic.Basic) error {
		return b.Update(func(b simple.Block) error {
			b.Model().Align = align
			return nil
		}, blockIds...)
	})
}

func (s *service) UploadBlockFile(req pb.RpcBlockUploadRequest) (err error) {
	return s.DoFile(req.ContextId, func(b file.File) error {
		err = b.Upload(req.BlockId, req.FilePath, req.Url)
		return err
	})
}

func (s *service) UploadFile(req pb.RpcUploadFileRequest) (hash string, err error) {
	var tempFile = simpleFile.NewFile(&model.Block{Content: &model.BlockContentOfFile{File: &model.BlockContentFile{}}}).(simpleFile.Block)
	u := simpleFile.NewUploader(s.Anytype(), func(f func(file simpleFile.Block)) {
		f(tempFile)
	})
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

func (s *service) Undo(req pb.RpcBlockUndoRequest) (err error) {
	return s.DoHistory(req.ContextId, func(b basic.IHistory) error {
		return b.Undo()
	})
}

func (s *service) Redo(req pb.RpcBlockRedoRequest) (err error) {
	return s.DoHistory(req.ContextId, func(b basic.IHistory) error {
		return b.Redo()
	})
}

func (s *service) BookmarkFetch(req pb.RpcBlockBookmarkFetchRequest) (err error) {
	return s.DoBasic(req.ContextId, func(b basic.Basic) error {
		return b.Update(func(b simple.Block) error {
			if bm, ok := b.(bookmark.Block); ok {
				return bm.Fetch(bookmark.FetchParams{
					Url:     req.Url,
					Anytype: s.anytype,
					Updater: func(ids []string, hist bool, apply func(b simple.Block) error) (err error) {
						return s.DoBasic(req.ContextId, func(b basic.Basic) error {
							return b.Update(apply, ids...)
						})
					},
					LinkPreview: s.linkPreview,
				})
			}
			return ErrUnexpectedBlockType
		}, req.BlockId)
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
		sb, err = s.createSmartBlock(id)
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

func (s *service) createSmartBlock(id string) (sb smartblock.SmartBlock, err error) {
	sc, err := source.NewSource(s.anytype, s.meta, id)
	if err != nil {
		return
	}
	switch sc.Type() {
	case core.SmartBlockTypePage:
		sb = editor.NewPage(s)
	case core.SmartBlockTypeDashboard:
		sb = editor.NewDashboard()
	case core.SmartBlockTypeArchive:
		sb = editor.NewArchive(s)
	default:
		return nil, fmt.Errorf("unexpected smartblock type: %v", sc.Type())
	}

	if err = sb.Init(sc); err != nil {
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
	return fmt.Errorf("unexpected operation for this block type: %T", sb)
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
	return fmt.Errorf("unexpected operation for this block type: %T", sb)
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
	return fmt.Errorf("unexpected operation for this block type: %T", sb)
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
	return fmt.Errorf("unexpected operation for this block type: %T", sb)
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
	return fmt.Errorf("unexpected operation for this block type: %T", sb)
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
	return fmt.Errorf("unexpected operation for this block type: %T", sb)
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
