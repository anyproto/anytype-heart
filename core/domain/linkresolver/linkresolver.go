package linkresolver

import "github.com/anyproto/anytype-heart/core/domain"

const (
	ResourceObject = "object"
	ResourceBlock  = "block"

	ParameterSpaceId  = "spaceId"
	ParameterObjectId = "objectId"
	ParameterBlockId  = "blockId"
)

var parametersByResource = map[string][]string{
	ResourceObject: {ParameterSpaceId, ParameterObjectId},
	ResourceBlock:  {ParameterSpaceId, ParameterObjectId, ParameterBlockId},
}

type Link string

func GetObjectLink(id domain.FullID) Link {
	return
}

func generateLink() {

}
