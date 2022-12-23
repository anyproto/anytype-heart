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

	chs := Diff(origin, changed, identityString, equalString)

	assert.Equal(t, chs, []Change[string]{
		MakeChangeMove[string]([]string{"008"}, "000"),
		MakeChangeMove[string]([]string{"004"}, "009"),
	})
}

type testItem struct {
	id        string
	someField int
}

func Test_Replace(t *testing.T) {
	origin := []testItem{
		{"000", 100},
		{"001", 101},
		{"002", 102},
	}
	changed := []testItem{
		{"001", 101},
		{"002", 102},
		{"000", 103},
	}

	getID := func(a testItem) string {
		return a.id
	}
	chs := Diff(origin, changed, getID, func(a, b testItem) bool {
		if a.id != b.id {
			return false
		}
		return a.someField == b.someField
	})

	assert.Equal(t, []Change[testItem]{
		MakeChangeReplace(testItem{"000", 103}, "000"),
		MakeChangeMove[testItem]([]string{"000"}, "002"),
	}, chs)

	got := ApplyChanges(origin, chs, getID)

	assert.Equal(t, changed, got)
}

func Test_ChangesApply(t *testing.T) {
	origin := []string{"000", "001", "002", "003", "004", "005", "006", "007", "008", "009"}
	changed := []string{"000", "008", "001", "002", "003", "005", "006", "007", "009", "004", "new"}

	chs := Diff(origin, changed, identityString, equalString)

	res := ApplyChanges(origin, chs, identityString)

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

		chs := Diff(origin, changed, identityString, equalString)
		res := ApplyChanges(origin, chs, identityString)

		assert.Equal(t, res, changed)
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

		chs := Diff(origin, changed, identityString, equalString)
		res := ApplyChanges(origin, chs, identityString)

		assert.Equal(t, res, changed)
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
