package v2model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorShape(t *testing.T) {
	t.Run("serializes the C6 envelope", func(t *testing.T) {
		// given
		err := AmbiguousInput("outline and block are mutually exclusive",
			Issue{Path: "outline", Message: "conflicts with block", Hint: "drop one"})
		want := `{"status":400,"code":"ambiguous_input","message":"outline and block are mutually exclusive","issues":[{"path":"outline","message":"conflicts with block","hint":"drop one"}]}`

		// when
		data, marshalErr := json.Marshal(err)

		// then
		require.NoError(t, marshalErr)
		assert.JSONEq(t, want, string(data))
	})

	t.Run("issues are omitted when empty", func(t *testing.T) {
		data, err := json.Marshal(NotFound("object gone"))
		require.NoError(t, err)
		assert.NotContains(t, string(data), "issues")
	})

	t.Run("etag mismatch carries the current etag", func(t *testing.T) {
		err := EtagMismatch("abcd1234")
		assert.Equal(t, 409, err.Status)
		assert.Equal(t, CodeEtagMismatch, err.Code)
		assert.Contains(t, err.Message, `"abcd1234"`)
	})

	t.Run("version unsupported names both versions verbatim", func(t *testing.T) {
		err := VersionUnsupported(2, 1)
		assert.Equal(t, 400, err.Status)
		assert.Equal(t, CodeVersionUnsupported, err.Code)
		assert.Contains(t, err.Message, "produced by a newer version")
		assert.Contains(t, err.Message, "document version 2")
		assert.Contains(t, err.Message, "supported version 1")
	})
}

func TestNewListResponse(t *testing.T) {
	t.Run("truncated lists carry a steering message", func(t *testing.T) {
		// given / when
		resp := NewListResponse([]TypeRow{{Key: "task"}}, 312, 0, 1, true, "narrow with prefix=")

		// then
		assert.True(t, resp.HasMore)
		assert.Contains(t, resp.Message, "312 matches")
		assert.Contains(t, resp.Message, "narrow with prefix=")
	})

	t.Run("complete lists have no message and data serializes as []", func(t *testing.T) {
		// given / when
		resp := NewListResponse[TypeRow](nil, 0, 0, 25, false, "unused")

		// then
		assert.Empty(t, resp.Message)
		data, err := json.Marshal(resp)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"data":[]`)
	})
}
