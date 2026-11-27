package core

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

func TestDefaultDeletionAuditProjectsTheAppWithTheAuthor(t *testing.T) {
	// red if the default projection returns lastModifiedBy without the app
	// the same change was made through
	assert.True(t, slices.Contains(defaultDeletionAuditKeys, bundle.RelationKeyLastModifiedBy))
	assert.True(t, slices.Contains(defaultDeletionAuditKeys, bundle.RelationKeyLastModifiedVia))
}
