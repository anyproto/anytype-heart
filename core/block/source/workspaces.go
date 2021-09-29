package source

import (
	"context"
	"github.com/anytypeio/go-anytype-middleware/change"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

func NewWorkspaces(a core.Service, id string) (s Source) {
	return &workspaces{
		id: id,
		a:  a,
	}
}

type workspaces struct {
	id string
	a  core.Service
}

func (v *workspaces) ReadOnly() bool {
	return true
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
	// should we use id here?
	s := state.NewDoc(v.id, nil).(*state.State)

	objects, err := v.a.GetAllObjectsInWorkspace(v.id)
	if err != nil {
		return nil, err
	}

	var lastTarget string
	for _, objId := range objects {
		link := simple.New(&model.Block{
			Content: &model.BlockContentOfLink{
				Link: &model.BlockContentLink{
					TargetBlockId: objId,
					Style:         model.BlockContentLink_Page,
				},
			},
		})
		s.Add(link)
		if s.Get(s.RootId()) != nil {
			if chIds := s.Get(s.RootId()).Model().ChildrenIds; len(chIds) > 0 {
				lastTarget = chIds[0]
			}
		}
		if err = s.InsertTo(lastTarget, model.Block_Inner, link.Model().Id); err != nil {
			return
		}
	}
	// TODO: save changeReceiver to react to new threads being added, we must add this to threadNotifier
	return s, nil
}

func (v *workspaces) ReadMeta(_ ChangeReceiver) (doc state.Doc, err error) {
	// TODO: should this be exactly like copy
	return v.ReadDoc(nil, false)
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
	return
}

func (v *workspaces) LogHeads() map[string]string {
	return nil
}
