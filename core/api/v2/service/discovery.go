package v2service

// discovery.go implements the Phase-1 discovery lists (APIV2.md):
// spaces, members, types, the per-type AnyBlock document, properties, and
// property options. All lists paginate per C10; rows are minimal and speak
// the format's vocabulary (C2: keys and names, no id/key duality).

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/util"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// ListSpaces returns minimal space rows from the tech space's space views —
// LIVE spaces only (isLiveSpaceView, the predicate shared with GET-one and
// the global-search fan-out): a deleted or left space's row is
// indistinguishable from a live one, and an agent picking it would write
// into a space that can never load. The row carries description too — it
// sits in the same record for free, and withholding it forced a 1+N of
// GET-one calls on the canonical "list my spaces, pick one" trace.
//
// A granted key sees ONLY its granted spaces: the route has no :space_id,
// so the gate lets it through as service-filtered and the intersection with
// the ctx grant happens here — a non-granted space's row (id, name,
// description alike) must never leave this method.
func (s *V2Service) ListSpaces(ctx context.Context, offset, limit int) ([]v2model.SpaceRow, int, bool, error) {
	grant := util.ApiGrantFromCtx(ctx)
	records, err := s.store.SpaceIndex(s.techSpaceId).Query(database.Query{
		Filters: []database.FilterRequest{{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Int64(int64(model.ObjectType_spaceView)),
		}},
	})
	if err != nil {
		return nil, 0, false, fmt.Errorf("query space views: %w", err)
	}

	rows := make([]v2model.SpaceRow, 0, len(records))
	seen := map[string]bool{}
	for _, record := range records {
		id := record.Details.GetString(bundle.RelationKeyTargetSpaceId)
		if id == "" || seen[id] {
			continue
		}
		if !isLiveSpaceView(record.Details) {
			continue
		}
		if grant != nil && !grant.AllowsSpace(id) {
			continue
		}
		seen[id] = true
		rows = append(rows, v2model.SpaceRow{
			Id:          id,
			Name:        record.Details.GetString(bundle.RelationKeyName),
			Description: record.Details.GetString(bundle.RelationKeyDescription),
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Id < rows[j].Id })

	total := len(rows)
	page, hasMore := pagination.Paginate(rows, offset, limit)
	return page, total, hasMore, nil
}

// ListMembers returns minimal member rows (active participants) — agents
// need member ids for assignee/creator property values.
func (s *V2Service) ListMembers(ctx context.Context, spaceId string, offset, limit int) ([]v2model.MemberRow, int, bool, error) {
	if err := s.ensureSpace(ctx, spaceId); err != nil {
		return nil, 0, false, err
	}
	records, total, err := s.store.SpaceIndex(spaceId).QueryAndCount(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_participant)),
			},
			{
				RelationKey: bundle.RelationKeyParticipantStatus,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ParticipantStatus_Active)),
			},
		},
		Sorts: []database.SortRequest{{
			RelationKey: bundle.RelationKeyName,
			Type:        model.BlockContentDataviewSort_Asc,
		}},
		Offset: offset,
		Limit:  limit + 1,
	})
	if err != nil {
		return nil, 0, false, fmt.Errorf("query members in space %s: %w", spaceId, err)
	}

	hasMore := len(records) > limit
	if hasMore {
		records = records[:limit]
	}
	rows := make([]v2model.MemberRow, 0, len(records))
	for _, record := range records {
		rows = append(rows, v2model.MemberRow{
			Id:       record.Details.GetString(bundle.RelationKeyId),
			Name:     record.Details.GetString(bundle.RelationKeyName),
			Role:     memberRole(model.ParticipantPermissions(record.Details.GetInt64(bundle.RelationKeyParticipantPermissions))),
			Identity: record.Details.GetString(bundle.RelationKeyIdentity),
		})
	}
	return rows, total, hasMore, nil
}

// GetMemberMe implements GET /v2/spaces/{spaceId}/members/me: the caller's
// own member row (§7.3 — the server-side identity behind the wrapper's `@me`
// sentinel; the same identity Phase 4's placeholder substitution uses). The
// participant id is deterministic, so the row is served even before the
// participant object reaches the store index (name/role empty then) — the id
// is what assignee/creator values need.
func (s *V2Service) GetMemberMe(ctx context.Context, spaceId string) (v2model.MemberRow, error) {
	if err := s.ensureSpace(ctx, spaceId); err != nil {
		return v2model.MemberRow{}, err
	}
	if s.accountId == "" {
		return v2model.MemberRow{}, v2model.NotFound(
			"the caller's account identity is not available on this server — list members with GET /v2/spaces/{spaceId}/members instead")
	}
	row := v2model.MemberRow{
		Id:       domain.NewParticipantId(spaceId, s.accountId),
		Identity: s.accountId,
	}
	// the store returns empty details (no error) for an unindexed id — only
	// trust the row's name/role once the participant object actually exists
	if details, err := s.store.SpaceIndex(spaceId).GetDetails(row.Id); err == nil && details.GetString(bundle.RelationKeyId) == row.Id {
		row.Name = details.GetString(bundle.RelationKeyName)
		row.Role = memberRole(model.ParticipantPermissions(details.GetInt64(bundle.RelationKeyParticipantPermissions)))
	}
	return row, nil
}

// memberRole maps participant permissions to the API role vocabulary
// (mirrors v1's mapping).
func memberRole(permissions model.ParticipantPermissions) string {
	switch permissions {
	case model.ParticipantPermissions_Reader:
		return "viewer"
	case model.ParticipantPermissions_Writer:
		return "editor"
	case model.ParticipantPermissions_Admin:
		return "admin"
	case model.ParticipantPermissions_Owner:
		return "owner"
	default:
		return "no_permissions"
	}
}

// ListTypes returns minimal type rows: keys + names.
func (s *V2Service) ListTypes(ctx context.Context, spaceId string, offset, limit int) ([]v2model.TypeRow, int, bool, error) {
	if err := s.ensureSpace(ctx, spaceId); err != nil {
		return nil, 0, false, err
	}
	records, total, err := s.store.SpaceIndex(spaceId).QueryAndCount(database.Query{
		// live filters: a UI-deleted (uninstalled) type must not list — the
		// §7.5-requirement-2 corpse policy (keys.go)
		Filters: append(liveTypeFilters(),
			database.FilterRequest{
				RelationKey: bundle.RelationKeyIsHidden,
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       domain.Bool(true),
			}),
		Sorts: []database.SortRequest{{
			RelationKey: bundle.RelationKeyName,
			Type:        model.BlockContentDataviewSort_Asc,
		}},
		Offset: offset,
		Limit:  limit + 1,
	})
	if err != nil {
		return nil, 0, false, fmt.Errorf("query types in space %s: %w", spaceId, err)
	}

	hasMore := len(records) > limit
	if hasMore {
		records = records[:limit]
	}
	liveEntries, err := s.liveTypes(spaceId)
	if err != nil {
		return nil, 0, false, err
	}
	keyTaken, slugCount := servedTypeKeySets(liveEntries)
	rows := make([]v2model.TypeRow, 0, len(records))
	for _, record := range records {
		key, err := domain.GetTypeKeyFromRawUniqueKey(record.Details.GetString(bundle.RelationKeyUniqueKey))
		if err != nil {
			continue
		}
		rows = append(rows, v2model.TypeRow{
			Key:  servedKey(string(key), record.Details.GetString(bundle.RelationKeyApiObjectKey), keyTaken, slugCount),
			Name: record.Details.GetString(bundle.RelationKeyName),
		})
	}
	return rows, total, hasMore, nil
}

// GetType returns the kind:"objectType" AnyBlock document for one type key,
// read via the live smartblock state like any object (§8). The query rides
// through to GetObject, so `?ids=full` works here exactly as on objects —
// the export shape must be one query parameter away on every document read
// (§8.25 promised it; hardcoding V2ObjectQuery{} broke it for types).
func (s *V2Service) GetType(ctx context.Context, spaceId, typeKey string, q V2ObjectQuery) ([]byte, string, error) {
	if err := s.ensureSpace(ctx, spaceId); err != nil {
		return nil, "", err
	}
	entry, err := s.requireLiveType(spaceId, typeKey, "/key")
	if err != nil {
		return nil, "", err
	}
	return s.GetObject(ctx, spaceId, entry.Id, q)
}

// GetTypeSchema is the [build] GenerateSchema endpoint — the derived
// artifact does not exist yet (SPEC §2a: planned, not implemented), so the
// route reports 501 until it lands.
func (s *V2Service) GetTypeSchema(ctx context.Context, spaceId, typeKey string) error {
	if err := s.ensureSpace(ctx, spaceId); err != nil {
		return err
	}
	return v2model.NewError(http.StatusNotImplemented, v2model.CodeNotImplemented,
		fmt.Sprintf("type schema generation is not implemented yet — read the type document at GET /v2/spaces/%s/types/%s instead", spaceId, typeKey))
}

// ListProperties returns minimal property rows: key, name, format.
func (s *V2Service) ListProperties(ctx context.Context, spaceId string, offset, limit int) ([]v2model.PropertyRow, int, bool, error) {
	if err := s.ensureSpace(ctx, spaceId); err != nil {
		return nil, 0, false, err
	}
	records, total, err := s.store.SpaceIndex(spaceId).QueryAndCount(database.Query{
		// live filters: a UI-deleted (uninstalled) property still carried the
		// relation layout and passed every filter here — the §2.3-6 defect;
		// the corpse policy excludes it (keys.go)
		Filters: append(livePropertyFilters(),
			database.FilterRequest{
				RelationKey: bundle.RelationKeyIsHidden,
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       domain.Bool(true),
			}),
		Sorts: []database.SortRequest{{
			RelationKey: bundle.RelationKeyName,
			Type:        model.BlockContentDataviewSort_Asc,
		}},
		Offset: offset,
		Limit:  limit + 1,
	})
	if err != nil {
		return nil, 0, false, fmt.Errorf("query properties in space %s: %w", spaceId, err)
	}

	hasMore := len(records) > limit
	if hasMore {
		records = records[:limit]
	}
	// the served spelling (§7.5a): a BSON-keyed property answers to its
	// slug, so the slug is what the row advertises — iff it round-trips
	// (servedKey); the count maps come from the full live set, not the page
	liveEntries, err := s.liveProperties(spaceId)
	if err != nil {
		return nil, 0, false, err
	}
	keyTaken, slugCount := servedPropertyKeySets(liveEntries)
	rows := make([]v2model.PropertyRow, 0, len(records))
	for _, record := range records {
		key := record.Details.GetString(bundle.RelationKeyRelationKey)
		if key == "" {
			continue
		}
		rows = append(rows, v2model.PropertyRow{
			Key:    servedKey(key, record.Details.GetString(bundle.RelationKeyApiObjectKey), keyTaken, slugCount),
			Name:   record.Details.GetString(bundle.RelationKeyName),
			Format: anyblockjson.FormatName(model.RelationFormat(record.Details.GetInt64(bundle.RelationKeyRelationFormat))),
		})
	}
	return rows, total, hasMore, nil
}

// ListPropertyOptions returns the option names (+color) of one
// select/multiSelect property, with a prefix filter (C10 — tag-like
// properties can hold thousands of options).
func (s *V2Service) ListPropertyOptions(ctx context.Context, spaceId, propertyKey, prefix string, offset, limit int) ([]v2model.OptionRow, int, bool, error) {
	if err := s.ensureSpace(ctx, spaceId); err != nil {
		return nil, 0, false, err
	}
	// live lookup, slug-aware — a corpse property's options are not an API
	// surface; options bind to the STORED key, so list by entry.Key
	entry, err := s.requireLiveProperty(spaceId, propertyKey)
	if err != nil {
		return nil, 0, false, err
	}
	options, err := s.store.SpaceIndex(spaceId).ListRelationOptions(domain.RelationKey(entry.Key))
	if err != nil {
		return nil, 0, false, fmt.Errorf("list options of property %s: %w", propertyKey, err)
	}

	rows := make([]v2model.OptionRow, 0, len(options))
	for _, option := range options {
		if option == nil {
			continue
		}
		if prefix != "" && !strings.HasPrefix(strings.ToLower(option.Text), strings.ToLower(prefix)) {
			continue
		}
		rows = append(rows, v2model.OptionRow{Name: option.Text, Color: option.Color})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })

	total := len(rows)
	page, hasMore := pagination.Paginate(rows, offset, limit)
	return page, total, hasMore, nil
}
