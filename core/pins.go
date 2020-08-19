package core

import (
	"context"
	"time"

	cafepb "github.com/anytypeio/go-anytype-library/cafe/pb"
	"github.com/anytypeio/go-anytype-library/util"
	"github.com/dgtony/collections/hashset"
)

func (a *Anytype) checkPins() {
	const (
		checkPeriod        = 10 * time.Minute
		cafeRequestTimeout = 30 * time.Second
	)

	if a.cafe == nil {
		return
	}

	t := util.NewImmediateTicker(checkPeriod)
	defer t.Stop()

	for {
		var (
			failedCIDs     []string
			queued, pinned int
			onlyLocal      = hashset.New()
		)

		select {
		case <-a.shutdownStartsCh:
			return
		case <-t.C:
			break
		}

		log.Debugf("checking pinned files statuses...")

		resp, err := a.cafe.GetFilePins(a.deriveContext(cafeRequestTimeout), &cafepb.GetFilePinsRequest{})
		if err != nil {
			log.Warnf("retrieving pinned files failed: %v", err)
			continue
		}

		if cids, err := a.localStore.Files.ListHashes(); err != nil {
			log.Warnf("retrieving local files failed: %v", err)
			continue
		} else {
			for _, cid := range cids {
				onlyLocal.Add(cid)
			}
		}

		for _, pin := range resp.GetPins() {
			switch pin.GetStatus() {
			case cafepb.PinStatus_Queued:
				queued++
			case cafepb.PinStatus_Done:
				pinned++
			case cafepb.PinStatus_Failed:
				failedCIDs = append(failedCIDs, pin.GetCid())
			}

			onlyLocal.Remove(pin.GetCid())
		}

		log.Debugf("cafe status: queued for pinning: %d, pinned: %d, failed: %d, local: %d",
			queued, pinned, len(failedCIDs), onlyLocal.Len())

		// add local files for the sync
		for _, cid := range onlyLocal.List() {
			failedCIDs = append(failedCIDs, cid.(string))
		}

		if len(failedCIDs) > 0 {
			log.Infof("retrying to pin %d files", len(failedCIDs))

			var reqCtx = a.deriveContext(cafeRequestTimeout)
			for _, failedCID := range failedCIDs {
				go func(c string) {
					if _, err := a.cafe.FilePin(reqCtx, &cafepb.FilePinRequest{Cid: c}); err != nil {
						log.Warnf("re-pinning file %s failed: %v", c, err)
					}
				}(failedCID)
			}
		}
	}
}

// get context with timeout that will be cancelled on service stop
func (a *Anytype) deriveContext(timeout time.Duration) context.Context {
	var ctx, cancel = context.WithTimeout(context.Background(), timeout)

	go func() {
		select {
		case <-time.After(timeout):
		case <-a.shutdownStartsCh:
			cancel()
		}
	}()

	return ctx
}
