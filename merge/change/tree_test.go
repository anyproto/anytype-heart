package change

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTree_Add(t *testing.T) {
	t.Run("add first el", func(t *testing.T) {
		tr := new(Tree)
		tr.Add(&Change{Id: "root"})
		assert.Equal(t, tr.root.Id, "root")
		assert.Equal(t, []string{"root"}, tr.headIds)
	})
	t.Run("linear add", func(t *testing.T) {
		tr := new(Tree)
		tr.Add(
			&Change{Id: "root"},
			&Change{Id: "one", PreviousIds: []string{"root"}},
			&Change{Id: "two", PreviousIds: []string{"one"}},
		)
		tr.Add(&Change{Id: "three", PreviousIds: []string{"two"}})
		el := tr.root
		var ids []string
		for el != nil {
			ids = append(ids, el.Id)
			if len(el.Next) > 0 {
				el = el.Next[0]
			} else {
				el = nil
			}
		}
		assert.Equal(t, []string{"root", "one", "two", "three"}, ids)
	})
	t.Run("branch", func(t *testing.T) {
		tr := new(Tree)
		tr.Add(
			&Change{Id: "root"},
			&Change{Id: "1", PreviousIds: []string{"root"}},
			&Change{Id: "2", PreviousIds: []string{"1"}},
		)
		tr.Add(
			&Change{Id: "1.2", PreviousIds: []string{"1.1"}},
			&Change{Id: "1.3", PreviousIds: []string{"1.2"}},
			&Change{Id: "1.1", PreviousIds: []string{"1"}},
		)
		assert.Len(t, tr.attached["1"].Next, 2)
		assert.Len(t, tr.unAttached, 0)
		assert.Len(t, tr.attached, 6)
	})
	t.Run("branch union", func(t *testing.T) {
		tr := new(Tree)
		tr.Add(
			&Change{Id: "root"},
			&Change{Id: "1", PreviousIds: []string{"root"}},
			&Change{Id: "2", PreviousIds: []string{"1"}},
			&Change{Id: "1.2", PreviousIds: []string{"1.1"}},
			&Change{Id: "1.3", PreviousIds: []string{"1.2"}},
			&Change{Id: "1.1", PreviousIds: []string{"1"}},
			&Change{Id: "3", PreviousIds: []string{"2", "1.3"}},
		)
		assert.Len(t, tr.unAttached, 0)
		assert.Len(t, tr.attached, 7)
	})
	t.Run("big set", func(t *testing.T) {
		tr := new(Tree)
		tr.Add(&Change{Id: "root"})
		var changes []*Change
		for i := 0; i < 10000; i++ {
			if i == 0 {
				changes = append(changes, &Change{Id: fmt.Sprint(i), PreviousIds: []string{"root"}})
			} else {
				changes = append(changes, &Change{Id: fmt.Sprint(i), PreviousIds: []string{fmt.Sprint(i - 1)}})
			}
		}
		rand.Shuffle(len(changes), func(i, j int) {
			changes[i], changes[j] = changes[j], changes[i]
		})
		st := time.Now()
		tr.Add(changes...)
		t.Log(time.Since(st))
		assert.Equal(t, tr.headIds, []string{"9999"})
	})
}

func TestTree_Hash(t *testing.T) {
	tr := new(Tree)
	tr.Add(&Change{Id: "root"})
	hash1 := tr.Hash()
	assert.Equal(t, tr.Hash(), hash1)
	tr.Add(&Change{Id: "1", PreviousIds: []string{"root"}})
	assert.NotEqual(t, tr.Hash(), hash1)
	assert.Equal(t, tr.Hash(), tr.Hash())
}

func TestTree_AddFuzzy(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	getChanges := func() []*Change {
		changes := []*Change{
			{Id: "1", PreviousIds: []string{"root"}},
			{Id: "2", PreviousIds: []string{"1"}},
			{Id: "1.2", PreviousIds: []string{"1.1"}},
			{Id: "1.3", PreviousIds: []string{"1.2"}},
			{Id: "1.1", PreviousIds: []string{"1"}},
			{Id: "3", PreviousIds: []string{"2", "1.3"}},
		}
		rand.Shuffle(len(changes), func(i, j int) {
			changes[i], changes[j] = changes[j], changes[i]
		})
		return changes
	}
	var phash string
	for i := 0; i < 100; i++ {
		tr := new(Tree)
		tr.Add(&Change{Id: "root"})
		tr.Add(getChanges()...)
		assert.Len(t, tr.unAttached, 0)
		assert.Len(t, tr.attached, 7)
		hash := tr.Hash()
		if phash != "" {
			assert.Equal(t, phash, hash)
		}
		phash = hash
	}
}

func BenchmarkTree_Add(b *testing.B) {
	for i := 0; i < b.N; i++ {
		tr := new(Tree)
		tr.Add(&Change{Id: "root"})
		tr.Add([]*Change{
			{Id: "1", PreviousIds: []string{"root"}},
			{Id: "2", PreviousIds: []string{"1"}},
			{Id: "1.2", PreviousIds: []string{"1.1"}},
			{Id: "1.3", PreviousIds: []string{"1.2"}},
			{Id: "1.1", PreviousIds: []string{"1"}},
			{Id: "3", PreviousIds: []string{"2", "1.3"}},
		}...)
	}
}
