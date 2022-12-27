package slice

import (
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"
	"time"

	"github.com/globalsign/mgo/bson"
	"github.com/stretchr/testify/assert"
)

func Test_Diff(t *testing.T) {
	origin := []string{"000", "001", "002", "003", "004", "005", "006", "007", "008", "009"}
	changed := []string{"000", "008", "001", "002", "003", "005", "006", "007", "009", "004"}

	chs := Diff(origin, changed, StringIdentity[string], Equal[string])

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

	chs := Diff(origin, changed, StringIdentity[string], Equal[string])

	res := ApplyChanges(origin, chs, StringIdentity[string])

	assert.Equal(t, changed, res)
}

type uniqueID string

func (id uniqueID) Generate(rand *rand.Rand, size int) reflect.Value {
	return reflect.ValueOf(uniqueID(bson.NewObjectId().Hex()))
}

func Test_SameLength(t *testing.T) {
	f := func(origin []uniqueID) bool {
		changed := make([]uniqueID, len(origin))
		copy(changed, origin)
		rand.Shuffle(len(changed),
			func(i, j int) { changed[i], changed[j] = changed[j], changed[i] })

		chs := Diff(origin, changed, StringIdentity[uniqueID], Equal[uniqueID])
		res := ApplyChanges(origin, chs, StringIdentity[uniqueID])

		return assert.Equal(t, res, changed)
	}

	assert.NoError(t, quick.Check(f, nil))
}

func Test_DifferentLength(t *testing.T) {
	f := func(origin []uniqueID) bool {
		changed := make([]uniqueID, len(origin))
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
			changed = Insert(changed, insIdx, []uniqueID{uniqueID(bson.NewObjectId().Hex())}...)
		}

		chs := Diff(origin, changed, StringIdentity[uniqueID], Equal[uniqueID])
		res := ApplyChanges(origin, chs, StringIdentity[uniqueID])

		return assert.Equal(t, res, changed)
	}

	assert.NoError(t, quick.Check(f, nil))
}

// nolint:gosec
func randNum(min, max int) int {
	if max <= min {
		return max
	}
	rand.Seed(time.Now().UnixNano())
	return rand.Intn(max-min) + min
}

// nolint:gosec
func genTestItems(count int) []*testItem {
	items := make([]*testItem, count)
	for i := 0; i < count; i++ {
		items[i] = &testItem{id: bson.NewObjectId().Hex(), someField: rand.Intn(1000)}
	}
	return items
}

/*
Original
BenchmarkApplyChanges-8             3135            433618 ns/op          540552 B/op        558 allocs/op

Use FilterMut that reuses original slice capacity
BenchmarkApplyChanges-8             4134            346602 ns/op           90448 B/op        206 allocs/op

Remove dropping items in Move operations if afterID is not found (can cause data loss)
BenchmarkApplyChanges-8             2691            421134 ns/op          236785 B/op        385 allocs/op

Faster insert and preallocations
BenchmarkApplyChanges-8             3098            426538 ns/op            7480 B/op        129 allocs/op
*/
func BenchmarkApplyChanges(b *testing.B) {
	const itemsCount = 100
	items := genTestItems(itemsCount)

	changes := make([]Change[*testItem], 500)
	for i := 0; i < 500; i++ {
		switch rand.Intn(4) {
		case 0:
			it := items[rand.Intn(itemsCount)]
			newItem := &(*it)
			newItem.someField = rand.Intn(1000)
			changes[i] = MakeChangeReplace(newItem, it.id)
		case 1:
			idx := rand.Intn(itemsCount + 1)
			var id string
			// Let it be a chance to use empty AfterID
			if idx < itemsCount {
				id = items[idx].id
			}
			changes[i] = MakeChangeAdd(genTestItems(rand.Intn(2)+1), id)
		case 2:
			changes[i] = MakeChangeRemove[*testItem]([]string{items[rand.Intn(itemsCount)].id})
		case 3:
			idx := rand.Intn(itemsCount + 1)
			var id string
			// Let it be a chance to use empty AfterID
			if idx < itemsCount {
				id = items[idx].id
			}
			changes[i] = MakeChangeMove[*testItem]([]string{items[rand.Intn(itemsCount)].id}, id)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ApplyChanges(items, changes, func(a *testItem) string {
			return a.id
		})
	}
}
