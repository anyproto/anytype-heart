package v2service

// service.go holds the API v2 service: shared dependencies, the C7 etag
// helpers, and the compact-JSON envelope assembly (APIV2.md §8).

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

// log is v2's own logger. v1's service package keeps "api-internal-service";
// a separate package needs a separate logger, and a separate name is what
// makes a v2 line greppable.
var log = logging.Logger("api-v2-service")

// V2Service implements the API v2 endpoints. It reads objects via the live
// smartblock state (apicore.ObjectReader), creates them via the snapshot
// create path (apicore.ObjectCreator), and lists via the objectstore — per
// APIV2.md §8, not via ObjectShow and not via lagging store snapshots for
// object content.
type V2Service struct {
	mw          apicore.ClientCommands
	reader      apicore.ObjectReader
	creator     apicore.ObjectCreator
	mutator     apicore.ObjectMutator
	store       objectstore.ObjectStore
	techSpaceId string
	// accountId is the caller's account identity — the input of the SPEC
	// §6.2 `_filter_template_2_` (current user) placeholder substitution on
	// stored-view execution (Phase 4). Empty = the placeholder degrades to a
	// warning instead of resolving.
	accountId string
}

// NewV2Service creates the API v2 service. creator may be nil when only the
// read surface is served (the router skips the create routes then); mutator
// may be nil when the edit surface is not served. accountId may be empty
// (degraded placeholder substitution only).
func NewV2Service(mw apicore.ClientCommands, reader apicore.ObjectReader, creator apicore.ObjectCreator, mutator apicore.ObjectMutator, store objectstore.ObjectStore, techSpaceId, accountId string) *V2Service {
	return &V2Service{mw: mw, reader: reader, creator: creator, mutator: mutator, store: store, techSpaceId: techSpaceId, accountId: accountId}
}

// ensureSpace rejects an unknown space_id before any per-space objectstore
// access (C2). objectstore.SpaceIndex mints — and persists to disk — a fresh
// index for ANY id, so without this guard an agent passing bogus space ids
// could grow the in-memory registry and backing DBs without bound.
// GetSpaceViewDetails resolves the spaceView against the tech space only and
// never mints an index for spaceId; the tech space itself is trusted.
func (s *V2Service) ensureSpace(spaceId string) error {
	if spaceId == "" {
		return v2model.NotFound("space id is required")
	}
	if spaceId == s.techSpaceId {
		return nil
	}
	if _, err := s.store.GetSpaceViewDetails(spaceId); err != nil {
		return v2model.NotFound(fmt.Sprintf("space %q not found", spaceId))
	}
	return nil
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
	"$schema", "version", "etag", "kind", "id", "type", "templateFor", "key",
	"properties", "typeProperties", "refs", "outline", "blocks", "items",
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
// canonical key order (C3: compact JSON always).
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
		out = append(out, raw...)
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

// rawJSON marshals a value into a raw envelope field.
func rawJSON(v any) (json.RawMessage, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("encode envelope field: %w", err)
	}
	return data, nil
}
