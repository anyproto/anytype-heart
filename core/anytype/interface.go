package anytype

import (
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
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
