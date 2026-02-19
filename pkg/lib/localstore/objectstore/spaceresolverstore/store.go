package spaceresolverstore

import (
	"context"
	"errors"
	"fmt"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"

	"github.com/anyproto/anytype-heart/core/domain"
)

const bindKey = "b"

type Store interface {
	BindSpaceId(spaceId, objectId string) error
	GetSpaceId(objectId string) (spaceId string, err error)
}

type dsObjectStore struct {
	componentCtx context.Context
	collection   anystore.Collection
	arenaPool    *anyenc.ArenaPool
}

func New(componentCtx context.Context, db anystore.DB) (Store, error) {
	collection, err := db.Collection(componentCtx, "bindId")
	if err != nil {
		return nil, fmt.Errorf("open bindId collection: %w", err)
	}
	return &dsObjectStore{
		componentCtx: componentCtx,
		arenaPool:    &anyenc.ArenaPool{},
		collection:   collection,
	}, nil
}

func (d *dsObjectStore) BindSpaceId(spaceId, objectId string) error {
	return d.modifyBind(d.componentCtx, objectId, spaceId)
}

func (d *dsObjectStore) GetSpaceId(objectId string) (spaceId string, err error) {
	doc, err := d.collection.FindId(d.componentCtx, objectId)
	if err != nil {
		if errors.Is(err, anystore.ErrDocNotFound) {
			return "", domain.ErrObjectNotFound
		}
		return "", err
	}
	return doc.Value().GetString(bindKey), nil
}

func (d *dsObjectStore) modifyBind(ctx context.Context, objectId, spaceId string) error {
	tx, err := d.collection.WriteTx(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()
	arena := d.arenaPool.Get()
	defer d.arenaPool.Put(arena)
	mod := query.ModifyFunc(func(a *anyenc.Arena, v *anyenc.Value) (result *anyenc.Value, modified bool, err error) {
		if v.GetString(bindKey) == spaceId {
			return v, false, nil
		}
		v.Set(bindKey, arena.NewString(spaceId))
		return v, true, nil
	})
	_, err = d.collection.UpsertId(tx.Context(), objectId, mod)
	if err != nil {
		return err
	}
	return tx.Commit()
}
