package chats

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/chatobject"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const CName = "core.block.chats"

type Service interface {
	AddMessage(ctx context.Context, chatObjectId string, message *model.ChatMessage) (string, error)
	EditMessage(ctx context.Context, chatObjectId string, messageId string, newMessage *model.ChatMessage) error
	DeleteMessage(ctx context.Context, chatObjectId string, messageId string) error
	GetMessages(ctx context.Context, chatObjectId string, beforeOrderId string, limit int) ([]*model.ChatMessage, error)
	SubscribeLastMessages(ctx context.Context, chatObjectId string, limit int) ([]*model.ChatMessage, int, error)
	Unsubscribe(chatObjectId string) error

	GetStoreDb() anystore.DB

	app.ComponentRunnable
}

type service struct {
	repoPath string
	db       anystore.DB

	objectGetter cache.ObjectGetter
}

func New() Service {
	return &service{}
}

func (s *service) Name() string {
	return CName
}

func (s *service) Init(a *app.App) error {
	s.objectGetter = app.MustComponent[cache.ObjectGetter](a)
	s.repoPath = app.MustComponent[wallet.Wallet](a).RepoPath()

	return nil
}

func (s *service) Run(ctx context.Context) error {
	dbDir := filepath.Join(s.repoPath, "objectstore")
	_, err := os.Stat(dbDir)
	if errors.Is(err, os.ErrNotExist) {
		err = os.MkdirAll(dbDir, 0700)
		if err != nil {
			return fmt.Errorf("create db dir: %w", err)
		}
	}
	return s.runDatabase(ctx, filepath.Join(dbDir, "chats.db"))
}

func (s *service) runDatabase(ctx context.Context, path string) error {
	store, err := anystore.Open(ctx, path, nil)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	s.db = store
	return nil
}

func (s *service) Close(ctx context.Context) error {
	var err error
	if s.db != nil {
		err = errors.Join(err, s.db.Close())
	}
	return err

}

func (s *service) GetStoreDb() anystore.DB {
	return s.db
}

func (s *service) AddMessage(ctx context.Context, chatObjectId string, message *model.ChatMessage) (string, error) {
	var messageId string
	err := cache.Do(s.objectGetter, chatObjectId, func(sb chatobject.StoreObject) error {
		var err error
		messageId, err = sb.AddMessage(ctx, message)
		return err
	})
	return messageId, err
}

func (s *service) EditMessage(ctx context.Context, chatObjectId string, messageId string, newMessage *model.ChatMessage) error {
	return cache.Do(s.objectGetter, chatObjectId, func(sb chatobject.StoreObject) error {
		return sb.EditMessage(ctx, messageId, newMessage)
	})
}

func (s *service) DeleteMessage(ctx context.Context, chatObjectId string, messageId string) error {
	return cache.Do(s.objectGetter, chatObjectId, func(sb chatobject.StoreObject) error {
		return sb.DeleteMessage(ctx, messageId)
	})
}

func (s *service) GetMessages(ctx context.Context, chatObjectId string, beforeOrderId string, limit int) ([]*model.ChatMessage, error) {
	var res []*model.ChatMessage
	err := cache.Do(s.objectGetter, chatObjectId, func(sb chatobject.StoreObject) error {
		msgs, err := sb.GetMessages(ctx, beforeOrderId, limit)
		if err != nil {
			return err
		}
		res = msgs
		return nil
	})
	return res, err
}

func (s *service) SubscribeLastMessages(ctx context.Context, chatObjectId string, limit int) ([]*model.ChatMessage, int, error) {
	var (
		msgs      []*model.ChatMessage
		numBefore int
	)
	err := cache.Do(s.objectGetter, chatObjectId, func(sb chatobject.StoreObject) error {
		var err error
		msgs, numBefore, err = sb.SubscribeLastMessages(ctx, limit)
		if err != nil {
			return err
		}
		return nil
	})
	return msgs, numBefore, err
}

func (s *service) Unsubscribe(chatObjectId string) error {
	return cache.Do(s.objectGetter, chatObjectId, func(sb chatobject.StoreObject) error {
		return sb.Unsubscribe()
	})
}
