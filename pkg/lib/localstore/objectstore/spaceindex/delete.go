package spaceindex

import (
	"context"
	"errors"
	"fmt"

	anystore "github.com/anyproto/any-store"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

func (s *dsObjectStore) DeleteDetails(ctx context.Context, ids []string) error {
	txn, err := s.db.WriteTx(ctx)
	if err != nil {
		return fmt.Errorf("write txn: %w", err)
	}
	for _, id := range ids {
		err := s.objects.DeleteId(txn.Context(), id)
		if err != nil && !errors.Is(err, anystore.ErrDocNotFound) {
			return errors.Join(txn.Rollback(), fmt.Errorf("delete object %s: %w", id, err))
		}

		err = s.headsState.DeleteId(txn.Context(), id)
		if err != nil && !errors.Is(err, anystore.ErrDocNotFound) {
			return errors.Join(txn.Rollback(), fmt.Errorf("delete headsState %s: %w", id, err))
		}
	}
	return txn.Commit()
}

// DeleteObject removes all details, leaving only id and isDeleted
func (s *dsObjectStore) DeleteObject(id string) error {
	txn, err := s.db.WriteTx(s.componentCtx)
	if err != nil {
		return fmt.Errorf("write txn: %w", err)
	}
	rollback := func(err error) error {
		return errors.Join(txn.Rollback(), err)
	}

	newDetails := domain.NewDetails()
	newDetails.SetString(bundle.RelationKeyId, id)
	newDetails.SetString(bundle.RelationKeySpaceId, s.spaceId)
	newDetails.SetBool(bundle.RelationKeyIsDeleted, true)

	// do not completely remove object details, so we can distinguish links to deleted and not-yet-loaded objects
	err = s.UpdateObjectDetails(txn.Context(), id, newDetails)
	if err != nil {
		return rollback(fmt.Errorf("failed to overwrite details and relations: %w", err))
	}
	err = s.fulltextQueue.RemoveIdsFromFullTextQueue([]string{id})
	if err != nil {
		return rollback(fmt.Errorf("delete: fulltext queue: %w", err))
	}

	err = s.headsState.DeleteId(txn.Context(), id)
	if err != nil && !errors.Is(err, anystore.ErrDocNotFound) {
		return rollback(fmt.Errorf("delete: heads state: %w", err))
	}
	err = s.eraseLinksForObject(txn.Context(), id)
	if err != nil {
		return rollback(err)
	}
	err = txn.Commit()
	if err != nil {
		return fmt.Errorf("delete object info: %w", err)
	}

	if err := s.fts.DeleteObject(id); err != nil {
		return err
	}
	return nil
}

func (s *dsObjectStore) DeleteLinks(ids []string) error {
	txn, err := s.links.WriteTx(s.componentCtx)
	if err != nil {
		return fmt.Errorf("read txn: %w", err)
	}
	for _, id := range ids {
		err := s.eraseLinksForObject(txn.Context(), id)
		if err != nil {
			return errors.Join(txn.Rollback(), fmt.Errorf("erase links for %s: %w", id, err))
		}
	}
	return txn.Commit()
}

func (s *dsObjectStore) eraseLinksForObject(ctx context.Context, from string) error {
	err := s.links.DeleteId(ctx, from)
	if err != nil && !errors.Is(err, anystore.ErrDocNotFound) {
		return err
	}
	return nil
}
