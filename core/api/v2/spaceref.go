package apiv2

// spaceref.go resolves the ONE route param under which /v2 addresses a
// space (SpaceParam) from a short reference to the full space id
// (APIV2.md §8.35). It is a middleware and not a per-handler call for two
// reasons, both structural:
//
//   - it must run BEFORE ensureSpaceGrant. The grant gate compares
//     c.Param(SpaceParam) against the key's granted space list, and grants
//     are keyed by full space id; a short reference reaching the gate
//     unresolved would be refused as a non-granted space. Resolving in the
//     services instead would be too late by one middleware.
//   - every space-addressing route uses the same param name (the
//     conformance walk refuses any other), so one middleware covers the
//     whole surface — including routes added later, which is the same
//     by-construction argument the grant gate itself rests on.
//
// The resolution can only ever land inside the caller's VISIBLE spaces
// (V2Service.ResolveSpaceRef → liveSpaceRows, live and grant-intersected),
// so a short reference is not a way to probe a space the key does not hold:
// a tail belonging to a non-granted space does not resolve, the param is
// left exactly as it arrived, and the request meets the same refusal any
// other unknown value meets.

import (
	"github.com/gin-gonic/gin"

	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
)

// resolveSpaceRef rewrites :space_id in place when the caller used a short
// reference, and records the caller's own spelling on the request context so
// refusals quote it back (v2handler.RespondV2Error).
//
// A full, space-shaped id costs NOTHING here: ResolveSpaceRef returns it
// untouched without reading the space list.
func resolveSpaceRef(svc *v2service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		ref := c.Param(SpaceParam)
		if ref == "" {
			c.Next()
			return
		}
		full, err := svc.ResolveSpaceRef(c.Request.Context(), ref)
		if err != nil {
			// the only error is the ambiguity refusal, which lists candidates
			respondV2Error(c, err)
			return
		}
		if full == ref {
			c.Next()
			return
		}
		setRouteParam(c, SpaceParam, full)
		c.Request = c.Request.WithContext(v2service.CtxWithSpaceEcho(c.Request.Context(), full, ref))
		c.Next()
	}
}

// setRouteParam replaces a gin route param's value in place. gin.Params is a
// slice of structs, so the assignment is to the live entry every later
// c.Param(name) reads — the handlers, the grant gate and the C8 idempotency
// key alike then all see the full id.
func setRouteParam(c *gin.Context, name, value string) {
	for i := range c.Params {
		if c.Params[i].Key == name {
			c.Params[i].Value = value
			return
		}
	}
}
