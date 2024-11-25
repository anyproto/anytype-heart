package objectstore

import (
	"errors"
	"fmt"

	anystore "github.com/anyproto/any-store"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func (s *dsObjectStore) SaveVirtualSpace(id string) (err error) {
	arena := s.arenaPool.Get()
	defer func() {
		arena.Reset()
		s.arenaPool.Put(arena)
	}()

	it, err := keyValueItem(arena, id, nil)
	if err != nil {
		return err
	}
	err = s.virtualSpaces.UpsertOne(s.componentCtx, it)
	return err
}

func (s *dsObjectStore) ListVirtualSpaces() ([]string, error) {
	iter, err := s.virtualSpaces.Find(nil).Iter(s.componentCtx)
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var spaceIds []string
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, fmt.Errorf("get doc: %w", err)
		}
		id := doc.Value().GetStringBytes("id")
		spaceIds = append(spaceIds, string(id))
	}
	return spaceIds, nil
}

func (s *dsObjectStore) DeleteVirtualSpace(spaceID string) error {
	ids, _, err := s.SpaceIndex(spaceID).QueryObjectIds(database.Query{
		Filters: []database.FilterRequest{
			{
				Condition:   model.BlockContentDataviewFilter_NotLike,
				RelationKey: bundle.RelationKeyId,
				Value:       domain.String(addr.BundledRelationURLPrefix),
			},
			{
				Condition:   model.BlockContentDataviewFilter_NotLike,
				RelationKey: bundle.RelationKeyId,
				Value:       domain.String(addr.BundledObjectTypeURLPrefix),
			},
			{
				Condition:   model.BlockContentDataviewFilter_NotLike,
				RelationKey: bundle.RelationKeyId,
				Value:       domain.String(addr.BundledTemplatesURLPrefix),
			},
			{
				Condition:   model.BlockContentDataviewFilter_NotLike,
				RelationKey: bundle.RelationKeyId,
				Value:       domain.String(addr.AnytypeProfileId),
			},
		},
	})
	if err != nil {
		return err
	}
	err = s.SpaceIndex(spaceID).DeleteDetails(s.componentCtx, ids)
	if err != nil {
		return err
	}

	return s.deleteSpace(spaceID)
}

func (s *dsObjectStore) deleteSpace(spaceID string) error {
	err := s.virtualSpaces.DeleteId(s.componentCtx, spaceID)
	if err != nil && !errors.Is(err, anystore.ErrDocNotFound) {
		return err
	}
	return nil
}
