package dependencies

import "github.com/gogo/protobuf/types"

type DetailsModifier interface {
	ModifyDetails(objectId string, modifier func(current *types.Struct) (*types.Struct, error)) (err error)
}
