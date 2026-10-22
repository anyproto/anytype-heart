package apiv2

// keyshape.go parses `?keys=` once per request and records the answer on
// the request context, so every surface that serves an AnyBlock body spells
// property and type keys in the one vocabulary this caller asked for
// choice: the minted api slug by default, the format's
// raw display names on `?keys=name`.
//
// A middleware for the reasons idshape.go states: the parameter is
// group-wide, its legal values are a closed set, an unknown value is a 400
// on every route, and a body-serving surface added tomorrow honours it
// without being told to.

import (
	"github.com/gin-gonic/gin"

	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
)

// ensureKeysShape validates `?keys=` (slug | name) and records a name-mode
// request on the context. Absent or `slug` records nothing — the slug
// default is what every code path and every internal caller assumes.
func ensureKeysShape() gin.HandlerFunc {
	return func(c *gin.Context) {
		names, err := v2service.ParseKeysShape(c.Query("keys"))
		if err != nil {
			respondV2Error(c, err)
			return
		}
		if names {
			c.Request = c.Request.WithContext(v2service.CtxWithNameKeys(c.Request.Context()))
		}
		c.Next()
	}
}
