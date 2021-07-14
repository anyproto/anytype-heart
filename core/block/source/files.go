package source

import (
	"context"
	"strings"
	"time"

	"github.com/anytypeio/go-anytype-middleware/change"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
)

var getFileTimeout = time.Second * 5

func NewFiles(a core.Service, id string) (s Source) {
	return &files{
		id: id,
		a:  a,
	}
}

type files struct {
	id string
	a  core.Service
}

func (v *files) ReadOnly() bool {
	return true
}

func (v *files) Id() string {
	return v.id
}

func (v *files) Anytype() core.Service {
	return v.a
}

func (v *files) Type() model.SmartBlockType {
	return model.SmartBlockType_File
}

func (v *files) Virtual() bool {
	return false
}

func getDetailsForFileOrImage(ctx context.Context, a core.Service, id string) (p *types.Struct, isImage bool, err error) {
	f, err := a.FileByHash(ctx, id)
	if err != nil {
		return nil, false, err
	}
	if strings.HasPrefix(f.Info().Media, "image") {
		i, err := a.ImageByHash(ctx, id)
		if err != nil {
			return nil, false, err
		}
		d, err := i.Details()
		if err != nil {
			return nil, false, err
		}
		return d, true, nil
	}

	d, err := f.Details()
	if err != nil {
		return nil, false, err
	}
	return d, false, nil
}

func (v *files) ReadDoc(receiver ChangeReceiver, empty bool) (doc state.Doc, err error) {
	s := state.NewDoc(v.id, nil).(*state.State)

	ctx, cancel := context.WithTimeout(context.Background(), getFileTimeout)
	defer cancel()

	d, _, err := getDetailsForFileOrImage(ctx, v.a, v.id)
	if err != nil {
		return nil, err
	}

	s.SetDetails(d)

	s.SetObjectTypes(pbtypes.GetStringList(d, bundle.RelationKeyType.String()))
	return s, nil
}

func (v *files) ReadMeta(_ ChangeReceiver) (doc state.Doc, err error) {
	s := &state.State{}

	ctx, cancel := context.WithTimeout(context.Background(), getFileTimeout)
	defer cancel()

	d, _, err := getDetailsForFileOrImage(ctx, v.a, v.id)
	if err != nil {
		return nil, err
	}

	s.SetDetails(d)
	s.SetLocalDetail(bundle.RelationKeyId.String(), pbtypes.String(v.id))
	s.SetObjectTypes(pbtypes.GetStringList(d, bundle.RelationKeyType.String()))
	return s, nil
}

func (v *files) PushChange(params PushChangeParams) (id string, err error) {
	return "", nil
}

func (v *files) FindFirstChange(ctx context.Context) (c *change.Change, err error) {
	return nil, change.ErrEmpty
}

func (v *files) ListIds() ([]string, error) {
	return v.a.FileStore().ListTargets()
}

func (v *files) Close() (err error) {
	return
}
