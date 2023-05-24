package time

import (
	"time"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func CutValueToDay(val *types.Value) *types.Value {
	t := time.Unix(int64(val.GetNumberValue()), 0)
	roundTime := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	return pbtypes.Int64(roundTime.Unix())
}
