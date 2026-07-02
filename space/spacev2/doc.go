// Package spacev2 is the clean-room reimplementation target for the space
// subsystem. It is built alongside the existing space/ package (which remains the
// behavior + byte-exact-contract oracle) so the repository keeps compiling
// throughout the rewrite.
//
//	Spec:   docs/SpaceController.md   (target design / north star)
//	Oracle: space/ (v1)              (exact contracts + behavior to match)
//	Plan:   space/spacev2/HANDOFF.md (mission, milestones, done criteria)
//
// Nothing here is registered in core/anytype/bootstrap.go yet. The package is
// inert (every method returns errNotImplemented) until it passes the parity
// tests described in HANDOFF.md, at which point CName is renamed to the v1
// "client.space" and consumers are cut over.
package spacev2
