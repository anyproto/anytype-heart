package time

import (
	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

func CutValueToDay(t *types.Value) *types.Value {
	fullDate := int64(t.GetNumberValue())
	seconds := fullDate % 86400
	return pbtypes.Int64(fullDate - seconds)
}
