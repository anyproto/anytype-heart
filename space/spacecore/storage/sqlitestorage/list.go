package sqlitestorage

import (
	"context"
	"database/sql"
	"errors"
	"sync"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/commonspace/spacestorage/oldstorage"
	"github.com/anyproto/any-sync/consensus/consensusproto"
)

var ErrUnknownRecord = errors.New("record does not exist")

func newListStorage(ss *spaceStorage, treeId string) (oldstorage.ListStorage, error) {
	ts := &listStorage{
		listId:       treeId,
		spaceStorage: ss,
		service:      ss.service,
	}
	if err := ss.service.stmt.loadTreeHeads.QueryRow(treeId).Scan(&ts.head); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, treestorage.ErrUnknownTreeId
		}
		return nil, err
	}
	return ts, nil
}

type listStorage struct {
	listId       string
	head         string
	spaceStorage *spaceStorage
	service      *storageService

	mu sync.RWMutex
}

func (t *listStorage) Root() (*consensusproto.RawRecordWithId, error) {
	tch, err := t.spaceStorage.TreeRoot(t.listId)
	if err != nil {
		return nil, replaceNoRowsErr(err, ErrUnknownRecord)
	}
	return &consensusproto.RawRecordWithId{
		Payload: tch.RawChange,
		Id:      tch.Id,
	}, nil
}

func (t *listStorage) Head() (string, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.head, nil
}

func (t *listStorage) SetHead(headId string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, err := t.service.stmt.updateTreeHeads.Exec(headId, t.listId)
	if err != nil {
		return err
	}
	t.head = headId
	return nil
}

func (t *listStorage) GetRawRecord(ctx context.Context, id string) (*consensusproto.RawRecordWithId, error) {
	tch, err := t.spaceStorage.TreeRoot(id)
	if err != nil {
		return nil, replaceNoRowsErr(err, ErrUnknownRecord)
	}
	return &consensusproto.RawRecordWithId{
		Payload: tch.RawChange,
		Id:      tch.Id,
	}, nil
}

func (t *listStorage) AddRawRecord(ctx context.Context, rec *consensusproto.RawRecordWithId) (err error) {
	_, err = t.service.stmt.createChange.Exec(rec.Id, t.spaceStorage.spaceId, t.listId, rec.Payload)
	if err != nil && isUniqueConstraint(err) {
		return nil
	}
	return
}

func (t *listStorage) Id() string {
	return t.listId
}
