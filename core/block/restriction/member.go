package restriction

import (
	"slices"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// MemberPolicy is what the space ACL says about the caller for THIS object. Its zero value adds
// nothing, so a holder that knows nothing about space membership keeps the restrictions it had
// before this policy existed - that is what keeps the marketplace, importers and every test stub
// on their old behaviour.
type MemberPolicy struct {
	// LockSpaceConfig is set when the caller may not change space-wide configuration: it is
	// neither the owner nor an admin, and the space is not one-to-one (where the ACL owner is a
	// key derived from both identities and both members are plain writers).
	LockSpaceConfig bool
	// LockForeignDelete is set when the caller did not sign this object's tree root. Read from the
	// signed root header, never from the creator detail: that detail is local and ObjectSetDetails
	// can overwrite it.
	LockForeignDelete bool
	// WorkspaceId identifies the space-level chat, which is the chatDerived object whose unique
	// key is the workspace id. Per-object chats and discussions are keyed by their parent object
	// instead and are ordinary content. Empty when unknown.
	WorkspaceId string
}

// spaceConfigRestrictions is the full lock put on space configuration objects. Relations is
// currently advisory - nothing checks it on a write path - and is included so the lock stays
// complete if that changes.
var spaceConfigRestrictions = []model.RestrictionsObjectRestriction{
	model.Restrictions_Details,
	model.Restrictions_Blocks,
	model.Restrictions_Relations,
	model.Restrictions_Delete,
}

// IsSpaceConfigObject reports whether this object holds space-wide configuration rather than
// content. Every clause reads the object's type and unique key, both of which come from the signed
// tree root (typeprovider.GetTypeAndKeyFromRoot) rather than the object store, so none of them can
// be steered by a detail. The type is matched first, so an ordinary object carrying a stray
// snapshot key can never pass as a system type.
// The policy is passed in rather than read from the holder: it is already in hand at every call
// site, and asking for it again would double the ACL reads this sits on.
func IsSpaceConfigObject(rh RestrictionHolder, policy MemberPolicy) bool {
	switch rh.Type() {
	case smartblock.SmartBlockTypeWorkspace, smartblock.SmartBlockTypeWidget:
		return true
	case smartblock.SmartBlockTypeChatDerivedObject:
		return policy.WorkspaceId != "" && internalKey(rh) == policy.WorkspaceId
	case smartblock.SmartBlockTypeObjectType:
		return slices.Contains(bundle.SystemTypes, domain.TypeKey(internalKey(rh)))
	case smartblock.SmartBlockTypeRelation:
		return slices.Contains(bundle.SystemRelations, domain.RelationKey(internalKey(rh)))
	}
	return false
}

func internalKey(rh RestrictionHolder) string {
	uk := rh.UniqueKey()
	if uk == nil {
		return ""
	}
	return uk.InternalKey()
}

// applyMemberPolicy layers the ACL-derived restrictions on top of the ones the object carries by
// its own nature. It runs last, after every early return in the base rules: the archived branch
// hands back objRestrictAll without Delete, so a policy applied before it would let a writer empty
// the bin of another member's objects.
//
// The base rules hand back shared package-level maps, so anything added here must go to a copy.
func applyMemberPolicy(r ObjectRestrictions, rh RestrictionHolder) ObjectRestrictions {
	policy := rh.MemberPolicy()
	lockConfig := policy.LockSpaceConfig && IsSpaceConfigObject(rh, policy)
	if !lockConfig && !policy.LockForeignDelete {
		return r
	}
	r = r.Copy()
	if lockConfig {
		r.Add(spaceConfigRestrictions...)
	}
	if policy.LockForeignDelete {
		r.Add(model.Restrictions_Delete)
	}
	return r
}
