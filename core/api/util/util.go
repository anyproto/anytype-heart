package util

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var (
	ErrFailedSearchType     = errors.New("failed to search for type")
	ErrorTypeNotFound       = errors.New("type not found")
	ErrFailedSearchRelation = errors.New("failed to search for relation")
	ErrorRelationNotFound   = errors.New("relation not found")
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

type Icon struct {
	Type  string `json:"type" enums:"emoji,file,icon" example:"emoji"`                                                                      // The type of the icon
	Emoji string `json:"emoji,omitempty" example:"📄"`                                                                                       // The emoji of the icon
	File  string `json:"file,omitempty" example:"http://127.0.0.1:31006/image/bafybeieptz5hvcy6txplcvphjbbh5yjc2zqhmihs3owkh5oab4ezauzqay"` // The file of the icon
	Name  string `json:"name,omitempty" example:"document"`                                                                                 // The name of the icon
	Color string `json:"color,omitempty" example:"red"`                                                                                     // The color of the icon
}

// GetIcon returns the icon to use for the object, which can be builtin icon, emoji or file
func GetIcon(accountInfo *model.AccountInfo, iconEmoji string, iconImage string, iconName string, iconOption float64) Icon {
	if iconName != "" {
		return Icon{
			Type:  "icon",
			Name:  iconName,
			Color: iconOptionToColor[iconOption],
		}
	}

	if iconEmoji != "" {
		return Icon{
			Type:  "emoji",
			Emoji: iconEmoji,
		}
	}

	if iconImage != "" {
		return Icon{
			Type: "file",
			File: fmt.Sprintf("%s/image/%s", accountInfo.GatewayUrl, iconImage),
		}
	}

	return Icon{}
}

func ResolveUniqueKeyToTypeId(mw service.ClientCommandsServer, spaceId string, uniqueKey string) (typeId string, err error) {
	// Call ObjectSearch for type with unique key and return the type's ID
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

	if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return "", ErrFailedSearchType
	}

	if len(resp.Records) == 0 {
		return "", ErrorTypeNotFound
	}

	return resp.Records[0].Fields[bundle.RelationKeyId.String()].GetStringValue(), nil
}

func ResolveRelationKeyToRelationName(mw service.ClientCommandsServer, spaceId string, relationKey string) (relation string, err error) {
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
		return "", ErrFailedSearchRelation
	}

	if len(resp.Records) == 0 {
		return "", ErrorRelationNotFound
	}

	return resp.Records[0].Fields[bundle.RelationKeyName.String()].GetStringValue(), nil
}
