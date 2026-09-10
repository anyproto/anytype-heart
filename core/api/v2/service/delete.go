package v2service

// delete.go implements DELETE /v2/spaces/{space_id}/objects/{object_id}:
// archive semantics (Bin, reversible in
// the app — v1 parity and v2 uniformity with DeleteType/DeleteProperty),
// gated by CREATOR PROVENANCE — deletion is permitted only for objects the
// calling API key created, read from validated change storage (§10), never
// from a detail. Fail-closed everywhere: no recorded key (every object that
// predates the stamp, every app/import/other-member creation) refuses for
// every caller — the settled §8 decision, not a gap — and any error or
// ambiguity in the enforcement read refuses, never allows.

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// deleteProbeIssue is the C6 issue every ownership refusal carries (§9.5):
// the probe idiom and where provenance is visible.
var deleteProbeIssue = v2model.Issue{
	Path: "object_id",
	// message is the one required member of an issue, and this one shipped
	// empty on four production 403s: the whole reason lived in the OPTIONAL
	// hint, so a reader that renders only the required field got nothing
	Message: "this key may delete only objects it created",
	Hint: "probe deletability without writing via DELETE …?dry_run=true; " +
		"the created_date/creator properties on GET show who created the object",
}

// DeleteObject implements DELETE /v2/spaces/{space_id}/objects/{object_id}
// (§9.4). The authorization is a CONJUNCTION (§9.3): for scoped keys the
// space and write grants run first and unchanged; the creator check is in
// addition — a readwrite grant means "create and edit broadly, destroy only
// your own output". A dry run executes every check this route OWNS —
// existence, steer, allowlist, grant, provenance — and skips the archive
// (§9.6 — the deletability probe). Its contract, stated plainly: archive-
// time restriction checks (restriction.CheckRestrictions, CanDeleteFile)
// run inside the archive RPC only, so a dry run does NOT evaluate them — a
// "deletable" verdict can still meet a 403 on the real call. Deliberate:
// those checks have no read-only surface, and after the allowlist (which
// excludes every restriction-carrying system type) and the provenance
// clause (own-account creations only) they are all but unreachable.
func (s *Service) DeleteObject(ctx context.Context, spaceId, objectId string, dryRun bool) (*v2model.CreateResult, error) {
	if err := s.ensureSpaceWrite(ctx, spaceId); err != nil {
		return nil, err
	}

	// resolve via the live read (the v2 house read path): unknown tree → 404
	read, err := s.reader.ReadObject(ctx, spaceId, objectId)
	if err != nil {
		return nil, mapReadError(spaceId, objectId, err)
	}
	// a tombstoned row (deleted derived object whose tree survives) is gone
	// as far as the API is concerned
	row, rowErr := s.store.SpaceIndex(spaceId).GetDetails(objectId)
	if rowErr == nil && row.GetBool(bundle.RelationKeyIsDeleted) {
		return nil, v2model.NotFound(fmt.Sprintf("object %q not found in space %q", objectId, spaceId))
	}

	// schema objects have their own delete routes with their own semantics —
	// steer, do not sort-of-serve (§9.4-3)
	if err := steerSchemaDelete(read.SbType, spaceId); err != nil {
		return nil, err
	}
	// POSITIVE user-content allowlist, BEFORE the provenance read: anything
	// outside it refuses regardless of what provenance would say. This is
	// load-bearing, not belt-and-braces — "derived trees can never pass the
	// root clause" is FALSE: derivePersonalPayload signs derived roots with
	// the account identity whenever personalSpaceId == space.Id() or
	// UseAccountSignature (objectcache/tree.go:81), and objectcreator sets
	// UseAccountSignature for EVERY FileObject (smartblock.go:143) — so a
	// derived system object can show accountMatch=true, and a derived tree
	// whose root exists locally with no content change yet could even take
	// its first content change (stamp included) from an API request
	// (cacheLoad sets IsNewObject unconditionally; smartBlock.Init refuses
	// only a non-empty doc). Provenance answers "whose is it", never "is
	// this deletable content" — this list answers that.
	if !deletableSbType(read.SbType) {
		return nil, v2model.NewError(http.StatusForbidden, v2model.CodeForbidden,
			fmt.Sprintf("%s objects are not deletable through the API — DELETE serves user content only (pages, templates, files, chats). "+
				"This is a system or derived surface; manage it in the Anytype app.", read.SbType.String()),
			v2model.Issue{Path: "object_id", Message: fmt.Sprintf("object %s is a %s", objectId, read.SbType.String())})
	}

	// the provenance conjunction (§10): both clauses from validated storage
	if err := s.checkDeleteProvenance(ctx, spaceId, objectId); err != nil {
		return nil, err
	}

	result := &v2model.CreateResult{Id: objectId, Type: objectTypeKey(read)}
	if dryRun {
		result.DryRun = true
	}
	// already archived → idempotent no-op receipt, consistent with C8 retries
	if rowErr == nil && row.GetBool(bundle.RelationKeyIsArchived) {
		result.Warnings = append(result.Warnings, v2model.Issue{Message: "already archived"})
		return result, nil
	}
	if dryRun {
		return result, nil
	}

	resp := s.mw.ObjectSetIsArchived(ctx, &pb.RpcObjectSetIsArchivedRequest{ContextId: objectId, IsArchived: true})
	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSetIsArchivedResponseError_NULL {
		// a permanent refusal is a 403, not a retry-shaped 500 (the
		// mapWriteError lesson, M2a). The RPC serves only UNKNOWN_ERROR plus
		// a description, so the match is textual — and there are TWO
		// permanent shapes behind it, not one: restriction.ErrRestricted
		// ("restricted") and fileobject CanDeleteFile's
		// "can't delete other's file" (fileobject/service.go). The first
		// build matched only the former; the review (F4) executed the
		// latter into a 500 two lines under the M2a citation.
		desc := resp.Error.Description
		if strings.Contains(desc, "restricted") || strings.Contains(desc, "can't delete other's file") {
			return nil, restrictionForbidden(objectId, fmt.Errorf("%s", desc))
		}
		return nil, fmt.Errorf("archive object %s: %s", objectId, desc)
	}
	return result, nil
}

// deletableSbType is the positive allowlist of what object-DELETE may ever
// archive: user content only. Derived from the creation surface —
// objectTypeKeysToSmartBlockType (core/block/object/objectcreator/
// smartblock.go) produces exactly Page (every layout-based object: pages,
// notes, tasks, bookmarks, sets, collections), Template, FileObject and the
// store-backed chat shapes (ChatDerivedObject, DiscussionObject) as user
// content; the schema trio it also produces is steered to its own routes
// before this check. Everything else — Workspace, Archive, Home, Widget,
// SpaceView, Participant, Profile, Date, the deprecated chat container,
// tech-space shapes — is a system surface and refuses here, whatever its
// root signature looks like. Note the chat shapes are allowlisted as user
// content but still refuse at the provenance read today (their changes are
// StoreChange, which carries no stamp): the list states the product
// surface, provenance stays fail-closed.
func deletableSbType(sbType model.SmartBlockType) bool {
	switch sbType {
	case model.SmartBlockType_Page,
		model.SmartBlockType_Template,
		model.SmartBlockType_FileObject,
		model.SmartBlockType_ChatDerivedObject,
		model.SmartBlockType_DiscussionObject:
		return true
	}
	return false
}

// steerSchemaDelete refuses type/property/tag-option targets with the route
// that owns their deletion — a steer is more useful than the allowlist's
// generic refusal, so it runs first. Participants, space views and other
// system objects are not steered: the deletableSbType allowlist refuses
// them, and the message names the app as the repair.
func steerSchemaDelete(sbType model.SmartBlockType, spaceId string) error {
	switch sbType {
	case model.SmartBlockType_STType:
		return v2model.ValidationFailed("types are deleted through their own route",
			v2model.Issue{Path: "object_id",
				Message: "this object is a type",
				Hint:    fmt.Sprintf("use DELETE /v2/spaces/%s/types/{typeKey}", spaceId)})
	case model.SmartBlockType_STRelation:
		return v2model.ValidationFailed("properties are deleted through their own route",
			v2model.Issue{Path: "object_id",
				Message: "this object is a property",
				Hint:    fmt.Sprintf("use DELETE /v2/spaces/%s/properties/{property_key}", spaceId)})
	case model.SmartBlockType_STRelationOption:
		return v2model.ValidationFailed("tag options are managed through their property",
			v2model.Issue{Path: "object_id",
				Message: "this object is a select/multiSelect option",
				Hint:    fmt.Sprintf("options are edited via their property — see GET /v2/spaces/%s/properties/{property_key}/options", spaceId)})
	}
	return nil
}

// checkDeleteProvenance evaluates the §10 conjunction and words the §9.5
// refusal variants — each names what IS recorded, so the repair is
// discoverable. Fail-closed: a nil provenance dependency and every read
// error refuse.
func (s *Service) checkDeleteProvenance(ctx context.Context, spaceId, objectId string) error {
	if s.provenance == nil {
		return fmt.Errorf("delete refused: creator-provenance reader not configured")
	}
	accountMatch, recorded, err := s.provenance.CreatorProvenance(ctx, spaceId, objectId)
	if err != nil {
		return fmt.Errorf("read creator provenance for %s: %w", objectId, err)
	}
	if !accountMatch {
		return v2model.NotCreatedByThisKey(
			"DELETE is limited to objects this API key created, and this object was created by another space member or by the system. "+
				"Ask its creator, or archive it in the Anytype app if your role permits.", deleteProbeIssue)
	}
	if recorded == "" {
		return v2model.NotCreatedByThisKey(
			"DELETE is limited to objects this API key created, and no API key is recorded as this object's creator "+
				"(created by the Anytype app or before provenance existed). To remove it, archive it in the Anytype app.", deleteProbeIssue)
	}
	caller := domain.IntegrationNameFromCtx(ctx)
	if caller == "" {
		// a nameless key can never match a recorded name (§5): name the repair
		return v2model.NotCreatedByThisKey(fmt.Sprintf(
			"DELETE is limited to objects this API key created, and this key has no recorded app name to compare against the recorded creator (%q). "+
				"Re-pair the app under that name, or archive the object in the Anytype app.", recorded), deleteProbeIssue)
	}
	// EXACT comparison of raw names, deliberately (§5): the former slug
	// normalization was many-to-one ("Claude/Desktop" could archive "Claude
	// Desktop"'s output) and lossy (a non-Latin name slugged to "" and its
	// key could delete nothing). The tolerance normalization bought —
	// re-pairing under a case-variant name still matching — is given up:
	// for an authorization comparison, tolerance is a liability.
	if recorded != caller {
		return v2model.NotCreatedByThisKey(fmt.Sprintf(
			"DELETE is limited to objects this API key created: this object was created via %q, not via this key (%q). "+
				"App names are compared exactly. Use a key paired as %q, or archive it in the Anytype app.", recorded, caller, recorded), deleteProbeIssue)
	}
	return nil
}
