package linkresolver

import (
	"fmt"
	"strings"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	ResourceObject = "object"
	ResourceBlock  = "block"

	ParameterSpaceId  = "spaceId"
	ParameterObjectId = "objectId"
	ParameterBlockId  = "blockId"
)

var ErrLinkParsing = fmt.Errorf("failed to parse link")

var parametersByResource = map[string][]string{
	ResourceObject: {ParameterSpaceId, ParameterObjectId},
	ResourceBlock:  {ParameterSpaceId, ParameterObjectId, ParameterBlockId},
}

func GetObjectLink(id domain.FullID) string {
	link, err := generateLink(ResourceObject, map[string]string{
		ParameterSpaceId:  id.SpaceID,
		ParameterObjectId: id.ObjectID,
	})
	if err != nil {
		panic(err)
	}
	return link
}

func GetBlockLink(objectId domain.FullID, blockId string) string {
	link, err := generateLink(ResourceBlock, map[string]string{
		ParameterSpaceId:  objectId.SpaceID,
		ParameterObjectId: objectId.ObjectID,
		ParameterBlockId:  blockId,
	})
	if err != nil {
		panic(err)
	}
	return link
}

func generateLink(resource string, pars map[string]string) (string, error) {
	parKeys, ok := parametersByResource[resource]
	if !ok {
		return "", fmt.Errorf("unknown resource %s", resource)
	}

	link := fmt.Sprintf("%s?", resource)
	for _, key := range parKeys {
		par, ok := pars[key]
		if !ok {
			return "", fmt.Errorf("no '%s' parameter is provided to build link to '%s' resource", key, resource)
		}
		link = link + key + "=" + par + "&"
	}
	return link[:len(link)-1], nil
}

func ParseObjectLink(link string) (domain.FullID, error) {
	resource, params, err := parseLink(link)
	if err != nil {
		return domain.FullID{}, err
	}

	if resource != ResourceObject {
		return domain.FullID{}, fmt.Errorf("%w: '%s' is expected as resource, got '%s'", ErrLinkParsing, ResourceObject, resource)
	}

	spaceId, ok := params[ParameterSpaceId]
	if !ok {
		return domain.FullID{}, fmt.Errorf("%w: no spaceId provided", ErrLinkParsing)
	}
	objectId, ok := params[ParameterObjectId]
	if !ok {
		return domain.FullID{}, fmt.Errorf("%w: no objectId provided", ErrLinkParsing)
	}

	return domain.FullID{SpaceID: spaceId, ObjectID: objectId}, nil
}

func ParseBlockLink(link string) (id domain.FullID, blockId string, err error) {
	resource, params, err := parseLink(link)
	if err != nil {
		return domain.FullID{}, "", err
	}

	if resource != ResourceObject {
		return domain.FullID{}, "", fmt.Errorf("%w: '%s' is expected as resource, got '%s'", ErrLinkParsing, ResourceObject, resource)
	}

	spaceId, ok := params[ParameterSpaceId]
	if !ok {
		return domain.FullID{}, "", fmt.Errorf("%w: no spaceId provided", ErrLinkParsing)
	}
	objectId, ok := params[ParameterObjectId]
	if !ok {
		return domain.FullID{}, "", fmt.Errorf("%w: no objectId provided", ErrLinkParsing)
	}
	blockId, ok = params[ParameterBlockId]
	if !ok {
		return domain.FullID{}, "", fmt.Errorf("%w: no blockId provided", ErrLinkParsing)
	}

	return domain.FullID{SpaceID: spaceId, ObjectID: objectId}, blockId, nil
}

func parseLink(link string) (resource string, result map[string]string, err error) {
	parts := strings.Split(link, "?")
	if len(parts) != 2 {
		return "", nil, fmt.Errorf("%w: wrong link format. '{resource}/{key1}={value1}&{key2}={value2}' expected, got '%s'", ErrLinkParsing, link)
	}

	resource = parts[0]
	params, ok := parametersByResource[resource]
	if !ok {
		return resource, nil, fmt.Errorf("%w: invalid resource '%s'", ErrLinkParsing, resource)
	}

	parts = strings.Split(parts[1], "&")
	if len(parts) != len(params) {
		return resource, nil, fmt.Errorf("%w: invalid number of parameters for resource '%s'. %d is expected, got %d",
			ErrLinkParsing, resource, len(params), len(parts))
	}

	result = make(map[string]string, len(params))

	for i, p := range parts {
		keyValue := strings.Split(p, "=")
		if len(keyValue) != 2 {
			return resource, nil, fmt.Errorf("%w: invalid parameter representation. 'key=value' is expected, got '%s'", ErrLinkParsing, keyValue)
		}

		if keyValue[0] != params[i] {
			return resource, nil, fmt.Errorf("%w: invalid parameters order. '%s' is expected, got '%s'", ErrLinkParsing, params[i], keyValue[0])
		}

		result[keyValue[0]] = keyValue[1]
	}

	return resource, result, nil
}

func IsObjectLink(link string) bool {
	_, err := ParseObjectLink(link)
	return err == nil
}

func ShortenObjectLinks(links ...string) []string {
	for i, link := range links {
		if id, err := ParseObjectLink(link); err == nil {
			links[i] = id.ObjectID
		}
	}
	return links
}

// GetObjectId returns multi-space link in case object is located in other space, and its id otherwise
func GetObjectId(currentSpaceId string, details *types.Struct) string {
	spaceId := pbtypes.GetString(details, bundle.RelationKeySpaceId.String())
	id := pbtypes.GetString(details, bundle.RelationKeyId.String())

	if currentSpaceId == spaceId {
		return id
	}
	return GetObjectLink(domain.FullID{SpaceID: spaceId, ObjectID: id})
}
