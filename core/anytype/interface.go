package anytype

import (
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore"
)

type Service interface {
	core.Service
}

type SmartBlock interface {
	core.SmartBlock
}

type SmartBlockSnapshot interface {
	core.SmartBlockSnapshot
}

type File interface {
	core.File
}

type Image interface {
	core.Image
}

type ObjectStore interface {
	localstore.ObjectStore
}
