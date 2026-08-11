package v2service

// spaceref.go is the short space reference (APIV2.md §8.35): a space id
// spelled as the last few characters of its CID half, served by default and
// accepted anywhere a space id is accepted.
//
// WHY the tail and not the head. A space id is `<CID>.<base36 replication
// key>` (any-sync spacepayloads.NewSpaceId). Every CID on one account is a
// CIDv1 in the same multibase and codec, so they all begin `bafyrei…` —
// about eight characters that distinguish nothing. The DISTINGUISHING end
// is the tail, which is why anyblockjson's block labels are
// shortest-unique-SUFFIX (mintedSuffixLabels) and why this is too.
//
// WHY the replication key is excluded. It is not per-space: derived spaces
// hash the account key and every client-created space reuses the personal
// space's, so all 17 spaces on the eval account end `.28y6mgnwgodt7`. It
// distinguishes nothing between spaces on one account — and dropping it
// removes the dot, which is the character `gemma4:e4b` truncated the id at
// in 83 of 93 `find` calls (§8.34). A reference with no dot cannot be
// truncated at one.
//
// The tail is therefore computed over the CID HALF ONLY. Feeding the
// composite ids into the census would make every tail identical, the
// collision rule would fire on every pair, and the feature would silently
// emit nothing on exactly the accounts it exists for.
//
// RESOLUTION reuses the rule this codebase already has rather than
// inventing a second one: exact full id first, then a unique suffix,
// ambiguity refused with the candidates listed — matchBlockRef's contract
// (object.go), applied to space ids over their CID halves.

import (
	"context"
	"fmt"
	"strings"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

// spaceShortRefLen is how many trailing characters of the CID half a served
// short reference carries.
//
// The CID half is 59 base32 characters (`b` multibase, CIDv1, sha-256), and
// its LAST character carries only 3 bits: 288 payload bits do not divide by
// 5, so the final base32 symbol is padded (the 17 real ids on the eval
// account use 7 of its 8 possible values). A tail of n characters therefore
// holds 3 + 5(n−1) bits — 28 bits at n = 6. Over 200 spaces that is a
// ~7 × 10⁻⁵ chance that any pair collides, and a collision is graceful
// (both keep their full spelling), not a failure.
const spaceShortRefLen = 6

// spaceIdMinCidLen is the floor on the CID half in the space-id shape test.
// A real one is 59 characters; 32 is a safe floor that no test fixture id
// or hand-written string reaches by accident.
const spaceIdMinCidLen = 32

// spaceIdCid returns the CID half of a space id — everything before the dot
// that joins it to the base36 replication key. An id with no dot is its own
// CID half.
func spaceIdCid(id string) string {
	if i := strings.IndexByte(id, '.'); i >= 0 {
		return id[:i]
	}
	return id
}

// isSpaceIdShaped reports whether id has the real space-id shape:
// `<base32 CID>.<base36 replication key>`. Only shaped ids get a short form
// and only shaped ids answer to a suffix — the same "anything unrecognised
// stays full" rule mintedSuffixLabels applies to block ids (isMintedLocalId).
// It is what keeps a synthetic id ("spaceLive", a tech space id from a
// fixture) out of the mechanism entirely: a false negative costs a few
// tokens, a false positive turns a meaningful identifier into a guess.
func isSpaceIdShaped(id string) bool {
	dot := strings.IndexByte(id, '.')
	if dot < spaceIdMinCidLen || dot == len(id)-1 {
		return false
	}
	cid, replKey := id[:dot], id[dot+1:]
	for i := 0; i < len(cid); i++ {
		if c := cid[i]; (c < 'a' || c > 'z') && (c < '2' || c > '7') {
			return false // base32 (RFC 4648 lowercase), the CIDv1 multibase `b`
		}
	}
	for i := 0; i < len(replKey); i++ {
		if c := replKey[i]; (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return false // base36, strconv.FormatUint(replicationKey, 36)
		}
	}
	return true
}

// spaceIdTail is the short-form CANDIDATE for one id: the last
// spaceShortRefLen characters of its CID half, or the id itself when the id
// is not space-shaped (which then wins no census entry it does not already
// own, so such ids never shorten).
func spaceIdTail(id string) string {
	if !isSpaceIdShaped(id) {
		return id
	}
	cid := spaceIdCid(id)
	if len(cid) <= spaceShortRefLen {
		return id
	}
	return cid[len(cid)-spaceShortRefLen:]
}

// shortSpaceRefs mints the served short reference for each id in a caller's
// visible space set, keyed by full id. Ids whose tails collide are ABSENT
// from the map and keep their full spelling — never serve an ambiguous
// short form (mintedSuffixLabels' `counts[suffix] == 1` guard). The census
// runs over the WHOLE visible set before anything is served, so a paginated
// page cannot hand out a tail that a space on another page also claims.
func shortSpaceRefs(ids []string) map[string]string {
	counts := make(map[string]int, len(ids))
	for _, id := range ids {
		counts[spaceIdTail(id)]++
	}
	out := make(map[string]string, len(ids))
	for _, id := range ids {
		tail := spaceIdTail(id)
		if tail == id || counts[tail] != 1 {
			continue
		}
		out[id] = tail
	}
	return out
}

// shortSpaceRef is shortSpaceRefs for one id against a census set: the
// short form when it is unambiguous, the full id otherwise.
func shortSpaceRef(ids []string, id string) string {
	if short, ok := shortSpaceRefs(ids)[id]; ok {
		return short
	}
	return id
}

// matchSpaceRef maps a space reference to an index into ids: an exact id
// match wins (matches = 1); otherwise SHAPED ids whose CID half ends with
// ref are counted and idx points at the last suffix match. This is
// matchBlockRef's rule (object.go) over the CID half — the same rule, not a
// second one.
//
// Matching the CID half rather than the full id is also what makes the
// §8.34 truncation resolve: the whole CID half is a suffix of itself.
func matchSpaceRef(ids []string, ref string) (idx, matches int) {
	suffix, suffixCount := -1, 0
	for i, id := range ids {
		if id == ref {
			return i, 1
		}
		if ref == "" || !isSpaceIdShaped(id) {
			continue
		}
		if strings.HasSuffix(spaceIdCid(id), ref) {
			suffix, suffixCount = i, suffixCount+1
		}
	}
	return suffix, suffixCount
}

// ResolveSpaceRef maps a space reference — a full id or a short one — to
// the full space id, for the ONE route param under which /v2 addresses a
// space (apiv2.SpaceParam). It is called by the /v2 route middleware BEFORE
// the grant gate, and its candidate set is deliberately the caller's
// VISIBLE spaces (live, and intersected with the key's grant by
// liveSpaceRows): a short reference must not become a way to probe spaces
// the key does not hold, so a tail belonging to a non-granted space simply
// does not resolve and the request continues to the same refusal any other
// unknown value gets.
//
// Contract:
//   - a space-SHAPED id is returned untouched: it needs no resolution, and
//     skipping the lookup is what keeps the common path free of a store
//     query.
//   - exactly one match → the full id.
//   - more than one → 400 ambiguous_input listing the candidates.
//   - none → the reference is returned UNCHANGED, so the existing 404
//     ("space %q not found") or the grant gate's 403 answers it, quoting
//     the caller's own value.
func (s *V2Service) ResolveSpaceRef(ctx context.Context, ref string) (string, error) {
	if ref == "" || isSpaceIdShaped(ref) {
		return ref, nil
	}
	rows, err := s.liveSpaceRows(ctx)
	if err != nil {
		// resolution must never be the reason a request fails: fall back to
		// the caller's value and let the ordinary path answer
		log.With("error", err).Warn("resolve space reference: list visible spaces")
		return ref, nil
	}
	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.Id
	}
	idx, matches := matchSpaceRef(ids, ref)
	switch {
	case matches == 1:
		return ids[idx], nil
	case matches > 1:
		// the candidates are named in the form GET /v2/spaces SERVES them
		// (short when unambiguous, full when their tails collide), so the
		// listed repair is a value the caller can paste back
		served := shortSpaceRefs(ids)
		candidates := make([]string, 0, matches)
		for i, id := range ids {
			if _, m := matchSpaceRef(ids[i:i+1], ref); m != 1 {
				continue
			}
			spelling := id
			if short, ok := served[id]; ok {
				spelling = short
			}
			if name := rows[i].Name; name != "" {
				spelling += " (" + name + ")"
			}
			candidates = append(candidates, spelling)
		}
		return "", v2model.AmbiguousInput(
			fmt.Sprintf("space %q matches more than one space", ref),
			v2model.Issue{
				Path:    "space",
				Message: "candidates: " + strings.Join(candidates, ", "),
				Hint:    "use a longer reference, or the id exactly as GET /v2/spaces prints it",
			})
	default:
		return ref, nil
	}
}

//
// ---- the echo (C2: serve short, accept either, echo what you were served)
//

// spaceEchoKey carries the spelling the caller used for a resolved space, so
// a refusal or a hint quotes that spelling back instead of the full id the
// caller never typed.
type spaceEchoKey struct{}

type spaceEcho struct{ full, ref string }

// CtxWithSpaceEcho records that ref resolved to full for this request.
func CtxWithSpaceEcho(ctx context.Context, full, ref string) context.Context {
	return context.WithValue(ctx, spaceEchoKey{}, spaceEcho{full: full, ref: ref})
}

// SpaceEchoFromCtx returns the (full id, caller's spelling) pair recorded by
// the resolution middleware, if any. Used by the ONE error-rendering choke
// point (v2handler.RespondV2Error) so every message and hint a refusal
// carries speaks the caller's own vocabulary — the alternative was the same
// substitution repeated at twenty Sprintf sites.
func SpaceEchoFromCtx(ctx context.Context) (full, ref string, ok bool) {
	echo, ok := ctx.Value(spaceEchoKey{}).(spaceEcho)
	if !ok || echo.full == "" || echo.ref == "" {
		return "", "", false
	}
	return echo.full, echo.ref, true
}
