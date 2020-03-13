package anytype

import (
	"github.com/anytypeio/go-anytype-library/core"
)

func NewService(c core.Service) Service {
	return &service{c}
}

type service struct {
	core.Service
}
