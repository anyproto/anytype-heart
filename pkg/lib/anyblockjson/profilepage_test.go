package anyblockjson

// profilepage_test.go — the deprecated profile object never travels (§2c).

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// The kind is deprecated: a `participant` document represents a person in a
// space now, and every space in a 77-space export that still holds a profile
// object also holds participants — 1 to 1,856 of them. What survives is the
// profile of whoever built each imported space, hidden, carrying
// `oldAnytypeID`, named after someone else.
//
// The drop is UNCONDITIONAL, unlike the space-document omission beside it. A
// space object is live and merely happens to be empty, so that one fails
// closed on any content. Nothing creates a profile object any more, so
// whatever one holds is residue from a data model that is gone — keeping the
// richest of them would preserve exactly the thing least worth preserving.
//
// How this can fail: make it conditional on emptiness and the account's
// eight become seven, kept by an empty paragraph the editor left behind;
// widen the kind gate and live objects stop being exported.
func TestProfilePage_NeverTravels(t *testing.T) {
	base := func(det map[string]*types.Value, blocks ...*model.Block) *model.SmartBlockSnapshotBase {
		return &model.SmartBlockSnapshotBase{Details: fields(det), Blocks: blocks}
	}

	t.Run("an empty one", func(t *testing.T) {
		assert.True(t, OmittedProfilePage(model.SmartBlockType_ProfilePage,
			base(map[string]*types.Value{"name": str("Onboarding 2.2"), "isHidden": boolVal(true)})))
	})

	t.Run("one with content goes too — the kind is what is deprecated", func(t *testing.T) {
		assert.True(t, OmittedProfilePage(model.SmartBlockType_ProfilePage,
			base(map[string]*types.Value{"name": str("Abby"), "description": str("a real note")},
				&model.Block{Id: "p", Content: &model.BlockContentOfText{
					Text: &model.BlockContentText{Text: "something someone wrote"}}})))
	})

	t.Run("no other kind is touched", func(t *testing.T) {
		for _, k := range []model.SmartBlockType{
			model.SmartBlockType_Page,
			model.SmartBlockType_Participant,
			model.SmartBlockType_Workspace,
			model.SmartBlockType_STType,
		} {
			assert.Falsef(t, OmittedProfilePage(k, base(map[string]*types.Value{"name": str("x")})),
				"kind %v must still be exported", k)
		}
	})

	t.Run("a nil snapshot is not an omission", func(t *testing.T) {
		assert.False(t, OmittedProfilePage(model.SmartBlockType_ProfilePage, nil))
	})
}
