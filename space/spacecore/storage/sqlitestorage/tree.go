package sqlitestorage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/commonspace/spacestorage/oldstorage"
)

func newTreeStorage(ss *spaceStorage, treeId string) (oldstorage.TreeStorage, error) {
	ts := &treeStorage{
		treeId:       treeId,
		spaceStorage: ss,
		service:      ss.service,
	}
	var headsPayload []byte
	if err := ss.service.stmt.loadTreeHeads.QueryRow(treeId).Scan(&headsPayload); err != nil {
		return nil, replaceNoRowsErr(err, treestorage.ErrUnknownTreeId)
	}
	ts.heads = treestorage.ParseHeads(headsPayload)
	return ts, nil
}

func createTreeStorage(ss *spaceStorage, payload treestorage.TreeStorageCreatePayload) (ts oldstorage.TreeStorage, err error) {
	ts = &treeStorage{
		treeId:       payload.RootRawChange.Id,
		spaceStorage: ss,
		service:      ss.service,
		heads:        payload.Heads,
	}

	tx, err := ss.service.writeDb.Begin()
	if err != nil {
		return nil, err
	}
	if _, err := tx.Stmt(ss.service.stmt.createTree).Exec(
		payload.RootRawChange.Id,
		ss.spaceId,
		treestorage.CreateHeadsPayload(payload.Heads),
		treeTypeTree,
	); err != nil {
		_ = tx.Rollback()
		if isUniqueConstraint(err) {
			return nil, treestorage.ErrTreeExists
		}
		return nil, err
	}
	if _, err = tx.Stmt(ss.service.stmt.createChange).Exec(
		// root change (id, spaceId, treeId, data)
		payload.RootRawChange.Id,
		ss.spaceId,
		payload.RootRawChange.Id,
		payload.RootRawChange.RawChange,
	); err != nil {
		_ = tx.Rollback()
		return nil, err
	}

	for _, change := range payload.Changes {
		_, chErr := tx.Stmt(ss.service.stmt.createChange).Exec(change.Id, ss.spaceId, ts.Id(), change.RawChange)
		if chErr != nil {
			if isUniqueConstraint(chErr) {
				continue
			}
			_ = tx.Rollback()
			return nil, chErr
		}
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return ts, nil
}

type treeStorage struct {
	treeId       string
	heads        []string
	spaceStorage *spaceStorage
	service      *storageService

	mu sync.RWMutex
}

func (t *treeStorage) Id() string {
	return t.treeId
}

func (t *treeStorage) Root() (*treechangeproto.RawTreeChangeWithId, error) {
	return t.spaceStorage.TreeRoot(t.treeId)
}

func (t *treeStorage) Heads() ([]string, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.heads, nil
}

func (t *treeStorage) GetAllChangeIds() (chs []string, err error) {
	rows, err := t.service.stmt.listChanges.Query(t.treeId)
	if err != nil {
		return nil, replaceNoRowsErr(err, nil)
	}
	for rows.Next() {
		var id string
		err = rows.Scan(&id)
		if err != nil {
			return nil, errors.Join(rows.Close(), err)
		}
		chs = append(chs, id)
	}
	return chs, rows.Close()
}

func (t *treeStorage) GetAllChanges() ([]*treechangeproto.RawTreeChangeWithId, error) {
	var changes []*treechangeproto.RawTreeChangeWithId
	err := t.IterateChanges(func(id string, rawChange []byte) error {
		changes = append(changes, &treechangeproto.RawTreeChangeWithId{
			Id:        id,
			RawChange: bytes.Clone(rawChange),
		})
		return nil
	})
	return changes, err
}

func (t *treeStorage) IterateChanges(proc func(id string, rawChange []byte) error) error {
	rows, err := t.service.stmt.iterateChanges.Query(t.treeId)
	if err != nil {
		return replaceNoRowsErr(err, nil)
	}

	buf := make([]byte, 0, 1024)
	for rows.Next() {
		var id string
		err = rows.Scan(&id, &buf)
		if err != nil {
			return errors.Join(rows.Close(), err)
		}

		err = proc(id, buf)
		if err != nil {
			return errors.Join(rows.Close(), err)
		}
		buf = buf[:0]
	}
	return rows.Close()
}

func (t *treeStorage) SetHeads(heads []string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, err := t.service.stmt.updateTreeHeads.Exec(treestorage.CreateHeadsPayload(heads), t.treeId)
	if err != nil {
		return replaceNoRowsErr(err, fmt.Errorf("set heads: %w", ErrTreeNotFound))
	}
	t.heads = heads
	return nil
}

func (t *treeStorage) AddRawChange(change *treechangeproto.RawTreeChangeWithId) error {
	_, err := t.service.stmt.createChange.Exec(change.Id, t.spaceStorage.spaceId, t.treeId, change.RawChange)
	if err != nil && isUniqueConstraint(err) {
		return nil
	}
	return err
}

func (t *treeStorage) AddRawChangesSetHeads(changes []*treechangeproto.RawTreeChangeWithId, heads []string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	tx, err := t.service.writeDb.Begin()
	if err != nil {
		return err
	}

	for _, change := range changes {
		_, chErr := tx.Stmt(t.service.stmt.createChange).Exec(change.Id, t.spaceStorage.spaceId, t.treeId, change.RawChange)
		if chErr != nil {
			if isUniqueConstraint(chErr) {
				continue
			}
			_ = tx.Rollback()
			return chErr
		}
	}

	_, err = tx.Stmt(t.service.stmt.updateTreeHeads).Exec(treestorage.CreateHeadsPayload(heads), t.treeId)
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	if err = tx.Commit(); err != nil {
		return err
	}
	t.heads = heads
	return nil
}

func (t *treeStorage) GetRawChange(ctx context.Context, id string) (*treechangeproto.RawTreeChangeWithId, error) {
	ch, err := t.spaceStorage.TreeRoot(id)
	if err != nil {
		return nil, replaceNoRowsErr(err, treestorage.ErrUnknownChange)
	}
	return ch, nil
}

func (t *treeStorage) GetAppendRawChange(ctx context.Context, buf []byte, id string) (*treechangeproto.RawTreeChangeWithId, error) {
	return t.GetRawChange(ctx, id)
}

func (t *treeStorage) HasChange(ctx context.Context, id string) (bool, error) {
	var res int
	if err := t.service.stmt.hasChange.QueryRow(id, t.treeId).Scan(&res); err != nil {
		return false, replaceNoRowsErr(err, nil)
	}
	return res > 0, nil
}

func (t *treeStorage) Delete() error {
	tx, err := t.service.writeDb.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Stmt(t.service.stmt.deleteTree).Exec(t.treeId, t.spaceStorage.spaceId); err != nil {
		_ = tx.Rollback()
		return err
	}

	if _, err := tx.Stmt(t.service.stmt.deleteChangesByTree).Exec(t.treeId); err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}
