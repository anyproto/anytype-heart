package change

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

var (
	ErrEmpty = errors.New("logs empty")
)

var log = logging.Logger("anytype-mw-change-builder")

func BuildTreeBefore(s core.SmartBlock, beforeLogId string, includeBeforeId bool) (t *Tree, err error) {
	sb := &stateBuilder{beforeId: beforeLogId, includeBeforeId: includeBeforeId}
	err = sb.Build(s)
	return sb.tree, err
}

func BuildTree(s core.SmartBlock) (t *Tree, logHeads map[string]*Change, err error) {
	sb := new(stateBuilder)
	err = sb.Build(s)
	return sb.tree, sb.logHeads, err
}

func BuildDetailsTree(s core.SmartBlock) (t *Tree, logHeads map[string]*Change, err error) {
	sb := &stateBuilder{onlyDetails: true}
	err = sb.Build(s)
	return sb.tree, sb.logHeads, err
}

type stateBuilder struct {
	cache           map[string]*Change
	logHeads        map[string]*Change
	tree            *Tree
	smartblock      core.SmartBlock
	qt              time.Duration
	onlyDetails     bool
	beforeId        string
	includeBeforeId bool
}

func (sb *stateBuilder) Build(s core.SmartBlock) (err error) {
	st := time.Now()
	sb.smartblock = s
	logs, err := sb.getLogs()
	if err != nil {
		return err
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
	log.Infof("tree build: len: %d; scanned: %d; dur: %v (lib %v)", sb.tree.Len(), len(sb.cache), time.Since(st), sb.qt)
	sb.cache = nil
	return
}

func (sb *stateBuilder) getLogs() (logs []core.SmartblockLog, err error) {
	sb.cache = make(map[string]*Change)
	if sb.beforeId != "" {
		before, e := sb.loadChange(sb.beforeId)
		if e != nil {
			return nil, e
		}
		if sb.includeBeforeId {
			return []core.SmartblockLog{
				{Head: sb.beforeId},
			}, nil
		}
		for _, pid := range before.PreviousIds {
			logs = append(logs, core.SmartblockLog{Head: pid})
		}
		return
	}
	logs, err = sb.smartblock.GetLogs()
	if err != nil {
		return nil, fmt.Errorf("GetLogs error: %w", err)
	}
	log.Debugf("build tree: logs: %v", logs)
	sb.logHeads = make(map[string]*Change)
	if len(logs) == 0 || len(logs) == 1 && len(logs[0].Head) <= 1 {
		return nil, ErrEmpty
	}
	var nonEmptyLogs = logs[:0]
	for _, l := range logs {
		if ch, err := sb.loadChange(l.Head); err != nil {
			log.Errorf("loading head %s of the log %s failed: %v", l.Head, l.ID, err)
		} else {
			sb.logHeads[l.ID] = ch
		}
		if l.Head != "" {
			nonEmptyLogs = append(nonEmptyLogs, l)
		}
	}
	return nonEmptyLogs, nil
}

func (sb *stateBuilder) buildTree(heads []string, breakpoint string) (err error) {
	ch, err := sb.loadChange(breakpoint)
	if err != nil {
		return
	}
	if sb.onlyDetails {
		sb.tree = NewDetailsTree()
	} else {
		sb.tree = NewTree()
	}
	sb.tree.AddFast(ch)
	var changes = make([]*Change, 0, len(heads)*2)
	var uniqMap = map[string]struct{}{breakpoint: {}}
	for _, id := range heads {
		changes, err = sb.loadChangesFor(id, uniqMap, changes)
		if err != nil {
			return
		}
	}
	if sb.onlyDetails {
		var filteredChanges = changes[:0]
		for _, ch := range changes {
			if ch.HasDetails() {
				filteredChanges = append(filteredChanges, ch)
			}
		}
		changes = filteredChanges
	}
	sb.tree.AddFast(changes...)
	return
}

func (sb *stateBuilder) loadChangesFor(id string, uniqMap map[string]struct{}, buf []*Change) ([]*Change, error) {
	if _, exists := uniqMap[id]; exists {
		return buf, nil
	}
	ch, err := sb.loadChange(id)
	if err != nil {
		return nil, err
	}
	for _, prev := range ch.GetPreviousIds() {
		if buf, err = sb.loadChangesFor(prev, uniqMap, buf); err != nil {
			return nil, err
		}
	}
	uniqMap[id] = struct{}{}
	return append(buf, ch), nil
}

func (sb *stateBuilder) findBreakpoint(heads []string) (breakpoint string, err error) {
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

func (sb *stateBuilder) findCommonSnapshot(snapshotIds []string) (snapshotId string, err error) {
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
				break
			}
		}

		log.Warnf("changes build tree: possible versions split")

		// prefer not first snapshot
		if len(ch1.PreviousIds) == 0 && len(ch2.PreviousIds) > 0 {
			log.Warnf("changes build tree: prefer %s(%d prevIds) over %s(%d prevIds)", s2, len(ch2.PreviousIds), s1, len(ch1.PreviousIds))
			return s2, nil
		} else if len(ch1.PreviousIds) > 0 && len(ch2.PreviousIds) == 0 {
			log.Warnf("changes build tree: prefer %s(%d prevIds) over %s(%d prevIds)", s1, len(ch1.PreviousIds), s2, len(ch2.PreviousIds))
			return s1, nil
		}

		isEmptySnapshot := func(ch *Change) bool {
			// todo: ignore root & header blocks
			if ch.Snapshot == nil || ch.Snapshot.Data == nil || len(ch.Snapshot.Data.Blocks) <= 1 {
				return true
			}

			return false
		}

		// prefer not empty snapshot
		if isEmptySnapshot(ch1) && !isEmptySnapshot(ch2) {
			log.Warnf("changes build tree: prefer %s(not empty) over %s(empty)", s2, s1)
			return s2, nil
		} else if isEmptySnapshot(ch2) && !isEmptySnapshot(ch1) {
			log.Warnf("changes build tree: prefer %s(not empty) over %s(empty)", s1, s2)
			return s1, nil
		}

		// unexpected behavior - just return lesser id
		if s1 < s2 {
			log.Warnf("changes build tree: prefer %s (%s<%s)", s1, s1, s2)
			return s1, nil
		}
		log.Warnf("changes build tree: prefer %s (%s<%s)", s2, s2, s1)

		return s2, nil
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

func (sb *stateBuilder) getActualHeads(logs []core.SmartblockLog) (heads []string, err error) {
	sort.Slice(logs, func(i, j int) bool {
		return logs[i].ID < logs[j].ID
	})
	var knownHeads []string
	var validLogs = logs[:0]
	for _, l := range logs {
		if slice.FindPos(knownHeads, l.Head) != -1 { // do not scan known heads
			continue
		}
		sh, err := sb.getNearSnapshot(l.Head)
		if err != nil {
			log.Warnf("can't get near snapshot: %v; ignore", err)
			continue
		}
		if sh.Snapshot.LogHeads != nil {
			for _, headId := range sh.Snapshot.LogHeads {
				knownHeads = append(knownHeads, headId)
			}
		}
		validLogs = append(validLogs, l)
	}
	for _, l := range validLogs {
		if slice.FindPos(knownHeads, l.Head) != -1 { // do not scan known heads
			continue
		} else {
			heads = append(heads, l.Head)
		}
	}
	if len(heads) == 0 {
		return nil, fmt.Errorf("no usable logs in head")
	}
	return
}

func (sb *stateBuilder) getNearSnapshot(id string) (sh *Change, err error) {
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

func (sb *stateBuilder) loadChange(id string) (ch *Change, err error) {
	if ch, ok := sb.cache[id]; ok {
		return ch, nil
	}
	if sb.smartblock == nil {
		return
	}
	st := time.Now()
	sr, err := sb.smartblock.GetRecord(context.TODO(), id)
	if err != nil {
		return
	}
	sb.qt += time.Since(st)
	chp := new(pb.Change)
	if err = sr.Unmarshal(chp); err != nil {
		return
	}
	ch = &Change{
		Id:      id,
		Account: sr.AccountID,
		Device:  sr.LogID,
		Change:  chp,
	}

	if sb.onlyDetails {
		ch.PreviousIds = ch.PreviousDetailsIds
	}
	sb.cache[id] = ch
	return
}
