package v2service

// idshape.go carries `?ids=` — the ONE parameter that chooses between the
// compact serving vocabulary and the full spelling (C4) — from the route
// layer down to the services that decide how an id is SPELLED in a response.
//
// WHY it needs a carrier at all. `?ids=` selects a serving SHAPE, and a
// response has more than one axis of spelling: the object read spells block
// ids (short doc-local labels vs the machine-minted ones), and every
// space-serving surface spells space ids (§8.35's short reference vs the
// full `<cid>.<replicationKey>` id). One parameter means one thing on all of
// them — "full spells everything in full" — so a caller asking for the
// export shape gets it everywhere in that response rather than on whichever
// axis the endpoint happens to own (§8.36).
//
// WHY the request context and not a parameter. servedSpaceRef is reached
// from GetSpace, CreateSpace, UpdateSpace, ListSpaces, the global-search
// fan-out and whoami — and several of those call each other, so threading a
// flag would mean touching every signature between the route and the
// spelling decision, and a surface added later would default to the wrong
// answer by omission. The ctx is the same carrier the §8.35 echo uses.

import (
	"context"
	"fmt"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

// ParseIdsShape validates a raw `?ids=` value and reports whether the caller
// asked for the FULL spelling.
//
// This is the ONE definition of the parameter's legal values and of the 400
// an unknown one earns. The route middleware (which carries the answer to
// the space surfaces) and the object read's own plan validation both go
// through it, so the two cannot come to disagree about what `ids=export`
// means — the second copy of a value list is how a surface starts accepting
// on one route what it refuses on another.
func ParseIdsShape(raw string) (full bool, err error) {
	switch raw {
	case "", V2IdsCompact:
		return false, nil
	case V2IdsFull:
		return true, nil
	default:
		return false, v2model.ValidationFailed("invalid ids value",
			v2model.Issue{Path: "ids", Message: fmt.Sprintf("unknown value %q", raw), Hint: "allowed: compact, full"})
	}
}

// fullIdsKey carries the request's answer to `?ids=`.
type fullIdsKey struct{}

// CtxWithFullIds records that this request asked for `?ids=full` — the
// export spelling, on every id the response carries.
func CtxWithFullIds(ctx context.Context) context.Context {
	return context.WithValue(ctx, fullIdsKey{}, true)
}

// fullIdsRequested reports whether this request asked for the full spelling.
// Absent — which is what every internal caller and every test that does not
// care carries — means compact, the default serving shape.
func fullIdsRequested(ctx context.Context) bool {
	full, _ := ctx.Value(fullIdsKey{}).(bool)
	return full
}
