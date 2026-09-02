package recovery

import (
	"errors"
	"time"

	"github.com/anyproto/any-sync/net/peerobserver"
	"github.com/anyproto/any-sync/net/secureservice/handshake"
	"github.com/anyproto/any-sync/nodeconf"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/space/spacecore/localdiscovery"
)

// waitingForNetworkAfter is how long an outage must last before the calm
// WaitingForNetwork overlay is shown. Short enough to explain a stall, long
// enough that a redial after an idle-TTL close never shows it.
const waitingForNetworkAfter = 10 * time.Second

// deviceNetworkStateCName mirrors core/device.CName. Importing the package
// would drag spacecore into this one; a drift test keeps the two equal.
const deviceNetworkStateCName = "networkState"

// nodeTypesResolver is the part of nodeconf.Service peer classification uses.
// NodeTypes takes an RLock with no I/O under it, so it is safe on the dial
// path.
type nodeTypesResolver interface {
	NodeTypes(nodeId string) []nodeconf.NodeType
}

// observerRegistry is the part of net/peerobservermux the tracker needs.
type observerRegistry interface {
	Add(peerobserver.Observer)
}

// discoveryHookRegistrar is the part of localdiscovery the tracker needs.
type discoveryHookRegistrar interface {
	RegisterDiscoveryPossibilityHook(hook func(state localdiscovery.DiscoveryPossibility))
}

// networkConnectivity is the part of core/device.NetworkState the tracker
// needs.
type networkConnectivity interface {
	IsOffline() bool
	RegisterConnectivityHook(hook func(online bool))
}

// ObservePeerEvent implements peerobserver.Observer. Dial-path events run
// inside the pool's single-flight load for the peer they name, so this does
// nothing but fold state under the mutex and broadcast (non-blocking). It
// counts open connections per peer and never latches a boolean: a Closed for
// a superseded connection may arrive after the Connected for its replacement,
// and the pool's idle-TTL closes carry no reason.
func (t *Tracker) ObservePeerEvent(ev peerobserver.Event) {
	defer containTelemetry("peer event")
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.begun {
		return
	}
	switch ev.Kind {
	case peerobserver.KindDialStarted:
		t.dialStartedLocked(ev)
	case peerobserver.KindConnected:
		t.connectedLocked(ev)
	case peerobserver.KindDialFailed:
		t.dialFailedLocked(ev)
	case peerobserver.KindClosed:
		t.closedLocked(ev)
	default:
		// new kinds are not a breaking change
		return
	}
	t.refreshPhaseLocked(false)
}

// OnLocalPeerDiscovered is the LAN discovery seam (spacecore.PeerDiscovered).
// A peer is announced once; repeats are dropped.
func (t *Tracker) OnLocalPeerDiscovered(peerId string, addrs []string) {
	defer containTelemetry("peer discovered")
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.begun {
		return
	}
	p := t.peerLocked(peerId)
	if p.discoveredLocally {
		return
	}
	p.discoveredLocally = true
	t.markLocked(&pb.EventAccountRecoveryUpdatePayloadOfPeerDiscovered{PeerDiscovered: &pb.EventAccountRecoveryPeerDiscovered{
		PeerId:    peerId,
		Addrs:     append([]string(nil), addrs...),
		Kind:      p.kind,
		NodeTypes: p.nodeTypes,
	}}, nil)
}

// onDiscoveryPossibility runs under localdiscovery's own hook lock: fold and
// broadcast only.
func (t *Tracker) onDiscoveryPossibility(state localdiscovery.DiscoveryPossibility) {
	defer containTelemetry("discovery possibility")
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.begun {
		return
	}
	next := discoveryToPb(state)
	if next == t.net.discovery {
		return
	}
	t.net.discovery = next
	t.markLocked(&pb.EventAccountRecoveryUpdatePayloadOfLocalDiscoveryState{LocalDiscoveryState: &pb.EventAccountRecoveryLocalDiscoveryState{
		State: next,
	}}, nil)
	t.evaluateWaitingLocked()
	t.refreshPhaseLocked(false)
}

// onConnectivity is the device network state hook. Offline enters the
// WaitingForNetwork overlay at once; online only clears the flag — the overlay
// lifts on a real connection.
func (t *Tracker) onConnectivity(online bool) {
	defer containTelemetry("connectivity")
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.begun {
		return
	}
	t.net.deviceOffline = !online
	t.evaluateWaitingLocked()
	t.refreshPhaseLocked(false)
}

func (t *Tracker) peerLocked(id string) *peerState {
	if p, ok := t.net.peers[id]; ok {
		return p
	}
	p := &peerState{kind: pb.EventAccountRecovery_LocalPeer}
	if t.nodeTypes != nil {
		if types := t.nodeTypes.NodeTypes(id); len(types) > 0 {
			p.kind = pb.EventAccountRecovery_NetworkNode
			p.nodeTypes = make([]string, len(types))
			for i, nt := range types {
				p.nodeTypes[i] = string(nt)
			}
		}
	}
	t.net.peers[id] = p
	return p
}

func (t *Tracker) recountLocked() {
	total, nodes := 0, 0
	for _, p := range t.net.peers {
		total += p.open
		if p.kind == pb.EventAccountRecovery_NetworkNode {
			nodes += p.open
		}
	}
	t.net.openTotal, t.net.openNodes = total, nodes
}

func (t *Tracker) dialStartedLocked(ev peerobserver.Event) {
	p := t.peerLocked(ev.PeerId)
	t.net.dialsStarted = true
	var key *coalesceKey
	if p.dialAttempts > 0 {
		key = &coalesceKey{kind: "dialStarted", id: ev.PeerId}
	}
	t.markLocked(&pb.EventAccountRecoveryUpdatePayloadOfDialStarted{DialStarted: &pb.EventAccountRecoveryDialStarted{
		PeerId:     ev.PeerId,
		Kind:       p.kind,
		NodeTypes:  p.nodeTypes,
		AddrsCount: int32(ev.AddrCount),
	}}, key)
}

func (t *Tracker) connectedLocked(ev peerobserver.Event) {
	p := t.peerLocked(ev.PeerId)
	p.open++
	p.dialAttempts = 0
	p.lastError = nil
	p.lastTransport = ev.Scheme
	p.lastProto = ev.ProtoVersion
	t.recountLocked()
	// a real connection ends any outage, whatever the device reported
	t.net.outageSince = time.Time{}
	t.stopOutageTimerLocked()
	t.net.deviceOffline = false
	t.net.waitingForNetwork = false
	t.net.waitingError = nil
	direction, durationMs := pb.EventAccountRecovery_Outbound, ev.Dur.Milliseconds()
	if ev.Inbound {
		direction, durationMs = pb.EventAccountRecovery_Inbound, 0
	}
	t.markLocked(&pb.EventAccountRecoveryUpdatePayloadOfPeerConnected{PeerConnected: &pb.EventAccountRecoveryPeerConnected{
		PeerId:          ev.PeerId,
		Kind:            p.kind,
		NodeTypes:       p.nodeTypes,
		Direction:       direction,
		Addr:            ev.Addr,
		Transport:       ev.Scheme,
		ProtoVersion:    ev.ProtoVersion,
		DurationMs:      durationMs,
		OpenConnections: int32(p.open),
	}}, nil)
}

func (t *Tracker) dialFailedLocked(ev peerobserver.Event) {
	p := t.peerLocked(ev.PeerId)
	p.dialAttempts++
	p.lastError = classify(ev.Err)
	if errors.Is(ev.Err, handshake.ErrIncompatibleVersion) {
		// the first failure is observed; the pool suppresses re-dials after
		t.net.incompatibleLatched = true
	}
	// a cancelled dial (the caller gave up) says nothing about the network
	if p.lastError != nil && t.net.openTotal == 0 && t.net.outageSince.IsZero() {
		t.net.outageSince = t.clock.Now()
		t.armOutageTimerLocked()
	}
	var key *coalesceKey
	if p.dialAttempts > 1 {
		key = &coalesceKey{kind: "dialFailed", id: ev.PeerId}
	}
	t.markLocked(&pb.EventAccountRecoveryUpdatePayloadOfDialFailed{DialFailed: &pb.EventAccountRecoveryDialFailed{
		PeerId:     ev.PeerId,
		Kind:       p.kind,
		NodeTypes:  p.nodeTypes,
		Error:      p.lastError.toPb(),
		Attempt:    int32(p.dialAttempts),
		DurationMs: ev.Dur.Milliseconds(),
	}}, key)
	t.evaluateWaitingLocked()
}

func (t *Tracker) closedLocked(ev peerobserver.Event) {
	p := t.peerLocked(ev.PeerId)
	if p.open > 0 {
		p.open--
	}
	t.recountLocked()
	t.markLocked(&pb.EventAccountRecoveryUpdatePayloadOfPeerDisconnected{PeerDisconnected: &pb.EventAccountRecoveryPeerDisconnected{
		PeerId:          ev.PeerId,
		Kind:            p.kind,
		NodeTypes:       p.nodeTypes,
		OpenConnections: int32(p.open),
	}}, &coalesceKey{kind: "disconnected", id: ev.PeerId})
}

// evaluateWaitingLocked applies the WaitingForNetwork rule: zero open
// connections AND a dial failure observed while nothing was open AND that
// outage has lasted waitingForNetworkAfter — or the device says it is offline.
// An idle-TTL close alone satisfies none of it. Leaving happens only in
// connectedLocked.
func (t *Tracker) evaluateWaitingLocked() {
	if t.net.waitingForNetwork || t.terminalLocked() {
		return
	}
	outage := t.net.openTotal == 0 && !t.net.outageSince.IsZero() &&
		t.clock.Now().Sub(t.net.outageSince) >= waitingForNetworkAfter
	if !t.net.deviceOffline && !outage {
		return
	}
	t.net.waitingForNetwork = true
	t.net.waitingError = t.waitingErrorLocked()
}

func (t *Tracker) waitingErrorLocked() *errInfo {
	switch {
	case t.net.deviceOffline:
		return &errInfo{class: pb.EventAccountRecovery_NoNetwork, retryable: true, debug: "device reports offline"}
	case t.net.discovery == pb.EventAccountRecovery_NoInterfaces:
		return &errInfo{class: pb.EventAccountRecovery_NoNetwork, retryable: true, debug: "no network interfaces"}
	case t.net.incompatibleLatched:
		return &errInfo{class: pb.EventAccountRecovery_IncompatibleVersion, debug: handshake.ErrIncompatibleVersion.Error()}
	default:
		return &errInfo{class: pb.EventAccountRecovery_PeerUnreachable, retryable: true, debug: "no peer reachable"}
	}
}

func (t *Tracker) armOutageTimerLocked() {
	if t.outageTimer != nil {
		return
	}
	t.outageTimer = t.clock.AfterFunc(waitingForNetworkAfter, t.onOutageTimer)
}

func (t *Tracker) stopOutageTimerLocked() {
	if t.outageTimer != nil {
		t.outageTimer.Stop()
		t.outageTimer = nil
	}
}

func (t *Tracker) onOutageTimer() {
	defer containTelemetry("outage timer")
	t.mu.Lock()
	defer t.mu.Unlock()
	t.outageTimer = nil
	if !t.begun {
		return
	}
	t.evaluateWaitingLocked()
	t.refreshPhaseLocked(false)
}

func discoveryToPb(state localdiscovery.DiscoveryPossibility) pb.EventAccountRecoveryDiscoveryState {
	switch state {
	case localdiscovery.DiscoveryNoInterfaces:
		return pb.EventAccountRecovery_NoInterfaces
	case localdiscovery.DiscoveryLocalNetworkRestricted:
		return pb.EventAccountRecovery_Restricted
	default:
		return pb.EventAccountRecovery_Possible
	}
}
