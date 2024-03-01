package integration

import (
	"golang.org/x/exp/constraints"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func filterEqualsToString(key domain.RelationKey, value string) *model.BlockContentDataviewFilter {
	return &model.BlockContentDataviewFilter{
		RelationKey: key.String(),
		Condition:   model.BlockContentDataviewFilter_Equal,
		Value:       pbtypes.String(value),
	}
}

func filterNotEmpty(key domain.RelationKey) *model.BlockContentDataviewFilter {
	return &model.BlockContentDataviewFilter{
		RelationKey: key.String(),
		Condition:   model.BlockContentDataviewFilter_NotEmpty,
	}
}

func filterEqualsToInteger[T constraints.Integer](key domain.RelationKey, value T) *model.BlockContentDataviewFilter {
	return &model.BlockContentDataviewFilter{
		RelationKey: key.String(),
		Condition:   model.BlockContentDataviewFilter_Equal,
		Value:       pbtypes.Int64(int64(value)),
	}
}
