package pin

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/cafe"
	cafepb "github.com/anytypeio/go-anytype-middleware/pkg/lib/cafe/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/dgtony/collections/hashset"
)

const (
	pinCheckPeriodActive = 15 * time.Second
	pinCheckPeriodIdle   = 10 * time.Minute
	cafeRequestTimeout   = 30 * time.Second
)

var ErrNoCafe = errors.New("no cafe client")

var log = logging.Logger("anytype-file-pinning")

type FilePinInfo struct {
	Status  cafepb.PinStatus
	Updated int64
}

type FilePinService interface {
	// on empty request must return status for all files
	PinStatus(cids ...string) map[string]FilePinInfo
	FilePin(cid string) error

	Start()
}

var _ FilePinService = (*filePinService)(nil)

type filePinService struct {
	ctx   context.Context
	cafe  cafe.Client
	store localstore.FileStore

	files    map[string]FilePinInfo
	activate chan struct{}
	mu       sync.RWMutex
}

func NewFilePinService(
	ctx context.Context,
	cafe cafe.Client,
	store localstore.FileStore,
) *filePinService {
	return &filePinService{
		ctx:      ctx,
		cafe:     cafe,
		store:    store,
		activate: make(chan struct{}),
		files:    make(map[string]FilePinInfo),
	}
}

func (f *filePinService) PinStatus(cids ...string) map[string]FilePinInfo {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.findCids(cids)
}

func (f *filePinService) FilePin(cid string) error {
	if f.cafe == nil {
		return ErrNoCafe
	}

	f.mu.RLock()
	status := f.files[cid]
	f.mu.RUnlock()

	if status.Status == cafepb.PinStatus_Done {
		return nil
	}

	_, err := f.cafe.FilePin(context.Background(), &cafepb.FilePinRequest{Cid: cid})

	f.mu.Lock()
	if err != nil {
		f.set(cid, cafepb.PinStatus_Failed)
	} else {
		f.set(cid, cafepb.PinStatus_Queued)
	}
	f.mu.Unlock()

	// interrupt idle sync phase
	select {
	case f.activate <- struct{}{}:
	default:
	}

	return err
}

func (f *filePinService) Start() {
	if f.cafe != nil {
		go f.syncCafe()
	}
}

func (f *filePinService) findCids(cids []string) map[string]FilePinInfo {
	var result = make(map[string]FilePinInfo, len(cids))
	for _, c := range cids {
		if status, found := f.files[c]; found {
			result[c] = status
		}
	}
	return result
}

func (f *filePinService) set(cid string, status cafepb.PinStatus) {
	f.files[cid] = FilePinInfo{
		Status:  status,
		Updated: time.Now().Unix(),
	}
}

// Periodically synchronize pin-statuses with cafe
func (f *filePinService) syncCafe() {
	var active = true

	for {
		var (
			queued, pinned, failed []string
			onlyLocal              = hashset.New()
			period                 time.Duration
		)

		if active {
			period = pinCheckPeriodActive
		} else {
			period = pinCheckPeriodIdle
		}

		t := time.NewTimer(period)

		select {
		case <-f.activate: // new file pinned
			t.Stop()
		case <-f.ctx.Done():
			return
		case <-t.C: // ready for periodic check
		}

		log.Debugf("checking pinned files statuses...")

		ctx, _ := context.WithTimeout(f.ctx, cafeRequestTimeout)
		resp, err := f.cafe.GetFilePins(ctx, &cafepb.GetFilePinsRequest{})
		if err != nil {
			log.Warnf("retrieving pinned files failed: %v", err)
			continue
		}

		if cids, err := f.store.ListTargets(); err != nil {
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
				queued = append(queued, cid)
			case cafepb.PinStatus_Done:
				pinned = append(pinned, cid)
			case cafepb.PinStatus_Failed:
				failed = append(failed, cid)
			}

			onlyLocal.Remove(cid)
		}

		var local = make([]string, onlyLocal.Len())
		for i, c := range onlyLocal.List() {
			local[i] = c.(string)
		}

		f.mu.Lock()
		// update statuses
		for _, s := range []struct {
			cids   []string
			status cafepb.PinStatus
		}{
			{queued, cafepb.PinStatus_Queued},
			{pinned, cafepb.PinStatus_Done},
			{failed, cafepb.PinStatus_Failed},
			// local files will be requested for pin right now
			{local, cafepb.PinStatus_Queued},
		} {
			for _, cid := range s.cids {
				f.set(cid, s.status)
			}
		}
		f.mu.Unlock()

		log.Debugf("file pinning status :: in progress: %d, pinned: %d, failed: %d, local: %d",
			len(queued), len(pinned), len(failed), len(local))

		// pinning is active until there are queued, retried or local files
		active = len(queued)+len(failed)+len(local) > 0

		if retried := len(failed) + len(local); retried > 0 {
			log.Infof("trying to pin %d files", retried)

			var reqCtx, _ = context.WithTimeout(f.ctx, cafeRequestTimeout)

			for _, cid := range failed {
				go func(c string) {
					if _, err := f.cafe.FilePin(reqCtx, &cafepb.FilePinRequest{Cid: c}); err != nil {
						log.Warnf("re-pinning file %s failed: %v", c, err)
					}
				}(cid)
			}

			for _, cid := range local {
				go func(c string) {
					if _, err := f.cafe.FilePin(reqCtx, &cafepb.FilePinRequest{Cid: c}); err != nil {
						log.Warnf("pinning local file %s failed: %v", c, err)
					}
				}(cid)
			}
		}
	}
}
