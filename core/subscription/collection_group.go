package subscription

import (
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type collectionGroupSub struct {
	*groupSub

	colObserver *collectionObserver
}

func (s *service) newCollectionGroupSub(id string, relKey string, f *database.Filters, groups []*model.BlockContentDataviewGroup, colObserver *collectionObserver) *collectionGroupSub {
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
