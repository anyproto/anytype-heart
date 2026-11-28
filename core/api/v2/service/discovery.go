package v2service

// discovery.go implements the discovery lists:
// spaces, members, types, the per-type AnyBlock document, properties, and
// property options. All lists paginate per C10; rows are minimal and speak
// the format's vocabulary (C2: keys and names, no id/key duality).

import (
	"cmp"
	"context"
	"fmt"
	"slices"
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
// into a space that can never load. A space that is still loading on this
// device is live and listed. The row carries description too — it
// sits in the same record for free, and withholding it forced a 1+N of
// GET-one calls on the canonical "list my spaces, pick one" trace.
//
// A granted key sees ONLY its granted spaces: the route has no :space_id,
// so the gate lets it through as service-filtered and the intersection with
// the ctx grant happens here — a non-granted space's row (id, name,
// description alike) must never leave this method.
func (s *Service) ListSpaces(ctx context.Context, offset, limit int) (rows []v2model.SpaceRow, total int, hasMore, hasNotGrantedSpaces bool, err error) {
	rows, hasNotGrantedSpaces, err = s.liveSpaceRowsWithGrantInfo(ctx)
	if err != nil {
		return nil, 0, false, false, err
	}
	// §8.35: the census runs over the WHOLE visible set, before pagination —
	// a page must not hand out a tail a space on another page also claims.
	// §8.36: `?ids=full` skips it, and the rows keep the full ids they
	// already carry — the spelling a caller can persist outside this API.
	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.Id
	}
	served := s.servedSpaceRefs(ctx, ids)
	for i := range rows {
		if short, ok := served[rows[i].Id]; ok {
			rows[i].Id = short
		}
	}

	total = len(rows)
	page, hasMore := pagination.Paginate(rows, offset, limit)
	return page, total, hasMore, hasNotGrantedSpaces, nil
}

// liveSpaceRows is the ONE enumeration of the spaces a caller can SEE: the
// tech space's space views, filtered to live ones (isLiveSpaceView) and
// intersected with the ctx grant, sorted by full id (visibleSpaces). Every surface that
// needs the caller's space set reads it here — the spaces list, the
// global-search fan-out (spaceRefs), whoami's name resolution and the
// §8.35 short-reference census and resolution — so the set a short
// reference resolves within is by construction the set the list served it
// from. Two enumerations would be two answers to "which spaces exist", and
// the short form is only unambiguous relative to a fixed set.
//
// Rows carry the FULL id: shortening is a serving decision, made by each
// caller of this method.
func (s *Service) liveSpaceRows(ctx context.Context) ([]v2model.SpaceRow, error) {
	rows, _, err := s.liveSpaceRowsWithGrantInfo(ctx)
	return rows, err
}

// liveSpaceRowsWithGrantInfo also reports whether the grant hid any live
// user spaces. Their identities and count remain outside the response.
//
// It is the store query plus visibleSpaces. Each record is projected to
// spaceViewKeys first — the keys the spaces stream subscribes with — so the
// definition cannot start reading a detail the stream does not carry.
func (s *Service) liveSpaceRowsWithGrantInfo(ctx context.Context) ([]v2model.SpaceRow, bool, error) {
	records, err := s.store.SpaceIndex(s.techSpaceId).Query(database.Query{Filters: spaceViewFilters()})
	if err != nil {
		return nil, false, fmt.Errorf("query space views: %w", err)
	}
	views := make([]*domain.Details, len(records))
	for i, record := range records {
		views[i] = record.Details.CopyOnlyKeys(spaceViewKeys...)
	}
	rows, hasNotGrantedSpaces := visibleSpaces(views, util.ApiGrantFromCtx(ctx), s.techSpaceId)
	return rows, hasNotGrantedSpaces, nil
}

// spaceViewKeys are the details the definition of a visible space reads —
// identity, target, the three statuses and the served row — and nothing
// else. The list projects its records to them and the spaces stream
// subscribes with them.
var spaceViewKeys = []domain.RelationKey{
	bundle.RelationKeyId,
	bundle.RelationKeyTargetSpaceId,
	bundle.RelationKeySpaceLocalStatus,
	bundle.RelationKeySpaceAccountStatus,
	bundle.RelationKeySpaceRemoteStatus,
	bundle.RelationKeyName,
	bundle.RelationKeyDescription,
	bundle.RelationKeyIconImage,
}

// spaceViewFilters is the query both the list and the spaces stream start
// from: every space view of the tech space. The store adds its default
// filters (not archived, not deleted) on both paths.
func spaceViewFilters() []database.FilterRequest {
	return []database.FilterRequest{{
		RelationKey: bundle.RelationKeyResolvedLayout,
		Condition:   model.BlockContentDataviewFilter_Equal,
		Value:       domain.Int64(int64(model.ObjectType_spaceView)),
	}}
}

// visibleSpaces is the ONE definition of the spaces a key can see
// (APIV2_SPACE_STREAM.md §3.2): of the given space views, the live ones
// (spaceViewState) the grant admits, one row per target space, sorted by full
// id — and whether the grant hid any live user space. The list calls it with
// the views it just queried; the spaces stream calls it with its view map
// after every change.
//
// It is pure. The grant is a PARAMETER, never read from a context: the
// stream's grant changes while it is open, and a helper reading the request
// context would keep serving the grant the stream was opened with.
//
// Between two live, granted views of one target the first by VIEW ID names
// the row. A view id is derived from its space, so two views of one target
// exist only in damaged data; the rule is what makes the answer the same for
// every input order (the stream's map has none).
func visibleSpaces(views []*domain.Details, grant *util.ApiGrant, techSpaceId string) (rows []v2model.SpaceRow, hasNotGrantedSpaces bool) {
	ordered := append([]*domain.Details(nil), views...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].GetString(bundle.RelationKeyId) < ordered[j].GetString(bundle.RelationKeyId)
	})
	rows = make([]v2model.SpaceRow, 0, len(ordered))
	seen := map[string]bool{}
	for _, view := range ordered {
		id := view.GetString(bundle.RelationKeyTargetSpaceId)
		if id == "" || seen[id] {
			continue
		}
		if !isLiveSpaceView(view) {
			continue
		}
		// The same shared check the route gate and the ensureSpaceGranted
		// backstop run, so this enumeration mirror cannot disagree with
		// them. AllowsSpace alone would be vacuously true for every id
		// under an allSpaces grant, including the tech space: safe only for
		// as long as the tech space owns no space view of itself, which is
		// a property of the stored data and not of this code. If it ever
		// did, ListSpaces and whoami would advertise a space the gate
		// refuses — the mirror/gate divergence the anti-drift test exists
		// to catch.
		if grant != nil && util.SpaceGrantRefusal(grant, id, techSpaceId) != "" {
			if id != techSpaceId {
				hasNotGrantedSpaces = true
			}
			continue
		}
		seen[id] = true
		rows = append(rows, v2model.SpaceRow{
			Id:          id,
			Name:        view.GetString(bundle.RelationKeyName),
			Description: view.GetString(bundle.RelationKeyDescription),
			IconImage:   view.GetString(bundle.RelationKeyIconImage),
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Id < rows[j].Id })
	return rows, hasNotGrantedSpaces
}

// ListMembers returns minimal member rows (active participants) — agents
// need member ids for assignee/creator property values.
func (s *Service) ListMembers(ctx context.Context, spaceId string, offset, limit int) ([]v2model.MemberRow, int, bool, error) {
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
		Sorts: database.WithIdTieBreak([]database.SortRequest{{
			RelationKey: bundle.RelationKeyName,
			Type:        model.BlockContentDataviewSort_Asc,
		}}),
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
			Id:        record.Details.GetString(bundle.RelationKeyId),
			Name:      record.Details.GetString(bundle.RelationKeyName),
			Role:      memberRole(model.ParticipantPermissions(record.Details.GetInt64(bundle.RelationKeyParticipantPermissions))),
			Identity:  record.Details.GetString(bundle.RelationKeyIdentity),
			IconImage: record.Details.GetString(bundle.RelationKeyIconImage),
		})
	}
	return rows, total, hasMore, nil
}

// GetMemberMe implements GET /v2/spaces/{space_id}/members/me: the caller's
// own member row (§7.3 — the server-side identity behind the wrapper's `@me`
// sentinel; the same identity placeholder substitution uses). The
// participant id is deterministic, so the row is served even before the
// participant object reaches the store index (name/role empty then) — the id
// is what assignee/creator values need.
func (s *Service) GetMemberMe(ctx context.Context, spaceId string) (v2model.MemberRow, error) {
	if err := s.ensureSpace(ctx, spaceId); err != nil {
		return v2model.MemberRow{}, err
	}
	if s.accountId == "" {
		return v2model.MemberRow{}, v2model.NotFound(
			"the caller's account identity is not available on this server",
			v2model.Issue{Message: "the server holds no account identity to answer with"}.
				Hintf("list members with %s instead", v2model.RefListMembers(spaceId)))
	}
	row := v2model.MemberRow{
		Id:       domain.NewParticipantId(spaceId, s.accountId),
		Identity: s.accountId,
	}
	// the store returns empty details (no error) for an unindexed id — only
	// trust the row's name/role once the participant object actually exists
	if details, err := s.store.SpaceIndex(spaceId).GetDetails(row.Id); err == nil && details.GetString(bundle.RelationKeyId) == row.Id {
		row.Name = details.GetString(bundle.RelationKeyName)
		row.IconImage = details.GetString(bundle.RelationKeyIconImage)
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
func (s *Service) ListTypes(ctx context.Context, spaceId string, offset, limit int) ([]v2model.TypeRow, int, bool, error) {
	if err := s.ensureSpace(ctx, spaceId); err != nil {
		return nil, 0, false, err
	}
	// one read of the live types serves the rows and the spelling sets,
	// which count hidden types too, and it is paged here: a record without a
	// valid unique key serves no row, so it must not count toward total or
	// has_more (a space holds few types). Live filters: a UI-deleted
	// (uninstalled) type must not list — the §7.5-requirement-2 corpse
	// policy (keys.go)
	records, err := s.store.SpaceIndex(spaceId).Query(database.Query{
		Filters: liveTypeFilters(),
		Sorts: database.WithIdTieBreak([]database.SortRequest{{
			RelationKey: bundle.RelationKeyName,
			Type:        model.BlockContentDataviewSort_Asc,
		}}),
	})
	if err != nil {
		return nil, 0, false, fmt.Errorf("query types in space %s: %w", spaceId, err)
	}
	entries := typeEntriesOf(records)
	keyTaken, slugHolders := servedTypeKeySets(entries)
	rows := make([]v2model.TypeRow, 0, len(entries))
	for _, entry := range entries {
		if entry.Hidden {
			continue
		}
		rows = append(rows, v2model.TypeRow{
			Key:  servedTypeKeyOf(entry.Key, entry.Slug, keyTaken, slugHolders),
			Name: entry.Name,
		})
	}
	page, hasMore := pagination.Paginate(rows, offset, limit)
	return page, len(rows), hasMore, nil
}

// GetType returns the kind:"object_type" AnyBlock document for one type key,
// read via the live smartblock state like any object (§8). The query rides
// through to GetObject, so `?ids=full` works here exactly as on objects —
// the export shape must be one query parameter away on every document read
// (§8.25 promised it; hardcoding ObjectQuery{} broke it for types).
func (s *Service) GetType(ctx context.Context, spaceId, typeKey string, q ObjectQuery) ([]byte, string, error) {
	if err := s.ensureSpace(ctx, spaceId); err != nil {
		return nil, "", err
	}
	// the route parameter is the member the caller wrote
	entry, err := s.requireLiveType(spaceId, typeKey, "type", errKeysFor(ctx))
	if err != nil {
		return nil, "", err
	}
	return s.GetObject(ctx, spaceId, entry.Id, q)
}

// ListProperties returns minimal property rows: key, name, format.
func (s *Service) ListProperties(ctx context.Context, spaceId string, offset, limit int) ([]v2model.PropertyRow, int, bool, error) {
	if err := s.ensureSpace(ctx, spaceId); err != nil {
		return nil, 0, false, err
	}
	// one read of the live properties serves the rows and the spelling
	// sets, which count hidden properties too, and it is paged here: a
	// record without a relation key serves no row, so it must not count
	// toward total or has_more (a space holds few properties). Live filters:
	// a UI-deleted (uninstalled) property still carried the relation layout
	// and passed every filter here — the §2.3-6 defect; the corpse policy
	// excludes it (keys.go)
	records, err := s.store.SpaceIndex(spaceId).Query(database.Query{
		Filters: livePropertyFilters(),
		Sorts: database.WithIdTieBreak([]database.SortRequest{{
			RelationKey: bundle.RelationKeyName,
			Type:        model.BlockContentDataviewSort_Asc,
		}}),
	})
	if err != nil {
		return nil, 0, false, fmt.Errorf("query properties in space %s: %w", spaceId, err)
	}
	// the served spelling (§7.5a): a BSON-keyed property answers to its
	// slug, so the slug is what the row advertises — iff it round-trips
	// (servedKey); the count maps come from the full live set, not the page
	entries := propertyEntriesOf(records)
	keyTaken, slugHolders := servedPropertyKeySets(entries)
	rows := make([]v2model.PropertyRow, 0, len(entries))
	for _, entry := range entries {
		if entry.Hidden {
			continue
		}
		rows = append(rows, v2model.PropertyRow{
			Key:    servedKey(entry.Key, entry.Slug, keyTaken, slugHolders),
			Name:   entry.Name,
			Format: anyblockjson.FormatName(entry.Format),
		})
	}
	page, hasMore := pagination.Paginate(rows, offset, limit)
	return page, len(rows), hasMore, nil
}

// ListPropertyOptions returns the option names (+color) of one
// select/multiSelect property, with a prefix filter (C10 — tag-like
// properties can hold thousands of options).
func (s *Service) ListPropertyOptions(ctx context.Context, spaceId, propertyKey, prefix string, offset, limit int) ([]v2model.OptionRow, int, bool, error) {
	if err := s.ensureSpace(ctx, spaceId); err != nil {
		return nil, 0, false, err
	}
	// live lookup, slug-aware — a corpse property's options are not an API
	// surface; options bind to the STORED key, so list by entry.Key
	entry, err := s.requireLiveProperty(spaceId, propertyKey, errKeysFor(ctx))
	if err != nil {
		return nil, 0, false, err
	}
	options, err := s.store.SpaceIndex(spaceId).ListRelationOptions(domain.RelationKey(entry.Key))
	if err != nil {
		return nil, 0, false, fmt.Errorf("list options of property %s: %w", propertyKey, err)
	}

	options = slices.DeleteFunc(options, func(option *model.RelationOption) bool {
		return option == nil || (prefix != "" && !strings.HasPrefix(strings.ToLower(option.Text), strings.ToLower(prefix)))
	})
	// by name, ties by id: the store returns options in no defined order, and
	// a page is a slice of this order
	slices.SortFunc(options, func(a, b *model.RelationOption) int {
		return cmp.Or(strings.Compare(a.Text, b.Text), strings.Compare(a.Id, b.Id))
	})
	rows := make([]v2model.OptionRow, 0, len(options))
	for _, option := range options {
		rows = append(rows, v2model.OptionRow{Name: option.Text, Color: option.Color})
	}

	total := len(rows)
	page, hasMore := pagination.Paginate(rows, offset, limit)
	return page, total, hasMore, nil
}
