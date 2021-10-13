package source

import (
	"context"
	"fmt"
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

const WorkspaceCollection = "workspaces"

func NewWorkspaces(a core.Service, id string) (s Source) {
	return &workspaces{
		id: id,
		a:  a,
	}
}

type workspaceDetailsConverter struct{}

func (w *workspaceDetailsConverter) ConvertToDetails(st *state.State) map[string]*types.Struct {
	injectedDetails := make(map[string]*types.Struct)

	workspaceCollection := st.GetCollection(WorkspaceCollection)
	if workspaceCollection != nil {
		for objId, workspaceId := range workspaceCollection {
			if injectedDetails[objId] == nil {
				injectedDetails[objId] = &types.Struct{Fields: map[string]*types.Value{}}
			}
			injectedDetails[objId].Fields[bundle.RelationKeyWorkspaceId.String()] = pbtypes.String(workspaceId.(string))
		}
	}

	highlightedCollection := st.GetCollection(threads.HighlightedCollectionName)
	if highlightedCollection != nil {
		for objId, isHighlighted := range highlightedCollection {
			if injectedDetails[objId] == nil {
				injectedDetails[objId] = &types.Struct{Fields: map[string]*types.Value{}}
			}
			injectedDetails[objId].Fields[bundle.RelationKeyIsHighlighted.String()] = pbtypes.Bool(isHighlighted.(bool))
		}
	}
	return injectedDetails
}

type workspaces struct {
	id string
	a  core.Service
	m  sync.Mutex

	receiver  ChangeReceiver
	listener  threadsDb.Listener
	processor threads.ThreadProcessor
	ctx       context.Context
	cancel    context.CancelFunc
}

func (v *workspaces) ReadOnly() bool {
	return false
}

func (v *workspaces) Id() string {
	return v.id
}

func (v *workspaces) Anytype() core.Service {
	return v.a
}

func (v *workspaces) Type() model.SmartBlockType {
	return model.SmartBlockType_Workspace
}

func (v *workspaces) Virtual() bool {
	return true
}

func (v *workspaces) ReadDoc(receiver ChangeReceiver, empty bool) (doc state.Doc, err error) {
	threads.WorkspaceLogger.
		With("workspace id", v.id).
		Info("reading document for workspace")

	s, err := v.createState()
	if err != nil {
		return nil, err
	}

	v.receiver = receiver

	go v.listenToChanges()

	return s, nil
}

func (v *workspaces) ReadMeta(_ ChangeReceiver) (doc state.Doc, err error) {
	return v.createState()
}

func (v *workspaces) PushChange(params PushChangeParams) (id string, err error) {
	return "", nil
}

func (v *workspaces) FindFirstChange(ctx context.Context) (c *change.Change, err error) {
	return nil, change.ErrEmpty
}

func (v *workspaces) ListIds() ([]string, error) {
	return v.a.GetAllWorkspaces()
}

func (v *workspaces) Close() (err error) {
	v.m.Lock()
	defer v.m.Unlock()
	if v.listener == nil {
		return
	}

	threads.WorkspaceLogger.
		With("workspace id", v.id).
		Info("closing listener channel")
	v.cancel()
	v.listener.Close()
	v.listener = nil

	return
}

func (v *workspaces) LogHeads() map[string]string {
	return nil
}

func (v *workspaces) listenToChanges() (err error) {
	v.m.Lock()
	defer v.m.Unlock()

	if v.listener != nil {
		return
	}

	v.listener, err = v.a.GetThreadActionsListenerForWorkspace(v.id)
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
				} else if strings.HasPrefix(action.Collection, threads.MetaCollectionName) {
					go v.processMetaAction(action)
				} else {
					go v.processHighlightedAction(action)
				}
			case <-v.ctx.Done():
				return
			}
		}
	}()
	threads.WorkspaceLogger.
		With("workspace id", v.id).
		Info("started listening to db changes")
	return nil
}

func (v *workspaces) processHighlightedAction(action threadsDb.Action) {
	threads.WorkspaceLogger.
		With("workspace id", v.id).
		With("thread id", action.ID).
		Info("processing new thread to highlight")

	err := v.receiver.StateAppend(func(d state.Doc) (s *state.State, err error) {
		s, ok := d.(*state.State)
		if !ok {
			err = fmt.Errorf("doc is not state")
			return
		}
		v.m.Lock()
		defer v.m.Unlock()
		if action.Type == threadsDb.ActionSave {
			s.SetInCollection(threads.HighlightedCollectionName, action.ID.String(), true)
		} else if action.Type == threadsDb.ActionDelete {
			s.SetInCollection(threads.HighlightedCollectionName, action.ID.String(), false)
		}
		return
	})
	if err != nil {
		log.Errorf("failed to append state with new workspace thread: %v", err)
	}
}

func (v *workspaces) processThreadAction(action threadsDb.Action) {
	threads.WorkspaceLogger.
		With("workspace id", v.id).
		With("thread id", action.ID).
		Info("processing new thread to link")

	err := v.receiver.StateAppend(func(d state.Doc) (s *state.State, err error) {
		s, ok := d.(*state.State)
		if !ok {
			err = fmt.Errorf("doc is not state")
			return
		}
		v.m.Lock()
		defer v.m.Unlock()
		if action.Type == threadsDb.ActionCreate {
			s.SetInCollection(WorkspaceCollection, action.ID.String(), v.id)
			s.SetInCollection(threads.HighlightedCollectionName, action.ID.String(), false)
		} else if action.Type == threadsDb.ActionDelete {
			s.RemoveFromCollection(WorkspaceCollection, action.ID.String())
			s.RemoveFromCollection(threads.HighlightedCollectionName, action.ID.String())
		}
		return
	})
	if err != nil {
		log.Errorf("failed to append state with new workspace thread: %v", err)
	}
}

func (v *workspaces) processMetaAction(action threadsDb.Action) {
	meta, err := v.a.GetLatestWorkspaceMeta(v.id)
	if err != nil {
		log.Errorf("failed to get workspace meta: %v", err)
		return
	}
	err = v.receiver.StateAppend(func(d state.Doc) (s *state.State, err error) {
		s, ok := d.(*state.State)
		if !ok {
			err = fmt.Errorf("doc is not state")
			return
		}

		v.m.Lock()
		defer v.m.Unlock()
		s.SetDetail(bundle.RelationKeyName.String(), pbtypes.String(meta.WorkspaceName()))
		return
	})
	if err != nil {
		log.Errorf("failed to append state with new workspace thread: %v", err)
	}
}

func (v *workspaces) getDetails(workspaceName string) (p *types.Struct) {
	return &types.Struct{Fields: map[string]*types.Value{
		bundle.RelationKeyName.String():        pbtypes.String(workspaceName),
		bundle.RelationKeyId.String():          pbtypes.String(v.id),
		bundle.RelationKeyIsReadonly.String():  pbtypes.Bool(true),
		bundle.RelationKeyIsArchived.String():  pbtypes.Bool(false),
		bundle.RelationKeyType.String():        pbtypes.String(bundle.TypeKeySpace.URL()),
		bundle.RelationKeyIsHidden.String():    pbtypes.Bool(false),
		bundle.RelationKeyLayout.String():      pbtypes.Float64(float64(model.ObjectType_space)),
		bundle.RelationKeyIconEmoji.String():   pbtypes.String("🌎"),
		bundle.RelationKeyWorkspaceId.String(): pbtypes.String(v.Id()),
	}}
}

func (v *workspaces) createState() (*state.State, error) {
	var err error
	v.processor, err = v.a.GetThreadProcessorForWorkspace(v.id)
	if err != nil {
		return nil, err
	}
	_, err = v.processor.AddCollectionWithPrefix(threads.HighlightedCollectionName, threads.CollectionUpdateInfo{})
	if err != nil {
		return nil, err
	}
	threads.WorkspaceLogger.
		With("workspace id", v.id).
		Info("finished adding collections in workspace")

	s := state.NewDoc(v.id, nil).(*state.State)
	s.DetailsCollectionConverter = &workspaceDetailsConverter{}

	meta, err := v.a.GetLatestWorkspaceMeta(v.id)
	if err != nil {
		threads.WorkspaceLogger.
			With("workspace id", v.id).
			Errorf("could not get latest meta: %v", err)
		meta = nil
	}

	objects, err := v.a.GetAllObjectsInWorkspace(v.id)
	if err != nil {
		return nil, err
	}

	for _, objId := range objects {
		s.SetInCollection(WorkspaceCollection, objId, v.id)
		s.SetInCollection(threads.HighlightedCollectionName, objId, false)
	}

	err = v.getCurrentHighlightedState(s)
	if err != nil {
		return nil, err
	}

	var workspaceName string
	if meta == nil {
		lastSymbols := v.id[len(v.id)-4 : len(v.id)]
		workspaceName = "Workspace_" + lastSymbols
	} else {
		workspaceName = meta.WorkspaceName()
	}
	s.SetDetails(v.getDetails(workspaceName))

	return s, nil
}

func (v *workspaces) getCurrentHighlightedState(s *state.State) error {
	collection := v.processor.GetCollectionWithPrefix(threads.HighlightedCollectionName)
	if collection == nil {
		threads.WorkspaceLogger.
			With("workspace id", v.id).
			Error("no highlighted collection")
		return nil
	}
	instancesBytes, err := collection.Find(&threadsDb.Query{})
	if err != nil {
		return err
	}

	for _, instanceBytes := range instancesBytes {
		collectionUpdate := threads.CollectionUpdateInfo{}
		threadsUtil.InstanceFromJSON(instanceBytes, &collectionUpdate)

		s.SetInCollection(threads.HighlightedCollectionName, collectionUpdate.ID.String(), true)
	}

	return nil
}
