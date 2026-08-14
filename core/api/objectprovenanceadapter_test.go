package api

// Fixture-tree tests for the §10 enforcement read (APIV2_OBJECT_DELETE.md
// §15). Each fixture is a tree whose ROOT IDENTITY and FIRST CONTENT CHANGE
// are independently controlled, so every clause can fail separately: a
// fixture whose root doesn't match cannot pass by accident, and a recorded
// slug is only served when BOTH clauses hold. The first change's bytes go
// through the real MarshalChange → UnmarshalTreeChange convert path, so a
// wire-level regression (the field dropped on read) fails here too.

import (
	"errors"
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	anycrypto "github.com/anyproto/any-sync/util/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/source/sourceimpl"
	"github.com/anyproto/anytype-heart/pb"
)

// fakeProvenanceTree serves changes the way objecttree.IterateFrom does:
// the root is handed over un-converted (skipped by id), every later change
// with raw Data runs through the convert func and lands in Model.
type fakeProvenanceTree struct {
	id      string
	header  *objecttree.Change
	changes []*objecttree.Change
	iterErr error
}

func (f *fakeProvenanceTree) Id() string                             { return f.id }
func (f *fakeProvenanceTree) UnmarshalledHeader() *objecttree.Change { return f.header }

func (f *fakeProvenanceTree) IterateRoot(convert objecttree.ChangeConvertFunc, iterate objecttree.ChangeIterateFunc) error {
	if f.iterErr != nil {
		return f.iterErr
	}
	for _, c := range f.changes {
		if c.Model == nil && c.Id != f.id && convert != nil {
			model, err := convert(c, c.Data)
			if err != nil {
				return err
			}
			c.Model = model
		}
		if !iterate(c) {
			break
		}
	}
	return nil
}

// contentChange builds a marshaled, snapshot-less pb.Change carrying the
// given integration key, signed-by proxy via the identity on the tree change.
func contentChange(t *testing.T, id string, identity anycrypto.PubKey, integrationKey string) *objecttree.Change {
	c := &pb.Change{
		Content: []*pb.ChangeContent{{Value: &pb.ChangeContentValueOfBlockRemove{
			BlockRemove: &pb.ChangeBlockRemove{Ids: []string{"b1"}},
		}}},
		IntegrationKey: integrationKey,
	}
	data, dataType, err := sourceimpl.MarshalChange(c)
	require.NoError(t, err)
	return &objecttree.Change{Id: id, Identity: identity, Data: data, DataType: dataType}
}

func TestCreatorProvenanceFromTree(t *testing.T) {
	_, ownKey, err := anycrypto.GenerateRandomEd25519KeyPair()
	require.NoError(t, err)
	_, otherKey, err := anycrypto.GenerateRandomEd25519KeyPair()
	require.NoError(t, err)
	ownAccount := ownKey.Account()

	tree := func(rootIdentity anycrypto.PubKey, changes ...*objecttree.Change) *fakeProvenanceTree {
		root := &objecttree.Change{Id: "root", Identity: rootIdentity}
		return &fakeProvenanceTree{
			id:      "root",
			header:  root,
			changes: append([]*objecttree.Change{root}, changes...),
		}
	}

	t.Run("own root and stamped first change → the recorded slug", func(t *testing.T) {
		// the ALLOW fixture — the only shape that may serve a slug. It can
		// fail three independent ways: root clause broken, first-change pick
		// broken, or the wire field dropped in the convert path.
		match, key, err := creatorProvenanceFromTree(
			tree(ownKey, contentChange(t, "c1", ownKey, "claude-desktop")), ownAccount)
		require.NoError(t, err)
		assert.True(t, match)
		assert.Equal(t, "claude-desktop", key)
	})

	t.Run("own root, unstamped first change (legacy shape) → no slug", func(t *testing.T) {
		// the fixture HAS a full history with a first change — so a refusal
		// for key=="" is distinguishable from 'the check is broken': the same
		// fixture with a stamp (above) serves it
		match, key, err := creatorProvenanceFromTree(
			tree(ownKey, contentChange(t, "c1", ownKey, "")), ownAccount)
		require.NoError(t, err)
		assert.True(t, match)
		assert.Equal(t, "", key)
	})

	t.Run("a slug recorded under ANOTHER slug is served verbatim", func(t *testing.T) {
		// the service compares; the read must not — a read that 'helpfully'
		// blanked foreign slugs would break the §9.5 message naming them
		match, key, err := creatorProvenanceFromTree(
			tree(ownKey, contentChange(t, "c1", ownKey, "linear")), ownAccount)
		require.NoError(t, err)
		assert.True(t, match)
		assert.Equal(t, "linear", key)
	})

	t.Run("derived root (no identity) → no account match", func(t *testing.T) {
		// types/properties/chats: BuildDerivedRoot emits no identity; the
		// stamp on their creating change must NOT rescue them here
		match, key, err := creatorProvenanceFromTree(
			tree(nil, contentChange(t, "c1", ownKey, "claude-desktop")), ownAccount)
		require.NoError(t, err)
		assert.False(t, match)
		assert.Equal(t, "", key)
	})

	t.Run("another member's root → no account match, slug not served", func(t *testing.T) {
		// their tree can carry any slug string (§6: it is just a string) —
		// clause 1 is what makes that unusable
		match, key, err := creatorProvenanceFromTree(
			tree(otherKey, contentChange(t, "c1", otherKey, "claude-desktop")), ownAccount)
		require.NoError(t, err)
		assert.False(t, match)
		assert.Equal(t, "", key)
	})

	t.Run("first change signed by a different identity → no slug", func(t *testing.T) {
		// §10-3: the key clause requires the first change's identity to equal
		// the root's; a foreign-signed first change contributes nothing
		match, key, err := creatorProvenanceFromTree(
			tree(ownKey, contentChange(t, "c1", otherKey, "claude-desktop")), ownAccount)
		require.NoError(t, err)
		assert.True(t, match)
		assert.Equal(t, "", key)
	})

	t.Run("root only, no content change yet → no slug", func(t *testing.T) {
		// the §10 creation-race edge: owned, but nothing records a key
		match, key, err := creatorProvenanceFromTree(tree(ownKey), ownAccount)
		require.NoError(t, err)
		assert.True(t, match)
		assert.Equal(t, "", key)
	})

	t.Run("empty own account → no match ever", func(t *testing.T) {
		match, key, err := creatorProvenanceFromTree(
			tree(ownKey, contentChange(t, "c1", ownKey, "claude-desktop")), "")
		require.NoError(t, err)
		assert.False(t, match)
		assert.Equal(t, "", key)
	})

	t.Run("iteration failure → error out, never a verdict", func(t *testing.T) {
		// fail-closed in the error direction: the caller must see err and
		// refuse; a (false, "", nil) here would be indistinguishable from a
		// legitimate 'not yours'
		broken := tree(ownKey)
		broken.iterErr = errors.New("storage failure")
		_, _, err := creatorProvenanceFromTree(broken, ownAccount)
		require.Error(t, err)
	})
}
