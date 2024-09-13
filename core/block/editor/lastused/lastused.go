package lastused

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/cheggaaa/mb/v3"
	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	CName = "object-usage-updater"

	maxInstallationTime = 5 * time.Minute
	updateInterval      = 5 * time.Second
)

type Key interface {
	URL() string
	String() string
}

type message struct {
	spaceId string
	key     Key
	time    int64
}

var log = logger.NewNamed("update-last-used-date")

type ObjectUsageUpdater interface {
	app.ComponentRunnable

	UpdateLastUsedDate(spaceId string, key Key, timeStamp int64)
}

func New() ObjectUsageUpdater {
	return &updater{}
}

type updater struct {
	store        objectstore.ObjectStore
	spaceService space.Service

	ctx    context.Context
	cancel context.CancelFunc

	msgBatch *mb.MB[message]
}

func (u *updater) Name() string {
	return CName
}

func (u *updater) Init(a *app.App) error {
	u.store = app.MustComponent[objectstore.ObjectStore](a)
	u.spaceService = app.MustComponent[space.Service](a)
	u.msgBatch = mb.New[message](0)
	return nil
}

func (u *updater) Run(context.Context) error {
	u.ctx, u.cancel = context.WithCancel(context.Background())
	go u.lastUsedUpdateHandler()
	return nil
}

func (u *updater) Close(context.Context) error {
	if u.cancel != nil {
		u.cancel()
	}
	if err := u.msgBatch.Close(); err != nil {
		log.Error("failed to close message batch", zap.Error(err))
	}
	return nil
}

func (u *updater) UpdateLastUsedDate(spaceId string, key Key, ts int64) {
	if err := u.msgBatch.Add(u.ctx, message{spaceId: spaceId, key: key, time: ts}); err != nil {
		log.Error("failed to add last used date info to message batch", zap.Error(err), zap.String("key", key.String()))
	}
}

func (u *updater) lastUsedUpdateHandler() {
	var (
		accumulator = make(map[string]map[Key]int64)
		lock        sync.Mutex
	)

	go func() {
		for {
			select {
			case <-u.ctx.Done():
				return
			case <-time.After(updateInterval):
				lock.Lock()
				if len(accumulator) == 0 {
					lock.Unlock()
					continue
				}
				for spaceId, keys := range accumulator {
					log.Debug("updating lastUsedDate for objects in space", zap.Int("objects num", len(keys)), zap.String("spaceId", spaceId))
					u.updateLastUsedDateForKeysInSpace(spaceId, keys)
				}
				accumulator = make(map[string]map[Key]int64)
				lock.Unlock()
			}
		}
	}()

	for {
		msgs, err := u.msgBatch.Wait(u.ctx)
		if err != nil {
			return
		}

		lock.Lock()
		for _, msg := range msgs {
			if keys := accumulator[msg.spaceId]; keys != nil {
				keys[msg.key] = msg.time
			} else {
				keys = map[Key]int64{
					msg.key: msg.time,
				}
				accumulator[msg.spaceId] = keys
			}
		}
		lock.Unlock()
	}
}

func (u *updater) updateLastUsedDateForKeysInSpace(spaceId string, keys map[Key]int64) {
	spc, err := u.spaceService.Get(u.ctx, spaceId)
	if err != nil {
		log.Error("failed to get space", zap.String("spaceId", spaceId), zap.Error(err))
		return
	}

	for key, timeStamp := range keys {
		if err = u.updateLastUsedDate(spc, key, timeStamp); err != nil {
			log.Error("failed to update last used date", zap.String("spaceId", spaceId), zap.String("key", key.String()), zap.Error(err))
		}
	}
}

func (u *updater) updateLastUsedDate(spc clientspace.Space, key Key, ts int64) error {
	uk, err := domain.UnmarshalUniqueKey(key.URL())
	if err != nil {
		return fmt.Errorf("failed to unmarshall key: %w", err)
	}

	if uk.SmartblockType() != coresb.SmartBlockTypeObjectType && uk.SmartblockType() != coresb.SmartBlockTypeRelation {
		return fmt.Errorf("cannot update lastUsedDate for object with invalid smartBlock type. Only object types and relations are expected")
	}

	details, err := u.store.GetObjectByUniqueKey(spc.Id(), uk)
	if err != nil {
		return fmt.Errorf("failed to get details: %w", err)
	}

	id := pbtypes.GetString(details.Details, bundle.RelationKeyId.String())
	if id == "" {
		return fmt.Errorf("failed to get id from details: %w", err)
	}

	if err = spc.DoCtx(u.ctx, id, func(sb smartblock.SmartBlock) error {
		st := sb.NewState()
		st.SetLocalDetail(bundle.RelationKeyLastUsedDate.String(), pbtypes.Int64(ts))
		return sb.Apply(st)
	}); err != nil {
		return fmt.Errorf("failed to set lastUsedDate to object: %w", err)
	}
	return nil
}

func SetLastUsedDateForInitialObjectType(id string, details *types.Struct) {
	if !strings.HasPrefix(id, addr.BundledObjectTypeURLPrefix) || details == nil || details.Fields == nil {
		return
	}

	var priority int64
	switch id {
	case bundle.TypeKeyNote.BundledURL():
		priority = 1
	case bundle.TypeKeyPage.BundledURL():
		priority = 2
	case bundle.TypeKeyTask.BundledURL():
		priority = 3
	case bundle.TypeKeySet.BundledURL():
		priority = 4
	case bundle.TypeKeyCollection.BundledURL():
		priority = 5
	default:
		priority = 7
	}

	// we do this trick to order crucial Anytype object types by last date
	lastUsed := time.Now().Add(time.Duration(-1 * priority * int64(maxInstallationTime))).Unix()
	details.Fields[bundle.RelationKeyLastUsedDate.String()] = pbtypes.Int64(lastUsed)
}
