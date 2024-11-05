package dependencies

import "github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"

type SpaceIndexStore interface {
	SpaceIndex(spaceId string) spaceindex.Store
}
