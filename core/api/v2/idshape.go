package apiv2

// idshape.go parses `?ids=` once per request and records the answer on the
// request context, so that every surface which SPELLS an id in a response
// gives the caller the one shape they asked for (APIV2.md §8.36).
//
// It is a middleware for the same reason ensureDryRun is one: the parameter
// is group-wide, its legal values are a closed set, and an unknown value has
// to be a 400 on every route rather than on the routes someone remembered to
// wire. That also means a space-serving surface added tomorrow honours
// `?ids=full` without being told to — the by-construction argument the grant
// gate and the reference resolver both rest on.
//
// It runs BEFORE resolveSpaceRef, so the candidate list an ambiguous
// reference is refused with is spelled the way this request asked for.

import (
	"github.com/gin-gonic/gin"

	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
)

// ensureIdsShape validates `?ids=` (C4: compact | full) and records a full
// request on the context. Absent or `compact` records nothing — the default
// serving shape is what every code path already assumes, and an untouched
// context is what every internal caller and every test carries.
//
// The value list and the refusal live in v2service.ParseIdsShape, which is
// also what the object read's own plan validation calls: the parameter has
// one definition, not one per layer.
func ensureIdsShape() gin.HandlerFunc {
	return func(c *gin.Context) {
		full, err := v2service.ParseIdsShape(c.Query("ids"))
		if err != nil {
			respondV2Error(c, err)
			return
		}
		if full {
			c.Request = c.Request.WithContext(v2service.CtxWithFullIds(c.Request.Context()))
		}
		c.Next()
	}
}
