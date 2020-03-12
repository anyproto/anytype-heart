package anytype

import (
	"github.com/anytypeio/go-anytype-library/core"
)

func NewAnytype(c core.Service) Service {
	return &anytype{c}
}

type anytype struct {
	core.Service
}
