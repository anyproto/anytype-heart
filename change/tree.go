package change

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"sort"

	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

type Mode int

const (
	Append Mode = iota
	Rebuild
	Nothing
)

func NewTree() *Tree {
	return &Tree{}
}

func NewDetailsTree() *Tree {
	return &Tree{detailsOnly: true}
}

type Tree struct {
	root           *Change
	headIds        []string
	detailsHeadIds []string
	attached       map[string]*Change
	unAttached     map[string]*Change
	// missed id -> list of dependency ids
	waitList    map[string][]string
	detailsOnly bool
}

func (t *Tree) RootId() string {
	if t.root != nil {
		return t.root.Id
	}
	return ""
}

func (t *Tree) Root() *Change {
	return t.root
}

func (t *Tree) AddFast(changes ...*Change) {
	for _, c := range changes {
		// ignore existing
		if _, ok := t.attached[c.Id]; ok {
			continue
		} else if _, ok := t.unAttached[c.Id]; ok {
			continue
		}
		t.add(c)
	}
	t.updateHeads()
}

func (t *Tree) Add(changes ...*Change) (mode Mode) {
	var beforeHeadIds = t.headIds
	var attached bool
	for _, c := range changes {
		// ignore existing
		if _, ok := t.attached[c.Id]; ok {
			continue
		} else if _, ok := t.unAttached[c.Id]; ok {
			continue
		}
		if t.add(c) {
			attached = true
		}
	}
	if !attached {
		return Nothing
	}
	t.updateHeads()
	for _, hid := range beforeHeadIds {
		for _, newCh := range changes {
			if _, ok := t.attached[newCh.Id]; ok {
				if !t.after(newCh.Id, hid) {
					return Rebuild
				}
			}
		}
	}
	return Append
}

func (t *Tree) add(c *Change) (attached bool) {
	if t.detailsOnly {
		c.PreviousIds = c.PreviousDetailsIds
	}
	if t.root == nil { // first element
		t.root = c
		t.attached = map[string]*Change{
			c.Id: c,
		}
		t.unAttached = make(map[string]*Change)
		t.waitList = make(map[string][]string)
		return true
	}
	sort.Strings(c.PreviousIds)
	for _, pid := range c.PreviousIds {
		if prev, ok := t.attached[pid]; ok {
			prev.Next = append(prev.Next, c)
			attached = true
			if len(prev.Next) > 1 {
				sort.Sort(sortChanges(prev.Next))
			}
		} else if prev, ok := t.unAttached[pid]; ok {
			prev.Next = append(prev.Next, c)
			if len(prev.Next) > 1 {
				sort.Sort(sortChanges(prev.Next))
			}
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
			if len(c.Next) > 1 {
				sort.Sort(sortChanges(c.Next))
			}
			t.attach(next, false)
		}
		delete(t.waitList, c.Id)
	}
}

func (t *Tree) after(id1, id2 string) (found bool) {
	t.iterate(t.attached[id2], func(c *Change) (isContinue bool) {
		if c.Id == id1 {
			found = true
			return false
		}
		return true
	})
	return
}

func (t *Tree) updateHeads() {
	var newHeadIds, newDetailsHeadIds []string
	t.iterate(t.root, func(c *Change) (isContinue bool) {
		if len(c.Next) == 0 {
			newHeadIds = append(newHeadIds, c.Id)
		}
		if c.HasDetails() {
			for _, prevDetId := range c.PreviousDetailsIds {
				newDetailsHeadIds = slice.Remove(newDetailsHeadIds, prevDetId)
			}
			newDetailsHeadIds = append(newDetailsHeadIds, c.Id)
		}
		return true
	})
	t.headIds = newHeadIds
	t.detailsHeadIds = newDetailsHeadIds
	sort.Strings(t.headIds)
	sort.Strings(t.detailsHeadIds)
}

func (t *Tree) iterate(start *Change, f func(c *Change) (isContinue bool)) bool {
	if start == nil {
		return false
	}
	if !f(start) {
		return false
	}
	for _, n := range start.Next {
		if len(n.PreviousIds) > 1 && start.Id != n.PreviousIds[len(n.PreviousIds)-1] {
			continue
		}
		if !t.iterate(n, f) {
			return false
		}
	}
	return true
}

func (t *Tree) Iterate(startId string, f func(c *Change) (isContinue bool)) {
	t.iterate(t.attached[startId], f)
}

func (t *Tree) IterateBranching(startId string, f func(c *Change, branchLevel int) (isContinue bool)) {
	// branchLevel indicates the number of parallel branches
	var bc int
	t.iterate(t.attached[startId], func(c *Change) (isContinue bool) {
		if pl := len(c.PreviousIds); pl > 1 {
			bc -= pl - 1
		}
		bl := bc
		if nl := len(c.Next); nl > 1 {
			bc += nl - 1
		}
		return f(c, bl)
	})
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

func (t *Tree) Len() int {
	return len(t.attached)
}

func (t *Tree) Heads() []string {
	return t.headIds
}

func (t *Tree) DetailsHeads() []string {
	return t.detailsHeadIds
}

func (t *Tree) String() string {
	var buf = bytes.NewBuffer(nil)
	t.Iterate(t.RootId(), func(c *Change) (isContinue bool) {
		buf.WriteString(c.Id)
		if len(c.Next) > 1 {
			buf.WriteString("-<")
		} else if len(c.Next) > 0 {
			buf.WriteString("->")
		} else {
			buf.WriteString("-|")
		}
		return true
	})
	return buf.String()
}

func (t *Tree) Get(id string) *Change {
	return t.attached[id]
}

type sortChanges []*Change

func (s sortChanges) Len() int {
	return len(s)
}

func (s sortChanges) Less(i, j int) bool {
	return s[i].Id < s[j].Id
}

func (s sortChanges) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
