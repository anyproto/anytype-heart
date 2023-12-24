package dependencies

import (
	"github.com/anyproto/anytype-heart/space/clientspace"
)

type SpaceIndexer interface {
	ReindexMarketplaceSpace(space clientspace.Space) error
	ReindexSpace(space clientspace.Space) error
	RemoveIndexes(spaceID string) (err error)
}
