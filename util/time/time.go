package time

import (
	"encoding/binary"
	"encoding/hex"
	"time"
)

func CutToDay(t time.Time) time.Time {
	roundTime := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	return roundTime
}

func BsonIdToTimestamp(id string) (int64, bool) {
	if len(id) != 24 {
		return 0, false
	}

	b, err := hex.DecodeString(id[:8]) // 4 bytes
	if err != nil || len(b) != 4 {
		return 0, false
	}

	return int64(binary.BigEndian.Uint32(b)), true
}
