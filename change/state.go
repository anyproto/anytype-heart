package change

import (
	"sort"
	"time"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
)

func NewStateCache() *stateCache {
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

// Simple implementation hopes for CRDT and ignores errors. No merge
func BuildStateSimpleCRDT(root *state.State, t *Tree) (s *state.State, err error) {
	var (
		startId   string
		applyRoot bool
		st        = time.Now()
		count     int
	)
	if startId = root.ChangeId(); startId == "" {
		startId = t.RootId()
		applyRoot = true
	}
	t.Iterate(startId, func(c *Change) (isContinue bool) {
		count++
		if startId == c.Id {
			s = root.NewState()
			if applyRoot {
				s.ApplyChangeIgnoreErr(c.Change.Content...)
				s.SetChangeId(c.Id)
			}
			return true
		}
		ns := s.NewState()
		ns.ApplyChangeIgnoreErr(c.Change.Content...)
		ns.SetChangeId(c.Id)
		_, _, err = state.ApplyStateFastOne(ns)
		if err != nil {
			return false
		}
		return true
	})
	if err != nil {
		return nil, err
	}
	log.Infof("build state (crdt): startId: %v; applyRoot: %v; changes: %d; dur: %v;", startId, applyRoot, count, time.Since(st))
	return s, err
}

// Full version found parallel branches and proposes to resolve conflicts
func BuildState(root *state.State, t *Tree) (s *state.State, err error) {
	var (
		sc        = NewStateCache()
		startId   string
		applyRoot bool
		st        = time.Now()
		count     int
	)
	if startId = root.ChangeId(); startId == "" {
		startId = t.RootId()
		applyRoot = true
	}
	t.IterateBranching(startId, func(c *Change, branchLevel int) (isContinue bool) {
		if root.ChangeId() == c.Id {
			s = root
			if applyRoot {
				s = s.NewState()
				if err = s.ApplyChange(c.Change.Content...); err != nil {
					return false
				}
				count++
			}
			sc.Set(c.Id, s, len(c.Next))
			return true
		}
		if len(c.PreviousIds) == 1 {
			ps := sc.Get(c.PreviousIds[0])
			s := ps.NewState()
			if err = s.ApplyChange(c.Change.Content...); err != nil {
				return false
			}
			count++
			s.SetChangeId(c.Id)
			if branchLevel == 0 {
				if _, _, err = state.ApplyStateFastOne(s); err != nil {
					return false
				}
				sc.Set(c.Id, ps, len(c.Next))
			} else {
				sc.Set(c.Id, s, len(c.Next))
			}
		} else if len(c.PreviousIds) > 1 {
			toMerge := make([]*state.State, len(c.PreviousIds))
			sort.Strings(c.PreviousIds)
			for i, prevId := range c.PreviousIds {
				toMerge[i] = sc.Get(prevId)
			}
			ps := merge(t, toMerge...)
			s := ps.NewState()
			if err = s.ApplyChange(c.Change.Content...); err != nil {
				return false
			}
			count++
			s.SetChangeId(c.Id)
			if branchLevel == 0 {
				if _, _, err = state.ApplyStateFastOne(s); err != nil {
					return false
				}
				sc.Set(c.Id, ps, len(c.Next))
			} else {
				sc.Set(c.Id, s, len(c.Next))
			}
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
		s = merge(t, toMerge...)
	}
	log.Infof("build state: changes: %d; dur: %v;", count, time.Since(st))
	return
}

func merge(t *Tree, states ...*state.State) (s *state.State) {
	for _, st := range states {
		if s == nil {
			s = st
		} else {
			s = s.Merge(st)
		}
	}
	return s
}
