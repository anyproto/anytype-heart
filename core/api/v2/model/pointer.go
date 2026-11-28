package v2model

import "strings"

// pointerEscaper escapes one JSON Pointer reference token (RFC 6901): "~"
// first as "~0", then "/" as "~1".
var pointerEscaper = strings.NewReplacer("~", "~0", "/", "~1")

// PointerToken escapes one member name as a JSON Pointer reference token. A
// member name such as "wins/losses" is ONE member: appended to a path raw, it
// addresses two nested ones. Every issue path that appends a name the caller
// chose goes through here, in the handler and service layers alike.
func PointerToken(name string) string {
	return pointerEscaper.Replace(name)
}

// MemberPointer is the pointer to one top-level member of a request body.
func MemberPointer(name string) string {
	return "/" + PointerToken(name)
}
