package restriction

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const testWorkspaceId = "workspaceId"

// managed is the policy an owner, an admin, or either member of a one-to-one space gets: the zero
// value, which must leave every object exactly as it was before this policy existed.
var managed = MemberPolicy{}

// plainMember is a writer who is neither owner nor admin, looking at an object somebody else made.
var plainMember = MemberPolicy{
	LockSpaceConfig:   true,
	LockForeignDelete: true,
	WorkspaceId:       testWorkspaceId,
}

// ownMember is the same writer looking at an object they created themselves.
var ownMember = MemberPolicy{
	LockSpaceConfig: true,
	WorkspaceId:     testWorkspaceId,
}

func givenHolder(sbType smartblock.SmartBlockType, internalKey string, policy MemberPolicy) RestrictionHolder {
	var uk domain.UniqueKey
	if internalKey != "" {
		uk = domain.MustUniqueKey(sbType, internalKey)
	}
	return &restrictionHolder{
		sbType:       sbType,
		layout:       noLayout,
		uniqueKey:    uk,
		memberPolicy: policy,
	}
}

func givenPage(policy MemberPolicy, archived bool) RestrictionHolder {
	details := domain.NewDetails()
	if archived {
		details.SetBool(bundle.RelationKeyIsArchived, true)
	}
	return &restrictionHolder{
		sbType:       smartblock.SmartBlockTypePage,
		layout:       model.ObjectType_basic,
		localDetails: details,
		memberPolicy: policy,
	}
}

func TestForeignDelete(t *testing.T) {
	t.Run("plain member cannot delete an object created by somebody else", func(t *testing.T) {
		got := GetRestrictions(givenPage(plainMember, false)).Object

		assert.ErrorIs(t, got.Check(model.Restrictions_Delete), ErrRestricted)
		assert.NoError(t, got.Check(model.Restrictions_Details), "editing another member's page stays allowed")
		assert.NoError(t, got.Check(model.Restrictions_Blocks))
	})

	t.Run("plain member can delete their own object", func(t *testing.T) {
		assert.NoError(t, GetRestrictions(givenPage(ownMember, false)).Object.Check(model.Restrictions_Delete))
	})

	t.Run("owner, admin and one-to-one members can delete anything", func(t *testing.T) {
		assert.NoError(t, GetRestrictions(givenPage(managed, false)).Object.Check(model.Restrictions_Delete))
	})

	// The archived branch of the base rules returns objRestrictAll WITHOUT Delete, so a policy
	// applied before it would let a plain member empty the bin of everybody else's objects.
	t.Run("archiving does not reopen delete for a foreign object", func(t *testing.T) {
		assert.ErrorIs(t, GetRestrictions(givenPage(plainMember, true)).Object.Check(model.Restrictions_Delete), ErrRestricted)
	})

	t.Run("an archived object of their own stays deletable", func(t *testing.T) {
		assert.NoError(t, GetRestrictions(givenPage(ownMember, true)).Object.Check(model.Restrictions_Delete))
	})
}

func TestSpaceConfigLock(t *testing.T) {
	locked := []model.RestrictionsObjectRestriction{
		model.Restrictions_Details,
		model.Restrictions_Blocks,
		model.Restrictions_Relations,
		model.Restrictions_Delete,
	}

	for _, tc := range []struct {
		name        string
		sbType      smartblock.SmartBlockType
		internalKey string
	}{
		{"workspace", smartblock.SmartBlockTypeWorkspace, ""},
		{"widgets", smartblock.SmartBlockTypeWidget, ""},
		{"space chat", smartblock.SmartBlockTypeChatDerivedObject, testWorkspaceId},
		{"system type", smartblock.SmartBlockTypeObjectType, bundle.TypeKeyPage.String()},
		{"system property", smartblock.SmartBlockTypeRelation, bundle.RelationKeyName.String()},
	} {
		t.Run(tc.name+" is locked for a plain member", func(t *testing.T) {
			got := GetRestrictions(givenHolder(tc.sbType, tc.internalKey, plainMember)).Object

			for _, r := range locked {
				assert.ErrorIs(t, got.Check(r), ErrRestricted, r.String())
			}
		})

		t.Run(tc.name+" is unchanged for owner, admin and one-to-one members", func(t *testing.T) {
			want := GetRestrictions(givenHolder(tc.sbType, tc.internalKey, managed)).Object

			assert.True(t, baseObjectRestrictions(givenHolder(tc.sbType, tc.internalKey, managed)).Equal(want))
		})
	}

	t.Run("editable system types lose details only for a plain member", func(t *testing.T) {
		// page, bookmark, set, collection and the file types are in editableSystemTypes, so the
		// base rules deliberately leave their details open. The lock is what closes them.
		pageType := bundle.TypeKeyPage.String()
		assert.NoError(t, GetRestrictions(givenHolder(smartblock.SmartBlockTypeObjectType, pageType, managed)).Object.
			Check(model.Restrictions_Details))
		assert.ErrorIs(t, GetRestrictions(givenHolder(smartblock.SmartBlockTypeObjectType, pageType, plainMember)).Object.
			Check(model.Restrictions_Details), ErrRestricted)
	})

	t.Run("custom types and properties stay editable for a plain member", func(t *testing.T) {
		customType := givenHolder(smartblock.SmartBlockTypeObjectType, "myCustomType", plainMember)
		assert.NoError(t, GetRestrictions(customType).Object.Check(model.Restrictions_Details))

		customRelation := givenHolder(smartblock.SmartBlockTypeRelation, "myCustomProperty", plainMember)
		assert.NoError(t, GetRestrictions(customRelation).Object.Check(model.Restrictions_Details))
	})

	// Derived objects have unsigned roots in a shared space, so no creator can be resolved for
	// them and LockForeignDelete is never set - ownMember is the policy they really get.
	t.Run("a per-object chat is content, not space configuration", func(t *testing.T) {
		objectChat := givenHolder(smartblock.SmartBlockTypeChatDerivedObject, "someObjectId", ownMember)

		assert.NoError(t, GetRestrictions(objectChat).Object.Check(model.Restrictions_Details))
		assert.NoError(t, GetRestrictions(objectChat).Object.Check(model.Restrictions_Delete))
	})

	t.Run("a discussion is content, not space configuration", func(t *testing.T) {
		discussion := givenHolder(smartblock.SmartBlockTypeDiscussionObject, "parentObjectId", ownMember)

		assert.NoError(t, GetRestrictions(discussion).Object.Check(model.Restrictions_Details))
		assert.NoError(t, GetRestrictions(discussion).Object.Check(model.Restrictions_Delete))
	})

	t.Run("an unknown workspace id never turns a chat into space configuration", func(t *testing.T) {
		policy := MemberPolicy{LockSpaceConfig: true}
		chat := givenHolder(smartblock.SmartBlockTypeChatDerivedObject, "", policy)

		assert.False(t, IsSpaceConfigObject(chat, policy))
	})
}

// The base rules hand back shared package-level maps. Adding to one in place would restrict every
// object of that type for the rest of the process.
func TestPolicyDoesNotMutateSharedTables(t *testing.T) {
	before := objectRestrictionsBySBType[smartblock.SmartBlockTypeWidget].Copy()

	GetRestrictions(givenHolder(smartblock.SmartBlockTypeWidget, "", plainMember))

	assert.True(t, before.Equal(objectRestrictionsBySBType[smartblock.SmartBlockTypeWidget]))
}
