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
	ErrorResolveToUniqueKey = errors.New("failed to resolve to unique key")
)

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
	Color  *Color     `json:"color,omitempty" example:"yellow" enums:"grey,yellow,orange,red,pink,purple,blue,ice,teal,lime"`                    // The color of the icon
}

type Color string

const (
	ColorGrey   Color = "grey"
	ColorYellow Color = "yellow"
	ColorOrange Color = "orange"
	ColorRed    Color = "red"
	ColorPink   Color = "pink"
	ColorPurple Color = "purple"
	ColorBlue   Color = "blue"
	ColorIce    Color = "ice"
	ColorTeal   Color = "teal"
	ColorLime   Color = "lime"
)

var iconOptionToColor = map[float64]Color{
	1:  ColorGrey,
	2:  ColorYellow,
	3:  ColorOrange,
	4:  ColorRed,
	5:  ColorPink,
	6:  ColorPurple,
	7:  ColorBlue,
	8:  ColorIce,
	9:  ColorTeal,
	10: ColorLime,
}

var ColorOptionToColor = map[string]Color{
	"grey":   ColorGrey,
	"yellow": ColorYellow,
	"orange": ColorOrange,
	"red":    ColorRed,
	"pink":   ColorPink,
	"purple": ColorPurple,
	"blue":   ColorBlue,
	"ice":    ColorIce,
	"teal":   ColorTeal,
	"lime":   ColorLime,
}

func StringPtr(s string) *string {
	return &s
}

func ColorPtr(c Color) *Color {
	return &c
}

// GetIcon returns the icon to use for the object, which can be builtin icon, emoji or file
func GetIcon(gatewayURL, iconEmoji string, iconImage string, iconName string, iconOption float64) Icon {
	if iconName != "" {
		return Icon{
			Format: IconFormatIcon,
			Name:   &iconName,
			Color:  ColorPtr(iconOptionToColor[iconOption]),
		}
	}

	if iconEmoji != "" {
		return Icon{
			Format: IconFormatEmoji,
			Emoji:  &iconEmoji,
		}
	}

	if iconImage != "" {
		return Icon{
			Format: IconFormatFile,
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
			return "", ErrorResolveToUniqueKey
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
		return "", ErrorResolveToUniqueKey
	}

	return resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyUniqueKey.String()].GetStringValue(), nil
}
