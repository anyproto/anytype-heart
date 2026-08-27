package anyblockjson

// profilepage.go — the deprecated per-space profile object, which a bundle
// does not carry (§2c).
//
// `kind: "profile_page"` is the pre-participant representation of a person in
// a space. A `participant` document does that job now, and every space that
// still holds a profile object also holds participants — from 1 to 1,856 of
// them across a 77-space export.
//
// What survives in a real account is not the account owner's own profile. It
// is the profile object of whoever built each imported space, dragged along
// by the import. Measured over 77 spaces, 8 remain, and every one of them
//
//   - is `isHidden: true`,
//   - carries `importType` and `origin`, so it ARRIVED rather than being made,
//   - carries `oldAnytypeID`, so it predates the current data model,
//   - holds nothing: seven have no blocks at all, the eighth an empty
//     paragraph — the one the editor leaves on any object ever opened,
//   - and is named after someone or something else: four are literally
//     "Onboarding 2.2", one is a space's name, three are other people.
//
// A bundle is shareable, and a hidden object carrying a stranger's name is
// not something a reader wants restored.
//
// UNCONDITIONAL, and deliberately unlike the omissions beside it. Those are
// fail-closed — a space document with real content on its page still travels
// — because a space object is a live thing that merely happens to be empty.
// A profile object is not: the kind is DEPRECATED, nothing creates one any
// more, and whatever a particular one holds is residue from a data model that
// no longer exists. Keeping the richest of them would preserve exactly the
// thing least worth preserving.

import (
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// OmittedProfilePage reports the deprecated profile object, which a bundle
// never writes (§2c).
func OmittedProfilePage(sbType model.SmartBlockType, base *model.SmartBlockSnapshotBase) bool {
	return sbType == model.SmartBlockType_ProfilePage && base != nil
}
