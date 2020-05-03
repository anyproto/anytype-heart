package change

import (
	"crypto/md5"
	"fmt"
	"hash"
	"sort"
)

type Tree struct {
	root       *Change
	headIds      []string
	attached   map[string]*Change
	unAttached map[string]*Change
	// missed id -> list of dependency ids
	waitList map[string][]string
}

func (t *Tree) Add(changes ...*Change) (needApply bool) {
	for _, c := range changes {
		if t.add(c) {
			needApply = true
		}
	}
	return
}

func (t *Tree) add(c *Change) (needApply bool) {
	if t.root == nil { // first element
		t.root = c
		t.headIds = []string{c.Id}
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
			needApply = true
		} else if prev, ok := t.unAttached[pid]; ok {
			prev.Next = append(prev.Next, c)
		} else {
			wl := t.waitList[pid]
			wl = append(wl, c.Id)
			t.waitList[pid] = wl
		}
	}
	if needApply {
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

func (t *Tree) Hash() string {
	h := md5.New()
	n := 0
	var calc func(c *Change, h hash.Hash)
	calc = func(c *Change, h hash.Hash) {
		n++
		fmt.Fprintf(h, "-%s", c.Id)
		if len(c.Next) > 0 {
			sort.Slice(c.Next, func(i, j int) bool {
				return c.Next[i].Id >  c.Next[j].Id
			})
			for _, n := range c.Next {
				calc(n, h)
			}
		}
	}
	if t.root != nil {
		calc(t.root, h)
	}
	return fmt.Sprintf("%d-%x", n, h.Sum(nil))
}
