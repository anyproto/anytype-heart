package objectstore

import (
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/badgerhelper"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (s *dsObjectStore) SaveVirtualSpace(id string) (err error) {
	return badgerhelper.SetValue(s.db, virtualSpaces.ChildString(id).Bytes(), nil)
}

func (s *dsObjectStore) ListVirtualSpaces() ([]string, error) {
	var ids []string
	err := iterateKeysByPrefix(s.db, virtualSpaces.Bytes(), func(key []byte) {
		ids = append(ids, extractIdFromKey(string(key)))
	})
	return ids, err
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
	return badgerhelper.DeleteValue(s.db, virtualSpaces.ChildString(spaceID).Bytes())
}
