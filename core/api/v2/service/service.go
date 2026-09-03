package v2service

// service.go holds the API v2 service: shared dependencies, the C7 etag
// helpers, and the compact-JSON envelope assembly (APIV2.md §8).

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/api/util"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

// log is v2's own logger. v1's service package keeps "api-internal-service";
// a separate package needs a separate logger, and a separate name is what
// makes a v2 line greppable.
var log = logging.Logger("api-v2-service")

// Service implements the API v2 endpoints. It reads objects via the live
// smartblock state (apicore.ObjectReader), creates them via the snapshot
// create path (apicore.ObjectCreator), and lists via the objectstore — per
// APIV2.md §8, not via ObjectShow and not via lagging store snapshots for
// object content.
type Service struct {
	mw      apicore.ClientCommands
	reader  apicore.ObjectReader
	creator apicore.ObjectCreator
	mutator apicore.ObjectMutator
	// provenance is the DELETE enforcement read: creator provenance from
	// validated change storage, never from details.
	// nil refuses every object DELETE — fail closed.
	provenance  apicore.ObjectProvenance
	store       objectstore.ObjectStore
	techSpaceId string
	// accountId is the caller's account identity — the input of the SPEC
	// §6.2 `_filter_template_2_` (current user) placeholder substitution on
	// stored-view execution. Empty = the placeholder degrades to a
	// warning instead of resolving.
	accountId string
	// chatSub backs the chat message stream. Nil means the stream route is
	// not registered, so OpenChatStream is never reached without it.
	chatSub apicore.ChatSubscriptionService
	// chatStreams caps how many streams are open at once; see
	// maxConcurrentChatStreams.
	chatStreams chatStreamSlots
}

// NewService creates the API v2 service. creator may be nil when only the
// read surface is served (the router skips the create routes then); mutator
// may be nil when the edit surface is not served; provenance may be nil
// (object DELETE then refuses everything — fail closed). accountId may be
// empty (degraded placeholder substitution only).
func NewService(mw apicore.ClientCommands, reader apicore.ObjectReader, creator apicore.ObjectCreator, mutator apicore.ObjectMutator, provenance apicore.ObjectProvenance, chatSub apicore.ChatSubscriptionService, store objectstore.ObjectStore, techSpaceId, accountId string) *Service {
	return &Service{mw: mw, reader: reader, creator: creator, mutator: mutator, provenance: provenance, chatSub: chatSub, store: store, techSpaceId: techSpaceId, accountId: accountId}
}

// ensureSpaceGranted is the space half of the service-level backstop of the
// route-layer grant gate (apiv2.ensureSpaceGrant, which owns the clean
// per-route 403): it consults the grant carried on the request context and
// fails closed, so a future route that forgets the middleware — or resolves
// space ids in some unusual way — still cannot reach a non-granted space.
// A nil grant is an unscoped/legacy key and passes; an empty granted-space
// list denies everything (util.ApiGrant.AllowsSpace — empty is never "all
// spaces"). The verb half is ensureWriteGranted, called by the write entry
// points via ensureSpaceWrite / ensureChatWrite.
func ensureSpaceGranted(ctx context.Context, spaceId string) error {
	grant := util.ApiGrantFromCtx(ctx)
	if grant == nil {
		return nil
	}
	if !grant.AllowsSpace(spaceId) {
		return v2model.SpaceNotGranted(fmt.Sprintf(
			"key not granted space %q; granted: %s", spaceId, grant.Describe()))
	}
	return nil
}

// ensureWriteGranted is the verb half of the service-level backstop: a
// write entry point reached with a read-only grant is refused even when the
// route middleware never ran. A nil grant is an unscoped/legacy key and
// passes; only the exact readwrite value writes (util.ApiGrant.CanWrite).
func ensureWriteGranted(ctx context.Context) error {
	grant := util.ApiGrantFromCtx(ctx)
	if grant == nil || grant.CanWrite() {
		return nil
	}
	return v2model.WriteNotGranted(fmt.Sprintf(
		"this operation is a write and the key's grant is read-only; granted: %s", grant.Describe()))
}

// ensureSpace rejects an unknown or non-live space_id before any per-space
// objectstore access (C2). objectstore.SpaceIndex mints — and persists to
// disk — a fresh index for ANY id, so without this guard an agent passing
// bogus or retained-but-deleted space ids could grow the in-memory registry,
// open backing DBs and read stale indexed state.
// GetSpaceViewDetails resolves the spaceView against the tech space only and
// never mints an index for spaceId; the tech space itself is trusted.
//
// The grant backstop runs FIRST — before the tech-space admission below,
// which deliberately treats the tech space as an ordinary space id: a
// scoped key must not reach the tech space unless it was explicitly
// granted.
func (s *Service) ensureSpace(ctx context.Context, spaceId string) error {
	if spaceId == "" {
		return v2model.NotFound("space id is required")
	}
	if err := ensureSpaceGranted(ctx, spaceId); err != nil {
		return err
	}
	if spaceId == s.techSpaceId {
		return nil
	}
	details, err := s.store.GetSpaceViewDetails(spaceId)
	if err != nil {
		// no candidate list here: ids are opaque (no did-you-mean can help)
		// and a per-caller grant means the full space list must not be
		// implied — the steer to the discovery route is the whole repair
		return v2model.NotFound(fmt.Sprintf("space %q not found — list spaces with GET /v2/spaces", spaceId))
	}
	if !isLiveSpaceView(details) {
		return spaceUnavailableError(spaceId)
	}
	return nil
}

// ensureSpaceWrite is ensureSpace for the service's WRITE entry points,
// with the route gate's precedence: grant space check, then the write-verb
// check, then the existence lookup — so space_not_granted wins over
// write_not_granted, and a read-only key is refused before anything
// resolves. (ensureSpace re-runs the space check; the duplication is the
// price of keeping each helper self-sufficiently fail-closed.)
func (s *Service) ensureSpaceWrite(ctx context.Context, spaceId string) error {
	if err := ensureSpaceGranted(ctx, spaceId); err != nil {
		return err
	}
	if err := ensureWriteGranted(ctx); err != nil {
		return err
	}
	return s.ensureSpace(ctx, spaceId)
}

//
// ---- etag (C7) ----
//

// etagDisplayLen is the agent-facing etag length: first 8 hex of the head
// hash (C7; display form — comparisons run against the full hash).
const etagDisplayLen = 8

// headsHash computes the full sha256 hex over the sorted tree heads.
func headsHash(heads []string) string {
	sorted := append([]string(nil), heads...)
	sort.Strings(sorted)
	h := sha256.New()
	for _, head := range sorted {
		h.Write([]byte(head))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ComputeEtag returns the agent-facing etag: the first 8 hex characters of
// sha256 over the object's sorted tree heads (C7). It is NOT the object's
// revision property.
func ComputeEtag(heads []string) string {
	return headsHash(heads)[:etagDisplayLen]
}

// QuoteEtag renders an etag as an RFC 7232 entity-tag for the ETag / If-Match
// headers (double-quoted). The envelope `etag` field stays bare.
func QuoteEtag(etag string) string {
	return `"` + etag + `"`
}

// unquoteEtag normalizes a client If-Match value: it strips an optional weak
// indicator and surrounding quotes, so both the header form (`"abc"`, `W/"abc"`)
// and the bare envelope form (`abc`) compare equal.
func unquoteEtag(v string) string {
	v = strings.TrimPrefix(v, "W/")
	v = strings.TrimPrefix(v, `"`)
	v = strings.TrimSuffix(v, `"`)
	return v
}

// EtagMatches checks a client-supplied If-Match value against the current
// head set. The comparison is done server-side against the full head-set
// hash; the 8-char display form is accepted as its prefix (C7/§8). The
// If-Match value is normalized first, so a quoted header tag matches (RFC 7232).
func EtagMatches(ifMatch string, heads []string) bool {
	if ifMatch == "" {
		return true // advisory by default: absent If-Match = last-write-wins
	}
	tag := unquoteEtag(ifMatch)
	full := headsHash(heads)
	return tag == full || tag == full[:etagDisplayLen]
}

//
// ---- envelope assembly ----
//

// envelopeKeyOrder is the canonical top-level key order of v2 response
// envelopes: the AnyBlock document order (SPEC §2) with the v2 additions
// (etag, outline, markdown, warnings) slotted in. Unknown keys append
// alphabetically after these.
var envelopeKeyOrder = []string{
	"$schema", "formatVersion", "etag", "kind", "id", "type", "template_for", "key",
	"properties", "type_properties", "refs", "outline", "blocks", "items",
	"store", "root", "markdown", "warnings",
}

// parseEnvelope decodes a JSON object's top level into raw fields.
func parseEnvelope(doc []byte) (map[string]json.RawMessage, error) {
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(doc, &fields); err != nil {
		return nil, fmt.Errorf("decode document envelope: %w", err)
	}
	return fields, nil
}

// encodeEnvelope re-emits the fields as one compact JSON object in the
// canonical key order (C3: compact JSON always) — INCLUDING the interior of
// every value.
//
// The interior matters because the values arrive verbatim: `anyblockjson.Marshal`
// returns the format's canonical byte form, which is two-space indented
// (SPEC §4, `marshalCanonical`), and parseEnvelope hands those bytes straight
// through as json.RawMessage. Before this compaction every object read was
// compact at the top level and pretty-printed underneath — measured at
// 16–26 % of the served tokens. Compacting HERE rather than in
// the format package keeps the canonical form untouched (it is what
// `Export ∘ Import` byte-stability is defined over) and covers every v2 body
// that re-embeds foreign bytes, not just the object read.
//
// json.Compact is whitespace-only: it does not re-escape strings (the escape
// pass is off for the exported Compact), so the format's non-HTML-escaping
// writer output survives byte-for-byte.
func encodeEnvelope(fields map[string]json.RawMessage) ([]byte, error) {
	known := map[string]bool{}
	for _, k := range envelopeKeyOrder {
		known[k] = true
	}
	var rest []string
	for k := range fields {
		if !known[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)

	out := []byte{'{'}
	first := true
	writeField := func(k string) error {
		raw, ok := fields[k]
		if !ok {
			return nil
		}
		if !first {
			out = append(out, ',')
		}
		first = false
		keyJSON, err := json.Marshal(k)
		if err != nil {
			return fmt.Errorf("encode envelope key %q: %w", k, err)
		}
		out = append(out, keyJSON...)
		out = append(out, ':')
		out = appendCompactJSON(out, raw)
		return nil
	}
	for _, k := range envelopeKeyOrder {
		if err := writeField(k); err != nil {
			return nil, err
		}
	}
	for _, k := range rest {
		if err := writeField(k); err != nil {
			return nil, err
		}
	}
	return append(out, '}'), nil
}

// appendCompactJSON appends value to dst with its insignificant whitespace
// stripped. A value encoding/json cannot scan is appended verbatim:
// encodeEnvelope is a formatter, not a validator, so malformed input keeps
// exactly the (equally malformed) body it produced before compaction existed.
func appendCompactJSON(dst, value []byte) []byte {
	var buf bytes.Buffer
	if err := json.Compact(&buf, value); err != nil {
		return append(dst, value...)
	}
	return append(dst, buf.Bytes()...)
}

// rawJSON marshals a value into a raw envelope field.
func rawJSON(v any) (json.RawMessage, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("encode envelope field: %w", err)
	}
	return data, nil
}
