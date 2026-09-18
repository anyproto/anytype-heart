package v2service

import (
	"fmt"
	"maps"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
)

// compactReceiptIDs uses the same export as a default GET, after all ops
// have run. Pairing identity slots with the full document keeps AnyBlock in
// charge of label collisions, table internals, and view IDs. Only response
// maps change; the editing state and the applier's identity maps stay full.
func (a *v2StateApplier) compactReceiptIDs() (blocks, views map[string]string, err error) {
	full, err := a.doc()
	if err != nil {
		return nil, nil, err
	}
	opts := a.marshalOptions()
	opts.CompactBlockLabels = true
	// The full after-document already passed the loss checks. Keep the same
	// warning-enabled export shape without collecting its diagnostics again.
	opts.OnWarning = func(anyblockjson.Issue) {}
	body, err := anyblockjson.Marshal(a.sbType, snapshotFromState(a.st), opts)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal receipt labels: %w", err)
	}
	compact, err := parseEditDoc(body)
	if err != nil {
		return nil, nil, err
	}
	fullSlots := receiptIDSlots(full)
	compactSlots := receiptIDSlots(compact)
	blockLabels, viewLabels := map[string]string{}, map[string]string{}
	for path, slot := range fullSlots {
		label, ok := compactSlots[path]
		if !ok || label.view != slot.view {
			return nil, nil, fmt.Errorf("receipt identity slot %s missing from compact document", path)
		}
		if slot.view {
			viewLabels[blockId(slot.m)] = blockId(label.m)
		} else {
			blockLabels[blockId(slot.m)] = blockId(label.m)
		}
	}
	return relabelReceipt(a.createdBlocks, blockLabels), relabelReceipt(a.createdViews, viewLabels), nil
}

func receiptIDSlots(doc *v2EditDoc) map[string]payloadIdSlot {
	slots := map[string]payloadIdSlot{}
	for i, block := range doc.blocks {
		_ = walkPayloadIdSlots(block, fmt.Sprintf("blocks[%d]", i), func(slot payloadIdSlot) error {
			if blockId(slot.m) != "" {
				slots[slot.path] = slot
			}
			return nil
		})
	}
	return slots
}

func relabelReceipt(created, labels map[string]string) map[string]string {
	result := maps.Clone(created)
	for path, id := range result {
		if label, ok := labels[id]; ok {
			result[path] = label
		}
		// An ID removed by a later op, or omitted from export, has no final
		// label. Preserve its minted spelling in the historical receipt.
	}
	return result
}
