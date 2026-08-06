package v2service

// space.go implements the Phase-7 space surface (APIV2_SURFACES.md §2):
// GET /v2/spaces/{spaceId}, POST /v2/spaces, PATCH /v2/spaces/{spaceId}.
// The read is one tech-space store query — NOT v1's WorkspaceOpen +
// ObjectShow RPC pair per space (service/space.go getSpaceInfo), which is
// the N+1 shape the phase bans. The mutations are thin over WorkspaceCreate
// / WorkspaceSetInfo; C8 Idempotency-Key rides the route middleware — a
// retried space create without it duplicates an entire space, the worst
// possible duplicate. C9 dry runs validate the body only (a space create
// cannot be simulated).

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gogo/protobuf/types"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
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

// isLiveSpaceView reports whether a space view describes a live, usable
// space — v1's ListSpaces predicate (service/space.go): local status
// Unknown/Ok AND account status Unknown/SpaceActive. Deleted, left,
// removing and still-joining spaces fail it. A missing status field reads
// as Unknown (0) and passes. Shared by the v2 spaces list, GET-one and the
// global-search fan-out so the three surfaces agree on what a space IS.
func isLiveSpaceView(details *domain.Details) bool {
	local := model.SpaceStatus(details.GetInt64(bundle.RelationKeySpaceLocalStatus))
	if local != model.SpaceStatus_Unknown && local != model.SpaceStatus_Ok {
		return false
	}
	account := model.SpaceStatus(details.GetInt64(bundle.RelationKeySpaceAccountStatus))
	return account == model.SpaceStatus_Unknown || account == model.SpaceStatus_SpaceActive
}

// GetSpace implements GET /v2/spaces/{spaceId}: the space row read from the
// tech space's space view (the view mirrors the workspace object's name and
// description — editor/spaceview.go workspaceKeysToCopy), so the read costs
// one store query and zero RPCs. Only LIVE spaces are served
// (isLiveSpaceView) — a deleted or left space's row is indistinguishable
// from a live one and an agent picking it would then write into a space
// that can never load.
func (s *V2Service) GetSpace(ctx context.Context, spaceId string) (v2model.Space, error) {
	if spaceId == "" {
		return v2model.Space{}, v2model.NotFound("space id is required")
	}
	details, err := s.store.GetSpaceViewDetails(spaceId)
	if err != nil {
		return v2model.Space{}, v2model.NotFound(
			fmt.Sprintf("space %q not found — list spaces with GET /v2/spaces", spaceId))
	}
	if !isLiveSpaceView(details) {
		return v2model.Space{}, v2model.NotFound(
			fmt.Sprintf("space %q is not available (deleted, left, or still joining) — list live spaces with GET /v2/spaces", spaceId))
	}
	return v2model.Space{
		Id:          spaceId,
		Name:        details.GetString(bundle.RelationKeyName),
		Description: details.GetString(bundle.RelationKeyDescription),
	}, nil
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
func (s *V2Service) CreateSpace(ctx context.Context, req v2model.CreateSpaceRequest, dryRun bool) (*v2model.Space, error) {
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
	return &v2model.Space{Id: resp.SpaceId, Name: name, Description: description}, nil
}

// UpdateSpace implements PATCH /v2/spaces/{spaceId}: thin over
// WorkspaceSetInfo. Omitted fields stay unchanged; at least one field is
// required (the setProperties empty-op precedent — an accepted no-op PATCH
// would let an agent believe it renamed something). The response overlays
// the patch onto the current space-view row rather than re-reading it: the
// workspace-object write propagates to the tech-space view asynchronously,
// so an immediate read-back could return the pre-patch name.
func (s *V2Service) UpdateSpace(ctx context.Context, spaceId string, req v2model.UpdateSpaceRequest, dryRun bool) (*v2model.Space, error) {
	current, err := s.GetSpace(ctx, spaceId)
	if err != nil {
		return nil, err
	}
	if req.Name == nil && req.Description == nil {
		return nil, v2model.ValidationFailed("update needs at least one of name, description",
			v2model.Issue{Message: "omitted fields stay unchanged — an empty update would change nothing"})
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
// classified on the description — the Phase-6 chat precedent — instead of
// defaulting the whole class to a retry-looping 500. The matched strings
// are pinned by the middleware: space/service.go's ErrSpaceNotExists
// ("space not exists"), ErrSpaceDeleted ("space is deleted"),
// ErrSpaceStorageMissig ("space storage missing"), and the smartblock
// restriction sentinel restriction.ErrRestricted ("restricted") that a
// reader's SetDetails hits in a shared space.
func v2SpaceRpcError(op string, code, badInputCode int32, description string) error {
	if code == badInputCode {
		return v2model.ValidationFailed(fmt.Sprintf("%s: invalid input", op),
			v2model.Issue{Message: description})
	}
	switch {
	case strings.Contains(description, "space not exists"),
		strings.Contains(description, "space is deleted"),
		strings.Contains(description, "space storage missing"):
		return v2model.NotFound(fmt.Sprintf("%s: %s — list live spaces with GET /v2/spaces", op, description))
	case strings.Contains(description, "restricted"):
		return v2model.NewError(http.StatusForbidden, v2model.CodeForbidden,
			fmt.Sprintf("%s: %s — this account's role cannot change the space's info", op, description))
	}
	msg := op + " failed"
	if description != "" {
		msg += ": " + description
	}
	return v2model.NewError(http.StatusInternalServerError, v2model.CodeInternalError, msg)
}
