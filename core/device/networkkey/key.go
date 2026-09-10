// Package networkkey holds the network identity type on its own, so the
// generated device mocks can name it without importing core/device back.
package networkkey

// Key identifies a network by the parts that were actually observed. It is
// deliberately not a single string: the client's report (type and path id)
// and the monitor's interface snapshot land at different times - the snapshot
// within milliseconds of Run, the report only once the account app is up,
// since DeviceNetworkStateSet is rejected before login. A concatenated key
// therefore differs from itself on one network depending on when it was read,
// which is why comparison goes through SameNetwork.
//
// The client's path id is only as stable as the client makes it (Android
// sends the network handle, which changes on every reconnect - see
// docs/mobile-network-integration.md), and the snapshot is the whole address
// set, so a VPN or a DHCP lease moves it. A genuinely stable per-network key
// from the client is what would make persistence reliable on mobile.
type Key struct {
	// Reported: the client has told us Type and PathId. Without it Type is
	// indistinguishable from WIFI, the enum's zero value.
	Reported bool   `json:"reported,omitempty"`
	Type     int32  `json:"type,omitempty"`
	PathId   string `json:"pathId,omitempty"`
	Snapshot string `json:"snapshot,omitempty"`
}

// Known reports whether anything DISCRIMINATING identifies the network. A
// bare type does not: "some Wi-Fi" matches every Wi-Fi, so a verdict filed
// under it would apply on every Wi-Fi the device ever joins.
func (k Key) Known() bool {
	return k.PathId != "" || k.Snapshot != ""
}

// SameNetwork reports whether two observations can be of the same network.
// Every part known on BOTH sides must agree; a part missing on one side is
// absence of evidence, not evidence of a different network. Comparing whole
// keys instead read a half-formed observation as a move and threw away state
// that belonged to the network the device was still on.
//
// The direction of the remaining doubt is deliberate: when two observations
// share no known part, this answers true and the caller keeps what it has.
// Using the slower transport to a healthy peer for a while is a smaller harm
// than re-learning the verdict on every launch, which is the failure this
// exists to prevent.
func (k Key) SameNetwork(other Key) bool {
	if k.Reported && other.Reported && k.Type != other.Type {
		return false
	}
	if k.PathId != "" && other.PathId != "" && k.PathId != other.PathId {
		return false
	}
	if k.Snapshot != "" && other.Snapshot != "" && k.Snapshot != other.Snapshot {
		return false
	}
	return true
}
