package importer

import (
	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/core/session"
)

type Task struct {
	sn        *converter.Snapshot
	relations []*converter.Relation
	existing  bool
	oc        Creator
}

type DataObject struct {
	oldIDtoNew map[string]string
	progress   *process.Progress
	ctx        *session.Context
}

type Result struct {
	details *types.Struct
	newID   string
	err     error
}

func NewDataObject(oldIDtoNew map[string]string, progress *process.Progress, ctx *session.Context) *DataObject {
	return &DataObject{oldIDtoNew: oldIDtoNew, progress: progress, ctx: ctx}
}

func NewTask(sn *converter.Snapshot, relations []*converter.Relation, existing bool, oc Creator) *Task {
	return &Task{sn: sn, relations: relations, existing: existing, oc: oc}
}

func (t *Task) Execute(data interface{}) interface{} {
	dataObject := data.(*DataObject)
	defer dataObject.progress.AddDone(1)
	details, newID, err := t.oc.Create(dataObject.ctx, t.sn, t.relations, dataObject.oldIDtoNew, t.existing)
	return &Result{
		details: details,
		newID:   newID,
		err:     err,
	}
}
