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
	ErrFailedSearchType = errors.New("failed to search for type")
	ErrorTypeNotFound   = errors.New("type not found")
)

// GetIconFromEmojiOrImage returns the icon to use for the object, which can be either an emoji or an image url
func GetIconFromEmojiOrImage(accountInfo *model.AccountInfo, iconEmoji string, iconImage string) string {
	if iconEmoji != "" {
		return iconEmoji
	}

	if iconImage != "" {
		return fmt.Sprintf("%s/image/%s", accountInfo.GatewayUrl, iconImage)
	}

	return ""
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
