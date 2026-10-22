package v2service

// discussion.go implements the object-discussion surface. A discussion is
// the comment thread under an object: the same store-backed chat object a
// space chat is, derived from its parent object, and reached through the
// chat routes by its own id (ensureChat admits the discussion layout). This
// file owns the two things the chat routes cannot: minting the discussion
// for an object — the desktop does it lazily on the first post, through
// the same ObjectAddDiscussion — and answering a caller the object's
// discussion id whether it existed or was just made. The object read serves
// the id as the `discussion` envelope member (object.go), so a reader never
// needs this op; a writer calls it once and then posts through the chat
// routes.

import (
	"context"
	"fmt"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// CreateDiscussion implements POST .../objects/{object_id}/discussion: the
// object's discussion id, minted when the object has none. The object is
// read live (the house read path, so the id is exact even when the index
// row lags a create from another device): an unknown or deleted id is a
// 404, an archived object, a chat or a discussion itself is a targeted
// 400, and only user content (pages and files — what the desktop shows a
// comment section for) can hold one. Created reports which branch ran; a
// dry run stops before the mint and reports the would-be outcome.
func (s *Service) CreateDiscussion(ctx context.Context, spaceId, objectId string, dryRun bool) (*v2model.DiscussionResult, error) {
	if err := s.ensureSpaceWrite(ctx, spaceId); err != nil {
		return nil, err
	}
	read, err := s.reader.ReadObject(ctx, spaceId, objectId)
	if err != nil {
		return nil, mapReadError(spaceId, objectId, err)
	}
	if row, rowErr := s.store.SpaceIndex(spaceId).GetDetails(objectId); rowErr == nil {
		if row.GetBool(bundle.RelationKeyIsDeleted) {
			return nil, v2model.NotFound(fmt.Sprintf("object %q not found in space %q", objectId, spaceId))
		}
		// an archived object is read-only in the desktop (its comment
		// section shows the thread but takes no new post), and the
		// object restriction that says so is not the one the parent link
		// write consults — so the refusal has to happen here
		if row.GetBool(bundle.RelationKeyIsArchived) {
			return nil, v2model.ValidationFailed(fmt.Sprintf("object %q is archived", objectId),
				v2model.Issue{Path: "object_id", Message: "an archived object cannot start a discussion; restore it from Bin in the Anytype app first"})
		}
	}
	if err := discussionTargetRefusal(spaceId, objectId, read.SbType); err != nil {
		return nil, err
	}
	if existing := discussionIdOf(read.Snapshot); existing != "" {
		return &v2model.DiscussionResult{Id: existing, DryRun: dryRun}, nil
	}
	if dryRun {
		return &v2model.DiscussionResult{Created: true, DryRun: true}, nil
	}
	// a concurrent mint (another device, a retry outside the idempotency
	// window) or a mint a crash left half-done is the RPC's to absorb: the
	// discussion id is derived from the parent, so ObjectAddDiscussion
	// recovers an existing tree and finishes the parent link rather than
	// refusing — which is why no fallback re-read happens here, and any
	// error it does return is a real failure the caller should see
	resp := s.mw.ObjectAddDiscussion(ctx, &pb.RpcObjectDiscussionAddRequest{ObjectId: objectId})
	if resp.Error != nil && resp.Error.Code != pb.RpcObjectDiscussionAddResponseError_NULL {
		return nil, v2ChatRpcError("create discussion", int32(resp.Error.Code), int32(pb.RpcObjectDiscussionAddResponseError_BAD_INPUT), resp.Error.Description)
	}
	return &v2model.DiscussionResult{Id: resp.DiscussionId, Created: true}, nil
}

// discussionTargetRefusal is the targeted 400 for an object that cannot
// hold a discussion. A chat shape is refused with the route that already
// serves its messages — a space chat by its own id, a discussion by the id
// the caller just sent — and anything that is not user content (a type, a
// template, a participant, a system surface) says so, since the desktop
// shows a comment section on pages and files only.
func discussionTargetRefusal(spaceId, objectId string, sbType model.SmartBlockType) error {
	switch sbType {
	case model.SmartBlockType_Page, model.SmartBlockType_FileObject:
		return nil
	case model.SmartBlockType_ChatDerivedObject:
		return v2model.ValidationFailed(fmt.Sprintf("object %q is a chat, not an object with a discussion", objectId),
			v2model.Issue{Path: "object_id", Message: "a chat holds messages directly"}.
				Hintf("read them with %s", v2model.RefGetChatMessages(spaceId, objectId)))
	case model.SmartBlockType_DiscussionObject:
		return v2model.ValidationFailed(fmt.Sprintf("object %q is a discussion already", objectId),
			v2model.Issue{Path: "object_id", Message: "this id is the discussion; the chat operations take it as chat_id"}.
				Hintf("read its messages with %s", v2model.RefGetChatMessages(spaceId, objectId)))
	}
	return v2model.ValidationFailed(fmt.Sprintf("object %q cannot hold a discussion", objectId),
		v2model.Issue{Path: "object_id", Message: fmt.Sprintf("it is a %s; discussions attach to pages and files only", sbType.String())})
}

// discussionIdOf is the object's discussion id from its live details: the
// parent keeps it in a hidden relation the property vocabulary never
// serves, so it is read here and served as its own envelope member.
func discussionIdOf(snapshot *model.SmartBlockSnapshotBase) string {
	if snapshot == nil || snapshot.Details == nil {
		return ""
	}
	return pbtypes.GetString(snapshot.Details, bundle.RelationKeyDiscussionId.String())
}
