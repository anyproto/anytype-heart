package anystorage

import (
	"context"
	"errors"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/commonspace/headsync/headstorage"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
)

type ClientSpaceStorage interface {
	spacestorage.SpaceStorage
	HasTree(ctx context.Context, id string) (has bool, err error)
	TreeRoot(ctx context.Context, id string) (root *treechangeproto.RawTreeChangeWithId, err error)
	MarkSpaceCreated(ctx context.Context) error
	IsSpaceCreated(ctx context.Context) (created bool, err error)
	UnmarkSpaceCreated(ctx context.Context) error
	AllDeletedTreeIds(ctx context.Context) (ids []string, err error)
}

var _ ClientSpaceStorage = (*clientStorage)(nil)

const (
	clientCollectionKey = "_client"
	clientDocumentKey   = "space"
	createdKey          = "created"
	rawChangeKey        = "r"
)

type clientStorage struct {
	spacestorage.SpaceStorage
	clientColl anystore.Collection
}

func (r *clientStorage) AllDeletedTreeIds(ctx context.Context) (ids []string, err error) {
	err = r.SpaceStorage.HeadStorage().IterateEntries(ctx, headstorage.IterOpts{Deleted: true}, func(entry headstorage.HeadsEntry) (bool, error) {
		ids = append(ids, entry.Id)
		return true, nil
	})
	return
}

func NewClientStorage(ctx context.Context, spaceStorage spacestorage.SpaceStorage) (*clientStorage, error) {
	storage := &clientStorage{
		SpaceStorage: spaceStorage,
	}
	return storage, nil
}

func (r *clientStorage) Close(ctx context.Context) (err error) {
	return r.SpaceStorage.Close(ctx)
}

func (r *clientStorage) HasTree(ctx context.Context, id string) (has bool, err error) {
	_, err = r.SpaceStorage.HeadStorage().GetEntry(ctx, id)
	isNotFound := errors.Is(err, anystore.ErrDocNotFound)
	if err != nil && !isNotFound {
		return false, err
	}
	return !isNotFound, nil
}

func (r *clientStorage) TreeRoot(ctx context.Context, id string) (root *treechangeproto.RawTreeChangeWithId, err error) {
	ts, err := r.TreeStorage(ctx, id)
	if err != nil {
		return nil, err
	}
	res, err := ts.Root(ctx)
	if err != nil {
		return nil, err
	}
	return res.RawTreeChangeWithId(), nil
}

func (r *clientStorage) MarkSpaceCreated(ctx context.Context) error {
	return nil
}

func (r *clientStorage) IsSpaceCreated(ctx context.Context) (isCreated bool, err error) {
	return
}

func (r *clientStorage) UnmarkSpaceCreated(ctx context.Context) error {
	return nil
}
