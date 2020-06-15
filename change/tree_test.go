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
		assert.Equal(t, Append, tr.Add(newSnapshot("root", "", nil)))
		assert.Equal(t, tr.root.Id, "root")
		assert.Equal(t, []string{"root"}, tr.Heads())
		assert.Equal(t, []string{"root"}, tr.DetailsHeads())
	})
	t.Run("linear add", func(t *testing.T) {
		tr := new(Tree)
		assert.Equal(t, Append, tr.Add(
			newSnapshot("root", "", nil),
			newDetailsChange("one", "root", "root", "root", true),
			newDetailsChange("two", "root", "one", "one", false),
		))
		assert.Equal(t, Append, tr.Add(newDetailsChange("three", "root", "two", "one", false)))
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
		assert.Equal(t, []string{"three"}, tr.Heads())
		assert.Equal(t, []string{"one"}, tr.DetailsHeads())
	})
	t.Run("branch", func(t *testing.T) {
		tr := new(Tree)
		assert.Equal(t, Append, tr.Add(
			newSnapshot("root", "", nil),
			newDetailsChange("1", "root", "root", "root", false),
			newDetailsChange("2", "root", "1", "root", true),
		))
		assert.Equal(t, Rebuild, tr.Add(
			newDetailsChange("1.2", "root", "1.1", "root", true),
			newDetailsChange("1.3", "root", "1.2", "root", false),
			newDetailsChange("1.1", "root", "1", "root", false),
		))
		assert.Len(t, tr.attached["1"].Next, 2)
		assert.Len(t, tr.unAttached, 0)
		assert.Len(t, tr.attached, 6)
		assert.Equal(t, []string{"1.3", "2"}, tr.Heads())
		assert.Equal(t, []string{"1.2", "2"}, tr.DetailsHeads())
	})
	t.Run("branch union", func(t *testing.T) {
		tr := new(Tree)
		c3 := newDetailsChange("3", "root", "", "", true)
		c3.PreviousDetailsIds = []string{"2", "1.3"}
		c3.PreviousIds = []string{"2", "1.3"}
		assert.Equal(t, Append, tr.Add(
			newSnapshot("root", "", nil),
			newDetailsChange("1", "root", "root", "root", false),
			newDetailsChange("2", "root", "1", "root", true),
			newDetailsChange("1.2", "root", "1.1", "root", false),
			newDetailsChange("1.3", "root", "1.2", "root", true),
			newDetailsChange("1.1", "root", "1", "root", false),
			c3,
			newDetailsChange("4", "root", "3", "3", false),
		))
		assert.Len(t, tr.unAttached, 0)
		assert.Len(t, tr.attached, 8)
		assert.Equal(t, []string{"4"}, tr.Heads())
		assert.Equal(t, []string{"3"}, tr.DetailsHeads())
	})
	t.Run("big set", func(t *testing.T) {
		tr := new(Tree)
		tr.Add(newSnapshot("root", "", nil))
		var changes []*Change
		for i := 0; i < 10000; i++ {
			if i == 0 {
				changes = append(changes, newDetailsChange(fmt.Sprint(i), "root", "root", "root", false))
			} else {
				changes = append(changes, newDetailsChange(fmt.Sprint(i), "root", fmt.Sprint(i-1), "root", false))
			}
		}
		rand.Shuffle(len(changes), func(i, j int) {
			changes[i], changes[j] = changes[j], changes[i]
		})
		st := time.Now()
		tr.Add(changes...)
		t.Log(time.Since(st))
		assert.Equal(t, []string{"9999"}, tr.Heads())
		assert.Equal(t, []string{"root"}, tr.DetailsHeads())
	})
}

func TestTree_Hash(t *testing.T) {
	tr := new(Tree)
	tr.Add(newSnapshot("root", "", nil))
	hash1 := tr.Hash()
	assert.Equal(t, tr.Hash(), hash1)
	tr.Add(newDetailsChange("1", "root", "root", "root", false))
	assert.NotEqual(t, tr.Hash(), hash1)
	assert.Equal(t, tr.Hash(), tr.Hash())
}

func TestTree_AddFuzzy(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	getChanges := func() []*Change {
		c3 := newDetailsChange("3", "root", "", "1.3", false)
		c3.PreviousIds = []string{"2", "1.3"}
		changes := []*Change{
			newDetailsChange("1", "root", "root", "root", false),
			newDetailsChange("2", "root", "1", "root", false),
			newDetailsChange("1.2", "root", "1.1", "root", false),
			newDetailsChange("1.3", "root", "1.2", "root", true),
			newDetailsChange("1.1", "root", "1", "root", false),
			c3,
		}
		rand.Shuffle(len(changes), func(i, j int) {
			changes[i], changes[j] = changes[j], changes[i]
		})
		return changes
	}
	var phash string
	for i := 0; i < 100; i++ {
		tr := new(Tree)
		tr.Add(newSnapshot("root", "", nil))
		tr.Add(getChanges()...)
		assert.Len(t, tr.unAttached, 0)
		assert.Len(t, tr.attached, 7)
		hash := tr.Hash()
		if phash != "" {
			assert.Equal(t, phash, hash)
		}
		phash = hash
		assert.Equal(t, []string{"3"}, tr.Heads())
		assert.Equal(t, []string{"1.3"}, tr.DetailsHeads())
	}
}

func BenchmarkTree_Add(b *testing.B) {
	c3 := newDetailsChange("3", "root", "", "1.3", false)
	c3.PreviousIds = []string{"2", "1.3"}
	changes := []*Change{
		newDetailsChange("1", "root", "root", "root", false),
		newDetailsChange("2", "root", "1", "root", false),
		newDetailsChange("1.2", "root", "1.1", "root", false),
		newDetailsChange("1.3", "root", "1.2", "root", true),
		newDetailsChange("1.1", "root", "1", "root", false),
		c3,
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tr := new(Tree)
		tr.Add(newSnapshot("root", "", nil))
		tr.Add(changes...)
	}
}
