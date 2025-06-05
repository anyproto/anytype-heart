package source

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/synctree/updatelistener"
	"github.com/anyproto/any-sync/commonspace/objecttreebuilder"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
)

var (
	ErrReadOnly          = errors.New("object is read only")
	ErrBigChangeSize     = errors.New("change size is above the limit")
	ErrUnknownDataFormat = fmt.Errorf("unknown data format: you may need to upgrade anytype in order to open this page")
)

type PushChangeHook func(params PushChangeParams) (id string, err error)

type PushStoreChangeParams struct {
	State   *storestate.StoreState
	Changes []*pb.StoreChangeContent
	Time    time.Time // used to derive the lastModifiedDate; Default is time.Now()
}

type ObjectTreeProvider interface {
	Tree() objecttree.ObjectTree
}

type Space interface {
	Id() string
	TreeBuilder() objecttreebuilder.TreeBuilder
	GetRelationIdByKey(ctx context.Context, key domain.RelationKey) (id string, err error)
	GetTypeIdByKey(ctx context.Context, key domain.TypeKey) (id string, err error)
	DeriveObjectID(ctx context.Context, uniqueKey domain.UniqueKey) (id string, err error)
	StoredIds() []string
	IsPersonal() bool
}

type Service interface {
	NewSource(ctx context.Context, space Space, id string, buildOptions BuildOptions) (source Source, err error)
	RegisterStaticSource(s Source) error
	NewStaticSource(params StaticSourceParams) SourceWithType

	DetailsFromIdBasedSource(id domain.FullID) (*domain.Details, error)
	IDsListerBySmartblockType(space Space, blockType smartblock.SmartBlockType) (IDsLister, error)
	app.Component
}

type ReadStoreTreeHook interface {
	BeforeIteration(ot objecttree.ObjectTree)
	OnIteration(ot objecttree.ObjectTree, change *objecttree.Change)
	AfterDiffManagersInit(ctx context.Context) error
}

type ReadStoreDocParams struct {
	OnUpdateHook      func()
	ReadStoreTreeHook ReadStoreTreeHook
}

type Store interface {
	Source
	ReadStoreDoc(ctx context.Context, stateStore *storestate.StoreState, params ReadStoreDocParams) (err error)
	PushStoreChange(ctx context.Context, params PushStoreChangeParams) (changeId string, err error)
	SetPushChangeHook(onPushChange PushChangeHook)

	// RegisterDiffManager sets a hook that will be called when a change is removed (marked as read) from the diff manager
	// must be called before ReadStoreDoc.
	//
	// If a head is marked as read in the diff manager, all earlier heads for that branch marked as read as well
	RegisterDiffManager(name string, onRemoveHook func(removed []string))
	// MarkSeenHeads marks heads as seen in a diff manager. Then the diff manager will call a hook from SetDiffManagerOnRemoveHook
	MarkSeenHeads(ctx context.Context, name string, heads []string) error
	// StoreSeenHeads persists current seen heads in any-store
	StoreSeenHeads(ctx context.Context, name string) error
	// InitDiffManager initializes a diff manager with specified seen heads
	InitDiffManager(ctx context.Context, name string, seenHeads []string) error
}

type PushChangeParams struct {
	State             *state.State
	Changes           []*pb.ChangeContent
	FileChangedHashes []string
	Time              time.Time // used to derive the lastModifiedDate; Default is time.Now()
	DoSnapshot        bool
}

type IDsLister interface {
	ListIds() ([]string, error)
}

type Source interface {
	Id() string
	SpaceID() string
	Type() smartblock.SmartBlockType
	Heads() []string
	GetFileKeysSnapshot() []*pb.ChangeFileKeys
	ReadOnly() bool
	ReadDoc(ctx context.Context, receiver ChangeReceiver, empty bool) (doc state.Doc, err error)
	PushChange(params PushChangeParams) (id string, err error)
	Close() (err error)
	GetCreationInfo() (creatorObjectId string, createdDate int64, err error)
}

type ChangeReceiver interface {
	StateAppend(func(d state.Doc) (s *state.State, changes []*pb.ChangeContent, err error)) error
	StateRebuild(d state.Doc) (err error)
}

type SourceWithType interface {
	Source
	IDsLister
}

type BuildOptions struct {
	DisableRemoteLoad bool
	Listener          updatelistener.UpdateListener
}

func (b *BuildOptions) BuildTreeOpts() objecttreebuilder.BuildTreeOpts {
	return objecttreebuilder.BuildTreeOpts{
		Listener: b.Listener,
		TreeBuilder: func(treeStorage objecttree.Storage, aclList list.AclList) (objecttree.ObjectTree, error) {
			ot, err := objecttree.BuildKeyFilterableObjectTree(treeStorage, aclList)
			if err != nil {
				return nil, err
			}
			sbt, _, err := typeprovider.GetTypeAndKeyFromRoot(ot.Header())
			if err != nil {
				return nil, err
			}
			if sbt == smartblock.SmartBlockTypeChatDerivedObject || sbt == smartblock.SmartBlockTypeAccountObject {
				ot.SetFlusher(objecttree.MarkNewChangeFlusher())
			}
			return ot, nil
		},
		TreeValidator: objecttree.ValidateFilterRawTree,
	}
}

type StaticSourceParams struct {
	Id        domain.FullID
	SbType    smartblock.SmartBlockType
	State     *state.State
	CreatorId string
}
