package chatobject

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/source"
)

// readCoreShadowEnabled gates the causal-ordinal (Option D) CORE shadow: when
// on, after read-state updates the CORE unread count — computed from the
// frontier + band, with no per-message read flags — is compared against the
// bool path and divergences are logged. Default off; the bool path remains the
// source of truth until cutover (spec §7 Stage 3).
var readCoreShadowEnabled = false

// computeReadCoreCount computes the CORE unread count for one counter:
// tail (counted live messages past the frontier cut) + band (counted live
// messages at/below the cut not causally covered by the frontier), per spec
// Theorem 1. Returns ok=false when the source does not expose the snapshot
// (e.g. test mocks) or the counter's diff manager is not initialized.
//
// The DAG walk runs inside the source's snapshot callback — under the object
// tree lock — and only the resulting id sets leave it.
func (s *storeObject) computeReadCoreCount(ctx context.Context, counterType chatmodel.CounterType) (coreCount, boolCount int, ok bool, err error) {
	provider, isProvider := s.storeSource.(source.ReadCoreSnapshotProvider)
	if !isProvider {
		return 0, 0, false, nil
	}
	var band chatmodel.BandResult
	if !provider.ReadCoreSnapshot(counterType.DiffManagerName(), func(frontier, localHeads []string, resolve func(id string) ([]string, string, bool)) {
		band = chatmodel.ComputeBand(frontier, localHeads, func(id string) (chatmodel.ChangeMeta, bool) {
			prevIds, orderId, rok := resolve(id)
			return chatmodel.ChangeMeta{PrevIds: prevIds, OrderId: orderId}, rok
		})
	}) {
		return 0, 0, false, nil
	}
	coreCount, err = s.repository.CountCoreUnread(ctx, counterType, band.MaxFrontierOrderId, band.Candidates, s.accountService.AccountID())
	if err != nil {
		return 0, 0, false, fmt.Errorf("count core unread: %w", err)
	}
	boolIds, err := s.repository.GetAllUnreadMessages(ctx, counterType)
	if err != nil {
		return 0, 0, false, fmt.Errorf("get bool unread: %w", err)
	}
	return coreCount, len(boolIds), true, nil
}

// shadowReadCoreCount runs the CORE computation alongside the bool path and
// logs a divergence. Non-fatal by design: shadow observability only.
func (s *storeObject) shadowReadCoreCount(ctx context.Context, counterType chatmodel.CounterType) {
	if !readCoreShadowEnabled {
		return
	}
	core, boolN, ok, err := s.computeReadCoreCount(ctx, counterType)
	if err != nil {
		log.Error("read-core shadow", zap.Error(err))
		return
	}
	if !ok {
		return
	}
	if core != boolN {
		log.Warn("read-core shadow diverged from bool path",
			zap.String("counter", counterType.DiffManagerName()),
			zap.Int("core", core),
			zap.Int("bool", boolN))
	}
}
