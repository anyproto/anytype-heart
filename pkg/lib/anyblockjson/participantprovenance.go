package anyblockjson

// participantprovenance.go — the stored details a PARTICIPANT document does
// not carry, because on that kind the value is not a fact about the member
// (§3; the transient-key policy, scoped by kind — typeProvenanceKeys'
// pattern, on the other machine-derived kind).
//
// A participant document is derived from the ACL: it has no creation change
// of its own, and an object whose root change carries no creation date gets
// `createdDate` stamped with time.Now() at load
// (core/block/editor/smartblock, detailsinject) — so the stored value is
// the moment the object was last COLD-BUILT wearing the name of a fact.
// Measured, which is what admitted the drop: two exports of the same 7
// spaces, 1,164 documents compared field-by-field — the only drifting kind
// is participant (22 of 22), and the only drifting field is `created_date`
// (22 of 22); every other kind is byte-stable across exports. On a full
// 155-space run the drift was 2,322 documents against 2,492 participants.
// A value that changes whenever the cache evicts between two reads of an
// unchanged object describes the reader, not the object.
//
// The other two provenance fields a participant carries — `creator` and
// `last_modified_by` — STAY, by decision: both are `_anytype_profile` on
// 2,492 of 2,492 corpus participants, which is upstream's bug to fix (a
// participant's creator should be the real identity), not this format's to
// paper over by omission.

import (
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// participantProvenanceKeys are the stored details export omits on a
// participant document and import drops there (stale, not wrong). Each
// entry needs the measured proof that the value describes the reading
// session rather than the member — the §15 #12 discipline.
var participantProvenanceKeys = map[string]string{
	// stamped time.Now() on every cold build (no creation change to derive
	// it from); the ONLY field that drifted across a 1,164-document
	// double-export comparison, on 22 of 22 participants
	"createdDate": "a load timestamp wearing the name of a fact: re-stamped on every cold build",
}

// isParticipantSmartBlock is the snapshot-side statement of which kind the
// rule scopes to, isTypeSmartBlock's shape.
func isParticipantSmartBlock(sbType model.SmartBlockType) bool {
	return sbType == model.SmartBlockType_Participant
}

// DroppedParticipantProvenanceKey reports a stored detail that export omits
// on a PARTICIPANT document because its value describes the reading session
// rather than the member (§3). It is the exported half of the rule, for the
// round-trip comparator — the predicate is the format's own, not a copy, so
// the comparator and the exporter cannot disagree (the drift class that
// once produced 1,344 false failures in one sweep).
func DroppedParticipantProvenanceKey(sbType model.SmartBlockType, key string) bool {
	if !isParticipantSmartBlock(sbType) {
		return false
	}
	_, dropped := participantProvenanceKeys[key]
	return dropped
}
