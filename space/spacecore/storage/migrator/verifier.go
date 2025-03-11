// nolint:unused
package migrator

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/query"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	oldstorage2 "github.com/anyproto/any-sync/commonspace/spacestorage/oldstorage"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceresolverstore"
	"github.com/anyproto/anytype-heart/space/spacecore/oldstorage"
	"github.com/anyproto/anytype-heart/space/spacecore/storage"
)

type verifier struct {
	fast          bool
	oldStorage    oldstorage.ClientStorage
	newStorage    storage.ClientStorage
	resolverStore spaceresolverstore.Store
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

func (v *verifier) verify(ctx context.Context) ([]*verificationReport, error) {
	allSpaceIds, err := v.oldStorage.AllSpaceIds()
	if err != nil {
		return nil, fmt.Errorf("list all space ids: %w", err)
	}
	reports := make([]*verificationReport, 0, len(allSpaceIds))
	for _, spaceId := range allSpaceIds {
		report, err := v.verifySpace(ctx, spaceId)
		if err != nil {
			return nil, fmt.Errorf("verify space: %w", err)
		}
		report.spaceId = spaceId
		reports = append(reports, report)
	}
	return reports, nil
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
		return 0, fmt.Errorf("old heads doesn't match tree storage")
	}

	err = v.verifySpaceBindings(oldStore, treeId)
	if err != nil {
		return 0, fmt.Errorf("verify space: %w", err)
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

func (v *verifier) verifySpaceBindings(oldStore oldstorage2.SpaceStorage, treeId string) error {
	gotSpaceId, err := v.resolverStore.GetSpaceId(treeId)
	// If it's not found in new store, check that it's not found in old store either
	if errors.Is(err, domain.ErrObjectNotFound) {
		_, err = v.oldStorage.GetSpaceID(treeId)
		if errors.Is(err, domain.ErrObjectNotFound) {
			return nil
		}
		if err == nil {
			return fmt.Errorf("binding is not found in new store")
		}
		return fmt.Errorf("check binding in old store: %w", err)
	} else if err != nil {
		return fmt.Errorf("resolve space id for object: %w", err)
	}
	if gotSpaceId != oldStore.Id() {
		return fmt.Errorf("resolved spaced id mismatch")
	}
	return nil
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
			return fmt.Errorf("old change id doesn't exist")
		}
	}
	return nil
}

// verifyChangesFull checks byte contents of changes
func (v *verifier) verifyChangesFull(ctx context.Context, newStoreCollection anystore.Collection, oldTreeStorage oldstorage2.TreeStorage) (int, error) {
	iterator, ok := oldTreeStorage.(oldstorage2.ChangesIterator)
	if !ok {
		return 0, fmt.Errorf("old tree storage doesn't implement ChangesIterator")
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
			return fmt.Errorf("new store iterator: id doesn't match")
		}

		bytesCompared += len(oldChange)
		if !bytes.Equal(oldChange, doc.Value().GetBytes("r")) {
			return fmt.Errorf("old tree change doesn't match tree storage")
		}
		return nil
	})
	return bytesCompared, err
}
