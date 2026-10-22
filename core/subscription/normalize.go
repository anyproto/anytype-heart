package subscription

import (
	"errors"
	"fmt"

	"github.com/cheggaaa/mb/v3"
	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// maxWindowBound caps client-controlled limit/offset so window arithmetic
// (offset+limit+margin) and the top-K heap allocation can never overflow
const maxWindowBound = 1 << 30

// subSpec is a normalized subscribe request: validated, subId assigned,
// keys deduplicated with id ensured, ready for filter compilation
type subSpec struct {
	subId   string
	spaceId string
	keys    []domain.RelationKey
	filters []database.FilterRequest
	sorts   []database.SortRequest
	source  []string

	// ordered: server-side ordering with a maintained window — every client
	// (non-internal) request and any request with sorts. For unordered subs
	// limit/offset truncate the snapshot only; live tracking always covers
	// the full set.
	ordered bool

	// scope gating: explicit ids (SubscribeIds, request-order Add events)
	// or a live collection membership stream
	scopeIds          []string
	scopeRequestOrder bool
	collectionId      string

	// withDeps enables render-dependency tracking ({subId}/dep)
	withDeps bool

	internal  bool
	asyncInit bool
	queue     *mb.MB[*pb.EventMessage] // caller-provided internal queue, may be nil

	limit, offset int
}

func normalizeSearch(req SubscribeRequest) (subSpec, error) {
	if req.SpaceId == "" {
		return subSpec{}, errors.New("spaceId is required")
	}
	if req.AsyncInit && !req.Internal {
		return subSpec{}, errors.New("asyncInit requires an internal subscription")
	}
	if req.InternalQueue != nil && !req.Internal {
		return subSpec{}, errors.New("internalQueue requires an internal subscription")
	}
	subId := req.SubId
	if subId == "" {
		subId = bson.NewObjectId().Hex()
	}
	// bounds-check the int64s BEFORE conversion: a huge client-sent limit
	// would overflow window arithmetic (offset+limit+margin) or the top-K
	// heap allocation, and int() truncates silently on 32-bit builds
	if req.Limit < 0 || req.Limit > maxWindowBound {
		return subSpec{}, fmt.Errorf("limit out of range: %d", req.Limit)
	}
	if req.Offset < 0 || req.Offset > maxWindowBound {
		return subSpec{}, fmt.Errorf("offset out of range: %d", req.Offset)
	}
	limit := int(req.Limit)
	offset := int(req.Offset)
	ordered := len(req.Sorts) > 0 || !req.Internal
	if req.AsyncInit && ordered {
		return subSpec{}, errors.New("asyncInit does not support sorted subscriptions")
	}
	withDeps := !req.NoDepSubscription
	if req.AsyncInit {
		// the dep snapshot would bypass the event stream the consumer reads
		// (deps return in-band while the main snapshot flows as events);
		// no asyncInit consumer uses deps — crossspacesub rejects them
		withDeps = false
	}
	// AfterId/BeforeId are dead request cursors: the client has always sent
	// them empty and no engine ever read them. Ignored by design.
	return subSpec{
		subId:        subId,
		spaceId:      req.SpaceId,
		keys:         normalizeKeys(req.Keys),
		filters:      req.Filters,
		sorts:        req.Sorts,
		source:       req.Source,
		ordered:      ordered,
		collectionId: req.CollectionId,
		withDeps:     withDeps,
		internal:     req.Internal,
		asyncInit:    req.AsyncInit,
		queue:        req.InternalQueue,
		limit:        limit,
		offset:       offset,
	}, nil
}

// normalizeSubscribeIds maps the explicit-id-list RPC onto the core
// primitive: a fixed id scope in request order, no filters at all (explicit
// ids are tracked even when archived or deleted), no ordering or counters
func normalizeSubscribeIds(req pb.RpcObjectSubscribeIdsRequest) (subSpec, error) {
	if req.SpaceId == "" {
		return subSpec{}, errors.New("spaceId is required")
	}
	subId := req.SubId
	if subId == "" {
		subId = bson.NewObjectId().Hex()
	}
	// dedupe preserving first occurrence: the scoped snapshot iterates the
	// scope list, so duplicates would yield duplicate records
	ids := make([]string, 0, len(req.Ids))
	seen := make(map[string]struct{}, len(req.Ids))
	for _, id := range req.Ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	// after filtering, so an all-blank list doesn't slip through as an
	// empty-scope subscription
	if len(ids) == 0 {
		return subSpec{}, errors.New("ids are required")
	}
	return subSpec{
		subId:             subId,
		spaceId:           req.SpaceId,
		keys:              normalizeKeys(req.Keys),
		scopeIds:          ids,
		scopeRequestOrder: true,
		withDeps:          !req.NoDepSubscription,
	}, nil
}

// normalizeKeys deduplicates the requested keys and guarantees id is present
func normalizeKeys(keys []string) []domain.RelationKey {
	res := make([]domain.RelationKey, 0, len(keys)+1)
	seen := make(map[string]struct{}, len(keys)+1)
	seen[bundle.RelationKeyId.String()] = struct{}{}
	res = append(res, bundle.RelationKeyId)
	for _, k := range keys {
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		res = append(res, domain.RelationKey(k))
	}
	return res
}

// resolveSources translates Source entries (setOf semantics) into a filter:
// object-type sources become `type In [...]`, relation sources become
// `relationKey Exists`, OR-combined. Each entry is tried as a unique key
// first (the JSON API sends `ot-…` unique keys), then as a type object id,
// then as a relation id — the same resolution order dataview uses.
func resolveSources(idx spaceindex.Store, source []string) ([]database.FilterRequest, error) {
	if len(source) == 0 {
		return nil, nil
	}
	var typeIds []string
	var relationKeys []string

	for _, entry := range source {
		if entry == "" {
			continue
		}
		if uk, err := domain.UnmarshalUniqueKey(entry); err == nil {
			switch uk.SmartblockType() {
			case coresb.SmartBlockTypeObjectType:
				details, err := idx.GetObjectByUniqueKey(uk)
				if err != nil {
					return nil, fmt.Errorf("resolve type source %s: %w", entry, err)
				}
				typeIds = append(typeIds, details.GetString(bundle.RelationKeyId))
				continue
			case coresb.SmartBlockTypeRelation:
				relationKeys = append(relationKeys, uk.InternalKey())
				continue
			}
		}
		if _, err := idx.GetObjectType(entry); err == nil {
			typeIds = append(typeIds, entry)
			continue
		}
		relation, err := idx.GetRelationById(entry)
		if err != nil {
			return nil, fmt.Errorf("source %s is neither an object type nor a relation: %w", entry, err)
		}
		relationKeys = append(relationKeys, relation.Key)
	}

	if len(typeIds) == 0 && len(relationKeys) == 0 {
		// a non-empty Source that resolves to nothing must not silently
		// degrade into an unscoped full-space subscription
		return nil, fmt.Errorf("source resolves to no types or relations: %v", source)
	}

	var alternatives []database.FilterRequest
	if len(typeIds) > 0 {
		alternatives = append(alternatives, database.FilterRequest{
			RelationKey: bundle.RelationKeyType,
			Condition:   model.BlockContentDataviewFilter_In,
			Value:       domain.StringList(typeIds),
		})
	}
	for _, key := range relationKeys {
		// Exists, not NotEmpty: setOf means "carries this property", so an
		// object holding it with an empty value still belongs to the set —
		// NotEmpty here also starved `Property → is empty` views (GO-7404)
		alternatives = append(alternatives, database.FilterRequest{
			RelationKey: domain.RelationKey(key),
			Condition:   model.BlockContentDataviewFilter_Exists,
		})
	}
	switch len(alternatives) {
	case 0:
		return nil, nil
	case 1:
		return alternatives, nil
	default:
		return []database.FilterRequest{{
			Operator:      model.BlockContentDataviewFilter_Or,
			NestedFilters: alternatives,
		}}, nil
	}
}

// truncateRecords applies the snapshot-only offset/limit to response records
func truncateRecords(records []*domain.Details, offset, limit int) []*domain.Details {
	if offset > 0 {
		if offset >= len(records) {
			return nil
		}
		records = records[offset:]
	}
	if limit > 0 && limit < len(records) {
		records = records[:limit]
	}
	return records
}
