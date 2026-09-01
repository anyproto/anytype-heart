package export

// anyblockjson.go routes model.Export_AnyBlockV2 to the native bundle
// exporter (core/block/export/anyblock). The format's own pipeline — plan,
// emit, compose, finish — lives there; what belongs HERE is only what the
// export service owns: the writer, the collected doc set, and the
// process.Queue every export reports progress and cancellation through.

import (
	"context"
	"errors"
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
	if err != nil {
		if errors.Is(err, process.ErrQueueCanceled) || errors.Is(err, context.Canceled) {
			// the cancel shape the legacy branch of exportByFormat uses:
			// nothing succeeded, the half-written output goes away, and the
			// RPC reports no error for the stop the user asked for
			cleanupFile(wr)
			return 0, nil
		}
		return 0, fmt.Errorf("export anyblock json bundle: %w", err)
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

// Run hands every task to the queue and blocks until they are all done, or
// until the queue is cancelled. The queue's own worker count bounds how
// many run at once (exportWorkers), and the tasks themselves watch ctx —
// so this does not, and takes ctx only to satisfy anyblock.EmitRunner.
func (r queueRunner) Run(_ context.Context, tasks []func()) error {
	queued := make([]process.Task, 0, len(tasks))
	for _, task := range tasks {
		queued = append(queued, task)
	}
	if err := r.queue.Wait(queued...); err != nil {
		return fmt.Errorf("run emit tasks on export queue: %w", err)
	}
	return nil
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
	data, err := exporter.ExportDocument(ctx, e.spaceId, objectId)
	if err != nil {
		return "", fmt.Errorf("export anyblock json document: %w", err)
	}
	return string(data), nil
}
