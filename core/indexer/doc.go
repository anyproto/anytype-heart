package indexer

import (
	"fmt"
	"time"

	"github.com/anytypeio/go-anytype-middleware/change"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
)

func newDoc(id string, a anytype.Service) (d *doc, err error) {
	sb, err := a.GetBlock(id)
	if err != nil {
		err = fmt.Errorf("anytype.GetBlock error: %v", err)
		return
	}
	d = &doc{
		id:        id,
		lastUsage: time.Now(),
	}
	d.tree, _, err = change.BuildMetaTree(sb)
	if err == change.ErrEmpty {
		d.tree = change.NewMetaTree()
		d.st = state.NewDoc(id, nil).(*state.State)
		err = nil
	} else if err != nil {
		return
	} else {
		if d.st, err = d.buildState(); err != nil {
			return
		}
	}
	return
}

type doc struct {
	id   string
	tree *change.Tree
	st   *state.State

	changesBuf []*change.Change

	lastUsage time.Time
}

func (d *doc) meta() core.SmartBlockMeta {
	return core.SmartBlockMeta{
		ObjectTypes: d.st.ObjectTypes(),
		Relations:   d.st.ExtraRelations(),
		Details:     d.st.Details(),
	}
}

func (d *doc) addRecords(records ...core.SmartblockRecordEnvelope) (lastChangeTS int64, hasMetaChanges bool) {
	d.lastUsage = time.Now()
	var changes = d.changesBuf[:0]
	for _, rec := range records {
		c, err := change.NewChangeFromRecord(rec)
		if err != nil {
			log.Warnf("indexer: can't make change from record: %v", err)
			continue
		}
		if n := time.Now().Unix(); c.Timestamp > n {
			c.Timestamp = n
		}

		if c.Timestamp > lastChangeTS {
			lastChangeTS = c.Timestamp
		}

		if c.HasMeta() {
			changes = append(changes, c)
		}
	}
	if len(changes) == 0 {
		return
	}

	switch d.tree.Add(changes...) {
	case change.Nothing:
		return
	case change.Append:
		s, err := change.BuildStateSimpleCRDT(d.st, d.tree)
		if err != nil {
			log.Warnf("indexer: can't build crdt state (append): %v", err)
			return
		}
		_, _, err = state.ApplyStateFast(s)
		if err != nil {
			log.Warnf("indexer: can't apply state: %v", err)
			return
		}
		hasMetaChanges = true
		s.FillLocalScopeDetails()
		return
	case change.Rebuild:
		doc, err := d.buildState()
		if err != nil {
			log.Warnf("indexer: can't build crdt state (rebuild): %v", err)
			return
		}
		d.st = doc
		hasMetaChanges = true
		return
	}
	return
}

func (d *doc) buildState() (doc *state.State, err error) {
	root := d.tree.Root()
	if root == nil || root.GetSnapshot() == nil {
		return nil, fmt.Errorf("root missing or not a snapshot")
	}
	doc = state.NewDocFromSnapshot(d.id, root.GetSnapshot()).(*state.State)
	doc.SetChangeId(root.Id)
	st, err := change.BuildStateSimpleCRDT(doc, d.tree)
	if err != nil {
		return
	}
	if _, _, err = state.ApplyStateFast(st); err != nil {
		return
	}
	doc.FillLocalScopeDetails()
	return
}
