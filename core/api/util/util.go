package util

import (
	"context"
	"errors"

	"github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var (
	ErrFailedSearchType     = errors.New("failed to search for type")
	ErrorResolveToUniqueKey = errors.New("failed to resolve to unique key")
)

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

// ResolveIdtoUniqueKeyAndRelationKey resolves the type's ID to the unique key
func ResolveIdtoUniqueKeyAndRelationKey(mw apicore.ClientCommands, spaceId string, objectId string) (uk string, rk string, err error) {
	resp := mw.ObjectShow(context.Background(), &pb.RpcObjectShowRequest{
		SpaceId:  spaceId,
		ObjectId: objectId,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectShowResponseError_NULL {
		return "", "", ErrorResolveToUniqueKey
	}

	return resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyUniqueKey.String()].GetStringValue(),
		resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyRelationKey.String()].GetStringValue(), nil
}
