package change

import (
	"sort"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
)

func newStateCache() *stateCache {
	return &stateCache{
		states: make(map[string]struct {
			refs  int
			state *state.State
		}),
	}
}

type stateCache struct {
	states map[string]struct {
		refs  int
		state *state.State
	}
}

func (sc *stateCache) Set(id string, s *state.State, refs int) {
	sc.states[id] = struct {
		refs  int
		state *state.State
	}{refs: refs, state: s}
}

func (sc *stateCache) Get(id string) *state.State {
	item := sc.states[id]
	item.refs--
	if item.refs == 0 {
		delete(sc.states, id)
	} else {
		sc.states[id] = item
	}
	return item.state
}

func BuildState(root *state.State, t *Tree) (s *state.State, err error) {
	var (
		sc        = newStateCache()
		startId   string
		applyRoot bool
	)
	if startId = root.ChangeId(); startId == "" {
		startId = t.RootId()
		applyRoot = true
	}
	t.Iterate(startId, func(c *Change) (isContinue bool) {
		if root.ChangeId() == c.Id {
			s = root
			if applyRoot {
				s = s.NewState()
				if err = s.ApplyChange(c.Change); err != nil {
					return false
				}
			}
			sc.Set(c.Id, s, len(c.Next))
			return true
		}
		if len(c.PreviousIds) == 1 {
			s = sc.Get(c.PreviousIds[0]).NewState()
			if err = s.ApplyChange(c.Change); err != nil {
				return false
			}
			s.SetChangeId(c.Id)
			sc.Set(c.Id, s, len(c.Next))
		} else if len(c.PreviousIds) > 1 {
			toMerge := make([]*state.State, len(c.PreviousIds))
			sort.Strings(c.PreviousIds)
			for i, prevId := range c.PreviousIds {
				toMerge[i] = sc.Get(prevId)
			}
			s = merge(toMerge...)
			if err = s.ApplyChange(c.Change); err != nil {
				return false
			}
			s.SetChangeId(c.Id)
			sc.Set(c.Id, s, len(c.Next))
		}
		return true
	})
	if err != nil {
		return nil, err
	}
	if len(t.headIds) > 1 {
		toMerge := make([]*state.State, len(t.headIds))
		sort.Strings(t.headIds)
		for i, hid := range t.headIds {
			if s.ChangeId() == hid {
				toMerge[i] = s
			} else {
				toMerge[i] = sc.Get(hid)
			}
		}
		s = merge(toMerge...)
	}
	return
}

func merge(states ...*state.State) (s *state.State) {
	for _, st := range states {
		if s == nil {
			s = st
		} else {
			s = s.Merge(st)
		}
	}
	return s
}
