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
	ErrFailedResolveToUniqueKey = errors.New("failed to resolve to unique key")
	ErrFailedGetById            = errors.New("failed to get object by id")
	ErrFailedGetByIdNotFound    = errors.New("failed to find object by id")
	ErrFailedGetRelationKeys    = errors.New("failed to get relation keys")
	ErrRelationKeysNotFound     = errors.New("failed to find relation keys")
)

func PtrBool(b bool) *bool {
	return &b
}

func PtrString(s string) *string {
	return &s
}

func PtrFloat64(f float64) *float64 {
	return &f
}

func PtrStrings(s []string) *[]string {
	return &s
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

// GetFieldsById retrieves the specified fields of an object by its ID.
func GetFieldsById(mw apicore.ClientCommands, spaceId string, objectId string, keys []string) (map[string]*types.Value, error) {
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

// GetAllRelationKeys retrieves all relation keys within a space, including hidden ones.
func GetAllRelationKeys(mw apicore.ClientCommands, spaceId string) ([]string, error) {
	resp := mw.ObjectSearch(context.Background(), &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyResolvedLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_relation)),
			},
		},
		Keys: []string{bundle.RelationKeyRelationKey.String()},
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, ErrFailedGetRelationKeys
	}

	if len(resp.Records) == 0 {
		return nil, ErrRelationKeysNotFound
	}

	relationKeys := make([]string, len(resp.Records))
	for i, record := range resp.Records {
		relationKeys[i] = record.Fields[bundle.RelationKeyRelationKey.String()].GetStringValue()
	}

	return relationKeys, nil
}
