package v2service

// space.go implements GET /v2/spaces/{space_id}, POST /v2/spaces, and
// PATCH /v2/spaces/{space_id}.
// The read is one tech-space store query — NOT v1's WorkspaceOpen +
// ObjectShow RPC pair per space (service/space.go getSpaceInfo), which is
// the N+1 shape this route avoids. The mutations are thin over WorkspaceCreate
// / WorkspaceSetInfo; C8 Idempotency-Key rides the route middleware — a
// retried space create without it duplicates an entire space, the worst
// possible duplicate. C9 dry runs validate the body only (a space create
// cannot be simulated).

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/api/util"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// spaceIconOptions is the icon-option range a create picks from at random
// (v1 parity: service/space.go CreateSpace).
const spaceIconOptions = 13

// maxSpaceFieldLength is the cap on a space's name and description — the
// SAME bound the `space` discovery kind advertises (schemas.go): the
// schema must not out-promise the endpoint (the chat text-length precedent).
// Counted in Unicode code points, matching JSON Schema maxLength.
const maxSpaceFieldLength = 4096

// spaceViewLiveness classifies a space view: whether the space it describes
// is usable through this API and, when it is not, why.
type spaceViewLiveness uint8

const (
	// spaceViewLive: the space is served. Local status Unknown, Loading or Ok
	// and account status Unknown or SpaceActive; a missing status field reads
	// as Unknown (0).
	spaceViewLive spaceViewLiveness = iota + 1
	// spaceViewUnavailable: the account still holds the space, but it is not
	// usable on this device — its load failed, its deletion is pending on the
	// network, or the account is still joining it. It can come back.
	spaceViewUnavailable
	// spaceViewGone: the account no longer holds the space — deleted, left,
	// removed from it, a declined join, or deleted on the network.
	spaceViewGone
)

// spaceViewState is the ONE liveness definition of API v2
// (APIV2_SPACE_STREAM.md §3.2–3.6). Every surface reads it, through
// isLiveSpaceView or visibleSpaces — the spaces list and the spaces stream,
// GET-one, ensureSpace and with it every per-space route, short-reference
// resolution and spelling, whoami's names and the global-search fan-out — so
// discovery and every nested route agree on what a space IS.
//
// A LOADING space is live. The loader publishes Loading for a space that was
// not on disk and Ok last session: every space on a new device's first
// start, a space created or joined during its first load, a failed load
// retried at the next start. Refusing it made a new space flap in and out of
// the visible set while it loaded. What a request into a loading space costs
// is bounded by ensureSpaceLoaded; /v1 keeps its own filter and still
// excludes it.
//
// The other two states only name why a view is not live; the spaces stream
// reports them as a removal's reason. Account Deleted or Removing decides
// first: SpaceDeleted is written the same way for a delete, a leave, a
// removal from the ACL and a declined join, so no finer reason exists.
func spaceViewState(details *domain.Details) spaceViewLiveness {
	local := model.SpaceStatus(details.GetInt64(bundle.RelationKeySpaceLocalStatus))
	account := model.SpaceStatus(details.GetInt64(bundle.RelationKeySpaceAccountStatus))
	remote := model.SpaceStatus(details.GetInt64(bundle.RelationKeySpaceRemoteStatus))
	switch {
	case account == model.SpaceStatus_SpaceDeleted, account == model.SpaceStatus_SpaceRemoving:
		return spaceViewGone
	case local == model.SpaceStatus_Missing && remote == model.SpaceStatus_RemoteDeleted:
		// the loader writes both in one change when it finds the space
		// deleted on the network
		return spaceViewGone
	}
	localLive := local == model.SpaceStatus_Unknown || local == model.SpaceStatus_Loading || local == model.SpaceStatus_Ok
	accountLive := account == model.SpaceStatus_Unknown || account == model.SpaceStatus_SpaceActive
	if localLive && accountLive {
		return spaceViewLive
	}
	return spaceViewUnavailable
}

// spaceViewOf returns the space view one space is JUDGED by — on GET-one and
// in ensureSpace, and so on every per-space route. Of the views whose target
// is the space it is the one visibleSpaces names the row by: the first live
// one by view id. With no live view it is the first by view id, whose state
// says why the space is not served.
//
// A view id is derived from its space, so a second view of one target exists
// only in damaged data. But the list and the spaces stream skip a dead view
// when a live one follows it, and a lookup that judged the space by whichever
// record the store returned first answered 404 for a space the list had just
// served. A space with no view is an error.
func (s *Service) spaceViewOf(spaceId string) (*domain.Details, error) {
	records, err := s.store.SpaceIndex(s.techSpaceId).Query(database.Query{
		Filters: append(spaceViewFilters(), database.FilterRequest{
			RelationKey: bundle.RelationKeyTargetSpaceId,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.String(spaceId),
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("query the space views of %s: %w", spaceId, err)
	}
	views := make([]*domain.Details, len(records))
	for i, record := range records {
		views[i] = record.Details
	}
	view := judgedSpaceView(views)
	if view == nil {
		return nil, fmt.Errorf("no space view of %s", spaceId)
	}
	return view, nil
}

// judgedSpaceView picks, among the views of ONE target, the view the space
// is judged by: the first live one by view id, else the first by view id;
// nil for no views.
func judgedSpaceView(views []*domain.Details) *domain.Details {
	var judged *domain.Details
	for _, view := range views {
		switch {
		case judged == nil:
			judged = view
		case isLiveSpaceView(view) != isLiveSpaceView(judged):
			if isLiveSpaceView(view) {
				judged = view
			}
		case view.GetString(bundle.RelationKeyId) < judged.GetString(bundle.RelationKeyId):
			judged = view
		}
	}
	return judged
}

// isLiveSpaceView reports whether a space view describes a live, usable
// space (spaceViewState). Deleted, left, removing and still-joining spaces
// fail it, and so does one whose load failed; a loading one passes.
func isLiveSpaceView(details *domain.Details) bool {
	return spaceViewState(details) == spaceViewLive
}

// LoadingSpaceWarnings is the warning a STORE read of one space carries
// while the space's view says it is still loading on this device: the read
// answers from what is indexed so far — empty or partial — and never waits,
// so it has to say so. Empty for a loaded space, the tech space and a space
// with no view. The space is named in the spelling this request is served.
func (s *Service) LoadingSpaceWarnings(ctx context.Context, spaceId string) []v2model.Issue {
	if spaceId == "" || spaceId == s.techSpaceId {
		return nil
	}
	details, err := s.spaceViewOf(spaceId)
	if err != nil || !isLoadingSpaceView(details) {
		return nil
	}
	return []v2model.Issue{spaceLoadingWarning(s.servedSpaceRef(ctx, spaceId))}
}

// isLoadingSpaceView reports whether a live view's space is still loading.
func isLoadingSpaceView(details *domain.Details) bool {
	return isLiveSpaceView(details) &&
		model.SpaceStatus(details.GetInt64(bundle.RelationKeySpaceLocalStatus)) == model.SpaceStatus_Loading
}

// spaceLoadingWarning is the one wording of the store-read warning; spelling
// is the space as the request is served it.
func spaceLoadingWarning(spelling string) v2model.Issue {
	return v2model.Issue{
		Path:    "space_id",
		Message: fmt.Sprintf("space %q is still loading on this device; results can be incomplete", spelling),
		Hint:    "repeat the request once the space has loaded for the complete result",
	}
}

func spaceUnavailableError(spaceId string) error {
	return v2model.NotFound(
		fmt.Sprintf("space %q is not available (deleted, left, or still joining)", spaceId),
		v2model.Issue{Path: "space_id", Message: "the space is not live on this account"}.
			Hintf("list live spaces with %s", v2model.RefListSpaces()))
}

// spaceNotFoundError is the 404 for a space id this account does not hold.
// No candidate list: ids are opaque (no did-you-mean can help) and a
// per-caller grant means the full space list must not be implied — the
// steer to the discovery operation is the whole repair.
func spaceNotFoundError(spaceId string) error {
	return v2model.NotFound(
		fmt.Sprintf("space %q not found", spaceId),
		v2model.Issue{Path: "space_id", Message: "no space with this id is open on this account"}.
			Hintf("list spaces with %s", v2model.RefListSpaces()))
}

// GetSpace implements GET /v2/spaces/{space_id}: the space row read from the
// tech space's space view (the view mirrors the workspace object's name and
// description — editor/spaceview.go workspaceKeysToCopy), so the read costs
// one store query and zero RPCs. Only LIVE spaces are served
// (isLiveSpaceView) — a deleted or left space's row is indistinguishable
// from a live one and an agent picking it would then write into a space
// that can never load. A space that is still loading is live and served.
func (s *Service) GetSpace(ctx context.Context, spaceId string) (v2model.Space, error) {
	if spaceId == "" {
		return v2model.Space{}, v2model.NotFound("space id is required")
	}
	// GetSpace deliberately bypasses ensureSpace (the read IS the space-view
	// lookup), so the grant backstop is consulted directly here.
	if err := s.ensureSpaceGranted(ctx, spaceId); err != nil {
		return v2model.Space{}, err
	}
	details, err := s.spaceViewOf(spaceId)
	if err != nil {
		return v2model.Space{}, spaceNotFoundError(spaceId)
	}
	if !isLiveSpaceView(details) {
		return v2model.Space{}, spaceUnavailableError(spaceId)
	}
	return v2model.Space{
		// served in the §8.35 short form, exactly as GET /v2/spaces prints
		// it — one space has one served spelling whichever route produced it
		Id:          s.servedSpaceRef(ctx, spaceId),
		Name:        details.GetString(bundle.RelationKeyName),
		Description: details.GetString(bundle.RelationKeyDescription),
		IconImage:   details.GetString(bundle.RelationKeyIconImage),
	}, nil
}

// servedSpaceRef renders one space id in the form this caller is served
// (§8.35): the short reference when its tail is unique across the caller's
// visible spaces, the full id when it collides or when the set cannot be
// read. Never fails — a serving decision must not be able to fail a read.
//
// `?ids=full` (§8.36) short-circuits it: the full id is what the caller
// asked for, and returning it here also spares the census its store query.
func (s *Service) servedSpaceRef(ctx context.Context, spaceId string, extra ...string) string {
	if fullIdsRequested(ctx) {
		return spaceId
	}
	rows, err := s.liveSpaceRows(ctx)
	if err != nil {
		return spaceId
	}
	ids := make([]string, 0, len(rows)+len(extra)+1)
	for _, row := range rows {
		ids = append(ids, row.Id)
	}
	// a just-created space is not in the tech-space index yet; it must still
	// enter the census, or its short form could collide with a space the
	// census did see
	for _, id := range append(extra, spaceId) {
		if id != "" && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	return shortSpaceRef(ids, spaceId)
}

// validateSpaceField enforces the advertised maxLength on one space field —
// without it a strict-mode agent is told a 4096 bound the endpoint never
// checks, and a 200 KB name would propagate to the workspace object, the
// mirrored space view and every member's device.
func validateSpaceField(path, value string) error {
	if length := utf8.RuneCountInString(value); length > maxSpaceFieldLength {
		return v2model.ValidationFailed(strings.TrimPrefix(path, "/")+" is too long",
			v2model.Issue{Path: path,
				Message: fmt.Sprintf("%d characters — the cap is %d (the space kind's advertised maxLength)", length, maxSpaceFieldLength)})
	}
	return nil
}

// CreateSpace implements POST /v2/spaces: thin over WorkspaceCreate (v1
// parity: CHAT_SPACE use case, random icon option, regular space type) —
// except that the description rides the SAME WorkspaceCreate call
// (CreateWorkspace applies every detail to the workspace object), where v1
// spent a second WorkspaceSetInfo RPC on it.
func (s *Service) CreateSpace(ctx context.Context, req v2model.CreateSpaceRequest, dryRun bool) (*v2model.Space, error) {
	// The route gate already refuses a narrowed key on POST /v2/spaces
	// (GlobalScopedDenied); this is the service-level backstop, because
	// this method calls no ensureSpace that could catch it.
	//
	// An unrestricted grant (allSpaces with readwrite) passes: allSpaces is
	// dynamic, so a space this key mints is already covered and nothing is
	// escaped — and /v1 honors the same grant on the same route. A NARROWED
	// key is refused: granted spaces A and B, it would mint and then own C.
	if grant := util.ApiGrantFromCtx(ctx); grant != nil && !grant.IsUnrestricted() {
		return nil, v2model.SpaceNotGranted(fmt.Sprintf(
			"a key granted only some spaces cannot create spaces; granted: %s", grant.Describe()))
	}
	name := strings.TrimSpace(req.Name)
	description := strings.TrimSpace(req.Description)
	if name == "" {
		return nil, v2model.ValidationFailed("space name is required",
			v2model.Issue{Path: "/name", Message: "the space row is {id, name} — an unnamed space is unaddressable by name"})
	}
	if err := validateSpaceField("/name", name); err != nil {
		return nil, err
	}
	if err := validateSpaceField("/description", description); err != nil {
		return nil, err
	}
	if dryRun {
		// C9, scoped honestly: a space create cannot be simulated — the dry
		// run validates the body only
		return &v2model.Space{Name: name, Description: description, DryRun: true}, nil
	}

	iconOption, err := rand.Int(rand.Reader, big.NewInt(spaceIconOptions))
	if err != nil {
		return nil, fmt.Errorf("pick a random icon option: %w", err)
	}
	fields := map[string]*types.Value{
		bundle.RelationKeyName.String():       pbtypes.String(name),
		bundle.RelationKeyIconOption.String(): pbtypes.Float64(float64(iconOption.Int64())),
		bundle.RelationKeyHomepage.String():   pbtypes.String(domain.HomepageWidgets),
		bundle.RelationKeySpaceType.String():  pbtypes.Float64(float64(model.SpaceType_SpaceTypeRegular)),
	}
	if description != "" {
		fields[bundle.RelationKeyDescription.String()] = pbtypes.String(description)
	}
	resp := s.mw.WorkspaceCreate(ctx, &pb.RpcWorkspaceCreateRequest{
		Details: &types.Struct{Fields: fields},
		UseCase: pb.RpcObjectImportUseCaseRequest_CHAT_SPACE,
	})
	if resp.Error != nil && resp.Error.Code != pb.RpcWorkspaceCreateResponseError_NULL {
		return nil, v2SpaceRpcError("create space",
			int32(resp.Error.Code), int32(pb.RpcWorkspaceCreateResponseError_BAD_INPUT), resp.Error.Description)
	}
	if resp.SpaceId == "" {
		// C8 states "the response always returns created ids" — a 201 with no
		// id is a success the agent cannot act on, and a keyed retry would
		// replay it forever. A 500 is NOT cached by the idempotency
		// middleware, so the retry re-executes.
		return nil, v2model.NewError(http.StatusInternalServerError, v2model.CodeInternalError,
			"create space: the workspace RPC returned no space id")
	}
	return &v2model.Space{Id: s.servedSpaceRef(ctx, resp.SpaceId), Name: name, Description: description}, nil
}

// UpdateSpace implements PATCH /v2/spaces/{space_id}: thin over
// WorkspaceSetInfo. Omitted fields stay unchanged; at least one field is
// required (the set_properties empty-op precedent — an accepted no-op PATCH
// would let an agent believe it renamed something). The response overlays
// the patch onto the current space-view row rather than re-reading it: the
// workspace-object write propagates to the tech-space view asynchronously,
// so an immediate read-back could return the pre-patch name.
func (s *Service) UpdateSpace(ctx context.Context, spaceId string, req v2model.UpdateSpaceRequest, dryRun bool) (*v2model.Space, error) {
	// UpdateSpace bypasses ensureSpace (GetSpace's read IS the space-view
	// lookup), so both backstop halves are consulted directly, in the route
	// gate's precedence: space grant first, then the write verb.
	if err := s.ensureSpaceGranted(ctx, spaceId); err != nil {
		return nil, err
	}
	if err := ensureWriteGranted(ctx); err != nil {
		return nil, err
	}
	current, err := s.GetSpace(ctx, spaceId)
	if err != nil {
		return nil, err
	}
	if req.Name == nil && req.Description == nil {
		return nil, v2model.ValidationFailed("update needs at least one of name, description",
			v2model.Issue{Message: "omitted fields stay unchanged — an empty update would change nothing; the space icon and the default object type are not writable through this API"})
	}
	if req.Name != nil && strings.TrimSpace(*req.Name) == "" {
		return nil, v2model.ValidationFailed("space name cannot be empty",
			v2model.Issue{Path: "/name", Message: "omit name to keep the current one; the space row is {id, name}"})
	}

	next := current
	fields := map[string]*types.Value{}
	if req.Name != nil {
		next.Name = strings.TrimSpace(*req.Name)
		if err := validateSpaceField("/name", next.Name); err != nil {
			return nil, err
		}
		fields[bundle.RelationKeyName.String()] = pbtypes.String(next.Name)
	}
	if req.Description != nil {
		next.Description = strings.TrimSpace(*req.Description)
		if err := validateSpaceField("/description", next.Description); err != nil {
			return nil, err
		}
		fields[bundle.RelationKeyDescription.String()] = pbtypes.String(next.Description)
	}
	if dryRun {
		next.DryRun = true
		return &next, nil
	}

	// WorkspaceSetInfo ignores its context and waits for a loading space
	// until the load ends or the app stops; the bounded wait comes first
	if err := s.ensureSpaceLoaded(ctx, spaceId); err != nil {
		return nil, err
	}
	resp := s.mw.WorkspaceSetInfo(ctx, &pb.RpcWorkspaceSetInfoRequest{
		SpaceId: spaceId,
		Details: &types.Struct{Fields: fields},
	})
	if resp.Error != nil && resp.Error.Code != pb.RpcWorkspaceSetInfoResponseError_NULL {
		return nil, v2SpaceRpcError("update space",
			int32(resp.Error.Code), int32(pb.RpcWorkspaceSetInfoResponseError_BAD_INPUT), resp.Error.Description)
	}
	return &next, nil
}

// v2SpaceRpcError classifies a workspace RPC failure into the C6 shape:
// BAD_INPUT → 400 validation_failed carrying the description. The workspace
// RPCs otherwise answer UNKNOWN_ERROR for everything (core mapErrorCode has
// no workspace errToCode mappings), so ordinary reachable failures are
// classified on the description, like chat failures, instead of
// defaulting the whole class to a retry-looping 500. The matched strings
// are taken FROM the producers' exported sentinels (space/service.go's
// ErrSpaceNotExists/ErrSpaceDeleted/ErrSpaceStorageMissig, and the
// smartblock restriction sentinel restriction.ErrRestricted that a reader's
// SetDetails hits in a shared space), so a producer rewording updates this
// matcher at compile time instead of silently un-classifying it back to a
// 500 (surface review M2d).
func v2SpaceRpcError(op string, code, badInputCode int32, description string) error {
	if code == badInputCode {
		return v2model.ValidationFailed(fmt.Sprintf("%s: invalid input", op),
			v2model.Issue{Message: description})
	}
	switch {
	case strings.Contains(description, space.ErrSpaceNotExists.Error()),
		strings.Contains(description, space.ErrSpaceDeleted.Error()),
		strings.Contains(description, space.ErrSpaceStorageMissig.Error()):
		// no path: this conversion also serves space creation, which has no
		// space_id parameter, and a missing store does not establish account
		// status — the description already says what failed
		return v2model.NotFound(fmt.Sprintf("%s: %s", op, description),
			v2model.Issue{Message: "the space could not be opened"}.
				Hintf("list live spaces with %s", v2model.RefListSpaces()))
	case strings.Contains(description, restriction.ErrRestricted.Error()):
		return v2model.NewError(http.StatusForbidden, v2model.CodeForbidden,
			fmt.Sprintf("%s: %s — this account's role cannot change the space's info", op, description))
	}
	msg := op + " failed"
	if description != "" {
		msg += ": " + description
	}
	return v2model.NewError(http.StatusInternalServerError, v2model.CodeInternalError, msg)
}

// dryRunLoadingNote marks a refusal a DRY RUN concluded from the index of a
// space that is still loading. A dry run does not wait for the load (it
// sends nothing), so its "no such property", or "that key is taken", is read
// from an index that may be partial: the refusal carries the store-read
// warning, so that it is not taken as final. A real write never gets here —
// it waited for the load before it read the index — and a refusal of the
// request itself is not passed through this.
//
// For a loaded space, a real write, or an error that is not a v2 refusal,
// err is returned as it is.
func (s *Service) dryRunLoadingNote(ctx context.Context, spaceId string, dryRun bool, err error) error {
	if !dryRun || err == nil {
		return err
	}
	var refusal *v2model.Error
	if !errors.As(err, &refusal) {
		return err
	}
	warnings := s.LoadingSpaceWarnings(ctx, spaceId)
	if len(warnings) == 0 {
		return err
	}
	noted := *refusal
	noted.Issues = append(append([]v2model.Issue{}, refusal.Issues...), warnings...)
	return &noted
}
