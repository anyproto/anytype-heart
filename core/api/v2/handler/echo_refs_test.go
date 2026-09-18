package v2handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
)

// TestEchoSpaceRefKeepsHintAndReferencesAligned pins the typed-hint contract
// across the echo: the prose is re-spelled to the caller's short space
// reference, so the references it carries must be re-spelled the same way —
// a wrapper finds the reference in the hint by rendering it, and a full id
// in the reference would render a route the hint no longer contains.
func TestEchoSpaceRefKeepsHintAndReferencesAligned(t *testing.T) {
	const full, short = "bafyreifullspaceid.28y6mgnwgodt7", "hxwz2i"
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodPost, "/v2/spaces/"+short+"/objects", nil)
	c.Request = req.WithContext(v2service.CtxWithSpaceEcho(context.Background(), full, short))

	RespondError(c, v2model.ValidationFailed("type is required",
		v2model.Issue{Path: "/type", Message: "the shortcut needs a type key"}.
			Hintf("list keys with %s", v2model.RefListTypes(full))))

	var body v2model.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Issues, 1)
	issue := body.Issues[0]
	assert.Equal(t, "list keys with GET /v2/spaces/"+short+"/types", issue.Hint)
	require.Len(t, issue.SeeAlso, 1)
	assert.Equal(t, v2model.RefListTypes(short), issue.SeeAlso[0])
	assert.Contains(t, issue.Hint, issue.SeeAlso[0].String(),
		"the reference renders to a string the hint contains — the find-and-replace contract")

	t.Run("a value that merely contains the space id is an identity and is left alone, in the hint too", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v2/spaces/"+short+"/objects", nil).
			WithContext(v2service.CtxWithSpaceEcho(context.Background(), full, short))
		key := "Audit " + full

		RespondError(c, v2model.ValidationFailed("option does not exist",
			v2model.Issue{Path: "/properties/x", Message: "no such option in " + full}.
				Hintf("check the spelling against %s in space %s", v2model.RefListPropertyOptions(full, key), full)))

		var body v2model.Error
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		issue := body.Issues[0]
		assert.Equal(t, v2model.RefListPropertyOptions(short, key), issue.SeeAlso[0], "space id echoed, the key untouched")
		assert.Equal(t, "check the spelling against GET /v2/spaces/"+short+"/properties/"+key+"/options in space "+short, issue.Hint,
			"the rendering follows the reference; the prose around it is echoed")
		assert.Equal(t, "no such option in "+short, issue.Message)
		assert.Contains(t, issue.Hint, issue.SeeAlso[0].String())
	})
}
