package anytype

import (
	"github.com/anytypeio/go-anytype-library/core"
)

type Service interface {
	core.Service
}

type SmartBlock interface {
	core.SmartBlock
}

type File interface {
	core.File
}

type Image interface {
	core.Image
}
