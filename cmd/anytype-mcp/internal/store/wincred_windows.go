//go:build windows

package store

import (
	"context"
	"errors"
	"time"

	"github.com/danieljoos/wincred"
)

// windowsStore keeps one generic credential per connection in the
// Credential Manager: target "<service>/<connection>", local-machine
// persistence (it does not roam), protected at rest by DPAPI. Windows has
// no per-application access list.
type windowsStore struct{ service string }

func openPlatform(namespace string) Store { return newPlatformStore(ServiceName(namespace)) }

// newPlatformStore takes the service name itself; tests use their own.
func newPlatformStore(service string) Store { return &windowsStore{service: service} }

func (w *windowsStore) Kind() string { return "wincred" }

func (w *windowsStore) target(id string) string { return w.service + "/" + id }

func (w *windowsStore) Probe(context.Context) error { return nil }

func (w *windowsStore) get(id string) ([]byte, error) {
	cred, err := wincred.GetGenericCredential(w.target(id))
	if err != nil {
		if errors.Is(err, wincred.ErrElementNotFound) {
			return nil, ErrNotFound
		}
		return nil, Temporary(ReasonUnreachable, err)
	}
	return cred.CredentialBlob, nil
}

// Read retries one failed read after 500 ms: the Credential Manager can
// refuse right after the session resumes from sleep.
func (w *windowsStore) Read(ctx context.Context, id string, _ bool) ([]byte, error) {
	raw, err := w.get(id)
	if err == nil || errors.Is(err, ErrNotFound) {
		return raw, err
	}
	select {
	case <-ctx.Done():
		return nil, err
	case <-time.After(500 * time.Millisecond):
	}
	return w.get(id)
}

func (w *windowsStore) Write(_ context.Context, id string, value []byte, _ bool) error {
	cred := wincred.NewGenericCredential(w.target(id))
	cred.UserName = id
	cred.CredentialBlob = value
	cred.Persist = wincred.PersistLocalMachine
	if err := cred.Write(); err != nil {
		return Temporary(ReasonUnreachable, err)
	}
	return nil
}

func (w *windowsStore) Delete(_ context.Context, id string) error {
	cred, err := wincred.GetGenericCredential(w.target(id))
	if err != nil {
		if errors.Is(err, wincred.ErrElementNotFound) {
			return ErrNotFound
		}
		return Temporary(ReasonUnreachable, err)
	}
	if err := cred.Delete(); err != nil {
		if errors.Is(err, wincred.ErrElementNotFound) {
			return ErrNotFound
		}
		return Temporary(ReasonUnreachable, err)
	}
	return nil
}
