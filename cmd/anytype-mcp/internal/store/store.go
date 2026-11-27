// Package store is the forwarder's key store: one OS keychain item per
// connection, created and read by this binary only.
package store

import (
	"context"
	"errors"
	"fmt"
)

// Namespaces. The namespace is fixed when the binary is linked; the
// development one is the default, so a local build can never create or
// read a production-named item.
const (
	NamespaceDev     = "dev"
	NamespaceRelease = "release"
)

// NormalizeNamespace maps anything that is not exactly the release
// namespace to the development one.
func NormalizeNamespace(ns string) string {
	if ns == NamespaceRelease {
		return NamespaceRelease
	}
	return NamespaceDev
}

// ServiceName is the keychain service (macOS) and the credential prefix
// (Windows) of a namespace. These names never change.
func ServiceName(ns string) string {
	if NormalizeNamespace(ns) == NamespaceRelease {
		return "Anytype MCP"
	}
	return "Anytype MCP Dev"
}

// SecretServiceName is the "service" attribute of a Secret Service item
// (Linux).
func SecretServiceName(ns string) string {
	if NormalizeNamespace(ns) == NamespaceRelease {
		return "anytype-mcp"
	}
	return "anytype-mcp-dev"
}

var (
	// ErrNotFound: the store answered and holds no item for the connection.
	ErrNotFound = errors.New("no item for this connection")
	// ErrNoStore: this system has no key store at all.
	ErrNoStore = errors.New("this system has no key store")
)

// TemporaryError is a store that exists but can't serve right now: locked,
// access denied, the session bus unreachable, the keyring still starting.
// It always means "try again" and never "the item is gone".
type TemporaryError struct {
	Reason string
	Err    error
}

func (e *TemporaryError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("key store not ready (%s): %v", e.Reason, e.Err)
	}
	return fmt.Sprintf("key store not ready (%s)", e.Reason)
}

func (e *TemporaryError) Unwrap() error { return e.Err }

// Temporary builds a TemporaryError.
func Temporary(reason string, err error) error {
	return &TemporaryError{Reason: reason, Err: err}
}

// IsTemporary reports whether err is a TemporaryError.
func IsTemporary(err error) bool {
	var t *TemporaryError
	return errors.As(err, &t)
}

// Reasons a store reports in a TemporaryError.
const (
	ReasonLocked      = "locked"
	ReasonDenied      = "denied"
	ReasonUnreachable = "unreachable"
	ReasonNotReady    = "not-ready"
)

// Store holds one opaque value per connection id. interactive allows the
// OS to show an unlock prompt; only a key set started by a user's click
// passes it.
type Store interface {
	// Kind names the backend: keychain, wincred, secret-service, file.
	Kind() string
	// Probe reports whether the store can take a write now: nil,
	// ErrNoStore or a TemporaryError. It never prompts.
	Probe(ctx context.Context) error
	// Read returns the value, ErrNotFound, ErrNoStore or a TemporaryError.
	Read(ctx context.Context, id string, interactive bool) ([]byte, error)
	// Write creates the item or replaces its value in place.
	Write(ctx context.Context, id string, value []byte, interactive bool) error
	// Delete removes the item; ErrNotFound when there is none.
	Delete(ctx context.Context, id string) error
}
