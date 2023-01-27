package change

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"sort"
	"time"

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

func NewMetaTree() *Tree {
	return &Tree{metaOnly: true}
}

type Tree struct {
	root        *Change
	headIds     []string
	metaHeadIds []string
	attached    map[string]*Change
	unAttached  map[string]*Change
	// missed id -> list of dependency ids
	waitList map[string][]string
	metaOnly bool

	// bufs
	iterCompBuf []*Change
	iterQueue   []*Change
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
	t.updateHeads(changes)
}

func (t *Tree) Add(changes ...*Change) (mode Mode) {
	var beforeHeadIds = t.headIds
	var attached bool
	var empty = t.Len() == 0
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
	t.updateHeads(changes)
	if empty {
		return Rebuild
	}
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
	if c == nil {
		return false
	}
	if t.metaOnly {
		c.PreviousIds = c.PreviousMetaIds
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
	if len(c.PreviousIds) > 1 {
		sort.Strings(c.PreviousIds)
	}
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
	if _, ok := t.attached[c.Id]; ok {
		return
	}
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

func (t *Tree) recalculateHeads() (heads []string, metaHeads []string) {
	start := time.Now()
	total := 0
	t.iterate(t.root, func(c *Change) (isContinue bool) {
		total++
		if len(c.Next) == 0 {
			heads = append(heads, c.Id)
		}
		if c.HasMeta() {
			for _, prevDetId := range c.PreviousMetaIds {
				metaHeads = slice.Remove(metaHeads, prevDetId)
			}
			metaHeads = append(metaHeads, c.Id)
		}
		return true
	})
	if time.Since(start) > time.Millisecond*100 {
		log.Errorf("recalculateHeads took %s for %d changes", time.Since(start), total)
	}

	return
}

func (t *Tree) updateHeads(chs []*Change) {
	var newHeadIds, newMetaHeadIds []string
	if len(chs) == 1 && slice.UnsortedEquals(chs[0].PreviousIds, t.headIds) {
		// shortcut when adding to the top of the tree
		// only cover edge case when adding one change, otherwise it's not worth it
		newHeadIds = []string{chs[0].Id}
	}
	if len(chs) == 1 && chs[0].HasMeta() && slice.UnsortedEquals(chs[0].PreviousMetaIds, t.metaHeadIds) {
		// shortcut when adding to the top of the tree
		// only cover edge case when adding one change, otherwise it's not worth it
		newMetaHeadIds = []string{chs[0].Id}
	}

	if newHeadIds == nil {
		newHeadIds, newMetaHeadIds = t.recalculateHeads()
	}
	if newHeadIds != nil {
		t.headIds = newHeadIds
		sort.Strings(t.headIds)
	}

	if newMetaHeadIds != nil {
		t.metaHeadIds = newMetaHeadIds
		sort.Strings(t.metaHeadIds)
	}
}

func (t *Tree) iterate(start *Change, f func(c *Change) (isContinue bool)) {
	it := newIterator()
	defer freeIterator(it)
	it.iterate(start, f)
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

func (t *Tree) MetaHeads() []string {
	return t.metaHeadIds
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

func (t *Tree) LastSnapshotId(ctx context.Context) string {
	var sIds []string
	for _, hid := range t.headIds {
		hd := t.attached[hid]
		sId := hd.Id
		if hd.Snapshot == nil {
			sId = hd.LastSnapshotId
		}
		if slice.FindPos(sIds, sId) == -1 {
			sIds = append(sIds, sId)
		}
	}
	if len(sIds) == 1 {
		return sIds[0]
	} else if len(sIds) == 0 {
		return ""
	}
	b := &stateBuilder{
		cache: t.attached,
	}
	sId, err := b.findCommonSnapshot(ctx, sIds)
	if err != nil {
		log.Errorf("can't find common snapshot: %v", err)
	}
	return sId
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
