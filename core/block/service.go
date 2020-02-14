package block

import (
	"errors"
	"fmt"
	"sync"

	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/bookmark"
	"github.com/anytypeio/go-anytype-middleware/util/linkpreview"
	logging "github.com/ipfs/go-log"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
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

	Paste(req pb.RpcBlockPasteRequest) error

	SplitBlock(req pb.RpcBlockSplitRequest) (blockId string, err error)
	MergeBlock(req pb.RpcBlockMergeRequest) error
	SetTextText(req pb.RpcBlockSetTextTextRequest) error
	SetTextStyle(contextId string, style model.BlockContentTextStyle, blockIds ...string) error
	SetTextChecked(req pb.RpcBlockSetTextCheckedRequest) error
	SetTextColor(contextId string, color string, blockIds ...string) error
	SetTextBackgroundColor(contextId string, color string, blockIds ...string) error

	UploadFile(req pb.RpcBlockUploadRequest) error

	SetIconName(req pb.RpcBlockSetIconNameRequest) error

	Undo(req pb.RpcBlockUndoRequest) error
	Redo(req pb.RpcBlockRedoRequest) error

	SetPageIsArchived(req pb.RpcBlockSetPageIsArchivedRequest) error

	BookmarkFetch(req pb.RpcBlockBookmarkFetchRequest) error

	Close() error
}

func NewService(accountId string, a anytype.Anytype, lp linkpreview.LinkPreview, sendEvent func(event *pb.Event)) Service {
	return &service{
		accountId: accountId,
		anytype:   a,
		sendEvent: func(event *pb.Event) {
			sendEvent(event)
		},
		smartBlocks: make(map[string]smartBlock),
		ls:          newLinkSubscriptions(a),
		linkPreview: lp,
	}
}

type service struct {
	anytype     anytype.Anytype
	accountId   string
	sendEvent   func(event *pb.Event)
	smartBlocks map[string]smartBlock
	linkPreview linkpreview.LinkPreview
	ls          *linkSubscriptions
	m           sync.RWMutex
}

func (s *service) OpenBlock(id string, breadcrumbsIds ...string) (err error) {
	s.m.Lock()
	defer s.m.Unlock()
	if sb, ok := s.smartBlocks[id]; ok {
		return sb.Show()
	}
	sb, err := openSmartBlock(s, id, true)
	fmt.Println("middle: open smart block:", id, err)
	if err != nil {
		return
	}
	s.smartBlocks[id] = sb
	for _, bid := range breadcrumbsIds {
		if b, ok := s.smartBlocks[bid]; ok {
			if bs, ok := b.(*breadcrumbs); ok {
				bs.OnSmartOpen(id)
			}
		}
	}
	return nil
}

func (s *service) OpenBreadcrumbsBlock() (blockId string, err error) {
	s.m.Lock()
	defer s.m.Unlock()

	bs := newBreadcrumbs(s)
	if err = bs.Open(nil, false); err != nil {
		return
	}
	bs.Init()
	s.smartBlocks[bs.GetId()] = bs
	return bs.GetId(), nil
}

func (s *service) CloseBlock(id string) (err error) {
	s.m.Lock()
	defer s.m.Unlock()
	if sb, ok := s.smartBlocks[id]; ok {
		delete(s.smartBlocks, id)
		fmt.Println("middle: close smart block:", id, err)
		return sb.Close()
	}

	return ErrBlockNotFound
}

func (s *service) SetPageIsArchived(req pb.RpcBlockSetPageIsArchivedRequest) (err error) {
	s.m.RLock()
	defer s.m.RUnlock()
	archiveId := s.anytype.PredefinedBlockIds().Archive
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
}

func (s *service) CutBreadcrumbs(req pb.RpcBlockCutBreadcrumbsRequest) (err error) {
	s.m.Lock()
	defer s.m.Unlock()
	if sb, ok := s.smartBlocks[req.BreadcrumbsId]; ok {
		if bc, ok := sb.(*breadcrumbs); ok {
			return bc.Cut(int(req.Index))
		}
	}
	return ErrBlockNotFound
}

func (s *service) CreateBlock(req pb.RpcBlockCreateRequest) (string, error) {
	s.m.RLock()
	defer s.m.RUnlock()
	if sb, ok := s.smartBlocks[req.ContextId]; ok {
		return sb.Create(req)
	}
	return "", ErrBlockNotFound
}

func (s *service) CreatePage(req pb.RpcBlockCreatePageRequest) (string, string, error) {
	s.m.RLock()
	defer s.m.RUnlock()
	if sb, ok := s.smartBlocks[req.ContextId]; ok {
		return sb.CreatePage(req)
	}
	return "", "", ErrBlockNotFound
}

func (s *service) DuplicateBlocks(req pb.RpcBlockListDuplicateRequest) ([]string, error) {
	s.m.RLock()
	defer s.m.RUnlock()
	if sb, ok := s.smartBlocks[req.ContextId]; ok {
		return sb.Duplicate(req)
	}
	return nil, ErrBlockNotFound
}

func (s *service) UnlinkBlock(req pb.RpcBlockUnlinkRequest) error {
	s.m.RLock()
	defer s.m.RUnlock()
	if sb, ok := s.smartBlocks[req.ContextId]; ok {
		return sb.Unlink(req.BlockIds...)
	}
	return ErrBlockNotFound
}

func (s *service) SplitBlock(req pb.RpcBlockSplitRequest) (blockId string, err error) {
	s.m.RLock()
	defer s.m.RUnlock()
	if sb, ok := s.smartBlocks[req.ContextId]; ok {
		return sb.Split(req.BlockId, req.CursorPosition)
	}
	return "", ErrBlockNotFound
}

func (s *service) MergeBlock(req pb.RpcBlockMergeRequest) error {
	s.m.RLock()
	defer s.m.RUnlock()
	if sb, ok := s.smartBlocks[req.ContextId]; ok {
		return sb.Merge(req.FirstBlockId, req.SecondBlockId)
	}
	return ErrBlockNotFound
}

func (s *service) MoveBlocks(req pb.RpcBlockListMoveRequest) error {
	s.m.RLock()
	defer s.m.RUnlock()
	if sb, ok := s.smartBlocks[req.ContextId]; ok {
		return sb.Move(req)
	}
	return ErrBlockNotFound
}

func (s *service) ReplaceBlock(req pb.RpcBlockReplaceRequest) (newId string, err error) {
	s.m.RLock()
	defer s.m.RUnlock()
	if sb, ok := s.smartBlocks[req.ContextId]; ok {
		return sb.Replace(req.BlockId, req.Block)
	}
	return "", ErrBlockNotFound
}

func (s *service) SetFields(req pb.RpcBlockSetFieldsRequest) (err error) {
	s.m.RLock()
	defer s.m.RUnlock()
	if sb, ok := s.smartBlocks[req.ContextId]; ok {
		return sb.SetFields(&pb.RpcBlockListSetFieldsRequestBlockField{
			BlockId: req.BlockId,
			Fields:  req.Fields,
		})
	}
	return ErrBlockNotFound
}

func (s *service) SetFieldsList(req pb.RpcBlockListSetFieldsRequest) (err error) {
	s.m.RLock()
	defer s.m.RUnlock()
	if sb, ok := s.smartBlocks[req.ContextId]; ok {
		return sb.SetFields(req.BlockFields...)
	}
	return ErrBlockNotFound
}

func (s *service) Paste(req pb.RpcBlockPasteRequest) error {
	s.m.RLock()
	defer s.m.RUnlock()
	if sb, ok := s.smartBlocks[req.ContextId]; ok {
		return sb.Paste(req)
	}
	return ErrBlockNotFound
}

func (s *service) SetTextText(req pb.RpcBlockSetTextTextRequest) error {
	return s.updateTextBlock(req.ContextId, []string{req.BlockId}, false, func(b text.Block) error {
		return b.SetText(req.Text, req.Marks)
	})
}

func (s *service) SetTextStyle(contextId string, style model.BlockContentTextStyle, blockIds ...string) error {
	return s.updateTextBlock(contextId, blockIds, true, func(b text.Block) error {
		b.SetStyle(style)
		return nil
	})
}

func (s *service) SetTextChecked(req pb.RpcBlockSetTextCheckedRequest) error {
	return s.updateTextBlock(req.ContextId, []string{req.BlockId}, true, func(b text.Block) error {
		b.SetChecked(req.Checked)
		return nil
	})
}

func (s *service) SetTextColor(contextId, color string, blockIds ...string) error {
	return s.updateTextBlock(contextId, blockIds, true, func(b text.Block) error {
		b.SetTextColor(color)
		return nil
	})
}

func (s *service) SetTextBackgroundColor(contextId, color string, blockIds ...string) error {
	return s.updateTextBlock(contextId, blockIds, true, func(b text.Block) error {
		b.SetTextBackgroundColor(color)
		return nil
	})
}

func (s *service) updateTextBlock(contextId string, blockIds []string, event bool, apply func(b text.Block) error) (err error) {
	s.m.RLock()
	defer s.m.RUnlock()
	sb, ok := s.smartBlocks[contextId]
	if !ok {
		err = ErrBlockNotFound
		return
	}
	return sb.UpdateTextBlocks(blockIds, event, apply)
}

func (s *service) SetIconName(req pb.RpcBlockSetIconNameRequest) (err error) {
	s.m.RLock()
	defer s.m.RUnlock()
	sb, ok := s.smartBlocks[req.ContextId]
	if !ok {
		err = ErrBlockNotFound
		return
	}
	return sb.UpdateIconBlock(req.BlockId, func(t base.IconBlock) error {
		return t.SetIconName(req.Name)
	})
}

func (s *service) UploadFile(req pb.RpcBlockUploadRequest) error {
	s.m.RLock()
	defer s.m.RUnlock()
	sb, ok := s.smartBlocks[req.ContextId]
	if !ok {
		return ErrBlockNotFound
	}
	return sb.Upload(req.BlockId, req.FilePath, req.Url)
}

func (s *service) Undo(req pb.RpcBlockUndoRequest) error {
	s.m.RLock()
	defer s.m.RUnlock()
	sb, ok := s.smartBlocks[req.ContextId]
	if !ok {
		return ErrBlockNotFound
	}
	return sb.Undo()
}

func (s *service) Redo(req pb.RpcBlockRedoRequest) error {
	s.m.RLock()
	defer s.m.RUnlock()
	sb, ok := s.smartBlocks[req.ContextId]
	if !ok {
		return ErrBlockNotFound
	}
	return sb.Redo()
}

func (s *service) BookmarkFetch(req pb.RpcBlockBookmarkFetchRequest) error {
	s.m.RLock()
	defer s.m.RUnlock()
	sb, ok := s.smartBlocks[req.ContextId]
	if !ok {
		return ErrBlockNotFound
	}
	return sb.UpdateBlock([]string{req.BlockId}, true, func(b simple.Block) error {
		if bm, ok := b.(bookmark.Block); ok {
			return bm.Fetch(bookmark.FetchParams{
				Url:         req.Url,
				Anytype:     s.anytype,
				Updater:     sb,
				LinkPreview: s.linkPreview,
			})
		}
		return ErrUnexpectedBlockType
	})
}

func (s *service) Close() error {
	s.m.Lock()
	defer s.m.Unlock()
	for _, sb := range s.smartBlocks {
		if err := sb.Close(); err != nil {
			log.Errorf("block[%s] close error: %v", sb.GetId(), err)
		}
	}
	return nil
}

// pickBlock returns opened smartBlock or opens smartBlock in silent mode
// must be called and released under RLock
func (s *service) pickBlock(id string) (sb smartBlock, release func(), err error) {
	if b, ok := s.smartBlocks[id]; ok {
		return b, func() {}, nil
	}
	if sb, err = openSmartBlock(s, id, false); err != nil {
		return
	}
	return sb, func() {
		sb.Close()
	}, nil
}
