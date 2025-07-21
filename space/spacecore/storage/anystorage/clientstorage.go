package anystorage

import (
	"context"
	"errors"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"
	"github.com/anyproto/any-sync/commonspace/headsync/headstorage"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
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
	anyStore := storage.AnyStore()
	client, err := anyStore.Collection(ctx, clientCollectionKey)
	if err != nil {
		return nil, err
	}
	storage.clientColl = client
	return storage, nil
}

func (r *clientStorage) Close(ctx context.Context) (err error) {
	spaceStorageErr := r.SpaceStorage.Close(ctx)
	anyStoreErr := r.SpaceStorage.AnyStore().Close()
	return errors.Join(spaceStorageErr, anyStoreErr)
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
	// it should be faster to do it that way, instead of calling TreeStorage
	coll, err := r.SpaceStorage.AnyStore().OpenCollection(ctx, objecttree.CollName)
	if err != nil {
		return nil, err
	}
	doc, err := coll.FindId(ctx, id)
	if err != nil {
		return nil, err
	}
	return &treechangeproto.RawTreeChangeWithId{
		Id:        id,
		RawChange: doc.Value().GetBytes(rawChangeKey),
	}, nil
}

func (r *clientStorage) MarkSpaceCreated(ctx context.Context) error {
	return r.modifyState(ctx, true)
}

func (r *clientStorage) IsSpaceCreated(ctx context.Context) (isCreated bool, err error) {
	doc, err := r.clientColl.FindId(ctx, clientDocumentKey)
	isNotFound := errors.Is(err, anystore.ErrDocNotFound)
	if err != nil && !isNotFound {
		return false, err
	}
	if isNotFound {
		return false, nil
	}
	return doc.Value().GetBool(createdKey), nil
}

func (r *clientStorage) UnmarkSpaceCreated(ctx context.Context) error {
	return r.modifyState(ctx, false)
}

func (r *clientStorage) modifyState(ctx context.Context, isCreated bool) error {
	tx, err := r.clientColl.WriteTx(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()
	arena := &anyenc.Arena{}
	val := arena.NewTrue()
	if !isCreated {
		val = arena.NewFalse()
	}
	mod := query.ModifyFunc(func(a *anyenc.Arena, v *anyenc.Value) (result *anyenc.Value, modified bool, err error) {
		v.Set(createdKey, val)
		return v, true, nil
	})
	_, err = r.clientColl.UpsertId(tx.Context(), clientDocumentKey, mod)
	if err != nil {
		return err
	}
	return tx.Commit()
}
