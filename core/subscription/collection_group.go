package subscription

import (
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type collectionGroupSub struct {
	*groupSub

	colObserver *collectionObserver
}

func (s *spaceSubscriptions) newCollectionGroupSub(id string, relKey domain.RelationKey, f *database.Filters, groups []*model.BlockContentDataviewGroup, colObserver *collectionObserver) *collectionGroupSub {
	sub := &collectionGroupSub{
		groupSub:    s.newGroupSub(id, relKey, f, groups),
		colObserver: colObserver,
	}
	return sub
}

func (s *collectionGroupSub) close() {
	s.colObserver.close()
	s.groupSub.close()
}
