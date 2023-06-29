package source

import (
	"context"
	"strings"
	"time"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var getFileTimeout = 60 * time.Second

func NewFile(a core.Service, fileStore filestore.FileStore, fileService files.Service, id string) (s Source) {
	return &file{
		id:          id,
		a:           a,
		fileStore:   fileStore,
		fileService: fileService,
	}
}

type file struct {
	id          string
	a           core.Service
	fileStore   filestore.FileStore
	fileService files.Service
}

func (f *file) ReadOnly() bool {
	return true
}

func (f *file) Id() string {
	return f.id
}

func (f *file) Type() model.SmartBlockType {
	return model.SmartBlockType_File
}

func (f *file) getDetailsForFileOrImage(ctx session.Context, id string) (p *types.Struct, isImage bool, err error) {
	file, err := f.fileService.FileByHash(ctx, id)
	if err != nil {
		return nil, false, err
	}
	if strings.HasPrefix(file.Info().Media, "image") {
		image, err := f.fileService.ImageByHash(ctx, id)
		if err != nil {
			return nil, false, err
		}
		details, err := image.Details(ctx.Context())
		if err != nil {
			return nil, false, err
		}
		return details, true, nil
	}

	d, err := file.Details(ctx.Context())
	if err != nil {
		return nil, false, err
	}
	return d, false, nil
}

func (f *file) ReadDoc(ctx session.Context, receiver ChangeReceiver, empty bool) (doc state.Doc, err error) {
	s := state.NewDoc(f.id, nil).(*state.State)

	cctx, cancel := context.WithTimeout(ctx.Context(), getFileTimeout)
	ctx = ctx.WithContext(cctx)
	defer cancel()

	d, _, err := f.getDetailsForFileOrImage(ctx, f.id)
	if err != nil {
		return nil, err
	}
	if d.GetFields() != nil {
		d.Fields[bundle.RelationKeyWorkspaceId.String()] = pbtypes.String(f.a.PredefinedBlocks().Account)
	}

	s.SetDetails(d)

	s.SetObjectTypes(pbtypes.GetStringList(d, bundle.RelationKeyType.String()))
	return s, nil
}

func (f *file) PushChange(params PushChangeParams) (id string, err error) {
	return "", nil
}

func (f *file) ListIds() ([]string, error) {
	return f.fileStore.ListTargets()
}

func (f *file) Close() (err error) {
	return
}

func (f *file) Heads() []string {
	return []string{f.id}
}

func (f *file) GetFileKeysSnapshot() []*pb.ChangeFileKeys {
	return nil
}
