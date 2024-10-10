package editor

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestPage_CreationStateMigration(t *testing.T) {
	t.Run("todo layout migration", func(t *testing.T) {
		// given

		initContext := &smartblock.InitContext{
			IsNewObject:    true,
			State:          state.NewDoc("rootId", nil).(*state.State),
			ObjectTypeKeys: []domain.TypeKey{bundle.TypeKeyTask},
		}

		storeFixture := objectstore.NewStoreFixture(t)
		storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:                pbtypes.String("task"),
				bundle.RelationKeyUniqueKey:         pbtypes.String(bundle.TypeKeyTask.URL()),
				bundle.RelationKeyRecommendedLayout: pbtypes.Int64(int64(model.ObjectType_todo)),
				bundle.RelationKeySpaceId:           pbtypes.String("spaceId"),
			},
		})

		p := &Page{}
		test := smarttest.New("rootId")
		test.SetSpaceId("spaceId")
		p.SmartBlock = test
		p.objectStore = storeFixture

		// when
		migration.RunMigrations(p, initContext)

		// then
		done := pbtypes.Get(initContext.State.Details(), bundle.RelationKeyDone.String())
		assert.NotNil(t, done)
		assert.False(t, done.GetBoolValue())
	})
}
