package slice

import (
	"math/rand"
	"testing"
	"time"

	"github.com/globalsign/mgo/bson"
	"github.com/stretchr/testify/assert"
)

func Test_Diff(t *testing.T) {
	origin := []string{"000", "001", "002", "003", "004", "005", "006", "007", "008", "009"}
	changed := []string{"000", "008", "001", "002", "003", "005", "006", "007", "009", "004"}

	chs := Diff(StringsToIDs(origin), StringsToIDs(changed))

	assert.Equal(t, chs, []Change[ID]{
		{Op: OperationMove, Items: []ID{"008"}, AfterId: "000"},
		{Op: OperationMove, Items: []ID{"004"}, AfterId: "009"}},
	)
}

func Test_ChangesApply(t *testing.T) {
	origin := []string{"000", "001", "002", "003", "004", "005", "006", "007", "008", "009"}
	changed := []ID{"000", "008", "001", "002", "003", "005", "006", "007", "009", "004", "new"}

	chs := Diff(StringsToIDs(origin), changed)

	res := ApplyChanges(StringsToIDs(origin), chs)

	assert.Equal(t, changed, res)
}

func Test_SameLength(t *testing.T) {
	// TODO use quickcheck here
	for i := 0; i < 10000; i++ {
		l := randNum(5, 200)
		origin := getRandArray(l)
		changed := make([]string, len(origin))
		copy(changed, origin)
		rand.Shuffle(len(changed),
			func(i, j int) { changed[i], changed[j] = changed[j], changed[i] })

		chs := Diff(StringsToIDs(origin), StringsToIDs(changed))
		res := ApplyChanges(StringsToIDs(origin), chs)

		assert.Equal(t, res, StringsToIDs(changed))
	}
}

func Test_DifferentLength(t *testing.T) {
	for i := 0; i < 10000; i++ {
		l := randNum(5, 200)
		origin := getRandArray(l)
		changed := make([]string, len(origin))
		copy(changed, origin)
		rand.Shuffle(len(changed),
			func(i, j int) { changed[i], changed[j] = changed[j], changed[i] })

		delCnt := randNum(0, 10)
		for i := 0; i < delCnt; i++ {
			l := len(changed) - 1
			if l <= 0 {
				continue
			}
			delIdx := randNum(0, l)
			changed = Remove(changed, changed[delIdx])
		}

		insCnt := randNum(0, 10)
		for i := 0; i < insCnt; i++ {
			l := len(changed) - 1
			if l <= 0 {
				continue
			}
			insIdx := randNum(0, l)
			changed = Insert(changed, insIdx, []string{bson.NewObjectId().Hex()}...)
		}

		chs := Diff(StringsToIDs(origin), StringsToIDs(changed))
		res := ApplyChanges(StringsToIDs(origin), chs)

		assert.Equal(t, res, StringsToIDs(changed))
	}
}

func randNum(min, max int) int {
	if max <= min {
		return max
	}
	rand.Seed(time.Now().UnixNano())
	return rand.Intn(max-min) + min
}

func getRandArray(len int) []string {
	res := make([]string, len)
	for i := 0; i < len; i++ {
		res[i] = bson.NewObjectId().Hex()
	}
	return res
}
