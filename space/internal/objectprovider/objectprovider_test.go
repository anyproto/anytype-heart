package objectprovider

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache/mock_objectcache"
)

var ctx = context.Background()

func TestObjectProvider_LoadObjects(t *testing.T) {

	var ids = make([]string, 15)
	for i := range ids {
		ids[i] = fmt.Sprintf("id%d", i)
	}
	t.Run("single object", func(t *testing.T) {
		fx := newFixture(t)
		fx.objectCache.EXPECT().GetObject(mock.Anything, ids[0]).Return(smarttest.New(ids[0]), nil)
		assert.NoError(t, fx.LoadObjects(ctx, ids[:1]))
	})
	t.Run("single error", func(t *testing.T) {
		fx := newFixture(t)
		fx.objectCache.EXPECT().GetObject(mock.Anything, ids[0]).Return(nil, fmt.Errorf("error"))
		assert.Error(t, fx.LoadObjects(ctx, ids[:1]))
	})
	t.Run("many fast", func(t *testing.T) {
		fx := newFixture(t)
		for _, id := range ids {
			fx.objectCache.EXPECT().GetObject(mock.Anything, id).Return(smarttest.New(id), nil)
		}
		assert.NoError(t, fx.LoadObjects(ctx, ids))
	})
	t.Run("many error", func(t *testing.T) {
		fx := newFixture(t)
		for i, id := range ids {
			if i == 0 {
				fx.objectCache.EXPECT().GetObject(mock.Anything, id).Return(nil, fmt.Errorf("error"))
			} else {
				fx.objectCache.EXPECT().GetObject(mock.Anything, id).Return(smarttest.New(id), nil).WaitUntil(time.After(time.Second)).Maybe()
			}
		}
		assert.Error(t, fx.LoadObjects(ctx, ids))
	})
	t.Run("ctx cancel", func(t *testing.T) {
		fx := newFixture(t)
		ctxCanceled, cancel := context.WithCancel(ctx)
		cancel()
		assert.ErrorIs(t, fx.LoadObjects(ctxCanceled, []string{"1", "2"}), context.Canceled)
	})
}

func TestObjectProvider_LoadObjectsIgnoreErrs(t *testing.T) {
	var ids = make([]string, 15)
	for i := range ids {
		ids[i] = fmt.Sprintf("id%d", i)
	}

	fx := newFixture(t)

	for i, id := range ids {
		if i == 0 {
			fx.objectCache.EXPECT().GetObject(mock.Anything, id).Return(nil, fmt.Errorf("error"))
		} else {
			fx.objectCache.EXPECT().GetObject(mock.Anything, id).Return(smarttest.New(id), nil)
		}
	}
	fx.LoadObjectsIgnoreErrs(ctx, ids)
}

type fixture struct {
	ObjectProvider
	objectCache *mock_objectcache.MockCache
}

func newFixture(t *testing.T) *fixture {
	oc := mock_objectcache.NewMockCache(t)
	fx := &fixture{
		ObjectProvider: NewObjectProvider("space.id", "perosnalSpace.id", oc),
		objectCache:    oc,
	}
	return fx
}
