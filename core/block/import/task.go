package importer

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/gogo/protobuf/types"
	"sync"
)

type Task struct {
	sn        *converter.Snapshot
	relations []*converter.Relation
	existing  bool
	oc        Creator
	wg        *sync.WaitGroup
}

func NewTask(sn *converter.Snapshot, relations []*converter.Relation, existing bool, oc Creator, wg *sync.WaitGroup) *Task {
	return &Task{sn: sn, relations: relations, existing: existing, oc: oc, wg: wg}
}

func (t *Task) Execute(ctx *session.Context, oldIDtoNew map[string]string, progress *process.Progress) (*types.Struct, string, error) {
	defer t.wg.Done()
	defer progress.AddDone(1)
	return t.oc.Create(ctx, t.sn, t.relations, oldIDtoNew, t.existing)
}
