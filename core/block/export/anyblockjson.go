package export

// anyblockjson.go routes model.Export_AnyBlockV2 to the native bundle
// exporter (core/block/export/anyblock). The format's own pipeline — plan,
// emit, compose, finish — lives there; what belongs HERE is only what the
// export service owns: the writer, the collected doc set, and the
// process.Queue every export reports progress and cancellation through.

import (
	"context"
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/export/anyblock"
	"github.com/anyproto/anytype-heart/core/block/process"
)

// exportAnyBlockJSON writes the native bundle through wr and returns the
// number of documents it accounted for.
//
// The collection is NOT run again: exportObjects ran it before the writer
// existed, and closureForFormat gives this format the same ClosureDerived
// set anyblock.CollectRequest asks for, so a second pass would query the
// whole space twice for one export.
//
// How this can fail: if closureForFormat ever stops mapping this format to
// ClosureDerived, the bundle quietly loses every derived document — types,
// options, templates — instead of failing. The end-to-end test's directory
// assertions are what catch that, since types/ and options/ exist only
// under the derived closure.
func (e *exportContext) exportAnyBlockJSON(ctx context.Context, wr writer, queue process.Queue) (int, error) {
	exporter := &anyblock.Exporter{
		Picker:      e.picker,
		ObjectStore: e.objectStore,
		SbtProvider: e.sbtProvider,
	}
	res, err := exporter.ExportCollected(ctx, anyblock.Request{
		SpaceId:          e.spaceId,
		NetworkId:        e.nodeConf.Configuration().NetworkId,
		Ids:              e.reqIds,
		IncludeNested:    e.includeNested,
		IncludeFiles:     e.includeFiles,
		IncludeArchived:  e.includeArchive,
		IncludeBacklinks: e.includeBackLinks,
		IncludeSpace:     e.includeSpace,
		StateFilters:     e.linkStateFilters,
		// SpaceName stays empty on purpose: it is only index.json's fallback
		// for a space whose OWN document states no name (§2c), and that
		// document travels in the collected set, so the composer already
		// holds the better answer.
		Runner: queueRunner{queue: queue},
	}, e.docs, wr)
	e.report.Merge(res.Report)
	if err != nil {
		return res.Succeed, fmt.Errorf("export anyblock json bundle: %w", err)
	}
	return res.Succeed, nil
}

// queueRunner runs the native exporter's emit tasks on the export queue —
// the same process.Queue the legacy formats hand their per-document tasks
// to. Routing emit through it is what keeps this format's progress
// (Total/Done) and its answer to ProcessCancel identical to every other
// format's, with one process per export rather than two.
type queueRunner struct {
	queue process.Queue
}

// Run uses the shared export queue, including draining active workers on cancel.
func (r queueRunner) Run(_ context.Context, tasks []func()) error {
	queued := make([]process.Task, len(tasks))
	for i, task := range tasks {
		queued[i] = task
	}
	return waitExportTasks(r.queue, queued...)
}

// exportSingleAnyBlockDocument serves ExportSingleInMemory for the native
// format: ONE document, no bundle files. That is the design's position
// (Q7) resting on rule 7 of the format's principles — a document stands
// alone, carrying its own property names and formats — so index.json and
// properties.json have nothing to add about a single object, and an
// in-memory string could not carry two more files anyway.
func (e *exportContext) exportSingleAnyBlockDocument(ctx context.Context, objectId string) (string, error) {
	details, err := e.objectStore.SpaceIndex(e.spaceId).GetDetails(objectId)
	if err != nil {
		return "", fmt.Errorf("get object details: %w", err)
	}
	if err := refuseInMemoryFileObject(details); err != nil {
		return "", err
	}
	exporter := &anyblock.Exporter{
		Picker:      e.picker,
		ObjectStore: e.objectStore,
		SbtProvider: e.sbtProvider,
	}
	data, diagnostics, err := exporter.ExportDocument(ctx, e.spaceId, objectId)
	e.report.Merge(diagnostics)
	if err != nil {
		return "", fmt.Errorf("export anyblock json document: %w", err)
	}
	return string(data), nil
}
