package anytype

import (
	"github.com/anytypeio/go-anytype-library/core"
)

func NewAnytype(c *core.Anytype) Service {
	return &anytype{c}
}

type anytype struct {
	*core.Anytype
}
