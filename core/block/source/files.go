package source

import (
	"context"
	"strings"
	"time"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var getFileTimeout = 60 * time.Second

func NewFile(a core.Service, fileStore filestore.FileStore, fileService files.Service, spaceID string, id string) (s Source) {
	return &file{
		id: domain.FullID{
			SpaceID:  spaceID,
			ObjectID: id,
		},
		a:           a,
		fileStore:   fileStore,
		fileService: fileService,
	}
}

type file struct {
	id          domain.FullID
	a           core.Service
	fileStore   filestore.FileStore
	fileService files.Service
}

func (f *file) ReadOnly() bool {
	return true
}

func (f *file) Id() string {
	return f.id.ObjectID
}

func (f *file) SpaceID() string {
	return f.id.SpaceID
}

func (f *file) Type() smartblock.SmartBlockType {
	return smartblock.SmartBlockTypeFile
}

func (f *file) getDetailsForFileOrImage(ctx context.Context) (*types.Struct, bundle.TypeKey, error) {
	file, err := f.fileService.FileByHash(ctx, f.id)
	if err != nil {
		return nil, "", err
	}
	if strings.HasPrefix(file.Info().Media, "image") {
		image, err := f.fileService.ImageByHash(ctx, f.id)
		if err != nil {
			return nil, "", err
		}
		details, err := image.Details(ctx)
		if err != nil {
			return nil, "", err
		}
		return details, bundle.TypeKeyImage, nil
	}

	d, typeKey, err := file.Details(ctx)
	if err != nil {
		return nil, "", err
	}
	return d, typeKey, nil
}

func (f *file) ReadDoc(ctx context.Context, receiver ChangeReceiver, empty bool) (doc state.Doc, err error) {
	s := state.NewDoc(f.id.ObjectID, nil).(*state.State)

	ctx, cancel := context.WithTimeout(ctx, getFileTimeout)
	defer cancel()

	d, typeKey, err := f.getDetailsForFileOrImage(ctx)
	if err != nil {
		return nil, err
	}
	if d.GetFields() != nil {
		d.Fields[bundle.RelationKeySpaceId.String()] = pbtypes.String(f.id.SpaceID)
	}

	s.SetDetails(d)
	s.SetObjectTypeKey(typeKey)
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
	return []string{f.id.ObjectID}
}

func (f *file) GetFileKeysSnapshot() []*pb.ChangeFileKeys {
	return nil
}
