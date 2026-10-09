package peerstore

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAllLocalPeersNoAlias pins that AllLocalPeers hands back a copy. It
// returned the live slice, so the one production caller — the re-exchange
// round, which clones after the lock is already released — raced every
// concurrent local peer update. Run with -race.
func TestAllLocalPeersNoAlias(t *testing.T) {
	p := New().(*peerStore)
	p.UpdateLocalPeer("a", []string{"s1"})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			_ = p.AllLocalPeers()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			p.UpdateLocalPeer("b", []string{"s2"})
			p.RemoveLocalPeer("b")
		}
	}()
	wg.Wait()

	// mutating the result must not reach the store
	got := p.AllLocalPeers()
	if len(got) > 0 {
		got[0] = "mutated"
	}
	for _, id := range p.AllLocalPeers() {
		if id == "mutated" {
			t.Fatal("AllLocalPeers returned the live slice")
		}
	}
}

// TestLocalPeerIdsNoAlias pins that LocalPeerIds hands back a copy: removing
// a peer from a space shifts the remaining ids within the stored slice, under
// the feet of a caller (sync, files, pubsub, p2p status) iterating the result.
// Run with -race.
func TestLocalPeerIdsNoAlias(t *testing.T) {
	// given
	p := New().(*peerStore)
	p.UpdateLocalPeer("a", []string{"s1"})
	p.UpdateLocalPeer("b", []string{"s1"})
	p.UpdateLocalPeer("c", []string{"s1"})

	// when
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			for _, id := range p.LocalPeerIds("s1") {
				_ = id
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			p.RemoveLocalPeer("a")
			p.UpdateLocalPeer("a", []string{"s1"})
		}
	}()
	wg.Wait()

	// then: mutating the result must not reach the store
	got := p.LocalPeerIds("s1")
	require.NotEmpty(t, got)
	got[0] = "mutated"
	assert.NotContains(t, p.LocalPeerIds("s1"), "mutated")
}
