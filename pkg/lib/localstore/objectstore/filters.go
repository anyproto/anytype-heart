package objectstore

import (
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
)

func newIdsFilter(ids []string) idsFilter {
	f := make(idsFilter)
	for i, id := range ids {
		f[id] = i
	}
	return f
}

type idsFilter map[string]int

func (f idsFilter) FilterObject(getter database.Getter) bool {
	id := getter.Get(bundle.RelationKeyId.String()).GetStringValue()
	_, ok := f[id]
	return ok
}

func (f idsFilter) Compare(a, b database.Getter) int {
	idA := a.Get(bundle.RelationKeyId.String()).GetStringValue()
	idB := b.Get(bundle.RelationKeyId.String()).GetStringValue()
	aIndex := f[idA]
	bIndex := f[idB]
	switch {
	case aIndex == bIndex:
		return 0
	case aIndex < bIndex:
		return -1
	default:
		return 1
	}
}

func (f idsFilter) String() string {
	return "idsFilter"
}
