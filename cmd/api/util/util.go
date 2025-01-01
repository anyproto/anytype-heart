package util

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var (
	ErrFailedSearchType = errors.New("failed to search for type")
	ErrorTypeNotFound   = errors.New("type not found")
)

// GetIconFromEmojiOrImage returns the icon to use for the object, which can be either an emoji or an image url
func GetIconFromEmojiOrImage(accountInfo *model.AccountInfo, iconEmoji string, iconImage string) string {
	if iconEmoji != "" {
		return iconEmoji
	}

	if iconImage != "" {
		return GetGatewayURLForMedia(accountInfo, iconImage, true)
	}

	return ""
}

// GetGatewayURLForMedia returns the URL of file gateway for the media object with the given ID
func GetGatewayURLForMedia(accountInfo *model.AccountInfo, objectId string, isIcon bool) string {
	widthParam := ""
	if isIcon {
		widthParam = "?width=100"
	}
	return fmt.Sprintf("%s/image/%s%s", accountInfo.GatewayUrl, objectId, widthParam)
}

// ResolveTypeToName resolves the type ID to the name of the type, e.g. "ot-page" to "Page" or "bafyreigyb6l5szohs32ts26ku2j42yd65e6hqy2u3gtzgdwqv6hzftsetu" to "Custom Type"
func ResolveTypeToName(mw service.ClientCommandsServer, spaceId string, typeId string) (typeName string, err error) {
	// Can't look up preinstalled types based on relation key, therefore need to use unique key
	relKey := bundle.RelationKeyId.String()
	if strings.Contains(typeId, "ot-") {
		relKey = bundle.RelationKeyUniqueKey.String()
	}

	// Call ObjectSearch for object of specified type and return the name
	resp := mw.ObjectSearch(context.Background(), &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: relKey,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(typeId),
			},
		},
		Keys: []string{"name"},
	})

	if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return "", ErrFailedSearchType
	}

	if len(resp.Records) == 0 {
		return "", ErrorTypeNotFound
	}

	return resp.Records[0].Fields["name"].GetStringValue(), nil
}
