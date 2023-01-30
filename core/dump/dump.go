package dump

import "github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"

type Service struct {
	objectStore objectstore.ObjectStore
}

func (s *Service) Dump() {
	s.objectStore.QueryObjectInfo()
}
