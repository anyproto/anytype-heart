package v2service

// delete.go implements DELETE /v2/spaces/{space_id}/objects/{object_id} and
// its inverse, POST /v2/spaces/{space_id}/objects/{object_id}/restore.
//
// DELETE moves user content to the Bin (archive: reversible in the app and
// through restore — v1 parity and v2 uniformity with DeleteType/
// DeleteProperty) for any key whose grant can write the space. That is the
// reach the key already has for edits, which can do more damage than an
// archive and are not creator-checked either; a creator gate on the one
// fully reversible action only blocked the commonest cleanup request
// ("archive these duplicates") for everything made in the app.
//
// ?permanent=true deletes for good, and only what the calling key created:
// CREATOR PROVENANCE read from validated change storage (§10), never from a
// detail, plus rules that keep a hard delete from destroying another space
// member's work or breaking other objects — no change signed by another
// member, no discussion, and for a file, no other object still using it.
// Same-account edits (the person in the app, another key) carry the same
// identity and cannot be told apart. Fail-closed everywhere: no recorded
// key (every object that predates the stamp, every app/import/other-member
// creation) refuses for every caller, and any error or ambiguity in the
// enforcement read refuses, never allows.

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/storeresolver"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// fileUserProbeLimit bounds the file-in-use probe: the refusal names this
// many users at most, which is enough to act on.
const fileUserProbeLimit = 5

// deleteProbeIssue is the C6 issue every ownership refusal carries (§9.5):
// the probe idiom and where provenance is visible.
var deleteProbeIssue = v2model.Issue{
	Path: "object_id",
	// message is the one required member of an issue, and this one shipped
	// empty on four production 403s: the whole reason lived in the OPTIONAL
	// hint, so a reader that renders only the required field got nothing
	Message: "this key may permanently delete only objects it created",
}.Hintf("probe without writing by resending with %s; the created_date/creator properties on a read show who created the object; without permanent, DELETE moves the object to the Bin instead",
	v2model.Resend("dry_run", "true"))

// DeleteObject implements DELETE /v2/spaces/{space_id}/objects/{object_id}
// (§9.4). For scoped keys the space and write grants run first and
// unchanged. Without permanent the route archives any user content the
// grant can write; with it, the creator rule and its two companions
// (permanentDeleteCheck) are in addition — a readwrite grant means "create,
// edit and archive broadly, destroy only your own output". A dry run
// executes every check this route OWNS — existence, steer, allowlist,
// grant, and for a permanent delete the provenance rules — and skips the
// write (§9.6). Its contract, stated plainly: restriction checks
// (restriction.CheckRestrictions, CanDeleteFile) run inside the archive and
// delete RPCs only, so a dry run does NOT evaluate them — a "deletable"
// verdict can still meet a 403 on the real call. Deliberate: those checks
// have no read-only surface, and after the allowlist (which excludes every
// restriction-carrying system type) they are rare.
func (s *Service) DeleteObject(ctx context.Context, spaceId, objectId string, dryRun, permanent bool) (*v2model.CreateResult, error) {
	read, archived, err := s.contentTarget(ctx, spaceId, objectId, steerSchemaDelete)
	if err != nil {
		return nil, err
	}
	result := &v2model.CreateResult{Id: objectId, Type: s.receiptType(spaceId, read), DryRun: dryRun}
	if permanent {
		return s.deletePermanently(ctx, spaceId, objectId, read, result)
	}

	// already archived → idempotent no-op receipt, consistent with C8 retries
	if archived {
		result.Warnings = append(result.Warnings, s.binNotice(ctx, spaceId, objectId, read, "already archived"))
		return result, nil
	}
	if dryRun {
		result.Warnings = append(result.Warnings, s.binNotice(ctx, spaceId, objectId, read, "a real request moves this object to the Bin"))
		return result, nil
	}
	if err := s.setArchived(ctx, objectId, true); err != nil {
		return nil, fmt.Errorf("archive %s: %w", objectId, err)
	}
	result.Warnings = append(result.Warnings, s.binNotice(ctx, spaceId, objectId, read, "moved to the Bin"))
	return result, nil
}

// RestoreObject implements POST /v2/spaces/{space_id}/objects/{object_id}/restore:
// the inverse of an archive, over the same reach — any user content the
// grant can write, whoever created it. Restoring an object that is not
// archived is an idempotent 200 with a warning.
func (s *Service) RestoreObject(ctx context.Context, spaceId, objectId string, dryRun bool) (*v2model.CreateResult, error) {
	read, archived, err := s.contentTarget(ctx, spaceId, objectId, steerSchemaRestore)
	if err != nil {
		return nil, err
	}
	result := &v2model.CreateResult{Id: objectId, Type: s.receiptType(spaceId, read), DryRun: dryRun}
	if !archived {
		result.Warnings = append(result.Warnings, v2model.Issue{Path: "object_id", Message: "not archived"})
		return result, nil
	}
	if dryRun {
		return result, nil
	}
	if err := s.setArchived(ctx, objectId, false); err != nil {
		return nil, fmt.Errorf("restore %s: %w", objectId, err)
	}
	if !s.awaitEditable(ctx, spaceId, objectId) {
		result.Warnings = append(result.Warnings, v2model.Issue{Path: "object_id",
			Message: "restored, but edits may be refused for a moment more"}.
			WithHint(v2model.Plain("an edit answered object_archived now succeeds once resent after a short pause")))
	}
	return result, nil
}

// restoreSettleTimeout bounds how long a restore waits for the object to
// become editable. A variable so tests can shorten it.
var restoreSettleTimeout = 2 * time.Second

// awaitEditable waits until the object's own isArchived detail follows the
// Bin. The restore RPC takes the object out of the Bin before returning,
// but the detail — which the edit restrictions are computed from — is
// cleared afterwards by the Bin's reconcile goroutine, so an edit sent
// straight after the restore met object_archived and was told to restore
// what it had just restored (found live). Waiting here makes the restore's
// 200 mean "editable". False when the wait ran out or the read failed.
func (s *Service) awaitEditable(ctx context.Context, spaceId, objectId string) bool {
	deadline := time.Now().Add(restoreSettleTimeout)
	for {
		read, err := s.reader.ReadObject(ctx, spaceId, objectId)
		if err != nil {
			return false
		}
		if !isArchivedRead(read) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// contentTarget runs the checks DELETE and restore share — write grant,
// existence, steer, the user-content allowlist — and returns the live read
// and whether the object is in the Bin. The Bin state is the Bin
// collection's own membership, which the archive RPC changes before it
// returns. Neither the store row nor the object's isArchived detail will
// do: both are written afterwards by the Bin's reconcile goroutine, and a
// restore right after an archive read "not archived" off the row and did
// nothing (found live).
func (s *Service) contentTarget(ctx context.Context, spaceId, objectId string, steer func(model.SmartBlockType, string) error) (apicore.ObjectRead, bool, error) {
	if err := s.ensureSpaceWrite(ctx, spaceId); err != nil {
		return apicore.ObjectRead{}, false, err
	}

	// resolve via the live read (the v2 house read path): unknown tree → 404
	read, err := s.reader.ReadObject(ctx, spaceId, objectId)
	if err != nil {
		return apicore.ObjectRead{}, false, mapReadError(spaceId, objectId, err)
	}
	// a deleted row — a corpse, or a tombstone an older build left — is gone
	// as far as the API is concerned
	row, rowErr := s.store.SpaceIndex(spaceId).GetDetails(objectId)
	if rowErr == nil && row.GetBool(bundle.RelationKeyIsDeleted) {
		return apicore.ObjectRead{}, false, deletedPermanently(spaceId, objectId)
	}

	// schema objects have their own routes with their own semantics —
	// steer, do not sort-of-serve (§9.4-3)
	if err := steer(read.SbType, spaceId); err != nil {
		return apicore.ObjectRead{}, false, err
	}
	// POSITIVE user-content allowlist, BEFORE any provenance read: anything
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
		return apicore.ObjectRead{}, false, v2model.NewError(http.StatusForbidden, v2model.CodeForbidden,
			fmt.Sprintf("%s objects cannot be archived, restored or deleted through the API — these routes serve user content only (pages, templates, files, chats). "+
				"This is a system or derived surface; manage it in the Anytype app.", read.SbType.String()),
			v2model.Issue{Path: "object_id", Message: fmt.Sprintf("object %s is a %s", objectId, read.SbType.String())})
	}
	inBin, err := s.reader.InBin(ctx, spaceId, objectId)
	if err != nil {
		return apicore.ObjectRead{}, false, fmt.Errorf("read the Bin state of %s: %w", objectId, err)
	}
	return read, inBin, nil
}

// isArchivedRead reports the isArchived detail of a live read: what the
// object's edit restrictions are computed from, so it is the right source
// for "will an edit be refused", and the wrong one for "is it in the Bin"
// (it trails the Bin collection by the reconcile goroutine).
func isArchivedRead(read apicore.ObjectRead) bool {
	return pbtypes.GetBool(read.Snapshot.GetDetails(), bundle.RelationKeyIsArchived.String())
}

// deletedPermanently is the 404 for an object whose tree was deleted for
// good: said plainly, so a caller can tell it from a mistyped id and stops
// trying to restore it.
func deletedPermanently(spaceId, objectId string) *v2model.Error {
	return v2model.NotFound(fmt.Sprintf("object %q in space %q was deleted permanently and cannot be restored", objectId, spaceId))
}

// receiptType spells the type as every read does (round-four eval R4-2: a
// custom type came back as its 24-hex stored key here, and as its slug from
// every other route).
func (s *Service) receiptType(spaceId string, read apicore.ObjectRead) string {
	return s.apiKeys(spaceId, storeresolver.New(s.store.SpaceIndex(spaceId))).TypeSlug(objectTypeKey(read))
}

// deletePermanently is DELETE ?permanent=true: the creator rule and its
// companions, then the delete RPC — archived first or not, as the app's
// delete from the Bin does.
func (s *Service) deletePermanently(ctx context.Context, spaceId, objectId string, read apicore.ObjectRead, result *v2model.CreateResult) (*v2model.CreateResult, error) {
	refusal, err := s.permanentDeleteCheck(ctx, spaceId, objectId, read)
	if err != nil {
		return nil, fmt.Errorf("check permanent delete of %s: %w", objectId, err)
	}
	if refusal != nil {
		return nil, refusal.err
	}
	result.Permanent = true
	if result.DryRun {
		return result, nil
	}
	resp := s.mw.ObjectListDelete(ctx, &pb.RpcObjectListDeleteRequest{ObjectIds: []string{objectId}})
	if resp.Error != nil && resp.Error.Code != pb.RpcObjectListDeleteResponseError_NULL {
		if isPermanentRefusal(resp.Error.Description) {
			return nil, restrictionForbidden(objectId, fmt.Errorf("%s", resp.Error.Description))
		}
		return nil, fmt.Errorf("delete %s permanently: %s", objectId, resp.Error.Description)
	}
	return result, nil
}

// setArchived moves an object to the Bin or back.
func (s *Service) setArchived(ctx context.Context, objectId string, archived bool) error {
	resp := s.mw.ObjectSetIsArchived(ctx, &pb.RpcObjectSetIsArchivedRequest{ContextId: objectId, IsArchived: archived})
	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSetIsArchivedResponseError_NULL {
		if isPermanentRefusal(resp.Error.Description) {
			return restrictionForbidden(objectId, fmt.Errorf("%s", resp.Error.Description))
		}
		return fmt.Errorf("set isArchived=%v: %s", archived, resp.Error.Description)
	}
	return nil
}

// isPermanentRefusal reports an RPC refusal that retrying cannot change —
// a 403, not a retry-shaped 500 (the mapWriteError lesson, M2a). The RPCs
// serve only UNKNOWN_ERROR plus a description, so the match is textual —
// and there are TWO permanent shapes behind it, not one:
// restriction.ErrRestricted ("restricted") and fileobject CanDeleteFile's
// "can't delete other's file" (fileobject/service.go). The first build
// matched only the former; the review (F4) executed the latter into a 500
// two lines under the M2a citation.
func isPermanentRefusal(desc string) bool {
	return strings.Contains(desc, "restricted") || strings.Contains(desc, "can't delete other's file")
}

// permanentRefusal is a verdict against a permanent delete: the C6 error the
// permanent request answers with, and the short reason an archive receipt
// quotes so the caller knows not to ask.
type permanentRefusal struct {
	why string
	err *v2model.Error
}

// permanentDeleteCheck evaluates every rule a permanent delete adds to the
// archive: the §10 creator conjunction, no change by another space member,
// no discussion, and for a file, no other object still using it. A nil
// refusal and nil error is the allow verdict; err is an infrastructure
// failure, on which the caller must refuse. Fail-closed: a nil provenance
// dependency refuses.
func (s *Service) permanentDeleteCheck(ctx context.Context, spaceId, objectId string, read apicore.ObjectRead) (*permanentRefusal, error) {
	// chats and discussions are store-backed: their changes carry no
	// creation stamp, so the creator rule could never pass — refused up
	// front, without building what can be a long message history
	if read.SbType == model.SmartBlockType_ChatDerivedObject || read.SbType == model.SmartBlockType_DiscussionObject {
		return &permanentRefusal{
			why: "chats cannot be deleted permanently through the API",
			err: v2model.NewError(http.StatusForbidden, v2model.CodeForbidden,
				"chats and discussions cannot be deleted permanently through the API",
				v2model.Issue{Path: "object_id", Message: "this object is a chat"}.
					WithHint(v2model.Plain("without permanent, DELETE moves it to the Bin"))),
		}, nil
	}
	if s.provenance == nil {
		return nil, fmt.Errorf("permanent delete refused: creator-provenance reader not configured")
	}
	prov, err := s.provenance.CreatorProvenance(ctx, spaceId, objectId)
	if err != nil {
		return nil, fmt.Errorf("read creator provenance for %s: %w", objectId, err)
	}
	if refusal := checkCreatorProvenance(ctx, prov); refusal != nil {
		return &permanentRefusal{why: "only objects this key created can be deleted permanently", err: refusal}, nil
	}
	if prov.EditedByOthers {
		// "changed", not "edited": another member's app also writes changes
		// on its own when it opens an object (layout migrations), and those
		// count — fail closed rather than guess which changes were work
		return &permanentRefusal{
			why: "it has changes by another space member, so it cannot be deleted permanently",
			err: v2model.NewError(http.StatusForbidden, v2model.CodeForbidden,
				"permanent deletion is limited to objects with no changes by another space member: deleting this one for good would erase their changes too",
				v2model.Issue{Path: "object_id", Message: "another space member changed this object"}.
					WithHint(v2model.Plain("without permanent, DELETE moves the object to the Bin, where it can be restored"))),
		}, nil
	}
	// a discussion is a tree of its own, so the co-author scan never sees
	// replies in it, and deleting the object would orphan them — the app
	// keeps a draft that has a discussion for the same reason
	if discussionIdOf(read.Snapshot) != "" {
		return &permanentRefusal{
			why: "it has a discussion, so it cannot be deleted permanently",
			err: v2model.NewError(http.StatusForbidden, v2model.CodeForbidden,
				"permanent deletion is limited to objects without a discussion: deleting this one for good would orphan its thread",
				v2model.Issue{Path: "object_id", Message: "this object has a discussion"}.
					WithHint(v2model.Plain("without permanent, DELETE moves the object to the Bin, where it can be restored"))),
		}, nil
	}
	if read.SbType == model.SmartBlockType_FileObject {
		users, err := s.fileUsers(spaceId, objectId)
		if err != nil {
			return nil, fmt.Errorf("find users of file %s: %w", objectId, err)
		}
		if len(users) > 0 {
			return &permanentRefusal{
				why: "other objects still use this file, so it cannot be deleted permanently yet",
				err: v2model.NewError(http.StatusConflict, v2model.CodeFileInUse,
					fmt.Sprintf("file %q is still used by other objects, and deleting it for good would break them: %s", objectId, strings.Join(users, ", ")),
					v2model.Issue{Path: "object_id", Message: "the file is still in use"}.
						WithHint(v2model.Plain("remove the file from those objects, or delete them first, then resend this request; without permanent, DELETE moves the file to the Bin"))),
			}, nil
		}
	}
	return nil, nil
}

// fileUsers lists up to fileUserProbeLimit objects, other than the file
// itself, that still use it: link it from a block, hold it in a file
// property (iconImage and picture are file properties too), or show it as
// their cover. Chat-message attachments are not objects and are not
// checked. Archived users count — restoring one must not bring back a
// broken embed — and deleted ones do not.
//
// One flat OR query. Its archived filter is a None condition the store
// drops as a filter, kept only because its presence switches off the
// implicit "not archived" default; the defaults are read off the first
// nested group, so nesting anything inside the OR would bring that default
// back and quietly drop the archived users this probe exists to see.
//
// Hidden system objects are not users through links alone: the Bin "links"
// everything in it, so an archived file read as used by the Bin and could
// never be deleted (found live). A hidden object holding the file in a
// property or cover IS a user — the workspace's space icon and the profile
// avatar are hidden objects with iconImage.
func (s *Service) fileUsers(spaceId, fileId string) ([]string, error) {
	fileKeys, err := s.fileRelationKeys(spaceId)
	if err != nil {
		return nil, err
	}
	uses := []database.FilterRequest{
		{RelationKey: bundle.RelationKeyIsArchived, Condition: model.BlockContentDataviewFilter_None},
		{RelationKey: bundle.RelationKeyLinks, Condition: model.BlockContentDataviewFilter_In, Value: domain.StringList([]string{fileId})},
		{RelationKey: bundle.RelationKeyCoverId, Condition: model.BlockContentDataviewFilter_Equal, Value: domain.String(fileId)},
	}
	for _, key := range fileKeys {
		uses = append(uses, database.FilterRequest{RelationKey: key, Condition: model.BlockContentDataviewFilter_In, Value: domain.StringList([]string{fileId})})
	}
	records, err := s.store.SpaceIndex(spaceId).Query(database.Query{
		Filters: []database.FilterRequest{{Operator: model.BlockContentDataviewFilter_Or, NestedFilters: uses}},
		// hidden link-only rows are dropped below; the slack keeps a few of
		// them from crowding real users out of the probe
		Limit: fileUserProbeLimit + 10,
	})
	if err != nil {
		return nil, fmt.Errorf("query users of file %s: %w", fileId, err)
	}
	var users []string
	for _, record := range records {
		id := record.Details.GetString(bundle.RelationKeyId)
		if id == fileId {
			continue
		}
		if record.Details.GetBool(bundle.RelationKeyIsHidden) && !holdsFile(record.Details, fileKeys, fileId) {
			continue
		}
		users = append(users, id)
		if len(users) == fileUserProbeLimit {
			break
		}
	}
	return users, nil
}

// fileRelationKeys lists the keys of every file-format property in the
// space, plus the bundled ones a space may not have installed.
func (s *Service) fileRelationKeys(spaceId string) ([]domain.RelationKey, error) {
	records, err := s.store.SpaceIndex(spaceId).Query(database.Query{Filters: []database.FilterRequest{
		{RelationKey: bundle.RelationKeyResolvedLayout, Condition: model.BlockContentDataviewFilter_Equal, Value: domain.Int64(int64(model.ObjectType_relation))},
		{RelationKey: bundle.RelationKeyRelationFormat, Condition: model.BlockContentDataviewFilter_Equal, Value: domain.Int64(int64(model.RelationFormat_file))},
	}})
	if err != nil {
		return nil, fmt.Errorf("query file properties in space %s: %w", spaceId, err)
	}
	keys := []domain.RelationKey{bundle.RelationKeyIconImage, bundle.RelationKeyPicture}
	for _, record := range records {
		key := domain.RelationKey(record.Details.GetString(bundle.RelationKeyRelationKey))
		if key != "" && !slices.Contains(keys, key) {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

// holdsFile reports whether a row holds the file in a property or as its
// cover — a use whatever the row is — rather than only linking it.
func holdsFile(details *domain.Details, fileKeys []domain.RelationKey, fileId string) bool {
	if details.GetString(bundle.RelationKeyCoverId) == fileId {
		return true
	}
	for _, key := range fileKeys {
		if details.GetString(key) == fileId || slices.Contains(details.GetStringList(key), fileId) {
			return true
		}
	}
	return false
}

// binNotice is the receipt line an archive carries: where the object went,
// how to undo it, and whether this key may also delete it for good. The
// verdict is evaluated per object, so a run of archives tells the caller
// exactly which objects a permanent resend would remove. It is advice, not
// the archive's own outcome: an infrastructure failure in the verdict drops
// the permanent clause rather than failing an archive that succeeded.
func (s *Service) binNotice(ctx context.Context, spaceId, objectId string, read apicore.ObjectRead, message string) v2model.Issue {
	issue := v2model.Issue{Path: "object_id", Message: message}
	restore := v2model.RefRestoreObject(spaceId, objectId)
	refusal, err := s.permanentDeleteCheck(ctx, spaceId, objectId, read)
	switch {
	case err != nil:
		return issue.Hintf("restore it with %s", restore)
	case refusal != nil:
		return issue.Hintf("restore it with %s; %s", restore, refusal.why)
	default:
		return issue.Hintf("restore it with %s; this key created it, so resending with %s deletes it for good", restore, v2model.Resend("permanent", "true"))
	}
}

// deletableSbType is the positive allowlist of what object-DELETE and
// restore may ever touch: user content only. Derived from the creation
// surface — objectTypeKeysToSmartBlockType (core/block/object/
// objectcreator/smartblock.go) produces exactly Page (every layout-based
// object: pages, notes, tasks, bookmarks, sets, collections), Template,
// FileObject and the store-backed chat shapes (ChatDerivedObject,
// DiscussionObject) as user content; the schema trio it also produces is
// steered to its own routes before this check. Everything else —
// Workspace, Archive, Home, Widget, SpaceView, Participant, Profile, Date,
// the deprecated chat container, tech-space shapes — is a system surface
// and refuses here, whatever its root signature looks like. Note the chat
// shapes are allowlisted as user content but still refuse a PERMANENT
// delete at the provenance read (their changes are StoreChange, which
// carries no stamp): the list states the product surface, provenance stays
// fail-closed.
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
			}.Hintf("use %s", v2model.NewRef(v2model.OpDeleteType, "space_id", spaceId)))
	case model.SmartBlockType_STRelation:
		return v2model.ValidationFailed("properties are deleted through their own route",
			v2model.Issue{Path: "object_id",
				Message: "this object is a property",
			}.Hintf("use %s", v2model.NewRef(v2model.OpDeleteProperty, "space_id", spaceId)))
	case model.SmartBlockType_STRelationOption:
		return v2model.ValidationFailed("tag options are managed through their property",
			v2model.Issue{Path: "object_id",
				Message: "this object is a select/multiSelect option",
			}.Hintf("options are edited via their property — see %s", v2model.NewRef(v2model.OpListPropertyOptions, "space_id", spaceId)))
	}
	return nil
}

// steerSchemaRestore refuses type/property/tag-option targets of a restore.
// Types and properties have no API restore: a deleted one gives up its key
// and slug for reuse, so bringing it back is a naming decision the app's
// Bin makes with the person, not one to take blind.
func steerSchemaRestore(sbType model.SmartBlockType, _ string) error {
	switch sbType {
	case model.SmartBlockType_STType, model.SmartBlockType_STRelation, model.SmartBlockType_STRelationOption:
		return v2model.ValidationFailed("types, properties and tag options are not restored through this route",
			v2model.Issue{Path: "object_id",
				Message: "this object is a type, a property or a tag option",
			}.WithHint(v2model.Plain("restore it from the Bin in the Anytype app")))
	}
	return nil
}

// checkCreatorProvenance evaluates the §10 conjunction and words the §9.5
// refusal variants — each names what IS recorded, so the repair is
// discoverable.
func checkCreatorProvenance(ctx context.Context, prov apicore.Provenance) *v2model.Error {
	if !prov.AccountMatch {
		return v2model.NotCreatedByThisKey(
			"permanent deletion is limited to objects this API key created, and this object was created by another space member or by the system. "+
				"Without permanent, DELETE moves it to the Bin instead.", deleteProbeIssue)
	}
	if prov.IntegrationName == "" {
		return v2model.NotCreatedByThisKey(
			"permanent deletion is limited to objects this API key created, and no API key is recorded as this object's creator "+
				"(created by the Anytype app or before provenance existed). Without permanent, DELETE moves it to the Bin; the app can empty the Bin.", deleteProbeIssue)
	}
	caller := domain.IntegrationNameFromCtx(ctx)
	if caller == "" {
		// a nameless key can never match a recorded name (§5): name the repair
		return v2model.NotCreatedByThisKey(fmt.Sprintf(
			"permanent deletion is limited to objects this API key created, and this key has no recorded app name to compare against the recorded creator (%q). "+
				"Without permanent, DELETE moves it to the Bin instead.", prov.IntegrationName), deleteProbeIssue)
	}
	// EXACT comparison of raw names, deliberately (§5): the former slug
	// normalization was many-to-one ("Claude/Desktop" could delete "Claude
	// Desktop"'s output) and lossy (a non-Latin name slugged to "" and its
	// key could delete nothing). The tolerance normalization bought —
	// re-pairing under a case-variant name still matching — is given up:
	// for an authorization comparison, tolerance is a liability.
	if prov.IntegrationName != caller {
		return v2model.NotCreatedByThisKey(fmt.Sprintf(
			"permanent deletion is limited to objects this API key created: this object was created via %q, not via this key (%q). "+
				"App names are compared exactly. Without permanent, DELETE moves it to the Bin instead.", prov.IntegrationName, caller), deleteProbeIssue)
	}
	return nil
}
