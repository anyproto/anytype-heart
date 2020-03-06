package meta

import "github.com/gogo/protobuf/types"

type Meta struct {
	BlockId string
	Details *types.Struct
}

type Service interface {
	NewSubscriber() Subscriber
	Close() (err error)
}
