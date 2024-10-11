package time

import (
	"time"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func CutValueToDay(val *types.Value) *types.Value {
	if val == nil {
		return val
	}
	t := time.Unix(int64(val.GetNumberValue()), 0)
	return pbtypes.Int64(CutToDay(t).Unix())
}

func CutToDay(t time.Time) time.Time {
	roundTime := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	return roundTime
}
