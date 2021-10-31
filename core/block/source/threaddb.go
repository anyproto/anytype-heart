package source

import (
	"context"
	"fmt"
	"github.com/textileio/go-threads/core/db"
	"github.com/textileio/go-threads/core/thread"
	threadsUtil "github.com/textileio/go-threads/util"
	"strings"
	"sync"

	"github.com/anytypeio/go-anytype-middleware/change"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
	threadsDb "github.com/textileio/go-threads/db"
)

const (
	collectionKeyId       = "id"
	collectionKeyKey      = "key"
	collectionKeyAddrs    = "addrs"
	WorkspaceCollection   = "threadDB"
	CreatorCollection     = "creator"
	AccountMigration      = "accountmigration"
	HighlightedCollection = "highlighted"
)

func NewThreadDB(a core.Service, id string) (s Source) {
	return &threadDB{
		id: id,
		a:  a,
	}
}

type threadDB struct {
	id string
	a  core.Service
	m  sync.Mutex

	receiver ChangeReceiver
	listener threadsDb.Listener
	ctx      context.Context
	cancel   context.CancelFunc
}

func (v *threadDB) ReadOnly() bool {
	return false
}

func (v *threadDB) Id() string {
	return v.id
}

func (v *threadDB) Anytype() core.Service {
	return v.a
}

func (v *threadDB) Type() model.SmartBlockType {
	return model.SmartBlockType_AccountOld
}

func (v *threadDB) Virtual() bool {
	return true
}

func (v *threadDB) ReadDoc(receiver ChangeReceiver, empty bool) (doc state.Doc, err error) {
	threads.WorkspaceLogger.
		With("workspace id", v.id).
		Debug("reading document for workspace")

	s, err := v.createState()
	if err != nil {
		return nil, err
	}

	v.receiver = receiver

	go v.listenToChanges()

	return s, nil
}

func (v *threadDB) ReadMeta(_ ChangeReceiver) (doc state.Doc, err error) {
	return v.createState()
}

func (v *threadDB) PushChange(params PushChangeParams) (id string, err error) {
	return "", nil
}

func (v *threadDB) FindFirstChange(ctx context.Context) (c *change.Change, err error) {
	return nil, change.ErrEmpty
}

func (v *threadDB) ListIds() ([]string, error) {
	return v.a.GetAllWorkspaces()
}

func (v *threadDB) Close() (err error) {
	v.m.Lock()
	defer v.m.Unlock()
	if v.listener == nil {
		return
	}

	threads.WorkspaceLogger.
		With("workspace id", v.id).
		Debug("closing listener channel")
	v.cancel()
	v.listener.Close()
	v.listener = nil

	return
}

func (v *threadDB) LogHeads() map[string]string {
	return nil
}

func (v *threadDB) listenToChanges() (err error) {
	v.m.Lock()
	defer v.m.Unlock()

	if v.listener != nil {
		return
	}

	v.listener, err = v.Anytype().ThreadsService().ThreadsDB().Listen()
	if err != nil {
		return
	}

	v.ctx, v.cancel = context.WithCancel(context.Background())

	go func() {
		for {
			select {
			case action := <-v.listener.Channel():
				if strings.HasPrefix(action.Collection, threads.ThreadInfoCollectionName) {
					go v.processThreadAction(action)
				}
			case <-v.ctx.Done():
				return
			}
		}
	}()
	threads.WorkspaceLogger.
		With("workspace id", v.id).
		Debug("started listening to db changes")
	return nil
}

func (v *threadDB) threadInfoValue(actionId db.InstanceID) (*types.Value, error) {
	tc := v.Anytype().ThreadsService().ThreadsCollection()
	instanceBytes, err := tc.FindByID(actionId)
	if err != nil {
		return nil, err
	}

	ti := threads.ThreadDBInfo{}
	threadsUtil.InstanceFromJSON(instanceBytes, &ti)
	tid, err := thread.Decode(ti.ID.String())
	if err != nil {
		return nil, err
	}
	return &types.Value{
		Kind: &types.Value_StructValue{
			StructValue: &types.Struct{
				Fields: map[string]*types.Value{
					collectionKeyId:    pbtypes.String(tid.String()),
					collectionKeyKey:   pbtypes.String(ti.Key),
					collectionKeyAddrs: pbtypes.StringList(ti.Addrs),
				},
			},
		},
	}, nil
}

func (v *threadDB) processThreadAction(action threadsDb.Action) {
	threads.WorkspaceLogger.
		With("workspace id", v.id).
		With("thread id", action.ID).
		Debug("processing new thread to link")

	err := v.receiver.StateAppend(func(d state.Doc) (s *state.State, err error) {
		s, ok := d.(*state.State)
		if !ok {
			err = fmt.Errorf("doc is not state")
			return
		}
		v.m.Lock()
		defer v.m.Unlock()

		if action.Type == threadsDb.ActionCreate {
			val, err := v.threadInfoValue(action.ID)
			if err != nil {
				return nil, err
			}
			s.SetInCollection([]string{WorkspaceCollection, action.ID.String()}, val)
		} else if action.Type == threadsDb.ActionDelete {
			s.RemoveFromCollection([]string{WorkspaceCollection, action.ID.String()})
		}
		return
	})
	if err != nil {
		log.Errorf("failed to append state with new workspace thread: %v", err)
	}
}

func (v *threadDB) getDetails() (p *types.Struct) {
	details := &types.Struct{Fields: map[string]*types.Value{
		bundle.RelationKeyId.String():          pbtypes.String(v.id),
		bundle.RelationKeyIsReadonly.String():  pbtypes.Bool(true),
		bundle.RelationKeyIsArchived.String():  pbtypes.Bool(false), // todo: replace with true
		bundle.RelationKeyIsHidden.String():    pbtypes.Bool(false), // todo: replace with true
		bundle.RelationKeyType.String():        pbtypes.String(bundle.TypeKeySpace.URL()),
		bundle.RelationKeyIsHidden.String():    pbtypes.Bool(false),
		bundle.RelationKeyLayout.String():      pbtypes.Float64(float64(model.ObjectType_space)),
		bundle.RelationKeyIconEmoji.String():   pbtypes.String("🌎"),
		bundle.RelationKeyWorkspaceId.String(): pbtypes.String(v.Id()),
		bundle.RelationKeyName.String():        pbtypes.String("Old account thread"),
	}}

	return details
}

func (v *threadDB) createState() (*state.State, error) {
	s := state.NewDoc(v.id, nil).(*state.State)
	objects, err := v.a.ThreadsService().GetAllThreadsInOldAccount()
	if err != nil {
		return nil, err
	}

	for _, objId := range objects {
		val, err := v.threadInfoValue(db.InstanceID(objId))
		if err != nil {
			log.Errorf("threadsDb source createState error: %s", err.Error())
			continue
		}

		s.SetInCollection([]string{WorkspaceCollection, objId}, val)
	}

	s.SetDetails(v.getDetails())

	return s, nil
}
