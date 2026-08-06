package v2service

// whoami.go implements GET /v2/auth/whoami (APIV2.md §8.11): the
// credential's self-description, so a holder — anytype-mcp above all — can
// shape its tool surface from what the key can actually do instead of
// discovering the boundary through 403s.

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/anytype-heart/core/api/util"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Whoami describes the authenticated CREDENTIAL, never the person.
//
// whoami is DISCOVERY, not enforcement — and its answer is DERIVED FROM THE
// SAME grant record the gate reads: util.ApiGrantFromCtx on the request
// context, the identical carrier and accessor apiv2.ensureSpaceGrant and
// this service's ensureSpaceGranted backstop consult. It must never be
// computed separately (a second wallet read, a re-parse of the key): a
// second derivation path is how the mirror and the gate drift apart and the
// mirror starts lying to the agents that plan against it. The anti-drift
// test in core/api/server pins that this answer and the gate's decisions
// cannot disagree for the same key.
func (s *V2Service) Whoami(ctx context.Context) (v2model.WhoamiResponse, error) {
	info, ok := util.ApiKeyInfoFromCtx(ctx)
	if !ok {
		// unreachable behind the shared auth middleware; fail closed rather
		// than describe a credential that was never authenticated
		return v2model.WhoamiResponse{}, fmt.Errorf("whoami without an authenticated session: the route must sit behind the shared auth middleware")
	}
	grant := util.ApiGrantFromCtx(ctx)

	resp := v2model.WhoamiResponse{
		Key: v2model.WhoamiKey{
			Id:        info.Id,
			Name:      info.Name,
			CreatedAt: unixToRfc3339(info.CreatedAt),
			ExpiresAt: unixToRfc3339(info.ExpiresAt),
		},
		Scope: scopeName(info.Scope),
		// scoped:false is the explicit legacy shape: spaces [] (NEVER null —
		// the null-vs-empty test fails open) and permission null.
		Grant:     v2model.WhoamiGrant{Scoped: false, Spaces: []v2model.WhoamiGrantSpace{}},
		Api:       v2model.WhoamiApi{Version: util.ApiVersion},
		KeyStatus: util.KeyStatus(grant),
	}
	if grant == nil {
		// The header signal repeated in the body: agents read bodies. Like
		// the header, the notice addresses only JSON-API keys — a grant is
		// only ever valid on JsonAPI scope (wallet.ValidateAppLinkGrant), so
		// a Full credential cannot follow the re-issue advice.
		if info.Scope == model.AccountAuth_JsonAPI {
			resp.Notice = util.LegacyKeyNotice
		}
		return resp, nil
	}

	names, err := s.resolveGrantedSpaceNames(ctx, grant)
	if err != nil {
		return v2model.WhoamiResponse{}, fmt.Errorf("resolve granted space names: %w", err)
	}

	perms := grant.Perms
	resp.Grant.Scoped = true
	resp.Grant.Permission = &perms
	for _, spaceId := range grant.Spaces {
		// per-entry permission, uniform today — the object shape is what
		// lets P2 introduce per-space permissions without a wire break
		resp.Grant.Spaces = append(resp.Grant.Spaces, v2model.WhoamiGrantSpace{
			Id:         spaceId,
			Name:       names[spaceId],
			Permission: perms,
		})
	}
	return resp, nil
}

// resolveGrantedSpaceNames resolves display names through the SAME
// grant-intersected ListSpaces path GET /v2/spaces serves: the ctx grant
// filters the enumeration at the input, so a non-granted space's name never
// enters the map. Deliberately NO second filter here — the map is exactly
// what ListSpaces yields, so its unit test fails the moment name resolution
// stops flowing through the intersected path (the outer loop over
// grant.Spaces in Whoami is the independent boundary for WHICH spaces are
// listed). The limit is the grant size: the intersected total can never
// exceed it.
func (s *V2Service) resolveGrantedSpaceNames(ctx context.Context, grant *util.ApiGrant) (map[string]string, error) {
	names := map[string]string{}
	if grant == nil || len(grant.Spaces) == 0 {
		return names, nil
	}
	rows, _, _, err := s.ListSpaces(ctx, 0, len(grant.Spaces))
	if err != nil {
		return nil, fmt.Errorf("list granted spaces: %w", err)
	}
	for _, row := range rows {
		names[row.Id] = row.Name
	}
	return names, nil
}

// scopeName renders the session scope in the whoami vocabulary (camelCase
// per C2). An unknown future enum member reports as its proto name rather
// than masquerading as one of the known kinds.
func scopeName(scope model.AccountAuthLocalApiScope) string {
	switch scope {
	case model.AccountAuth_JsonAPI:
		return "jsonApi"
	case model.AccountAuth_Full:
		return "full"
	case model.AccountAuth_Limited:
		return "limited"
	default:
		return scope.String()
	}
}

// unixToRfc3339 renders a unix timestamp as RFC 3339 UTC; 0 stays null
// (unknown for createdAt, never for expiresAt).
func unixToRfc3339(unix int64) *string {
	if unix == 0 {
		return nil
	}
	rendered := time.Unix(unix, 0).UTC().Format(time.RFC3339)
	return &rendered
}
