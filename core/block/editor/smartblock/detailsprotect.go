package smartblock

import (
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
)

// identityDetails say what a relation or a type object IS, as opposed to how it looks.
// relationKey is the key every value of the relation is stored under, relationFormat is how those
// values are read back, and sourceObject is the bundled definition the object was installed from -
// the key InstallBundledObjects matches an installed object on. Repointing one of them on an object
// that already exists reinterprets or orphans every value ever written under it, and no editor,
// migration or importer has a reason to, so the stored value wins over whatever a writer brings.
//
// Two neighbouring keys are deliberately not here:
//   - relationFormatObjectTypes, which SystemObjectReviser extends when a bundled definition gains
//     an allowed type (see checkRelationFormatObjectTypes);
//   - uniqueKey, whose data source is derived, so it never reaches the tree and injectDerivedDetails
//     rebuilds it from the object's own header on every Apply.
var identityDetails = []domain.RelationKey{
	bundle.RelationKeyRelationKey,
	bundle.RelationKeyRelationFormat,
	bundle.RelationKeySourceObject,
}

// hasIdentityDetails reports whether identityDetails describe the identity of this object.
// The bundled counterparts live in the read-only marketplace space and are rebuilt from the bundle
// on every read, so there is nothing to preserve for them.
func hasIdentityDetails(sbType smartblock.SmartBlockType) bool {
	return sbType == smartblock.SmartBlockTypeRelation || sbType == smartblock.SmartBlockTypeObjectType
}

// preserveIdentityDetails restores every identityDetails value that the incoming state changes or
// drops on an object that already carries one. It runs as HookBeforeApply, so the restored value is
// the one that reaches the tree and the writer's attempt leaves nothing behind but a log line.
//
// It never returns an error on purpose. Apply discards the whole state when HookBeforeApply fails,
// and reports success to the caller while doing so, so erroring here would turn one bad detail into
// a lost document - during an import, into a lost import.
func (sb *smartBlock) preserveIdentityDetails(info ApplyInfo) error {
	stored := committedState(info.State)
	for _, key := range identityDetails {
		storedValue := stored.Details().Get(key)
		if isDetailUnset(storedValue) {
			// the first write wins, so both creation and a backfill of a missing value pass through
			continue
		}
		newValue := info.State.Details().Get(key)
		if newValue.Equal(storedValue) {
			continue
		}
		info.State.SetDetail(key, storedValue)
		log.With("objectId", sb.Id(), "spaceId", sb.SpaceID(), "sbType", sb.Type().String(),
			"detail", key.String(), "stored", storedValue.Raw(), "rejected", newValue.Raw()).
			Warnf("identity detail of an existing object can not be changed, keeping the stored value")
	}
	return nil
}

// committedState returns the state Apply is about to merge into. Writers may stack several states
// on top of it, and the identity to compare against is the one already in the tree, not one an
// intermediate state introduced along the way. A state with no parent is its own committed state,
// which turns every comparison into a no-op: there is nothing yet to preserve.
func committedState(s *state.State) *state.State {
	for s.ParentState() != nil {
		s = s.ParentState()
	}
	return s
}

// isDetailUnset reports whether a detail carries no value yet. A missing key and an empty string
// both count, a zero number does not: 0 is the longtext relation format, not the absence of one.
func isDetailUnset(v domain.Value) bool {
	if !v.Ok() || v.IsNull() {
		return true
	}
	str, isString := v.TryString()
	return isString && str == ""
}
