package service

// v2_discovery.go implements the Phase-1 discovery lists (APIV2.md):
// spaces, members, types, the per-type AnyBlock document, properties, and
// property options. All lists paginate per C10; rows are minimal and speak
// the format's vocabulary (C2: keys and names, no id/key duality).

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// ListSpaces returns minimal space rows from the tech space's space views.
func (s *V2Service) ListSpaces(ctx context.Context, offset, limit int) ([]apimodel.V2SpaceRow, int, bool, error) {
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

	rows := make([]apimodel.V2SpaceRow, 0, len(records))
	seen := map[string]bool{}
	for _, record := range records {
		id := record.Details.GetString(bundle.RelationKeyTargetSpaceId)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		rows = append(rows, apimodel.V2SpaceRow{Id: id, Name: record.Details.GetString(bundle.RelationKeyName)})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Id < rows[j].Id })

	total := len(rows)
	page, hasMore := pagination.Paginate(rows, offset, limit)
	return page, total, hasMore, nil
}

// ListMembers returns minimal member rows (active participants) — agents
// need member ids for assignee/creator property values.
func (s *V2Service) ListMembers(ctx context.Context, spaceId string, offset, limit int) ([]apimodel.V2MemberRow, int, bool, error) {
	if err := s.ensureSpace(spaceId); err != nil {
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
	rows := make([]apimodel.V2MemberRow, 0, len(records))
	for _, record := range records {
		rows = append(rows, apimodel.V2MemberRow{
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
func (s *V2Service) GetMemberMe(ctx context.Context, spaceId string) (apimodel.V2MemberRow, error) {
	if err := s.ensureSpace(spaceId); err != nil {
		return apimodel.V2MemberRow{}, err
	}
	if s.accountId == "" {
		return apimodel.V2MemberRow{}, apimodel.V2NotFound(
			"the caller's account identity is not available on this server — list members with GET /v2/spaces/{spaceId}/members instead")
	}
	row := apimodel.V2MemberRow{
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
func (s *V2Service) ListTypes(ctx context.Context, spaceId string, offset, limit int) ([]apimodel.V2TypeRow, int, bool, error) {
	if err := s.ensureSpace(spaceId); err != nil {
		return nil, 0, false, err
	}
	records, total, err := s.store.SpaceIndex(spaceId).QueryAndCount(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_objectType)),
			},
			{
				RelationKey: bundle.RelationKeyIsHidden,
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       domain.Bool(true),
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
		return nil, 0, false, fmt.Errorf("query types in space %s: %w", spaceId, err)
	}

	hasMore := len(records) > limit
	if hasMore {
		records = records[:limit]
	}
	rows := make([]apimodel.V2TypeRow, 0, len(records))
	for _, record := range records {
		key, err := domain.GetTypeKeyFromRawUniqueKey(record.Details.GetString(bundle.RelationKeyUniqueKey))
		if err != nil {
			continue
		}
		rows = append(rows, apimodel.V2TypeRow{Key: string(key), Name: record.Details.GetString(bundle.RelationKeyName)})
	}
	return rows, total, hasMore, nil
}

// GetType returns the kind:"objectType" AnyBlock document for one type key,
// read via the live smartblock state like any object (§8).
func (s *V2Service) GetType(ctx context.Context, spaceId, typeKey string) ([]byte, string, error) {
	if err := s.ensureSpace(spaceId); err != nil {
		return nil, "", err
	}
	uk, err := domain.NewUniqueKey(coresb.SmartBlockTypeObjectType, typeKey)
	if err != nil {
		return nil, "", apimodel.V2ValidationFailed("invalid type key",
			apimodel.V2Issue{Path: "type", Message: fmt.Sprintf("invalid type key %q", typeKey)})
	}
	details, err := s.store.SpaceIndex(spaceId).GetObjectByUniqueKey(uk)
	if err != nil || details.GetString(bundle.RelationKeyId) == "" {
		return nil, "", apimodel.V2NotFound(fmt.Sprintf("type %q not found in space %q — list available keys with GET /v2/spaces/%s/types", typeKey, spaceId, spaceId))
	}
	return s.GetObject(ctx, spaceId, details.GetString(bundle.RelationKeyId), V2ObjectQuery{})
}

// GetTypeSchema is the [build] GenerateSchema endpoint — the derived
// artifact does not exist yet (SPEC §2a: planned, not implemented), so the
// route reports 501 until it lands.
func (s *V2Service) GetTypeSchema(ctx context.Context, spaceId, typeKey string) error {
	if err := s.ensureSpace(spaceId); err != nil {
		return err
	}
	return apimodel.NewV2Error(http.StatusNotImplemented, apimodel.V2CodeNotImplemented,
		fmt.Sprintf("type schema generation is not implemented yet — read the type document at GET /v2/spaces/%s/types/%s instead", spaceId, typeKey))
}

// ListProperties returns minimal property rows: key, name, format.
func (s *V2Service) ListProperties(ctx context.Context, spaceId string, offset, limit int) ([]apimodel.V2PropertyRow, int, bool, error) {
	if err := s.ensureSpace(spaceId); err != nil {
		return nil, 0, false, err
	}
	records, total, err := s.store.SpaceIndex(spaceId).QueryAndCount(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_relation)),
			},
			{
				RelationKey: bundle.RelationKeyIsHidden,
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       domain.Bool(true),
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
		return nil, 0, false, fmt.Errorf("query properties in space %s: %w", spaceId, err)
	}

	hasMore := len(records) > limit
	if hasMore {
		records = records[:limit]
	}
	rows := make([]apimodel.V2PropertyRow, 0, len(records))
	for _, record := range records {
		key := record.Details.GetString(bundle.RelationKeyRelationKey)
		if key == "" {
			continue
		}
		rows = append(rows, apimodel.V2PropertyRow{
			Key:    key,
			Name:   record.Details.GetString(bundle.RelationKeyName),
			Format: anyblockjson.FormatName(model.RelationFormat(record.Details.GetInt64(bundle.RelationKeyRelationFormat))),
		})
	}
	return rows, total, hasMore, nil
}

// ListPropertyOptions returns the option names (+color) of one
// select/multiSelect property, with a prefix filter (C10 — tag-like
// properties can hold thousands of options).
func (s *V2Service) ListPropertyOptions(ctx context.Context, spaceId, propertyKey, prefix string, offset, limit int) ([]apimodel.V2OptionRow, int, bool, error) {
	if err := s.ensureSpace(spaceId); err != nil {
		return nil, 0, false, err
	}
	index := s.store.SpaceIndex(spaceId)
	if _, err := index.GetRelationByKey(propertyKey); err != nil {
		return nil, 0, false, apimodel.V2NotFound(fmt.Sprintf("property %q not found in space %q — list available keys with GET /v2/spaces/%s/properties", propertyKey, spaceId, spaceId))
	}
	options, err := index.ListRelationOptions(domain.RelationKey(propertyKey))
	if err != nil {
		return nil, 0, false, fmt.Errorf("list options of property %s: %w", propertyKey, err)
	}

	rows := make([]apimodel.V2OptionRow, 0, len(options))
	for _, option := range options {
		if option == nil {
			continue
		}
		if prefix != "" && !strings.HasPrefix(strings.ToLower(option.Text), strings.ToLower(prefix)) {
			continue
		}
		rows = append(rows, apimodel.V2OptionRow{Name: option.Text, Color: option.Color})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })

	total := len(rows)
	page, hasMore := pagination.Paginate(rows, offset, limit)
	return page, total, hasMore, nil
}
