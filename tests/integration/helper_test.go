package integration

import (
	"golang.org/x/exp/constraints"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func filterEqualsToString(key domain.RelationKey, value string) database.FilterRequest {
	return database.FilterRequest{
		RelationKey: key,
		Condition:   model.BlockContentDataviewFilter_Equal,
		Value:       domain.String(value),
	}
}

func filterNotEmpty(key domain.RelationKey) database.FilterRequest {
	return database.FilterRequest{
		RelationKey: key,
		Condition:   model.BlockContentDataviewFilter_NotEmpty,
	}
}

func filterEqualsToInteger[T constraints.Integer](key domain.RelationKey, value T) database.FilterRequest {
	return database.FilterRequest{
		RelationKey: key,
		Condition:   model.BlockContentDataviewFilter_Equal,
		Value:       domain.Int64(value),
	}
}
