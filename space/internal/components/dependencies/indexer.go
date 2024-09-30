package dependencies

import (
	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/space/clientspace"
)

type SpaceIndexer interface {
	app.Component
	ReindexMarketplaceSpace(space clientspace.Space) error
	ReindexSpace(space clientspace.Space) error
	RemoveIndexes(spaceID string) (err error)
	RemoveAclIndexes(spaceID string) (err error)
}
