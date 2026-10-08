package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

// TestFullTableDerivesFromTheEmbeddedDocument is the build-time proof that
// the lazy derivation cannot fail in a shipped binary: the same embedded
// bytes, the same served schemas, derived here.
func TestFullTableDerivesFromTheEmbeddedDocument(t *testing.T) {
	table, err := FullTable()
	require.NoError(t, err)
	assert.Len(t, table.Tools, len(v2model.OperationIds())-len(table.Excluded))
	again, err := FullTable()
	require.NoError(t, err)
	assert.Same(t, table, again, "derived once per process")
}
