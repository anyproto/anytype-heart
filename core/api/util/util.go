package util

import (
	"context"
	"errors"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pb"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var (
	ErrFailedSearchType         = errors.New("failed to search for type")
	ErrFailedResolveToUniqueKey = errors.New("failed to resolve to unique key")
	ErrFailedGetById            = errors.New("failed to get object by id")
	ErrFailedGetByIdNotFound    = errors.New("failed to find object by id")
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
		if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
			return "", ErrFailedSearchType
		}

		if len(resp.Records) == 0 {
			return "", ErrFailedSearchType
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
		return "", "", ErrFailedResolveToUniqueKey
	}

	return resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyUniqueKey.String()].GetStringValue(),
		resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyRelationKey.String()].GetStringValue(), nil
}

// GetFieldsByID retrieves the specified fields of an object by its ID.
func GetFieldsByID(mw apicore.ClientCommands, spaceId string, objectId string, keys []string) (map[string]*types.Value, error) {
	resp := mw.ObjectSearch(context.Background(), &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(objectId),
			},
		},
		Keys: keys,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, ErrFailedGetById
	}

	if len(resp.Records) == 0 {
		return nil, ErrFailedGetByIdNotFound
	}

	return resp.Records[0].Fields, nil
}
