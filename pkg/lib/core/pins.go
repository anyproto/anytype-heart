package core

import (
	"context"
	"sync"
	"time"

	cafepb "github.com/anytypeio/go-anytype-middleware/pkg/lib/cafe/pb"
	"github.com/dgtony/collections/hashset"
	ct "github.com/dgtony/collections/time"
)

func (a *Anytype) checkPins() {
	const (
		checkPeriod        = 10 * time.Minute
		cafeRequestTimeout = 30 * time.Second
	)

	if a.cafe == nil {
		return
	}

	t := ct.NewRightAwayTicker(checkPeriod)
	defer t.Stop()

	for {
		var (
			failedCIDs     []string
			queued, pinned int
			onlyLocal      = hashset.New()
		)

		select {
		case <-t.C:
		case <-a.shutdownStartsCh:
			return
		}

		log.Debugf("checking pinned files statuses...")

		resp, err := a.cafe.GetFilePins(a.deriveContext(cafeRequestTimeout), &cafepb.GetFilePinsRequest{})
		if err != nil {
			log.Warnf("retrieving pinned files failed: %v", err)
			continue
		}

		if cids, err := a.localStore.Files.ListTargets(); err != nil {
			log.Warnf("retrieving local files failed: %v", err)
			continue
		} else {
			for _, cid := range cids {
				onlyLocal.Add(cid)
			}
		}

		for _, pin := range resp.GetPins() {
			var (
				cid    = pin.GetCid()
				status = pin.GetStatus()
			)

			switch status {
			case cafepb.PinStatus_Queued:
				queued++
			case cafepb.PinStatus_Done:
				pinned++
			case cafepb.PinStatus_Failed:
				failedCIDs = append(failedCIDs, pin.GetCid())
			}

			onlyLocal.Remove(cid)
			a.pinRegistry.Update(cid, status)
		}

		log.Debugf("cafe status: queued for pinning: %d, pinned: %d, failed: %d, local: %d",
			queued, pinned, len(failedCIDs), onlyLocal.Len())

		for _, i := range onlyLocal.List() {
			var cid = i.(string)
			// add local files for the sync
			failedCIDs = append(failedCIDs, cid)
			// local files will be requested for pin right now
			a.pinRegistry.Update(cid, cafepb.PinStatus_Queued)
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

/* File status service */

type (
	FilePinStatus struct {
		Status  cafepb.PinStatus
		Updated int64
	}

	FilePinSummary struct {
		Pinned, InProgress, Failed int
		LastUpdated                int64
	}

	FileInfo interface {
		PinStatus(cids ...string) map[string]FilePinStatus
		FileSummary() FilePinSummary
	}
)

var _ FileInfo = (*filePinRegistry)(nil)

type filePinRegistry struct {
	files map[string]FilePinStatus
	mu    sync.RWMutex
}

func newFilePinRegistry() *filePinRegistry {
	return &filePinRegistry{files: make(map[string]FilePinStatus)}
}

func (f *filePinRegistry) PinStatus(cids ...string) map[string]FilePinStatus {
	if len(cids) == 0 {
		return nil
	}

	f.mu.RLock()
	defer f.mu.RUnlock()

	var result = make(map[string]FilePinStatus, len(cids))
	for _, c := range cids {
		if status, found := f.files[c]; found {
			result[c] = status
		}
	}

	return result
}

func (f *filePinRegistry) FileSummary() FilePinSummary {
	f.mu.RLock()
	defer f.mu.RUnlock()

	var summary FilePinSummary
	for _, status := range f.files {
		switch status.Status {
		case cafepb.PinStatus_Queued:
			summary.InProgress++
		case cafepb.PinStatus_Failed:
			summary.Failed++
		case cafepb.PinStatus_Done:
			summary.Pinned++
		}

		if status.Updated > summary.LastUpdated {
			summary.LastUpdated = status.Updated
		}
	}

	return summary
}

func (f *filePinRegistry) Update(cid string, status cafepb.PinStatus) {
	f.mu.Lock()
	f.files[cid] = FilePinStatus{
		Status:  status,
		Updated: time.Now().Unix(),
	}
	f.mu.Unlock()
}
