package status

import (
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/dgtony/collections/hashset"
	"github.com/dgtony/collections/queue"
	ct "github.com/dgtony/collections/time"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/textileio/go-threads/core/net"
	"github.com/textileio/go-threads/core/thread"
)

const (
	threadStatusUpdatePeriod     = 5 * time.Second
	threadStatusEventBatchPeriod = 2 * time.Second

	// truncate device names and account IDs
	// to specified number of last symbols
	maxNameLength = 8
)

type LogTime struct {
	AccountID string
	DeviceID  string
	LastEdit  int64
}

type Service interface {
	Watch(tid thread.ID)
	Unwatch(tid thread.ID)
	UpdateTimeline(tid thread.ID, tl []LogTime)

	Start() error
	Stop()
}

var _ Service = (*service)(nil)

type service struct {
	cafeID string
	tInfo  net.SyncInfo
	fInfo  core.FileInfo

	watchers map[thread.ID]func()
	threads  map[thread.ID]*threadStatus

	// deviceID => { thread.ID }
	devThreads map[string]hashset.HashSet
	// deviceID => accountID
	devAccount map[string]string
	// peerID => connected
	connMap map[string]bool

	tsTrigger *queue.BulkQueue
	emitter   func(event *pb.Event)
	mu        sync.Mutex
}

func NewService(
	ts net.SyncInfo,
	fs core.FileInfo,
	emitter func(event *pb.Event),
	cafe peer.ID,
) *service {
	return &service{
		cafeID:     cafe.String(),
		tInfo:      ts,
		fInfo:      fs,
		emitter:    emitter,
		watchers:   make(map[thread.ID]func()),
		threads:    make(map[thread.ID]*threadStatus),
		devThreads: make(map[string]hashset.HashSet),
		devAccount: make(map[string]string),
		connMap:    make(map[string]bool),
		tsTrigger:  queue.NewBulkQueue(threadStatusEventBatchPeriod, 5),
	}
}

func (s *service) Watch(tid thread.ID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exist := s.watchers[tid]; exist {
		return
	}

	var (
		stop   = make(chan struct{})
		ticker = ct.NewRightAwayTicker(threadStatusUpdatePeriod)
		closer = func() { close(stop); ticker.Stop() }
	)

	s.watchers[tid] = closer

	go func() {
		for {
			select {
			case <-ticker.C:
			case <-stop:
				return
			}

			view, _ := s.tInfo.View(tid)

			s.mu.Lock()
			ts := s.getThreadStatus(tid)
			s.mu.Unlock()

			ts.Lock()
			for pid, status := range view {
				ts.UpdateStatus(pid.String(), status)
			}

			if ts.modified {
				s.tsTrigger.Push(tid)
			}
			ts.Unlock()
		}
	}()
}

func (s *service) Unwatch(tid thread.ID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if stop, found := s.watchers[tid]; found {
		delete(s.watchers, tid)
		stop()
	}
}

func (s *service) UpdateTimeline(tid thread.ID, timeline []LogTime) {
	s.mu.Lock()
	for _, logTime := range timeline {
		// update account information for devices
		s.devAccount[logTime.DeviceID] = logTime.AccountID

		// update device threads
		dt, exist := s.devThreads[logTime.DeviceID]
		if !exist {
			dt = hashset.New()
		}
		dt.Add(tid)
		s.devThreads[logTime.DeviceID] = dt
	}
	ts := s.getThreadStatus(tid)
	s.mu.Unlock()

	ts.Lock()
	defer ts.Unlock()

	for _, logTime := range timeline {
		ts.UpdateTimeline(logTime.DeviceID, logTime.LastEdit)
	}

	if ts.modified {
		s.tsTrigger.Push(tid)
	}
}

//func (s *service) ThreadSummary() net.SyncSummary {
//	ps, _ := s.tInfo.PeerSummary(s.cafe)
//	return ps
//}
//
//func (s *service) FileSummary() core.FilePinSummary {
//	return s.fInfo.FileSummary()
//}

func (s *service) Start() error {
	if err := s.startConnectivityTracking(); err != nil {
		return err
	}
	go s.startSendingThreadStatus()
	return nil
}

func (s *service) Stop() {
	s.tsTrigger.Stop()

	s.mu.Lock()
	defer s.mu.Unlock()

	// just shutdown all thread status watchers, connectivity tracking
	// will be stopped automatically on closing the network layer
	for tid, stop := range s.watchers {
		delete(s.watchers, tid)
		stop()
	}
}

func (s *service) startConnectivityTracking() error {
	connEvents, err := s.tInfo.Connectivity()
	if err != nil {
		return err
	}

	go func() {
		for event := range connEvents {
			var (
				devID = event.Peer.String()
				ts    = make(map[thread.ID]*threadStatus)
			)

			s.mu.Lock()
			// update peer connectivity
			s.connMap[devID] = event.Connected

			// find threads shared with peer
			if tids, exist := s.devThreads[devID]; exist {
				for _, i := range tids.List() {
					var tid = i.(thread.ID)
					ts[tid] = s.getThreadStatus(tid)
				}
			}
			s.mu.Unlock()

			for tid, t := range ts {
				t.Lock()
				t.UpdateConnectivity(devID, event.Connected)
				if t.modified {
					s.tsTrigger.Push(tid)
				}
				t.Unlock()
			}
		}
	}()

	return nil
}

func (s *service) startSendingThreadStatus() {
	for is := range s.tsTrigger.RunBulk() {
		var ts = make(map[thread.ID]*threadStatus, len(is))

		s.mu.Lock()
		for i := 0; i < len(is); i++ {
			id := is[i].(thread.ID)
			ts[id] = s.getThreadStatus(id)
		}
		s.mu.Unlock()

		for id, t := range ts {
			t.Lock()
			event := s.constructEvent(t)
			t.Unlock()

			s.sendEvent(
				id.String(),
				&pb.EventMessageValueOfThreadStatus{ThreadStatus: &event},
			)
		}
	}
}

// Unsafe, use under the global lock!
func (s *service) getThreadStatus(tid thread.ID) *threadStatus {
	ts, exist := s.threads[tid]
	if !exist {
		ts = newThreadStatus(func(devID string) bool {
			s.mu.Lock()
			defer s.mu.Unlock()
			return s.connMap[devID]
		})
		s.threads[tid] = ts
	}
	return ts
}

func (s *service) constructEvent(ts *threadStatus) (event pb.EventStatusThread) {
	type devInfo struct {
		id string
		ds deviceStatus
	}

	var (
		accounts = make(map[string][]devInfo)
		cafe     deviceStatus
		dss      []deviceStatus

		max = func(x, y int64) int64 {
			if x > y {
				return x
			}
			return y
		}

		shorten = func(name string) string {
			if len(name) <= maxNameLength {
				return name
			}
			return name[len(name)-maxNameLength:]
		}
	)

	ts.Lock()
	s.mu.Lock()

	// construct account tree
	for devID, status := range ts.devices {
		if devID == s.cafeID {
			cafe = *status
			continue
		}
		var accID = s.devAccount[devID]
		accounts[accID] = append(accounts[accID], devInfo{devID, *status})
	}

	// clear modification status
	ts.modified = false

	s.mu.Unlock()
	ts.Unlock()

	// accounts
	for accID, devices := range accounts {
		var accountInfo = pb.EventStatusThreadAccount{Id: shorten(accID)}
		for _, device := range devices {
			accountInfo.Devices = append(accountInfo.Devices, &pb.EventStatusThreadDevice{
				Name:       shorten(device.id),
				Online:     device.ds.online,
				LastPulled: device.ds.status.LastPull,
				LastEdited: device.ds.lastEdited,
			})

			// account considered online if any device is online
			accountInfo.Online = accountInfo.Online || device.ds.online
			// the very last edit among all devices
			accountInfo.LastEdited = max(accountInfo.LastEdited, device.ds.lastEdited)
			// the very last pull among all devices
			accountInfo.LastPulled = max(accountInfo.LastPulled, device.ds.status.LastPull)
			// collect individual device statuses for summary
			dss = append(dss, device.ds)
		}
		event.Accounts = append(event.Accounts, &accountInfo)
	}

	// cafe
	event.Cafe.LastPulled = cafe.status.LastPull
	event.Cafe.LastPushSucceed = cafe.status.Up == net.Success
	if !cafe.online {
		event.Cafe.Status = pb.EventStatusThread_Offline
	} else {
		switch cafe.status.Down {
		case net.Unknown:
			event.Cafe.Status = pb.EventStatusThread_Unknown
		case net.InProgress:
			event.Cafe.Status = pb.EventStatusThread_Syncing
		case net.Success:
			event.Cafe.Status = pb.EventStatusThread_Synced
		case net.Failure:
			event.Cafe.Status = pb.EventStatusThread_Failed
		}
	}

	// decide sync status summary
	event.Summary.Status = summary(event.Cafe.Status, dss...)

	return
}

func (s *service) sendEvent(ctx string, event pb.IsEventMessageValue) {
	s.emitter(&pb.Event{
		Messages:  []*pb.EventMessage{{Value: event}},
		ContextId: ctx,
	})
}

// Infer sync status summary from individual devices and cafe
func summary(cafe pb.EventStatusThreadSyncStatus, devices ...deviceStatus) pb.EventStatusThreadSyncStatus {
	var unknown, offline, inProgress, synced, failed int
	for _, device := range devices {
		switch device.status.Down {
		case net.Unknown:
			unknown += 1
		case net.InProgress:
			inProgress += 1
		case net.Success:
			synced += 1
		case net.Failure:
			failed += 1
		}
		if !device.online {
			offline += 1
		}
	}

	switch {
	case synced > 0 || cafe == pb.EventStatusThread_Synced:
		// if thread was synced with cafe or at least one device,
		// it could be considered as a successfully synchronised
		return pb.EventStatusThread_Synced
	case inProgress > 0 || cafe == pb.EventStatusThread_Syncing:
		// sync with some devices or cafe is in progress
		return pb.EventStatusThread_Syncing
	case len(devices) == offline && cafe == pb.EventStatusThread_Offline:
		// no connection with cafe or devices
		return pb.EventStatusThread_Offline
	case unknown > 0 && cafe == pb.EventStatusThread_Unknown:
		// no status information at all
		return pb.EventStatusThread_Unknown
	default:
		return pb.EventStatusThread_Failed
	}
}

type (
	deviceStatus struct {
		status     net.SyncStatus
		lastEdited int64
		online     bool
	}

	threadStatus struct {
		devices  map[string]*deviceStatus
		devConn  func(devID string) bool
		modified bool
		sync.Mutex
	}
)

func newThreadStatus(conn func(devID string) bool) *threadStatus {
	return &threadStatus{
		devices: make(map[string]*deviceStatus),
		devConn: conn,
	}
}

func (s *threadStatus) UpdateStatus(devID string, ss net.SyncStatus) {
	var dev = s.getDevice(devID)
	if dev.status != ss {
		dev.status = ss
		s.modified = true
	}
}

func (s *threadStatus) UpdateTimeline(devID string, lastEdit int64) {
	var dev = s.getDevice(devID)
	if dev.lastEdited < lastEdit {
		dev.lastEdited = lastEdit
		s.modified = true
	}
}

func (s *threadStatus) UpdateConnectivity(devID string, online bool) {
	var dev = s.getDevice(devID)
	if dev.online != online {
		dev.online = online
		s.modified = true
	}
}

func (s *threadStatus) getDevice(id string) *deviceStatus {
	dev, found := s.devices[id]
	if !found {
		dev = &deviceStatus{online: s.devConn(id)}
		s.devices[id] = dev
		s.modified = true
	}

	return dev
}
