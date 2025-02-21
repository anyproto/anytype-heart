package util

import (
	"context"
	"errors"
	"fmt"
	"time"

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

func PosixToISO8601(posix float64) string {
	t := time.Unix(int64(posix), 0).UTC()
	return t.Format(time.RFC3339)
}

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
				RelationKey: bundle.RelationKeyLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_relation)),
			},
		},
		Keys: []string{bundle.RelationKeyId.String(), bundle.RelationKeyName.String(), bundle.RelationKeyLayout.String()},
	})

	if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return "", ErrFailedSearchType
	}

	if len(resp.Records) == 0 {
		return "", ErrorTypeNotFound
	}

	return resp.Records[0].Fields[bundle.RelationKeyName.String()].GetStringValue(), nil
}
