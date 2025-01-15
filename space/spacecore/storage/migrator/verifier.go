package migrator

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/query"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	oldstorage2 "github.com/anyproto/any-sync/commonspace/spacestorage/oldstorage"

	"github.com/anyproto/anytype-heart/space/spacecore/oldstorage"
	"github.com/anyproto/anytype-heart/space/spacecore/storage"
)

type verifier struct {
	fast       bool
	oldStorage oldstorage.ClientStorage
	newStorage storage.ClientStorage
}

type errorEntry struct {
	id  string
	err error
}

type verificationReport struct {
	spaceId string

	treesCompared      int
	errors             []errorEntry
	totalBytesCompared int

	duration time.Duration
}

func (v *verifier) verify(ctx context.Context) error {
	allSpaceIds, err := v.oldStorage.AllSpaceIds()
	if err != nil {
		return fmt.Errorf("list all space ids: %w", err)
	}
	for _, spaceId := range allSpaceIds {
		report, err := v.verifySpace(ctx, spaceId)
		if err != nil {
			return fmt.Errorf("verify space: %w", err)
		}
		fmt.Printf("%#v\n%s\n", report, report.duration)
	}
	return nil
}

func (v *verifier) verifySpace(ctx context.Context, spaceId string) (*verificationReport, error) {
	oldStore, err := v.oldStorage.WaitSpaceStorage(ctx, spaceId)
	if err != nil {
		return nil, fmt.Errorf("open old store: %w", err)
	}

	newStore, err := v.newStorage.WaitSpaceStorage(ctx, spaceId)
	if err != nil {
		return nil, fmt.Errorf("open new store: %w", err)
	}

	storedIds, err := oldStore.StoredIds()
	if err != nil {
		return nil, err
	}

	newStoreCollection, err := newStore.AnyStore().Collection(ctx, "changes")
	if err != nil {
		return nil, fmt.Errorf("get new store collection: %w", err)
	}

	report := &verificationReport{}
	now := time.Now()
	for _, treeId := range storedIds {
		bytesCompared, err := v.verifyTree(ctx, oldStore, newStore, newStoreCollection, treeId)
		if err != nil {
			report.errors = append(report.errors, errorEntry{id: treeId, err: err})
		}
		report.treesCompared++
		report.totalBytesCompared += bytesCompared
	}
	report.duration = time.Since(now)

	err = oldStore.Close(ctx)
	if err != nil {
		return nil, err
	}
	return report, nil
}

func (v *verifier) verifyTree(ctx context.Context, oldStore oldstorage2.SpaceStorage, newStore spacestorage.SpaceStorage, newStoreCollection anystore.Collection, treeId string) (int, error) {
	newHeadStorage := newStore.HeadStorage()

	entry, err := newHeadStorage.GetEntry(ctx, treeId)
	if err != nil {
		return 0, fmt.Errorf("get heads entry: %w", err)
	}

	oldTreeStorage, err := oldStore.TreeStorage(treeId)
	if err != nil {
		return 0, fmt.Errorf("open old tree storage: %w", err)
	}
	oldHeads, err := oldTreeStorage.Heads()
	if err != nil {
		return 0, fmt.Errorf("open old heads storage: %w", err)
	}

	if !slices.Equal(oldHeads, entry.Heads) {
		return 0, fmt.Errorf("old heads does not match tree storage")
	}

	newTreeStorage, err := newStore.TreeStorage(ctx, treeId)
	if err != nil {
		return 0, fmt.Errorf("open new tree storage: %w", err)
	}

	var bytesCompared int
	if v.fast {
		err = v.verifyChangesFast(ctx, oldTreeStorage, newTreeStorage)
		if err != nil {
			return 0, fmt.Errorf("verify tree fast: %w", err)
		}
	} else {
		bytesCompared, err = v.verifyChangesFull(ctx, newStoreCollection, oldTreeStorage)
		if err != nil {
			return 0, fmt.Errorf("verify tree fast: %w", err)
		}
	}
	return bytesCompared, nil
}

// verifyChangesFast checks only existence of changes
func (v *verifier) verifyChangesFast(ctx context.Context, oldTreeStorage oldstorage2.TreeStorage, newTreeStorage objecttree.Storage) error {
	oldChangeIds, err := oldTreeStorage.GetAllChangeIds()
	if err != nil {
		return fmt.Errorf("get old change ids: %w", err)
	}

	if len(oldChangeIds) == 0 {
		return fmt.Errorf("old change ids is empty")
	}
	for _, oldChangeId := range oldChangeIds {
		ok, err := newTreeStorage.Has(ctx, oldChangeId)
		if err != nil {
			return fmt.Errorf("get old change id: %w", err)
		}
		if !ok {
			return fmt.Errorf("old change id does not exist")
		}
	}
	return nil
}

// verifyChangesFull checks byte contents of changes
func (v *verifier) verifyChangesFull(ctx context.Context, newStoreCollection anystore.Collection, oldTreeStorage oldstorage2.TreeStorage) (int, error) {
	iterator, ok := oldTreeStorage.(oldstorage2.ChangesIterator)
	if !ok {
		return 0, fmt.Errorf("old tree storage does not implement ChangesIterator")
	}
	var bytesCompared int
	iter, err := newStoreCollection.Find(query.Key{Path: []string{"t"}, Filter: query.NewComp(query.CompOpEq, oldTreeStorage.Id())}).Sort("id").Iter(ctx)
	if err != nil {
		return 0, fmt.Errorf("new store: changes iterator: %w", err)
	}
	defer iter.Close()
	err = iterator.IterateChanges(func(id string, oldChange []byte) error {
		if !iter.Next() {
			return fmt.Errorf("new store iterator: no more changes")
		}
		doc, err := iter.Doc()
		if err != nil {
			return fmt.Errorf("new store iterator: read doc: %w", err)
		}

		newId := doc.Value().GetString("id")
		if newId != id {
			return fmt.Errorf("new store iterator: id does not match")
		}

		bytesCompared += len(oldChange)
		if !bytes.Equal(oldChange, doc.Value().GetBytes("r")) {
			return fmt.Errorf("old tree change does not match tree storage")
		}
		return nil
	})
	return bytesCompared, err
}
