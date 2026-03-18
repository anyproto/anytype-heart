package spacedomain

import (
	"errors"

	"github.com/anyproto/any-sync/commonspace/spacesyncproto"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type SpaceType string

const (
	SpaceTypeRegular  SpaceType = "anytype.space"
	SpaceTypeTech     SpaceType = "anytype.techspace"
	SpaceTypeChat     SpaceType = "anytype.chatspace"
	SpaceTypeOneToOne SpaceType = "anytype.onetoone"
)

func (t SpaceType) ToModel() model.SpaceType {
	switch t {
	case SpaceTypeRegular:
		return model.SpaceType_SpaceTypeRegular
	case SpaceTypeTech:
		return model.SpaceType_SpaceTypeTech
	case SpaceTypeChat:
		return model.SpaceType_SpaceTypeChat
	case SpaceTypeOneToOne:
		return model.SpaceType_SpaceTypeOneToOne
	default:
		return model.SpaceType_SpaceTypeUnknown
	}
}

func (t SpaceType) String() string {
	return string(t)
}

func SpaceTypeFromModel(spaceType model.SpaceType) (SpaceType, error) {
	switch spaceType {
	case model.SpaceType_SpaceTypeRegular:
		return SpaceTypeRegular, nil
	case model.SpaceType_SpaceTypeTech:
		return SpaceTypeTech, nil
	case model.SpaceType_SpaceTypeChat:
		return SpaceTypeChat, nil
	case model.SpaceType_SpaceTypeOneToOne:
		return SpaceTypeOneToOne, nil
	default:
		return "", errors.New("invalid SpaceType")
	}
}

// ReadSpaceTypeFromHeader extracts SpaceType from raw space header bytes
func ReadSpaceTypeFromHeader(rawHeaderBytes []byte) (SpaceType, error) {
	var rawHeader spacesyncproto.RawSpaceHeader
	if err := rawHeader.UnmarshalVT(rawHeaderBytes); err != nil {
		return "", err
	}
	var header spacesyncproto.SpaceHeader
	if err := header.UnmarshalVT(rawHeader.SpaceHeader); err != nil {
		return "", err
	}
	return SpaceType(header.SpaceType), nil
}

const ChangeType = "anytype.object"
