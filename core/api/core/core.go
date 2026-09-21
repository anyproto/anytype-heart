package apicore

import (
	"context"
	"errors"

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
	BlocksRefused     error
	DetailsRefused    error
	TypeChangeRefused error
}

// EditNeeds declares which object-level restriction axes an edit touches, so
// the gate can be per-op instead of per-request. Item ops (add_items /
// remove_items) need NEITHER: they mutate the collection store, which no
// object restriction governs — matching v1's ObjectCollectionAdd.
type EditNeeds struct {
	Blocks     bool
	Details    bool
	TypeChange bool
}

// ObjectReader reads the live smartblock state of an object — the API v2
// read path (APIV2.md §8: not ObjectShow, not the store snapshot).
type ObjectReader interface {
	ReadObject(ctx context.Context, spaceId string, objectId string) (ObjectRead, error)
}

// ErrTemplateUnavailable is what CreateObjectFromSnapshot returns when the
// template it was handed could not be turned into a state: deleted or
// archived since the caller's id was checked, or unloadable.
//
// It exists because the two callers of that outcome want opposite things. A
// template the CALLER named must refuse — the object would otherwise be
// created without the content the request asked for, under a response naming
// the template. A template the TYPE named must not fail the create at all;
// the create is retried without it and the result says so. Neither decision
// belongs in the adapter, which knows nothing of who chose the id, so the
// adapter reports the fact and the API layer decides.
var ErrTemplateUnavailable = errors.New("template unavailable")

// ErrSpaceReadOnly is InstallBundledType's answer for a space this account
// can only read: the installer itself returns success without installing
// there, which would otherwise look like an install that never became
// readable.
var ErrSpaceReadOnly = errors.New("space is read-only for this account")

// ObjectCreator creates objects from AnyBlock snapshots — the API v2 create
// path (APIV2.md §2 Phase 2). CreateObjectFromSnapshot builds the object's
// initial state from the snapshot and creates it in one change set (atomic
// composite creates, §8/R10); the object's type keys come from the
// snapshot's ObjectTypes. TypeIdByKey derives the space-local object id of a
// type key (needed for setOf/targetObjectType details before the object
// exists). RelationIdByKey is the relation twin: derived objects' ids are a
// pure function of (space, kind, internal key) (ADDRESSING §2.4), so the id
// is computable whether or not the relation object — or even its index
// row — exists; the corpse probes use it to see a tombstone an older build
// left, which no key-filtered query can return (§8.41).
//
// templateId, when set, is the template the new object starts from: the
// adapter builds the same state a client-initiated create builds from it and
// rebases the document on top. It is a store id the CALLER's layer has
// already validated (v2service.resolveCreateTemplate) — the adapter resolves
// nothing and applies what it is given, so a template that has vanished
// between the two is an error here, not a silent blank object.
type ObjectCreator interface {
	CreateObjectFromSnapshot(ctx context.Context, spaceId string, snapshot *model.SmartBlockSnapshotBase, templateId string) (CreateOutcome, error)
	TypeIdByKey(ctx context.Context, spaceId string, key domain.TypeKey) (string, error)
	RelationIdByKey(ctx context.Context, spaceId string, key domain.RelationKey) (string, error)
	// InstallBundledType installs a bundled type into the space, the way a
	// create of an object of that type does — a no-op when it is already
	// installed. The editor's type change reads the target type from the
	// space's store, so an uninstalled bundled target has to be installed
	// first, and outside the object lock, since an install is its own
	// object create.
	InstallBundledType(ctx context.Context, spaceId string, key domain.TypeKey) error
}

// CreateOutcome is what a snapshot create produced. It is a struct rather
// than a bare id because a create that started from a template did something
// to the object the caller cannot infer from the request: TemplateBlocks is
// how many blocks the template contributed, so the response can say that the
// object holds content the request did not send without the caller reading
// the object back to find out.
//
// The count excludes the header the object would carry either way (its
// title, description and featured relations): a number that moved because an
// object has a title would say nothing about the template.
type CreateOutcome struct {
	Id             string
	TemplateBlocks int
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
	// SetObjectType changes the object's type IN st through the editor's own
	// path (basic.SetObjectTypesInState): the type-change and layout
	// restrictions, the template refusal and the layout conversion that
	// rewrites the document all run there, on the same child state the
	// ops edit, so later ops in the batch see the converted document. Nil
	// when no editor is behind the state — a dry run — and the applier then
	// records the new type keys alone and says the conversion is not
	// simulated.
	SetObjectType func(st *state.State, key domain.TypeKey) error
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
	ObjectCrossSpaceSearch(context.Context, *pb.RpcObjectCrossSpaceSearchRequest) *pb.RpcObjectCrossSpaceSearchResponse
	ObjectSearchSubscribe(context.Context, *pb.RpcObjectSearchSubscribeRequest) *pb.RpcObjectSearchSubscribeResponse
	ObjectSearchUnsubscribe(context.Context, *pb.RpcObjectSearchUnsubscribeRequest) *pb.RpcObjectSearchUnsubscribeResponse
	ObjectSetDetails(context.Context, *pb.RpcObjectSetDetailsRequest) *pb.RpcObjectSetDetailsResponse
	ObjectSetIsArchived(context.Context, *pb.RpcObjectSetIsArchivedRequest) *pb.RpcObjectSetIsArchivedResponse
	ObjectListSetIsFavorite(context.Context, *pb.RpcObjectListSetIsFavoriteRequest) *pb.RpcObjectListSetIsFavoriteResponse
	ObjectListDelete(context.Context, *pb.RpcObjectListDeleteRequest) *pb.RpcObjectListDeleteResponse
	ObjectExport(context.Context, *pb.RpcObjectExportRequest) *pb.RpcObjectExportResponse
	ObjectSetObjectType(context.Context, *pb.RpcObjectSetObjectTypeRequest) *pb.RpcObjectSetObjectTypeResponse
	ObjectAddDiscussion(context.Context, *pb.RpcObjectDiscussionAddRequest) *pb.RpcObjectDiscussionAddResponse

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

// WidgetScope names which sidebar root a widget lives in. A space has two:
// the space root (the desktop's "Pinned" section, shared by every member and
// writable by the owner and admins only) and the personal root (the desktop's
// "My Favorites", this account's own, held in the tech space).
type WidgetScope string

const (
	WidgetScopeSpace    WidgetScope = "space"
	WidgetScopePersonal WidgetScope = "personal"
)

// WidgetEntry is one sidebar widget as the root holds it: the wrapper block
// (Id, what every mutation addresses), its link child (LinkId), and the
// members the desktop reads. Target is the STORED spelling — an object id or
// a bare built-in listing id such as favorite — never the format's
// underscore spelling.
type WidgetEntry struct {
	Id     string
	LinkId string
	Scope  WidgetScope
	Target string
	Layout model.BlockContentWidgetLayout
	Limit  int32
	ViewId string
	// Placed is set on the entry a create or an update answers when the
	// request placed the widget: the placement as it was applied under the
	// lock, in wrapper ids — which may differ from what the caller asked
	// (a default placement lands before a bin that became last meanwhile).
	Placed *WidgetPlacement
}

// WidgetPlacement says where a widget goes in its root's order: after or
// before a sibling wrapper id, at the head (First), or — the zero value — at
// the end.
type WidgetPlacement struct {
	AfterId  string
	BeforeId string
	First    bool
}

// WidgetCreate is what a create writes: the stored target spelling and the
// wrapper's members, already validated by the caller — the adapter checks
// shape, not vocabulary.
type WidgetCreate struct {
	Target    string
	Layout    model.BlockContentWidgetLayout
	Limit     int32
	ViewId    string
	Placement WidgetPlacement
}

// WidgetUpdate is a partial update of one wrapper: nil leaves a member as
// it is. The target is immutable — the desktop cannot change it either, and
// the personal root's store models a target change as delete + create.
//
// An update is PLANNED under the root's lock: UpdateWidget hands the plan
// the widget as it is at that moment, so a layout and a limit validated
// against each other are validated against the stored pair, not against
// a read that another request may have overtaken.
type WidgetUpdate struct {
	Layout    *model.BlockContentWidgetLayout
	Limit     *int32
	ViewId    *string
	Placement *WidgetPlacement
}

// ErrWidgetNotFound is what a widget mutation returns when the wrapper id
// names no widget in the root.
var ErrWidgetNotFound = errors.New("widget not found")

// ErrBinPlacement is what a placement returns when, under the lock, it
// would put a widget after the bin widget: the desktop lands every drop
// near the bin before it, and never lets the bin itself be dragged.
var ErrBinPlacement = errors.New("nothing goes after the bin widget")

// ErrWidgetRetargeted is what a delete returns when the widget's target under
// the lock is not the one the caller resolved it by.
var ErrWidgetRetargeted = errors.New("widget target changed")

// ErrWidgetExists is what a create returns when the root already holds a
// widget for the target. The service checks this before calling, but only
// the adapter checks it under the root's lock, so two concurrent creates
// for one target cannot both pass.
var ErrWidgetExists = errors.New("widget for this target already exists")

// Widgets is the sidebar-widget port of API v2: the two widget roots of a
// space read and written through the same editor path the desktop uses (one
// locked state, one Apply per request), so ordering, wrapper+link pairing
// and the personal root's store projection all ride the existing machinery.
type Widgets interface {
	// CanEditWidgets reports whether this account may write the root of
	// scope in the space: the owner or an admin for the space root, any
	// writing participant for the personal root.
	CanEditWidgets(ctx context.Context, spaceId string, scope WidgetScope) (bool, error)
	ListWidgets(ctx context.Context, spaceId string, scope WidgetScope) ([]WidgetEntry, error)
	CreateWidget(ctx context.Context, spaceId string, scope WidgetScope, create WidgetCreate) (WidgetEntry, error)
	UpdateWidget(ctx context.Context, spaceId string, scope WidgetScope, widgetId string, plan func(current WidgetEntry) (WidgetUpdate, error)) (WidgetEntry, error)
	// DeleteWidget removes the wrapper and its link. expectedTarget is the
	// target the caller resolved the widget by; a widget retargeted since
	// (heart's own RPC can) is refused with ErrWidgetRetargeted rather than
	// removed under a stale name. The removed entry is answered.
	DeleteWidget(ctx context.Context, spaceId string, scope WidgetScope, widgetId string, expectedTarget string) (WidgetEntry, error)
}
