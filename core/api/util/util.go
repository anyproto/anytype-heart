package util

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/anytype-heart/core/api/apicore"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var (
	ErrFailedSearchType     = errors.New("failed to search for type")
	ErrorTypeNotFound       = errors.New("type not found")
	ErrFailedSearchProperty = errors.New("failed to search for property")
	ErrorPropertyNotFound   = errors.New("property not found")
)

var iconOptionToColor = map[float64]string{
	1:  "grey",
	2:  "yellow",
	3:  "orange",
	4:  "red",
	5:  "pink",
	6:  "purple",
	7:  "blue",
	8:  "ice",
	9:  "teal",
	10: "lime",
}

type IconFormat string

const (
	IconFormatEmoji IconFormat = "emoji"
	IconFormatFile  IconFormat = "file"
	IconFormatIcon  IconFormat = "icon"
)

type Icon struct {
	Format IconFormat `json:"format" enums:"emoji,file,icon" example:"emoji"`                                                                    // The type of the icon
	Emoji  *string    `json:"emoji,omitempty" example:"📄"`                                                                                       // The emoji of the icon
	File   *string    `json:"file,omitempty" example:"http://127.0.0.1:31006/image/bafybeieptz5hvcy6txplcvphjbbh5yjc2zqhmihs3owkh5oab4ezauzqay"` // The file of the icon
	Name   *string    `json:"name,omitempty" example:"document"`                                                                                 // The name of the icon
	Color  *string    `json:"color,omitempty" example:"red"`                                                                                     // The color of the icon
}

// StringPtr returns a pointer to the string
func StringPtr(s string) *string {
	return &s
}

// GetIcon returns the icon to use for the object, which can be builtin icon, emoji or file
func GetIcon(gatewayURL, iconEmoji string, iconImage string, iconName string, iconOption float64) Icon {
	if iconName != "" {
		return Icon{
			Format: "icon",
			Name:   &iconName,
			Color:  StringPtr(iconOptionToColor[iconOption]),
		}
	}

	if iconEmoji != "" {
		return Icon{
			Format: "emoji",
			Emoji:  &iconEmoji,
		}
	}

	if iconImage != "" {
		return Icon{
			Format: "file",
			File:   StringPtr(fmt.Sprintf("%s/image/%s", gatewayURL, iconImage)),
		}
	}

	return Icon{}
}

// ResolveUniqueKeyToTypeId resolves the unique key to the type's ID
func ResolveUniqueKeyToTypeId(mw apicore.ClientCommands, spaceId string, uniqueKey string) (typeId string, err error) {
	resp := mw.ObjectSearch(context.Background(), &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyUniqueKey.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(uniqueKey),
			},
		},
		Keys: []string{bundle.RelationKeyId.String()},
	})

	if resp.Error != nil {
		if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
			return "", ErrFailedSearchType
		}

		if len(resp.Records) == 0 {
			return "", ErrorTypeNotFound
		}
	}

	return resp.Records[0].Fields[bundle.RelationKeyId.String()].GetStringValue(), nil
}

// ResolveIdtoUniqueKey resolves the type's ID to the unique key
func ResolveIdtoUniqueKey(mw apicore.ClientCommands, spaceId string, typeId string) (uniqueKey string, err error) {
	resp := mw.ObjectShow(context.Background(), &pb.RpcObjectShowRequest{
		SpaceId:  spaceId,
		ObjectId: typeId,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectShowResponseError_NULL {
		return "", ErrorTypeNotFound
	}

	return resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyUniqueKey.String()].GetStringValue(), nil
}

// ResolveRelationKeyToPropertyName resolves the property key to the property's name
func ResolveRelationKeyToPropertyName(mw apicore.ClientCommands, spaceId string, relationKey string) (property string, err error) {
	resp := mw.ObjectSearch(context.Background(), &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyRelationKey.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(relationKey),
			},
			{
				RelationKey: bundle.RelationKeyResolvedLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_relation)),
			},
		},
		Keys: []string{bundle.RelationKeyId.String(), bundle.RelationKeyName.String(), bundle.RelationKeyResolvedLayout.String()},
	})

	if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return "", ErrFailedSearchProperty
	}

	if len(resp.Records) == 0 {
		return "", ErrorPropertyNotFound
	}

	return resp.Records[0].Fields[bundle.RelationKeyName.String()].GetStringValue(), nil
}
