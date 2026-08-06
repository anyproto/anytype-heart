package service

// v2_space.go implements the Phase-7 space surface (APIV2_SURFACES.md §2):
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

	"github.com/gogo/protobuf/types"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// spaceIconOptions is the icon-option range a create picks from at random
// (v1 parity: service/space.go CreateSpace).
const spaceIconOptions = 13

// GetSpace implements GET /v2/spaces/{spaceId}: the space row read from the
// tech space's space view (the view mirrors the workspace object's name and
// description — editor/spaceview.go workspaceKeysToCopy), so the read costs
// one store query and zero RPCs.
func (s *V2Service) GetSpace(ctx context.Context, spaceId string) (apimodel.V2Space, error) {
	if spaceId == "" {
		return apimodel.V2Space{}, apimodel.V2NotFound("space id is required")
	}
	details, err := s.store.GetSpaceViewDetails(spaceId)
	if err != nil {
		return apimodel.V2Space{}, apimodel.V2NotFound(
			fmt.Sprintf("space %q not found — list spaces with GET /v2/spaces", spaceId))
	}
	return apimodel.V2Space{
		Id:          spaceId,
		Name:        details.GetString(bundle.RelationKeyName),
		Description: details.GetString(bundle.RelationKeyDescription),
	}, nil
}

// CreateSpace implements POST /v2/spaces: thin over WorkspaceCreate (v1
// parity: CHAT_SPACE use case, random icon option, regular space type) —
// except that the description rides the SAME WorkspaceCreate call
// (CreateWorkspace applies every detail to the workspace object), where v1
// spent a second WorkspaceSetInfo RPC on it.
func (s *V2Service) CreateSpace(ctx context.Context, req apimodel.V2CreateSpaceRequest, dryRun bool) (*apimodel.V2Space, error) {
	name := strings.TrimSpace(req.Name)
	description := strings.TrimSpace(req.Description)
	if name == "" {
		return nil, apimodel.V2ValidationFailed("space name is required",
			apimodel.V2Issue{Path: "/name", Message: "the space row is {id, name} — an unnamed space is unaddressable by name"})
	}
	if dryRun {
		// C9, scoped honestly: a space create cannot be simulated — the dry
		// run validates the body only
		return &apimodel.V2Space{Name: name, Description: description, DryRun: true}, nil
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
	return &apimodel.V2Space{Id: resp.SpaceId, Name: name, Description: description}, nil
}

// UpdateSpace implements PATCH /v2/spaces/{spaceId}: thin over
// WorkspaceSetInfo. Omitted fields stay unchanged; at least one field is
// required (the setProperties empty-op precedent — an accepted no-op PATCH
// would let an agent believe it renamed something). The response overlays
// the patch onto the current space-view row rather than re-reading it: the
// workspace-object write propagates to the tech-space view asynchronously,
// so an immediate read-back could return the pre-patch name.
func (s *V2Service) UpdateSpace(ctx context.Context, spaceId string, req apimodel.V2UpdateSpaceRequest, dryRun bool) (*apimodel.V2Space, error) {
	current, err := s.GetSpace(ctx, spaceId)
	if err != nil {
		return nil, err
	}
	if req.Name == nil && req.Description == nil {
		return nil, apimodel.V2ValidationFailed("update needs at least one of name, description",
			apimodel.V2Issue{Message: "omitted fields stay unchanged — an empty update would change nothing"})
	}
	if req.Name != nil && strings.TrimSpace(*req.Name) == "" {
		return nil, apimodel.V2ValidationFailed("space name cannot be empty",
			apimodel.V2Issue{Path: "/name", Message: "omit name to keep the current one; the space row is {id, name}"})
	}

	next := current
	fields := map[string]*types.Value{}
	if req.Name != nil {
		next.Name = strings.TrimSpace(*req.Name)
		fields[bundle.RelationKeyName.String()] = pbtypes.String(next.Name)
	}
	if req.Description != nil {
		next.Description = strings.TrimSpace(*req.Description)
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
// BAD_INPUT → 400 validation_failed carrying the description; anything else
// stays 500 with the description carried (the chat precedent, minus the
// string classification — the workspace RPCs have no known description
// vocabulary worth pinning).
func v2SpaceRpcError(op string, code, badInputCode int32, description string) error {
	if code == badInputCode {
		return apimodel.V2ValidationFailed(fmt.Sprintf("%s: invalid input", op),
			apimodel.V2Issue{Message: description})
	}
	msg := op + " failed"
	if description != "" {
		msg += ": " + description
	}
	return apimodel.NewV2Error(http.StatusInternalServerError, apimodel.V2CodeInternalError, msg)
}
