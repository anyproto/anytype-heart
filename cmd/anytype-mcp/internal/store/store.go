// Package store is the forwarder's item store: one private file per
// connection in a per-user Anytype folder, read and written only through
// the forwarder's verbs. The forwarder runs as its caller, so it reads only
// what its caller could read: there is no keychain item that would give it
// more rights than whoever started it.
package store

import (
	"context"
	"errors"
	"fmt"
)

// Namespaces. The namespace is fixed when the binary is linked; the
// development one is the default, so a local build can never create or
// read a production item.
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

// FolderName is the items folder of a namespace, under
// <home>/.anytype/ai-connect. These names never change.
func FolderName(ns string) string {
	if NormalizeNamespace(ns) == NamespaceRelease {
		return "items"
	}
	return "items-dev"
}

// ErrNotFound: the store answered and holds no item for the connection.
var ErrNotFound = errors.New("no item for this connection")

// TemporaryError is a store that can't serve right now: a folder or item
// that fails a safety rule, or one that can't be reached. It always means
// "nothing was read or written, try again" and never "the item is gone".
type TemporaryError struct {
	Reason string
	Err    error
}

func (e *TemporaryError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("item store not ready (%s): %v", e.Reason, e.Err)
	}
	return fmt.Sprintf("item store not ready (%s)", e.Reason)
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
	// ReasonDenied: a folder or the item fails a safety rule (a link, a
	// foreign owner, a mode that lets others in).
	ReasonDenied = "denied"
	// ReasonUnreachable: any other failure to read or write.
	ReasonUnreachable = "unreachable"
)

// denied is a failed safety rule. The text names the rule, never a value.
func denied(rule string) error { return Temporary(ReasonDenied, errors.New(rule)) }

func unreachable(err error) error { return Temporary(ReasonUnreachable, err) }

// UnconfirmedError is a write the store took (the new item is in place)
// that then failed to confirm. It is never "nothing was written".
type UnconfirmedError struct{ Err error }

func (e *UnconfirmedError) Error() string {
	return fmt.Sprintf("the item was written but not confirmed: %v", e.Err)
}

func (e *UnconfirmedError) Unwrap() error { return e.Err }

// Store holds one opaque value per connection id.
type Store interface {
	// Kind names the backend.
	Kind() string
	// Probe reports whether the store is usable: nil or a TemporaryError.
	Probe(ctx context.Context) error
	// Read returns the value, ErrNotFound, ErrInvalidItem (a value too
	// large to be an item) or a TemporaryError.
	Read(ctx context.Context, id string) ([]byte, error)
	// Write replaces the item atomically. A TemporaryError means nothing
	// was written; an UnconfirmedError means the new value is in place.
	Write(ctx context.Context, id string, value []byte) error
	// Delete removes the item; ErrNotFound when there is none.
	Delete(ctx context.Context, id string) error
	// Lock takes the connection's exclusive lock, which serializes the
	// verbs of cooperating Anytype profiles.
	Lock(ctx context.Context, id string) (unlock func(), err error)
}
