package merge

import (
	"sort"
	"testing"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/vclock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	_"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/stretchr/testify/assert"
)


type change struct {
	name string
	apply func(s *state.State)
	vclock.VClock
}

func Test_Merge(t *testing.T) {
	var changesA, changesB []change
	uA := vclock.New()
	uA.Increment("a")
	changesA = append(changesA, newChange("create doc", uA, func(s *state.State) {
		s.Add(newBlock("root"))
	}))
	uA.Increment("a")
	changesA = append(changesA, newChange("create block a1", uA, func(s *state.State) {
		s.Add(newBlock("a1"))
		s.InsertTo("root", model.Block_Inner, "a1")
	}))

	uB := uA.Copy()
	uB.Increment("b")
	changesB = append(changesB, newChange("create block b1 after a1", uB, func(s *state.State) {
		s.Add(newBlock("b1"))
		s.InsertTo("a1", model.Block_Bottom, "b1")
	}))

	uA.Increment("a")
	changesA = append(changesA, newChange("create block a2 after a1", uA, func(s *state.State) {
		s.Add(newBlock("a2"))
		s.InsertTo("a1", model.Block_Bottom, "a2")
	}))

	uA.Increment("a")
	uA.Merge(uB)
	changesA = append(changesA, changesB...)
	uA.Increment("a")
	changesA = append(changesA, newChange("create block a3 after a2", uA, func(s *state.State) {
		s.Add(newBlock("a3"))
		s.InsertTo("a2", model.Block_Bottom, "a3")
	}))

	uB.Increment("b")
	uB.Merge(uA)

	t.Log("Doc A:")
	printDoc(t, changesA)

	t.Log("Doc B:")
	printDoc(t, append(changesB, changesA...))

}

func printDoc(t *testing.T, changes []change) {
	sortChanges(changes)
	doc := state.NewDoc("root", nil)
	for _, ch := range changes {
		t.Log(ch.name, ch.VClock.String())
		s := doc.NewState()
		ch.apply(s)
		_, _, err := state.ApplyState(s)
		assert.NoError(t, err)
	}
	t.Log("Result:",doc.(*state.State).String())
}

func newBlock(id string) simple.Block {
	return simple.New(&model.Block{Id:id})
}

func sortChanges(changes []change) {
	sort.Slice(changes, func(i, j int) bool {
		return changes[i].Compare(changes[j].VClock, vclock.Descendant)
	})
}


func newChange(name string, vc vclock.VClock, apply func(s *state.State)) change {
	return change{
		name: name,
		apply: apply,
		VClock: vc.Copy(),
	}
}