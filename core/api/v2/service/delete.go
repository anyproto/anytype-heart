package v2service

// delete.go implements DELETE /v2/spaces/{spaceId}/objects/{objectId}
// (plan 3.3, APIV2_OBJECT_DELETE.md): archive semantics (Bin, reversible in
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
	Path: "objectId",
	Hint: "probe deletability without writing via DELETE …?dry_run=true; " +
		"the created_date/creator properties on GET show who created the object",
}

// DeleteObject implements DELETE /v2/spaces/{spaceId}/objects/{objectId}
// (§9.4). The authorization is a CONJUNCTION (§9.3): for scoped keys the
// space and write grants run first and unchanged; the creator check is in
// addition — a readwrite grant means "create and edit broadly, destroy only
// your own output". A dry run executes every step including the provenance
// verdict and skips only the archive (§9.6 — the deletability probe).
func (s *V2Service) DeleteObject(ctx context.Context, spaceId, objectId string, dryRun bool) (*v2model.CreateResult, error) {
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
		// a restriction refusal is PERMANENT for this object — 403, not a
		// retry-shaped 500 (the mapWriteError lesson, M2a)
		if strings.Contains(resp.Error.Description, "restricted") {
			return nil, restrictionForbidden(objectId, fmt.Errorf("%s", resp.Error.Description))
		}
		return nil, fmt.Errorf("archive object %s: %s", objectId, resp.Error.Description)
	}
	return result, nil
}

// steerSchemaDelete refuses type/property/tag-option targets with the route
// that owns their deletion. Participants, space views and other system
// objects are NOT steered — they flow into the provenance check, which
// refuses them (derived or foreign roots never match), and the message
// names the app as the repair.
func steerSchemaDelete(sbType model.SmartBlockType, spaceId string) error {
	switch sbType {
	case model.SmartBlockType_STType:
		return v2model.ValidationFailed("types are deleted through their own route",
			v2model.Issue{Path: "objectId",
				Message: "this object is a type",
				Hint:    fmt.Sprintf("use DELETE /v2/spaces/%s/types/{typeKey}", spaceId)})
	case model.SmartBlockType_STRelation:
		return v2model.ValidationFailed("properties are deleted through their own route",
			v2model.Issue{Path: "objectId",
				Message: "this object is a property",
				Hint:    fmt.Sprintf("use DELETE /v2/spaces/%s/properties/{propertyKey}", spaceId)})
	case model.SmartBlockType_STRelationOption:
		return v2model.ValidationFailed("tag options are managed through their property",
			v2model.Issue{Path: "objectId",
				Message: "this object is a select/multiSelect option",
				Hint:    fmt.Sprintf("options are edited via their property — see GET /v2/spaces/%s/properties/{propertyKey}/options", spaceId)})
	}
	return nil
}

// checkDeleteProvenance evaluates the §10 conjunction and words the §9.5
// refusal variants — each names what IS recorded, so the repair is
// discoverable. Fail-closed: a nil provenance dependency and every read
// error refuse.
func (s *V2Service) checkDeleteProvenance(ctx context.Context, spaceId, objectId string) error {
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
	caller := domain.IntegrationKeyFromCtx(ctx)
	if caller == "" {
		// a nameless key can never match a recorded slug (§5): name the repair
		return v2model.NotCreatedByThisKey(fmt.Sprintf(
			"DELETE is limited to objects this API key created, and this key has no recorded app name to compare against the recorded creator (%q). "+
				"Re-pair the app under that name, or archive the object in the Anytype app.", recorded), deleteProbeIssue)
	}
	if recorded != caller {
		return v2model.NotCreatedByThisKey(fmt.Sprintf(
			"DELETE is limited to objects this API key created: this object was created via %q, not via this key (%q). "+
				"Use the %s key, or archive it in the Anytype app.", recorded, caller, recorded), deleteProbeIssue)
	}
	return nil
}
