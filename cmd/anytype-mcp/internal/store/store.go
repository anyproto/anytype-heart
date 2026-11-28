// Package store reads the endpoint file: the one non-secret file that
// tells the forwarder which port the local Anytype listens on. The desktop
// app writes it; the forwarder only reads it, and only through the safety
// rules of file.go, because the bearer goes wherever the file points. The
// forwarder runs as its caller, so it reads only what its caller could.
package store

import (
	"errors"
	"fmt"
)

// Namespaces. The namespace is fixed when the binary is linked; the
// development one is the default, so a local build never follows a
// production Anytype.
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

// FileName is the endpoint file of a namespace, in
// <home>/.anytype/ai-connect. These names never change.
func FileName(ns string) string {
	if NormalizeNamespace(ns) == NamespaceRelease {
		return "endpoint.json"
	}
	return "endpoint-dev.json"
}

// Reasons an endpoint read fails. None of them is final: the next request
// reads again.
const (
	// ReasonMissing: a folder on the path or the file does not exist.
	ReasonMissing = "missing"
	// ReasonInvalid: the file is not an endpoint this binary reads.
	ReasonInvalid = "invalid"
	// ReasonDenied: a folder, the file or the forwarder's own executable
	// fails a safety rule (a link, a foreign owner, a mode or an ACL that
	// lets others in).
	ReasonDenied = "denied"
	// ReasonUnreachable: any other failure to read, the read timeout
	// included.
	ReasonUnreachable = "unreachable"
)

// Error is a failed read. Path is the component it failed on and Rule the
// rule that failed; neither ever carries file content.
type Error struct {
	Reason string
	Path   string
	Rule   string
	Err    error
}

func (e *Error) Error() string {
	msg := e.Reason
	if e.Rule != "" {
		msg += ": " + e.Rule
	}
	if e.Path != "" {
		msg += ": " + e.Path
	}
	if e.Err != nil {
		msg = fmt.Sprintf("%s (%v)", msg, e.Err)
	}
	return msg
}

func (e *Error) Unwrap() error { return e.Err }

// AsError is err as an *Error; an error of any other kind is unreachable.
func AsError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return &Error{Reason: ReasonUnreachable, Err: err}
}

func deniedAt(path, rule string) error { return &Error{Reason: ReasonDenied, Path: path, Rule: rule} }

func missing(path string) error { return &Error{Reason: ReasonMissing, Path: path} }

func invalid(path, rule string) error { return &Error{Reason: ReasonInvalid, Path: path, Rule: rule} }

func unreachable(path string, err error) error {
	return &Error{Reason: ReasonUnreachable, Path: path, Err: err}
}
