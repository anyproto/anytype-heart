package spaceindex

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

// A deleted object's tombstone keeps who changed it last; the app that
// change was made through belongs to the same record.
func TestSnapshotOnDeleteKeepsTheAppWithTheAuthor(t *testing.T) {
	// red if lastModifiedVia is dropped from the snapshot while
	// lastModifiedBy stays: the deletion audit would name an author and
	// lose how the change was made
	assert.True(t, slices.Contains(SnapshotOnDelete, bundle.RelationKeyLastModifiedBy))
	assert.True(t, slices.Contains(SnapshotOnDelete, bundle.RelationKeyLastModifiedVia))
}
