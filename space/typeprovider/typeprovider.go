package typeprovider

import (
	"context"
	"errors"
	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/space"
	"github.com/gogo/protobuf/proto"
	"sync"
)

const CName = "space.typeprovider"

var log = logging.Logger(CName)

var (
	ErrUnknownChangeType = errors.New("error unknown change type")
)

type ObjectTypeProvider interface {
	app.Component
	Type(id string) (smartblock.SmartBlockType, error)
}

func New() ObjectTypeProvider {
	return &objectTypeProvider{}
}

type objectTypeProvider struct {
	sync.Mutex
	spaceService space.Service
	cache        map[string]*model.ObjectChangePayload
}

func (o *objectTypeProvider) Init(a *app.App) (err error) {
	o.spaceService = a.MustComponent(space.CName).(space.Service)
	o.cache = map[string]*model.ObjectChangePayload{}
	return
}

func (o *objectTypeProvider) Name() (name string) {
	return CName
}

func (o *objectTypeProvider) Type(id string) (tp smartblock.SmartBlockType, err error) {
	tp, err = smartblock.SmartBlockTypeFromID(id)
	if err != nil || tp != smartblock.SmartBlockTypePage {
		return
	}
	return o.objectTypeFromSpace(id)
}

func (o *objectTypeProvider) objectTypeFromSpace(id string) (tp smartblock.SmartBlockType, err error) {
	o.Lock()
	payload, exists := o.cache[id]
	if exists {
		o.Unlock()
		tp = smartblock.SmartBlockType(payload.ObjectType)
		return
	}
	o.Unlock()

	sp, err := o.spaceService.AccountSpace(context.Background())
	if err != nil {
		return
	}
	store := sp.Storage()
	rawRoot, err := store.TreeRoot(id)
	if err != nil {
		return
	}
	root, err := o.unmarshallRoot(rawRoot)
	if err != nil {
		return
	}
	if root.ChangeType != space.ChangeType {
		err = ErrUnknownChangeType
		return
	}
	payload, err = o.objectType(root.ChangePayload)
	if err != nil {
		return
	}
	o.Lock()
	defer o.Unlock()
	o.cache[id] = payload
	tp = smartblock.SmartBlockType(payload.ObjectType)
	return
}

func (o *objectTypeProvider) objectType(changePayload []byte) (payload *model.ObjectChangePayload, err error) {
	payload = &model.ObjectChangePayload{}
	err = proto.Unmarshal(changePayload, payload)
	return
}

func (o *objectTypeProvider) unmarshallRoot(rawRoot *treechangeproto.RawTreeChangeWithId) (root *treechangeproto.RootChange, err error) {
	raw := &treechangeproto.RawTreeChange{}
	err = proto.Unmarshal(rawRoot.GetRawChange(), raw)
	if err != nil {
		return
	}

	root = &treechangeproto.RootChange{}
	err = proto.Unmarshal(raw.Payload, root)
	if err != nil {
		return
	}
	return
}
