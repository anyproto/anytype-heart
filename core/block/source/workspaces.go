package source

import (
	"context"
	"fmt"
	"sync"

	"github.com/anytypeio/go-anytype-middleware/change"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/globalsign/mgo/bson"
	threadsDb "github.com/textileio/go-threads/db"
)

const workspaceTitlePageKey = "_workspace_title_page"

func NewWorkspaces(a core.Service, id string) (s Source) {
	return &workspaces{
		id: id,
		a:  a,
	}
}

type workspaces struct {
	id string
	a  core.Service
	m  sync.Mutex

	receiver ChangeReceiver
	listener threadsDb.Listener
	ctx      context.Context
	cancel   context.CancelFunc
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
				if action.Collection == threads.GetThreadCollectionName(v.id) {
					go v.processThreadAction(action)
				} else {
					go v.processMetaAction(action)
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

func (v *workspaces) processThreadAction(action threadsDb.Action) {
	if action.Type != threadsDb.ActionCreate {
		return
	}
	threads.WorkspaceLogger.
		With("workspace id", v.id).
		With("thread id", action.ID).
		Info("processing new thread to link")
	link := simple.New(&model.Block{
		Content: &model.BlockContentOfLink{
			Link: &model.BlockContentLink{
				TargetBlockId: action.ID.String(),
				Style:         model.BlockContentLink_Page,
			},
		},
	})
	err := v.receiver.StateAppend(func(d state.Doc) (s *state.State, err error) {
		s, ok := d.(*state.State)
		if !ok {
			err = fmt.Errorf("doc is not state")
			return
		}

		s.Add(link)
		err = s.InsertTo("", model.Block_Inner, link.Model().Id)
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
	mention := WorkspaceTitleMention(workspaceTitlePageKey, meta.TitleWorkspaceId())
	err = v.receiver.StateAppend(func(d state.Doc) (s *state.State, err error) {
		s, ok := d.(*state.State)
		if !ok {
			err = fmt.Errorf("doc is not state")
			return
		}
		b := s.Get(workspaceTitlePageKey)
		if b == nil {
			err = fmt.Errorf("workspace title should not be empty")
			return
		}
		_, empty := b.Model().Content.(*model.BlockContentOfSmartblock)
		if !empty {
			// converting previous mention to link
			previousObjectId := b.Model().Content.(*model.BlockContentOfText).Text.Marks.Marks[0].Param
			if previousObjectId == meta.TitleWorkspaceId() {
				return
			}
			link := simple.New(&model.Block{
				Content: &model.BlockContentOfLink{
					Link: &model.BlockContentLink{
						TargetBlockId: previousObjectId,
						Style:         model.BlockContentLink_Page,
					},
				},
			})
			s.Add(link)
			err = s.InsertTo("", model.Block_Inner, link.Model().Id)
			if err != nil {
				return
			}

			// removing current mention as link
			s.Iterate(func(b simple.Block) (isContinue bool) {
				if link := b.Model().GetLink(); link != nil && link.TargetBlockId == meta.TitleWorkspaceId() {
					s.Unlink(b.Model().Id)
					return false
				}
				return true
			})
		}

		s.Set(simple.New(mention))
		return
	})
	if err != nil {
		log.Errorf("failed to append state with new workspace thread: %v", err)
	}

}

func (v *workspaces) createState() (*state.State, error) {
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

	var blocks []*model.Block

	for _, objId := range objects {
		if meta != nil && meta.TitleWorkspaceId() == objId {
			continue
		}
		link := &model.Block{
			Id: bson.NewObjectId().Hex(),
			Content: &model.BlockContentOfLink{
				Link: &model.BlockContentLink{
					TargetBlockId: objId,
					Style:         model.BlockContentLink_Page,
				},
			},
		}
		threads.WorkspaceLogger.
			With("workspace id", v.id).
			With("thread id", objId).
			Info("adding initial link")
		blocks = append(blocks, link)
	}
	if meta != nil && meta.TitleWorkspaceId() != "" {
		mention := WorkspaceTitleMention(workspaceTitlePageKey, meta.TitleWorkspaceId())
		blocks = append([]*model.Block{mention}, blocks...)
	} else {
		emptyMention := &model.Block{
			Id: workspaceTitlePageKey,
			Content: &model.BlockContentOfSmartblock{
				Smartblock: &model.BlockContentSmartblock{},
			},
		}
		blocks = append([]*model.Block{emptyMention}, blocks...)
	}
	s := state.NewDoc(v.id, nil).(*state.State)
	initBlocksAndAddToRoot(s, blocks)

	if meta == nil {
		lastSymbols := v.id[len(v.id)-4 : len(v.id)]
		s.SetDetail(bundle.RelationKeyName.String(), pbtypes.String("Workspace_"+lastSymbols))
	} else {
		s.SetDetail(bundle.RelationKeyName.String(), pbtypes.String(meta.WorkspaceName()))
	}

	return s, nil
}

func initBlocksAndAddToRoot(s *state.State, blocks []*model.Block) {
	// we could have used template.WithRootBlocks, but these causes circular references
	s.Add(simple.New(&model.Block{
		Id: s.RootId(),
		Content: &model.BlockContentOfSmartblock{
			Smartblock: &model.BlockContentSmartblock{},
		},
	}))

	for _, block := range blocks {
		if block.Id == "" {
			panic("blocks must contains exact ids")
		}
		s.Add(simple.New(block))
		err := s.InsertTo(s.RootId(), model.Block_Inner, block.Id)
		if err != nil {
			log.Errorf("template WithDataview failed to insert: %w", err)
		}
	}
}

func WorkspaceTitleMention(objectId string, targetId string) *model.Block {
	return &model.Block{
		Id: objectId,
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text: "",
				Marks: &model.BlockContentTextMarks{
					Marks: []*model.BlockContentTextMark{
						{
							Range: &model.Range{0, 0},
							Type:  model.BlockContentTextMark_Mention,
							Param: targetId,
						},
					},
				},
				Style: model.BlockContentText_Header1,
			},
		},
	}
}
