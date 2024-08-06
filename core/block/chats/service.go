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
	"github.com/anyproto/anytype-heart/core/block/editor/storeobject"
	"github.com/anyproto/anytype-heart/core/wallet"
)

const CName = "core.block.chats"

type Service interface {
	AddMessage(chatObjectId string, message string) (string, error)
	EditMessage(chatObjectId string, messageId string, newText string) error
	GetMessages(chatObjectId string) ([]string, error)

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

func (s *service) AddMessage(chatObjectId string, message string) (string, error) {
	var messageId string
	err := cache.Do(s.objectGetter, chatObjectId, func(sb storeobject.StoreObject) error {
		var err error
		messageId, err = sb.AddMessage(context.Background(), message)
		return err
	})
	return messageId, err
}

func (s *service) EditMessage(chatObjectId string, messageId string, newText string) error {
	return cache.Do(s.objectGetter, chatObjectId, func(sb storeobject.StoreObject) error {
		return sb.EditMessage(context.Background(), messageId, newText)
	})
}

func (s *service) GetMessages(chatObjectId string) ([]string, error) {
	var res []string
	err := cache.Do(s.objectGetter, chatObjectId, func(sb storeobject.StoreObject) error {
		msgs, err := sb.GetMessages(context.Background())
		if err != nil {
			return err
		}
		res = msgs
		return nil
	})
	return res, err
}
