package block

import (
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/file"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

var (
	ErrBlockNotFound       = errors.New("block not found")
	ErrBlockAlreadyOpen    = errors.New("block already open")
	ErrUnexpectedBlockType = errors.New("unexpected block type")
)

type Service interface {
	OpenBlock(id string) error
	CloseBlock(id string) error
	CreateBlock(req pb.RpcBlockCreateRequest) (string, error)
	DuplicateBlock(req pb.RpcBlockDuplicateRequest) (string, error)
	UnlinkBlock(req pb.RpcBlockUnlinkRequest) error
	ReplaceBlock(req pb.RpcBlockReplaceRequest) error

	MoveBlocks(req pb.RpcBlockListMoveRequest) error

	SetFields(req pb.RpcBlockSetFieldsRequest) error

	SplitBlock(req pb.RpcBlockSplitRequest) (blockId string, err error)
	MergeBlock(req pb.RpcBlockMergeRequest) error
	SetTextText(req pb.RpcBlockSetTextTextRequest) error
	SetTextStyle(req pb.RpcBlockSetTextStyleRequest) error
	SetTextChecked(req pb.RpcBlockSetTextCheckedRequest) error
	SetTextColor(req pb.RpcBlockSetTextColorRequest) error
	SetTextBackgroundColor(req pb.RpcBlockSetTextBackgroundColorRequest) error

	UploadFile(req pb.RpcBlockUploadRequest) error

	SetIconName(req pb.RpcBlockSetIconNameRequest) error

	Close() error
}

func NewService(accountId string, lib anytype.Anytype, sendEvent func(event *pb.Event)) Service {
	return &service{
		accountId: accountId,
		anytype:   lib,
		sendEvent: func(event *pb.Event) {
			sendEvent(event)
		},
		smartBlocks: make(map[string]smartBlock),
	}
}

type service struct {
	anytype     anytype.Anytype
	accountId   string
	sendEvent   func(event *pb.Event)
	smartBlocks map[string]smartBlock
	m           sync.RWMutex
}

func (s *service) OpenBlock(id string) (err error) {
	s.m.Lock()
	defer s.m.Unlock()
	if _, ok := s.smartBlocks[id]; ok {
		return ErrBlockAlreadyOpen
	}
	sb, err := openSmartBlock(s, id)
	fmt.Println("middle: open smart block:", id, err)
	if err != nil {
		return
	}
	s.smartBlocks[id] = sb
	return nil
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

func (s *service) CreateBlock(req pb.RpcBlockCreateRequest) (string, error) {
	s.m.RLock()
	defer s.m.RUnlock()
	if sb, ok := s.smartBlocks[req.ContextId]; ok {
		return sb.Create(req)
	}
	return "", ErrBlockNotFound
}

func (s *service) DuplicateBlock(req pb.RpcBlockDuplicateRequest) (string, error) {
	s.m.RLock()
	defer s.m.RUnlock()
	if sb, ok := s.smartBlocks[req.ContextId]; ok {
		return sb.Duplicate(req)
	}
	return "", ErrBlockNotFound
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

func (s *service) ReplaceBlock(req pb.RpcBlockReplaceRequest) error {
	s.m.RLock()
	defer s.m.RUnlock()
	if sb, ok := s.smartBlocks[req.ContextId]; ok {
		return sb.Replace(req.BlockId, req.Block)
	}
	return ErrBlockNotFound
}

func (s *service) SetFields(req pb.RpcBlockSetFieldsRequest) (err error) {
	s.m.RLock()
	defer s.m.RUnlock()
	if sb, ok := s.smartBlocks[req.ContextId]; ok {
		return sb.SetFields(req.BlockId, req.Fields)
	}
	return ErrBlockNotFound
}

func (s *service) SetTextText(req pb.RpcBlockSetTextTextRequest) error {
	return s.updateTextBlock(req.ContextId, req.BlockId, func(b text.Block) error {
		return b.SetText(req.Text, req.Marks)
	})
}

func (s *service) SetTextStyle(req pb.RpcBlockSetTextStyleRequest) error {
	return s.updateTextBlock(req.ContextId, req.BlockId, func(b text.Block) error {
		b.SetStyle(req.Style)
		return nil
	})
}

func (s *service) SetTextChecked(req pb.RpcBlockSetTextCheckedRequest) error {
	return s.updateTextBlock(req.ContextId, req.BlockId, func(b text.Block) error {
		b.SetChecked(req.Checked)
		return nil
	})
}

func (s *service) SetTextColor(req pb.RpcBlockSetTextColorRequest) error {
	return s.updateTextBlock(req.ContextId, req.BlockId, func(b text.Block) error {
		b.SetTextColor(req.Color)
		return nil
	})
}

func (s *service) SetTextBackgroundColor(req pb.RpcBlockSetTextBackgroundColorRequest) error {
	return s.updateTextBlock(req.ContextId, req.BlockId, func(b text.Block) error {
		b.SetTextBackgroundColor(req.Color)
		return nil
	})
}

func (s *service) updateTextBlock(contextId, blockId string, apply func(b text.Block) error) (err error) {
	s.m.RLock()
	defer s.m.RUnlock()
	sb, ok := s.smartBlocks[contextId]
	if !ok {
		err = ErrBlockNotFound
		return
	}
	return sb.UpdateTextBlock(blockId, apply)
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

func (s *service) Close() error {
	s.m.Lock()
	defer s.m.Unlock()
	for _, sb := range s.smartBlocks {
		if err := sb.Close(); err != nil {
			log.Printf("block[%s] close error: %v", sb.GetId(), err)
		}
	}
	return nil
}
