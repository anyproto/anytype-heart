package application

import (
	"crypto/subtle"
	"sync/atomic"
)

// localAPISecret is the per-launch shared secret the parent process delivers
// over the server's stdin pipe (see cmd/grpcserver/lifeline.go). Only a caller
// that holds the write end of that pipe knows it, so requiring it on the
// account-bootstrap RPCs keeps a caller who merely reached the gRPC port from
// minting itself a Full-scope session.
//
// It is registered once at startup, before the gRPC server serves, and read on
// every gated request — hence an atomic rather than s.lock, which is held for
// the whole of AccountSelect/AccountStop and would stall the interceptor.
// Mobile never registers one, which leaves enforcement off.

type localAPISecret struct {
	value atomic.Pointer[string]
}

// SetLocalAPISecret registers the parent-delivered secret and turns enforcement
// on. An empty secret is ignored, leaving the server permissive.
//
// It is write-once: the first non-empty value wins and every later call is a
// no-op. The secret is a per-launch startup fact delivered over exactly one
// channel (the parent's stdin pipe), so nothing legitimate ever replaces it,
// and a write-once store means no later code path — or bug — can swap in a
// value an attacker chose and then walk through the gate with it.
func (s *Service) SetLocalAPISecret(secret string) {
	if secret == "" {
		return
	}
	if !s.localAPISecret.value.CompareAndSwap(nil, &secret) {
		log.Warn("local api secret is already registered, ignoring the new one")
	}
}

// LocalAPISecretEnforced reports whether a secret was registered, i.e. whether
// the gated bootstrap methods require one.
func (s *Service) LocalAPISecretEnforced() bool {
	return s.localAPISecret.value.Load() != nil
}

// CheckLocalAPISecret compares the caller-provided secret with the registered
// one in constant time. It fails when nothing is registered, so a caller can
// never pass the check by omitting the value.
func (s *Service) CheckLocalAPISecret(provided string) bool {
	expected := s.localAPISecret.value.Load()
	if expected == nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(*expected), []byte(provided)) == 1
}
