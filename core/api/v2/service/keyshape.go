package v2service

// keyshape.go carries `?keys=` — the ONE parameter that chooses which
// vocabulary property and type keys are SPELLED in served AnyBlock bodies
// for serving: the stable minted api slug (default —
// what a long-lived integration remembers survives a rename) or the
// format's raw display names (what an agent-facing caller reads back to a
// user). Accept is unaffected: every write resolves BOTH vocabularies
// regardless (D3), so the parameter is a serving shape, exactly like
// `?ids=` — and it is carried the same way, for the same reasons idshape.go
// states: one parameter, one meaning, every body-serving surface, including
// the ones added later.
//
// The write channels' internal read-modify-write stays pinned to the slug
// vocabulary whatever this request asked for (§4.2): what a view op
// compares against must be what the canonical document spells, or the merge
// re-opens the duplicate-column class the pinning closed.

import (
	"context"
	"fmt"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

const (
	// V2KeysSlug serves property/type keys as the minted api slug (default).
	V2KeysSlug = "slug"
	// V2KeysName serves them as the format's raw display names.
	V2KeysName = "name"
)

// ParseKeysShape validates a raw `?keys=` value and reports whether the
// caller asked for the NAME vocabulary. The one definition of the legal
// values and of the 400 an unknown one earns — the route middleware is its
// only production caller, but the rule from idshape.go holds: a second copy
// of a value list is how routes start disagreeing.
func ParseKeysShape(raw string) (names bool, err error) {
	switch raw {
	case "", V2KeysSlug:
		return false, nil
	case V2KeysName:
		return true, nil
	default:
		return false, v2model.ValidationFailed("invalid keys value",
			v2model.Issue{Path: "keys", Message: fmt.Sprintf("unknown value %q", raw), Hint: "allowed: slug, name"})
	}
}

// nameKeysKey carries the request's answer to `?keys=`.
type nameKeysKey struct{}

// CtxWithNameKeys records that this request asked for `?keys=name`.
func CtxWithNameKeys(ctx context.Context) context.Context {
	return context.WithValue(ctx, nameKeysKey{}, true)
}

// nameKeysRequested reports whether this request asked for the name
// vocabulary. Absent — every internal caller, every test that does not
// care — means the slug default.
func nameKeysRequested(ctx context.Context) bool {
	names, _ := ctx.Value(nameKeysKey{}).(bool)
	return names
}
