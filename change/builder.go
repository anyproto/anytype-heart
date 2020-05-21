package change

import (
	"context"
	"fmt"
	"sort"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

type StateBuilder struct {
	cache      map[string]*Change
	Tree       *Tree
	smartblock core.SmartBlock
}

func (sb *StateBuilder) Build(s core.SmartBlock) (err error) {
	sb.cache = make(map[string]*Change)
	sb.smartblock = s
	logs, err := sb.smartblock.GetLogs()
	if err != nil {
		return fmt.Errorf("GetLogs error: %v", err)
	}
	heads, err := sb.getActualHeads(logs)
	if err != nil {
		return fmt.Errorf("getActualHeads error: %v", err)
	}
	breakpoint, err := sb.findBreakpoint(heads)
	if err != nil {
		return fmt.Errorf("findBreakpoint error: %v", err)
	}
	if err = sb.buildTree(heads, breakpoint); err != nil {
		return fmt.Errorf("buildTree error: %v", err)
	}
	sb.cache = nil
	return
}

func (sb *StateBuilder) buildTree(heads []string, breakpoint string) (err error) {
	ch, err := sb.loadChange(breakpoint)
	if err != nil {
		return
	}
	sb.Tree = new(Tree)
	sb.Tree.Add(ch)
	var changes = make([]*Change, 0, len(heads)*2)
	var uniqMap = map[string]struct{}{breakpoint: {}}
	for _, id := range heads {
		changes, err = sb.loadChangesFor(id, uniqMap, changes)
		if err != nil {
			return
		}
	}
	sb.Tree.Add(changes...)
	return
}

func (sb *StateBuilder) loadChangesFor(id string, uniqMap map[string]struct{}, buf []*Change) ([]*Change, error) {
	if _, exists := uniqMap[id]; exists {
		return buf, nil
	}
	ch, err := sb.loadChange(id)
	if err != nil {
		return nil, err
	}
	for _, prev := range ch.PreviousIds {
		if buf, err = sb.loadChangesFor(prev, uniqMap, buf); err != nil {
			return nil, err
		}
	}
	uniqMap[id] = struct{}{}
	return append(buf, ch), nil
}

func (sb *StateBuilder) findBreakpoint(heads []string) (breakpoint string, err error) {
	var (
		ch          *Change
		snapshotIds []string
	)
	for _, head := range heads {
		if ch, err = sb.loadChange(head); err != nil {
			return
		}
		shId := ch.GetLastSnapshotId()
		if slice.FindPos(snapshotIds, shId) == -1 {
			snapshotIds = append(snapshotIds, shId)
		}
	}
	return sb.findCommonSnapshot(snapshotIds)
}

func (sb *StateBuilder) findCommonSnapshot(snapshotIds []string) (snapshotId string, err error) {
	if len(snapshotIds) == 1 {
		return snapshotIds[0], nil
	} else if len(snapshotIds) == 0 {
		return "", fmt.Errorf("snapshots not found")
	}
	findCommon := func(s1, s2 string) (s string, err error) {
		// fast cases
		if s1 == s2 {
			return s1, nil
		}
		ch1, err := sb.loadChange(s1)
		if err != nil {
			return "", err
		}
		if ch1.LastSnapshotId == s2 {
			return s2, nil
		}
		ch2, err := sb.loadChange(s2)
		if err != nil {
			return "", err
		}
		if ch2.LastSnapshotId == s1 {
			return s1, nil
		}
		if ch1.LastSnapshotId == ch2.LastSnapshotId && ch1.LastSnapshotId != "" {
			return ch1.LastSnapshotId, nil
		}
		// traverse
		var t1 = make([]string, 0, 5)
		var t2 = make([]string, 0, 5)
		t1 = append(t1, ch1.Id, ch1.LastSnapshotId)
		t2 = append(t2, ch2.Id, ch2.LastSnapshotId)
		for {
			lid1 := t1[len(t1)-1]
			if lid1 != "" {
				l1, e := sb.loadChange(lid1)
				if e != nil {
					return "", e
				}
				if l1.LastSnapshotId != "" {
					if slice.FindPos(t2, l1.LastSnapshotId) != -1 {
						return l1.LastSnapshotId, nil
					}
				}
				t1 = append(t1, l1.LastSnapshotId)
			}
			lid2 := t2[len(t2)-1]
			if lid2 != "" {
				l2, e := sb.loadChange(t2[len(t2)-1])
				if e != nil {
					return "", e
				}
				if l2.LastSnapshotId != "" {
					if slice.FindPos(t1, l2.LastSnapshotId) != -1 {
						return l2.LastSnapshotId, nil
					}
				}
				t2 = append(t2, l2.LastSnapshotId)
			}
			if lid1 == "" && lid2 == "" {
				// unexpected behavior - just return lesser id
				break
			}
		}
		return "", fmt.Errorf("unexpected: possible versions split")
	}

	for len(snapshotIds) > 1 {
		l := len(snapshotIds)
		shId, e := findCommon(snapshotIds[l-2], snapshotIds[l-1])
		if e != nil {
			return "", e
		}
		snapshotIds[l-2] = shId
		snapshotIds = snapshotIds[:l-1]
	}
	return snapshotIds[0], nil
}

func (sb *StateBuilder) getActualHeads(logs []core.SmartblockLog) (heads []string, err error) {
	sort.Slice(logs, func(i, j int) bool {
		return logs[i].ID < logs[j].ID
	})
	var knownHeads []string
	for _, l := range logs {
		if slice.FindPos(knownHeads, l.Head) != -1 { // do not scan known heads
			continue
		}
		sh, err := sb.getNearSnapshot(l.Head)
		if err != nil {
			return nil, err
		}
		if sh.Snapshot.LogHeads != nil {
			for _, headId := range sh.Snapshot.LogHeads {
				knownHeads = append(knownHeads, headId)
			}
		}
	}
	for _, l := range logs {
		if slice.FindPos(knownHeads, l.Head) != -1 { // do not scan known heads
			continue
		} else {
			heads = append(heads, l.Head)
		}
	}
	return
}

func (sb *StateBuilder) getNearSnapshot(id string) (sh *Change, err error) {
	ch, err := sb.loadChange(id)
	if err != nil {
		return
	}
	if ch.Snapshot != nil {
		return ch, nil
	}
	sch, err := sb.loadChange(ch.LastSnapshotId)
	if err != nil {
		return
	}
	if sch.Snapshot == nil {
		return nil, fmt.Errorf("snapshot %s is empty", ch.LastSnapshotId)
	}
	return sch, nil
}

func (sb *StateBuilder) loadChange(id string) (ch *Change, err error) {
	if ch, ok := sb.cache[id]; ok {
		return ch, nil
	}
	sr, err := sb.smartblock.GetRecord(context.TODO(), id)
	if err != nil {
		return
	}
	chp := new(pb.Change)
	if err = sr.Unmarshal(chp); err != nil {
		return
	}
	ch = &Change{Id: id, Change: chp}
	sb.cache[id] = ch
	return
}
