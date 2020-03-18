package block

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/cipboard"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/file"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/stext"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/util/linkpreview"
	logging "github.com/ipfs/go-log"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/bookmark"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/file"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/link"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

var (
	ErrBlockNotFound       = errors.New("block not found")
	ErrBlockAlreadyOpen    = errors.New("block already open")
	ErrUnexpectedBlockType = errors.New("unexpected block type")
)

var log = logging.Logger("anytype-mw")

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
	CreatePage(req pb.RpcBlockCreatePageRequest) (string, string, error)
	DuplicateBlocks(req pb.RpcBlockListDuplicateRequest) ([]string, error)
	UnlinkBlock(req pb.RpcBlockUnlinkRequest) error
	ReplaceBlock(req pb.RpcBlockReplaceRequest) (newId string, err error)

	MoveBlocks(req pb.RpcBlockListMoveRequest) error

	SetFields(req pb.RpcBlockSetFieldsRequest) error
	SetFieldsList(req pb.RpcBlockListSetFieldsRequest) error

	Paste(req pb.RpcBlockPasteRequest) (blockIds []string, err error)
	Copy(req pb.RpcBlockCopyRequest) (html string, err error)
	Cut(req pb.RpcBlockCutRequest) (textSlot string, htmlSlot string, anySlot []*model.Block, err error)
	Export(req pb.RpcBlockExportRequest) (html string, err error)

	SplitBlock(req pb.RpcBlockSplitRequest) (blockId string, err error)
	MergeBlock(req pb.RpcBlockMergeRequest) error
	SetTextText(req pb.RpcBlockSetTextTextRequest) error
	SetTextStyle(contextId string, style model.BlockContentTextStyle, blockIds ...string) error
	SetTextChecked(req pb.RpcBlockSetTextCheckedRequest) error
	SetTextColor(contextId string, color string, blockIds ...string) error
	SetBackgroundColor(contextId string, color string, blockIds ...string) error
	SetAlign(contextId string, align model.BlockAlign, blockIds ...string) (err error)

	UploadFile(req pb.RpcBlockUploadRequest) error
	DropFiles(req pb.RpcExternalDropFilesRequest) (err error)

	Undo(req pb.RpcBlockUndoRequest) error
	Redo(req pb.RpcBlockRedoRequest) error

	SetPageIsArchived(req pb.RpcBlockSetPageIsArchivedRequest) error

	BookmarkFetch(req pb.RpcBlockBookmarkFetchRequest) error

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
	}
	go s.cleanupTicker()
	return s
}

type openedBlock struct {
	smartblock.SmartBlock
	lastUsage time.Time
	refs      int32
}

type service struct {
	anytype      anytype.Service
	accountId    string
	sendEvent    func(event *pb.Event)
	openedBlocks map[string]*openedBlock
	closed       bool
	linkPreview  linkpreview.LinkPreview
	process      process.Service
	m            sync.RWMutex
}

func (s *service) OpenBlock(id string, breadcrumbsIds ...string) (err error) {
	s.m.Lock()
	defer s.m.Unlock()
	sb, ok := s.openedBlocks[id]
	if ok {
		sb.SetEventFunc(s.sendEvent)
		if err = sb.Show(); err != nil {
			return
		}
	} else {
		sb, e := s.createSmartBlock(id)
		if e != nil {
			return e
		}
		s.openedBlocks[id] = &openedBlock{
			SmartBlock: sb,
			lastUsage:  time.Now(),
			refs:       1,
		}
	}
	/*
		for _, bid := range breadcrumbsIds {
			if b, ok := s.openedBlocks[bid]; ok {
				if bs, ok := b.smartBlock.(*breadcrumbs); ok {
					bs.OnSmartOpen(id)
				} else {
					log.Warningf("unexpected smart block type %T; wand breadcrumbs", b)
				}
			} else {
				log.Warningf("breadcrumbs block not found")
			}
		}*/
	return nil
}

func (s *service) OpenBreadcrumbsBlock() (blockId string, err error) {
	s.m.Lock()
	defer s.m.Unlock()
	/*
		bs := newBreadcrumbs(s)
		if err = bs.Open(nil, false); err != nil {
			return
		}
		bs.Init()
		s.openedBlocks[bs.GetId()] = &openedBlock{
			smartBlock: bs,
			lastUsage:  time.Now(),
			refs:       1,
		}
		return bs.GetId(), nil

	*/
	return
}

func (s *service) CloseBlock(id string) (err error) {
	s.m.Lock()
	defer s.m.Unlock()
	if ob, ok := s.openedBlocks[id]; ok {
		ob.SetEventFunc(nil)
		ob.refs--
		return
	}
	return ErrBlockNotFound
}

func (s *service) SetPageIsArchived(req pb.RpcBlockSetPageIsArchivedRequest) (err error) {
	/*
		archiveId := s.anytype.PredefinedBlocks().Archive
		sb, release, err := s.pickBlock(archiveId)
		if err != nil {
			return
		}
		defer release()
		if archiveBlock, ok := sb.(*archive); ok {
			if req.IsArchived {
				err = archiveBlock.archivePage(req.BlockId)
			} else {
				err = archiveBlock.unArchivePage(req.BlockId)
			}
			return
		}
		return fmt.Errorf("unexpected archive block type: %T", sb)
	*/
	return
}

func (s *service) CutBreadcrumbs(req pb.RpcBlockCutBreadcrumbsRequest) (err error) {
	/*
		sb, release, err := s.pickBlock(req.BreadcrumbsId)
		if err != nil {
			return
		}
		defer release()
		if bc, ok := sb.(*breadcrumbs); ok {
			return bc.BreadcrumbsCut(int(req.Index))
		}
		return ErrUnexpectedSmartBlockType

	*/
	return
}

func (s *service) CreateBlock(req pb.RpcBlockCreateRequest) (id string, err error) {
	err = s.DoBasic(req.ContextId, func(b basic.Basic) error {
		id, err = b.Create(req)
		return err
	})
	return
}

func (s *service) CreatePage(req pb.RpcBlockCreatePageRequest) (id string, targetId string, err error) {
	/*
		sb, release, err := s.pickBlock(req.ContextId)
		if err != nil {
			return
		}
		defer release()
		return sb.CreatePage(req)
		*
	*/
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
		blockId, err = b.Split(req.BlockId, req.CursorPosition)
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
	return s.DoBasic(req.ContextId, func(b basic.Basic) error {
		return b.Move(req)
	})
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

func (s *service) SetFieldsList(req pb.RpcBlockListSetFieldsRequest) (err error) {
	return s.DoBasic(req.ContextId, func(b basic.Basic) error {
		return b.SetFields(req.BlockFields...)
	})
}

func (s *service) Copy(req pb.RpcBlockCopyRequest) (html string, err error) {
	/*sb, release, err := s.pickBlock(req.ContextId)
	if err != nil {
		return
	}
	defer release()
	return sb.Copy(req)

	*/
	return
}

func (s *service) Paste(req pb.RpcBlockPasteRequest) (blockIds []string, err error) {
	/*sb, release, err := s.pickBlock(req.ContextId)
	if err != nil {
		return
	}
	defer release()
	return sb.Paste(req)

	*/
	return
}

func (s *service) Cut(req pb.RpcBlockCutRequest) (textSlot string, htmlSlot string, anySlot []*model.Block, err error) {
	/*	sb, release, err := s.pickBlock(req.ContextId)
		if err != nil {
			return
		}
		defer release()
		return sb.Cut(req)

	*/
	return
}

func (s *service) Export(req pb.RpcBlockExportRequest) (path string, err error) {
	/*	sb, release, err := s.pickBlock(req.ContextId)
		if err != nil {
			return
		}
		defer release()
		return sb.Export(req)

	*/
	return
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
		return b.UpdateTextBlocks([]string{req.ContextId}, true, func(t text.Block) error {
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

func (s *service) UploadFile(req pb.RpcBlockUploadRequest) (err error) {
	return s.DoFile(req.ContextId, func(b file.File) error {
		return b.Upload(req.BlockId, req.FilePath, req.Url)
	})
}

func (s *service) DropFiles(req pb.RpcExternalDropFilesRequest) (err error) {
	return s.DoFile(req.ContextId, func(b file.File) error {
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
	sc, err := source.NewSource(s.anytype, id)
	if err != nil {
		return
	}
	switch sc.Type() {
	case core.SmartBlockTypePage:
		sb = editor.NewPage()
	case core.SmartBlockTypeDashboard:
		sb = editor.NewDashboard()
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

func (s *service) DoClipboard(id string, apply func(b cipboard.Clipboard) error) error {
	sb, release, err := s.pickBlock(id)
	if err != nil {
		return err
	}
	defer release()
	if bb, ok := sb.(cipboard.Clipboard); ok {
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
	var closedCount int
	for id, ob := range s.openedBlocks {
		if ob.refs == 0 && time.Now().After(ob.lastUsage.Add(blockCacheTTL)) {
			if err := ob.Close(); err != nil {
				log.Warnf("error while close block[%s]: %v", id, err)
			}
			delete(s.openedBlocks, id)
			closedCount++
		}
	}
	log.Infof("cleanup: block closed %d", closedCount)
	return s.closed
}
