//go:build darwin && cgo

package store

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
#include <Security/Security.h>

static OSStatus amcpSetInteraction(Boolean allowed) {
	return SecKeychainSetUserInteractionAllowed(allowed);
}

// amcpDefaultStatus reports the default keychain's status bits.
static OSStatus amcpDefaultStatus(UInt32 *status) {
	SecKeychainRef kc = NULL;
	OSStatus s = SecKeychainCopyDefault(&kc);
	if (s != errSecSuccess) {
		return s;
	}
	s = SecKeychainGetStatus(kc, status);
	CFRelease(kc);
	return s;
}
*/
import "C"

import (
	"context"
	"errors"
	"fmt"

	"github.com/keybase/go-keychain"
)

// darwinStore keeps one generic-password item per connection in the
// user's keychain. The binary that creates an item is the one trusted to
// read it, so the same signed forwarder does both; an update goes through
// SecItemUpdate, which keeps the item's access list.
type darwinStore struct{ service string }

func openPlatform(namespace string) Store { return newPlatformStore(ServiceName(namespace)) }

// newPlatformStore takes the service name itself; tests use their own.
func newPlatformStore(service string) Store { return &darwinStore{service: service} }

func (d *darwinStore) Kind() string { return "keychain" }

// interaction turns the system's keychain prompts on or off for this
// process. Off is the rule; only an interactive key set turns it on.
func interaction(allowed bool) {
	var b C.Boolean
	if allowed {
		b = 1
	}
	C.amcpSetInteraction(b)
}

const (
	errSecNoDefaultKeychain = -25307
	errSecUnlockedBit       = 1 // kSecUnlockStateStatus
)

func (d *darwinStore) mapErr(err error) error {
	if err == nil {
		return nil
	}
	var kerr keychain.Error
	if !errors.As(err, &kerr) {
		return err
	}
	switch kerr {
	case keychain.ErrorItemNotFound:
		return ErrNotFound
	case keychain.ErrorInteractionNotAllowed:
		return Temporary(ReasonLocked, nil)
	case keychain.ErrorAuthFailed, keychain.ErrorUserCanceled, keychain.ErrorNoAccessForItem:
		return Temporary(ReasonDenied, nil)
	case keychain.ErrorNotAvailable, keychain.ErrorNoSuchKeychain, keychain.Error(errSecNoDefaultKeychain):
		return Temporary(ReasonUnreachable, nil)
	}
	return fmt.Errorf("keychain error %d", int(kerr))
}

func (d *darwinStore) Probe(context.Context) error {
	interaction(false)
	var status C.UInt32
	if s := C.amcpDefaultStatus(&status); s != 0 {
		return d.mapErr(keychain.Error(s))
	}
	if status&errSecUnlockedBit == 0 {
		return Temporary(ReasonLocked, nil)
	}
	return nil
}

// refine names a refusal that a locked keychain caused "locked": with
// prompts off, the system reports some of those as a failed
// authorisation.
func (d *darwinStore) refine(err error) error {
	var t *TemporaryError
	if errors.As(err, &t) && t.Reason == ReasonDenied {
		var p *TemporaryError
		if errors.As(d.Probe(context.Background()), &p) && p.Reason == ReasonLocked {
			return p
		}
	}
	return err
}

func (d *darwinStore) query(id string) keychain.Item {
	q := keychain.NewItem()
	q.SetSecClass(keychain.SecClassGenericPassword)
	q.SetService(d.service)
	q.SetAccount(id)
	return q
}

func (d *darwinStore) Read(_ context.Context, id string, interactive bool) ([]byte, error) {
	interaction(interactive)
	defer interaction(false)
	q := d.query(id)
	q.SetMatchLimit(keychain.MatchLimitOne)
	q.SetReturnData(true)
	res, err := keychain.QueryItem(q)
	if err != nil {
		return nil, d.refine(d.mapErr(err))
	}
	if len(res) == 0 {
		return nil, ErrNotFound
	}
	return res[0].Data, nil
}

func (d *darwinStore) Write(_ context.Context, id string, value []byte, interactive bool) error {
	interaction(interactive)
	defer interaction(false)
	upd := keychain.NewItem()
	upd.SetData(value)
	err := keychain.UpdateItem(d.query(id), upd)
	if err == nil {
		return nil
	}
	if !errors.Is(d.mapErr(err), ErrNotFound) {
		return d.refine(d.mapErr(err))
	}
	item := keychain.NewGenericPassword(d.service, id, d.service+": "+id, value, "")
	return d.refine(d.mapErr(keychain.AddItem(item)))
}

func (d *darwinStore) Delete(_ context.Context, id string) error {
	interaction(false)
	return d.refine(d.mapErr(keychain.DeleteItem(d.query(id))))
}
