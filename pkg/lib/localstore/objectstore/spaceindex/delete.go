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
	defer func() {
		_ = txn.Rollback()
	}()
	for _, id := range ids {
		err := s.objects.DeleteId(txn.Context(), id)
		if err != nil && !errors.Is(err, anystore.ErrDocNotFound) {
			return fmt.Errorf("delete object %s: %w", id, err)
		}

		err = s.headsState.DeleteId(txn.Context(), id)
		if err != nil && !errors.Is(err, anystore.ErrDocNotFound) {
			return fmt.Errorf("delete headsState %s: %w", id, err)
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
	defer func() {
		txn.Rollback()
	}()

	newDetails := domain.NewDetails()
	newDetails.SetString(bundle.RelationKeyId, id)
	newDetails.SetString(bundle.RelationKeySpaceId, s.spaceId)
	newDetails.SetBool(bundle.RelationKeyIsDeleted, true)

	// do not completely remove object details, so we can distinguish links to deleted and not-yet-loaded objects
	err = s.UpdateObjectDetails(txn.Context(), id, newDetails, nil)
	if err != nil {
		return fmt.Errorf("delete: update details: %w", err)
	}

	err = s.headsState.DeleteId(txn.Context(), id)
	if err != nil && !errors.Is(err, anystore.ErrDocNotFound) {
		return fmt.Errorf("delete: heads state delete: %w", err)
	}
	err = s.eraseLinksForObject(txn.Context(), id)
	if err != nil {
		return fmt.Errorf("delete: erase links: %w", err)
	}
	// add to ft index queue in order to remove the object
	// it will find the object not found error and remove all the docs
	err = s.fulltextQueue.AddToIndexQueue(txn.Context(), domain.FullID{
		ObjectID: id,
		SpaceID:  s.spaceId,
	})
	if err != nil {
		log.Errorf("delete object %s: add to fulltext queue: %v", id, err)
	}

	err = txn.Commit()
	if err != nil {
		return fmt.Errorf("delete object info: %w", err)
	}

	return nil
}

func (s *dsObjectStore) DeleteLinks(ids []string) error {
	txn, err := s.links.WriteTx(s.componentCtx)
	if err != nil {
		return fmt.Errorf("read txn: %w", err)
	}
	defer func() {
		_ = txn.Rollback()
	}()
	for _, id := range ids {
		err := s.eraseLinksForObject(txn.Context(), id)
		if err != nil {
			return fmt.Errorf("erase links for %s: %w", id, err)
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
