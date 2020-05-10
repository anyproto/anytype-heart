package change

import (
	"crypto/md5"
	"fmt"
	"sort"
)

type Mode int

const (
	Append Mode = iota
	Rebuild
	Nothing
)

type Tree struct {
	root       *Change
	headIds    []string
	attached   map[string]*Change
	unAttached map[string]*Change
	// missed id -> list of dependency ids
	waitList map[string][]string
}

func (t *Tree) RootId() string {
	if t.root != nil {
		return t.root.Id
	}
	return ""
}

func (t *Tree) Add(changes ...*Change) (mode Mode) {
	var beforeHeadIds = t.headIds
	var attached bool
	for _, c := range changes {
		if t.add(c) {
			attached = true
		}
	}
	if !attached {
		return Nothing
	}
	t.updateHeads()
	if l := len(beforeHeadIds); l > 0 && l < len(t.headIds) {
		return Rebuild
	}
	for _, oldId := range beforeHeadIds {
		for _, newId := range t.headIds {
			if !t.after(oldId, newId) {
				return Rebuild
			}
		}
	}
	return Append
}

func (t *Tree) add(c *Change) (attached bool) {
	if t.root == nil { // first element
		t.root = c
		t.attached = map[string]*Change{
			c.Id: c,
		}
		t.unAttached = make(map[string]*Change)
		t.waitList = make(map[string][]string)
		return true
	}
	for _, pid := range c.PreviousIds {
		if prev, ok := t.attached[pid]; ok {
			prev.Next = append(prev.Next, c)
			attached = true
		} else if prev, ok := t.unAttached[pid]; ok {
			prev.Next = append(prev.Next, c)
		} else {
			wl := t.waitList[pid]
			wl = append(wl, c.Id)
			t.waitList[pid] = wl
		}
	}
	if attached {
		t.attach(c, true)
	} else {
		t.unAttached[c.Id] = c
	}
	return
}

func (t *Tree) attach(c *Change, newEl bool) {
	t.attached[c.Id] = c
	if !newEl {
		delete(t.unAttached, c.Id)
	}
	for _, next := range c.Next {
		t.attach(next, false)
	}
	if waitIds, ok := t.waitList[c.Id]; ok {
		for _, wid := range waitIds {
			next := t.unAttached[wid]
			if next == nil {
				next = t.attached[wid]
			}
			c.Next = append(c.Next, next)
			t.attach(next, false)
		}
		delete(t.waitList, c.Id)
	}
}

func (t *Tree) after(id1, id2 string) (found bool) {
	t.iterate(t.attached[id2], func(c *Change) (isContinue bool) {
		if c.Id == id2 {
			found = true
			return false
		}
		return true
	})
	return
}

func (t *Tree) updateHeads() {
	var newHeadIds []string
	t.iterate(t.root, func(c *Change) (isContinue bool) {
		if len(c.Next) == 0 {
			newHeadIds = append(newHeadIds, c.Id)
		}
		return true
	})
	t.headIds = newHeadIds
	sort.Strings(t.headIds)
}

func (t *Tree) iterate(start *Change, f func(c *Change) (isContinue bool)) bool {
	if start == nil {
		return false
	}
	if !f(start) {
		return false
	}
	if len(start.Next) > 0 {
		sort.Slice(start.Next, func(i, j int) bool {
			return start.Next[i].Id > start.Next[j].Id
		})
		for _, n := range start.Next {
			if !t.iterate(n, f) {
				return false
			}
		}
	}
	return true
}

func (t *Tree) Iterate(startId string, f func(c *Change) (isContinue bool)) {
	t.iterate(t.attached[startId], f)
}

func (t *Tree) Hash() string {
	h := md5.New()
	n := 0
	t.iterate(t.root, func(c *Change) (isContinue bool) {
		n++
		fmt.Fprintf(h, "-%s", c.Id)
		return true
	})
	return fmt.Sprintf("%d-%x", n, h.Sum(nil))
}
