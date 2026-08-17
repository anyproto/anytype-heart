package apicore

import (
	"context"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/crossspacesub"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type ChatSubscriptionService interface {
	SubscribeLastMessages(ctx context.Context, chatObjectId string, limit int, subId string, sink chan<- *pb.Event) ([]*model.ChatMessage, error)
	Unsubscribe(chatObjectId string, subId string) error
}

type AccountService interface {
	GetInfo(ctx context.Context) (*model.AccountInfo, error)
}

type EventService interface {
	Broadcast(event *pb.Event)
}

type CrossSpaceSubscriptionService interface {
	Subscribe(req subscription.SubscribeRequest, predicate crossspacesub.Predicate) (*subscription.SubscribeResponse, error)
	Unsubscribe(subId string) error
}

// FileObjectService exposes the subset of fileobject.Service that the API
// needs to stream file/image content directly to clients.
type FileObjectService interface {
	GetFileData(ctx context.Context, objectId string) (files.File, error)
	GetImageData(ctx context.Context, objectId string) (files.Image, error)
	GetImageDataFromRawId(ctx context.Context, fileId domain.FileId) (files.Image, error)
}

// ObjectRead is one consistent read of an object's live state: snapshot and
// tree heads come from the same locked state read, so the derived etag and
// the content always agree (APIV2.md §8 read path).
type ObjectRead struct {
	SbType   model.SmartBlockType
	Snapshot *model.SmartBlockSnapshotBase
	Heads    []string
	// BlocksRefused and DetailsRefused carry the object-level restriction
	// verdicts from the same locked read, so a dry run reaches the same
	// conclusion as the real edit (review C′3). nil means that axis is
	// editable. They are separate because the restrictions are: a set and a
	// collection carry Restrictions_Blocks but NOT Restrictions_Details, so
	// one blanket verdict made renaming a set — and every add_items — refuse
	// (surface review M1).
	BlocksRefused  error
	DetailsRefused error
}

// EditNeeds declares which object-level restriction axes an edit touches, so
// the gate can be per-op instead of per-request. Item ops (add_items /
// remove_items) need NEITHER: they mutate the collection store, which no
// object restriction governs — matching v1's ObjectCollectionAdd.
type EditNeeds struct {
	Blocks  bool
	Details bool
}

// Needs reports whether anything is required at all.
func (n EditNeeds) Needs() bool { return n.Blocks || n.Details }

// ObjectReader reads the live smartblock state of an object — the API v2
// read path (APIV2.md §8: not ObjectShow, not the store snapshot).
type ObjectReader interface {
	ReadObject(ctx context.Context, spaceId string, objectId string) (ObjectRead, error)
}

// ObjectCreator creates objects from AnyBlock snapshots — the API v2 create
// path (APIV2.md §2 Phase 2). CreateObjectFromSnapshot builds the object's
// initial state from the snapshot and creates it in one change set (atomic
// composite creates, §8/R10); the object's type keys come from the
// snapshot's ObjectTypes. TypeIdByKey derives the space-local object id of a
// type key (needed for setOf/targetObjectType details before the object
// exists). RelationIdByKey is the relation twin: derived objects' ids are a
// pure function of (space, kind, internal key) (ADDRESSING §2.4), so the id
// is computable whether or not the relation object — or even its index
// row — exists; the corpse probes use it to see a tombstoned row that no
// key-filtered query can return (§8.41).
type ObjectCreator interface {
	CreateObjectFromSnapshot(ctx context.Context, spaceId string, snapshot *model.SmartBlockSnapshotBase) (id string, err error)
	TypeIdByKey(ctx context.Context, spaceId string, key domain.TypeKey) (string, error)
	RelationIdByKey(ctx context.Context, spaceId string, key domain.RelationKey) (string, error)
}

// ObjectEdit is one locked editing session on a live object — what the
// PATCH pipeline works on (APIV2.md §2 Phase 3). SbType and Heads are the
// same consistent read the Phase-1 reader produces (the If-Match inputs);
// State is a child state of the live document, so ops mutate it directly and
// the adapter commits it with ONE ordinary smartblock Apply — per-block
// restriction checks, undo recording, hooks/events and the minimal
// id-matched change diff all ride the normal editor path.
type ObjectEdit struct {
	SbType model.SmartBlockType
	Heads  []string
	State  *state.State
}

// ObjectMutator applies one atomic mutation to a live object — the API v2
// edit path (APIV2.md §2 Phase 3).
//
// MutateObject (PATCH) locks the object, hands apply an ObjectEdit whose
// State is a fresh child of the live state, and — when apply returns nil —
// commits that state with one ordinary Apply. The returned heads are the
// post-apply tree heads, the input of the new etag.
//
// There is deliberately no snapshot-shaped sibling: the reset-to-version
// document replace that once served PUT was removed with that surface
// (APIV2.md §8.27 — snapshots are for creates, edits are ops), and with it
// the state repair a snapshot round trip needed.
type ObjectMutator interface {
	// MutateObject takes the restriction axes the batch actually touches;
	// the adapter re-checks them under the lock (the apply itself runs
	// without object-level restriction checks, so this gate is the only one).
	MutateObject(ctx context.Context, spaceId string, objectId string, needs EditNeeds, apply func(edit ObjectEdit) error) (heads []string, err error)
}

// ObjectProvenance reads an object's creator provenance from validated
// change storage — never from details (APIV2_OBJECT_DELETE.md §10). It is
// the enforcement read behind DELETE /v2/spaces/{space_id}/objects/{object_id}:
//
//   - accountMatch reports the root clause — the tree's signed root carries
//     this account's identity (other members' objects report false; note
//     that some derived shapes DO carry a signed root — the personal space's
//     derive path and every FileObject sign with the account key — so a
//     true here does not by itself mean "user content": the route's sbType
//     allowlist owns that exclusion);
//   - integrationName is the key clause — the raw app name recorded on the
//     first account-signed content change (never normalized, compared
//     exactly by the caller), or "" when none is recorded (legacy objects,
//     app-created objects, the §10 same-account race edge);
//   - err is an infrastructure failure (space/tree unavailable). Callers
//     MUST refuse on err — the rule is fail-closed in every direction.
type ObjectProvenance interface {
	CreatorProvenance(ctx context.Context, spaceId string, objectId string) (accountMatch bool, integrationName string, err error)
}

type ClientCommands interface {
	// Wallet
	AccountLocalLinkNewChallenge(context.Context, *pb.RpcAccountLocalLinkNewChallengeRequest) *pb.RpcAccountLocalLinkNewChallengeResponse
	AccountLocalLinkSolveChallenge(context.Context, *pb.RpcAccountLocalLinkSolveChallengeRequest) *pb.RpcAccountLocalLinkSolveChallengeResponse
	WalletCreateSession(context.Context, *pb.RpcWalletCreateSessionRequest) *pb.RpcWalletCreateSessionResponse

	// Space
	WorkspaceCreate(context.Context, *pb.RpcWorkspaceCreateRequest) *pb.RpcWorkspaceCreateResponse
	WorkspaceOpen(context.Context, *pb.RpcWorkspaceOpenRequest) *pb.RpcWorkspaceOpenResponse
	WorkspaceSetInfo(context.Context, *pb.RpcWorkspaceSetInfoRequest) *pb.RpcWorkspaceSetInfoResponse
	SpaceRequestApprove(context.Context, *pb.RpcSpaceRequestApproveRequest) *pb.RpcSpaceRequestApproveResponse
	SpaceRequestDecline(context.Context, *pb.RpcSpaceRequestDeclineRequest) *pb.RpcSpaceRequestDeclineResponse
	SpaceParticipantRemove(context.Context, *pb.RpcSpaceParticipantRemoveRequest) *pb.RpcSpaceParticipantRemoveResponse
	SpaceParticipantPermissionsChange(context.Context, *pb.RpcSpaceParticipantPermissionsChangeRequest) *pb.RpcSpaceParticipantPermissionsChangeResponse

	// Object
	ObjectShow(context.Context, *pb.RpcObjectShowRequest) *pb.RpcObjectShowResponse
	ObjectCreate(context.Context, *pb.RpcObjectCreateRequest) *pb.RpcObjectCreateResponse
	ObjectCreateBookmark(context.Context, *pb.RpcObjectCreateBookmarkRequest) *pb.RpcObjectCreateBookmarkResponse
	ObjectSearch(context.Context, *pb.RpcObjectSearchRequest) *pb.RpcObjectSearchResponse
	ObjectSearchSubscribe(context.Context, *pb.RpcObjectSearchSubscribeRequest) *pb.RpcObjectSearchSubscribeResponse
	ObjectSearchUnsubscribe(context.Context, *pb.RpcObjectSearchUnsubscribeRequest) *pb.RpcObjectSearchUnsubscribeResponse
	ObjectSetDetails(context.Context, *pb.RpcObjectSetDetailsRequest) *pb.RpcObjectSetDetailsResponse
	ObjectSetIsArchived(context.Context, *pb.RpcObjectSetIsArchivedRequest) *pb.RpcObjectSetIsArchivedResponse
	ObjectListDelete(context.Context, *pb.RpcObjectListDeleteRequest) *pb.RpcObjectListDeleteResponse
	ObjectExport(context.Context, *pb.RpcObjectExportRequest) *pb.RpcObjectExportResponse
	ObjectSetObjectType(context.Context, *pb.RpcObjectSetObjectTypeRequest) *pb.RpcObjectSetObjectTypeResponse

	// Type
	ObjectCreateObjectType(context.Context, *pb.RpcObjectCreateObjectTypeRequest) *pb.RpcObjectCreateObjectTypeResponse

	// List
	ObjectCollectionAdd(context.Context, *pb.RpcObjectCollectionAddRequest) *pb.RpcObjectCollectionAddResponse
	ObjectCollectionRemove(context.Context, *pb.RpcObjectCollectionRemoveRequest) *pb.RpcObjectCollectionRemoveResponse

	// Property
	ObjectRelationAddFeatured(context.Context, *pb.RpcObjectRelationAddFeaturedRequest) *pb.RpcObjectRelationAddFeaturedResponse
	ObjectCreateRelation(context.Context, *pb.RpcObjectCreateRelationRequest) *pb.RpcObjectCreateRelationResponse

	// Tags
	ObjectCreateRelationOption(context.Context, *pb.RpcObjectCreateRelationOptionRequest) *pb.RpcObjectCreateRelationOptionResponse
	RelationListRemoveOption(context.Context, *pb.RpcRelationListRemoveOptionRequest) *pb.RpcRelationListRemoveOptionResponse
	RelationOptions(context.Context, *pb.RpcRelationOptionsRequest) *pb.RpcRelationOptionsResponse

	// File
	FileUpload(context.Context, *pb.RpcFileUploadRequest) *pb.RpcFileUploadResponse

	// Block
	BlockCreate(context.Context, *pb.RpcBlockCreateRequest) *pb.RpcBlockCreateResponse
	BlockPaste(context.Context, *pb.RpcBlockPasteRequest) *pb.RpcBlockPasteResponse
	BlockListDelete(context.Context, *pb.RpcBlockListDeleteRequest) *pb.RpcBlockListDeleteResponse

	// Chat
	ChatAddMessage(context.Context, *pb.RpcChatAddMessageRequest) *pb.RpcChatAddMessageResponse
	ChatEditMessageContent(context.Context, *pb.RpcChatEditMessageContentRequest) *pb.RpcChatEditMessageContentResponse
	ChatToggleMessageReaction(context.Context, *pb.RpcChatToggleMessageReactionRequest) *pb.RpcChatToggleMessageReactionResponse
	ChatDeleteMessage(context.Context, *pb.RpcChatDeleteMessageRequest) *pb.RpcChatDeleteMessageResponse
	ChatGetMessages(context.Context, *pb.RpcChatGetMessagesRequest) *pb.RpcChatGetMessagesResponse
	ChatGetMessagesByIds(context.Context, *pb.RpcChatGetMessagesByIdsRequest) *pb.RpcChatGetMessagesByIdsResponse
	ChatReadMessages(context.Context, *pb.RpcChatReadMessagesRequest) *pb.RpcChatReadMessagesResponse
	ChatReadReactions(context.Context, *pb.RpcChatReadReactionsRequest) *pb.RpcChatReadReactionsResponse
	ChatSearch(context.Context, *pb.RpcChatSearchRequest) *pb.RpcChatSearchResponse
}
