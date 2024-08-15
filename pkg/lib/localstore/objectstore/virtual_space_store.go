package objectstore

import (
	"errors"

	anystore "github.com/anyproto/any-store"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
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
	_, err = s.virtualSpaces.UpsertOne(s.componentCtx, it)
	return err
}

func (s *dsObjectStore) ListVirtualSpaces() ([]string, error) {
	iter, err := s.virtualSpaces.Find(nil).Iter(s.componentCtx)
	if err != nil {
		return nil, err
	}
	var spaceIds []string
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, errors.Join(iter.Close(), err)
		}
		id := doc.Value().GetStringBytes("id")
		spaceIds = append(spaceIds, string(id))
	}
	return spaceIds, iter.Close()
}

func (s *dsObjectStore) DeleteVirtualSpace(spaceID string) error {
	ids, _, err := s.QueryObjectIDs(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeySpaceId.String(),
				Value:       pbtypes.String(spaceID),
			},
			{
				Condition:   model.BlockContentDataviewFilter_NotLike,
				RelationKey: bundle.RelationKeyId.String(),
				Value:       pbtypes.String(addr.BundledRelationURLPrefix),
			},
			{
				Condition:   model.BlockContentDataviewFilter_NotLike,
				RelationKey: bundle.RelationKeyId.String(),
				Value:       pbtypes.String(addr.BundledObjectTypeURLPrefix),
			},
			{
				Condition:   model.BlockContentDataviewFilter_NotLike,
				RelationKey: bundle.RelationKeyId.String(),
				Value:       pbtypes.String(addr.BundledTemplatesURLPrefix),
			},
			{
				Condition:   model.BlockContentDataviewFilter_NotLike,
				RelationKey: bundle.RelationKeyId.String(),
				Value:       pbtypes.String(addr.AnytypeProfileId),
			},
		},
	})
	if err != nil {
		return err
	}
	err = s.DeleteDetails(ids...)
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
