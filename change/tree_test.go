package change

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		assert.Equal(t, []string{"two"}, tr.Heads())
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
		assert.Equal(t, []string{"2"}, tr.Heads())
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
		st := time.Now()
		tr.AddFast(changes...)
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

func TestTree_IterateBranching(t *testing.T) {
	tr := new(Tree)
	tr.Add(
		newSnapshot("0", "", nil),
		newChange("1", "0", "0"),
		newChange("1.1", "0", "1"),
		newChange("1.2", "0", "1.1"),
		newChange("1.4", "0", "1.2", "2.3", "3.3"),
		newChange("1.5", "0", "1.4"),
		newChange("2.1", "0", "1"),
		newChange("2.2", "0", "1.1", "2.1"),
		newChange("2.3", "0", "2.2"),
		newChange("3.2", "0", "2.1"),
		newChange("3.3", "0", "3.2"),
	)
	var list []string
	var branching []int
	tr.IterateBranching("0", func(c *Change, branchLevel int) (isContinue bool) {
		list = append(list, c.Id)
		branching = append(branching, branchLevel)
		return true
	})
	var expectedList = []string{
		"0", "1", "1.1", "2.1", "1.2", "2.2", "2.3", "3.2", "3.3", "1.4", "1.5",
	}
	require.Equal(t, expectedList, list)
	var expectedBranching = []int{
		0, 0, 1, 2, 3, 2, 2, 2, 2, 0, 0,
	}
	assert.Equal(t, expectedBranching, branching)
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
	b.Run("by one", func(b *testing.B) {
		tr := new(Tree)
		tr.Add(newSnapshot("root", "", nil))
		tr.Add(changes...)
		for i := 0; i < b.N; i++ {
			tr.Add(newDetailsChange(fmt.Sprint(i+4), "root", fmt.Sprint(i+3), "root", false))
		}
	})
	b.Run("add", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			tr := new(Tree)
			tr.Add(newSnapshot("root", "", nil))
			tr.Add(changes...)
		}
	})
	b.Run("add fast", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			tr := new(Tree)
			tr.AddFast(newSnapshot("root", "", nil))
			tr.AddFast(changes...)
		}
	})
}

func TestTree_LastSnapshotId(t *testing.T) {
	t.Run("trivial", func(t *testing.T) {
		tr := new(Tree)
		assert.Equal(t, Append, tr.Add(
			newSnapshot("root", "", nil),
			newDetailsChange("one", "root", "root", "root", true),
			newDetailsChange("two", "root", "one", "one", false),
		))
		assert.Equal(t, "root", tr.LastSnapshotId())
		assert.Equal(t, Append, tr.Add(newSnapshot("three", "root", nil, "two")))
		assert.Equal(t, "three", tr.LastSnapshotId())
	})
	t.Run("empty", func(t *testing.T) {
		tr := new(Tree)
		assert.Equal(t, "", tr.LastSnapshotId())
	})
	t.Run("builder", func(t *testing.T) {
		tr := new(Tree)
		tr.Add(
			newSnapshot("root", "", nil),
			newDetailsChange("one", "root", "root", "root", true),
			newDetailsChange("two", "root", "one", "one", false),
			newSnapshot("newSh", "root", nil, "one"),
		)
		assert.Equal(t, []string{"newSh", "two"}, tr.Heads())
		assert.Equal(t, "root", tr.LastSnapshotId())
	})
}
