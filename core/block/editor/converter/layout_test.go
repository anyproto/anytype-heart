package converter

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	root    = "root"
	spaceId = "space"
)

func TestLayoutConverter_Convert(t *testing.T) {
	store := objectstore.NewStoreFixture(t)
	store.AddObjects(t, spaceId, []spaceindex.TestObject{{
		bundle.RelationKeyId:        domain.String(bundle.TypeKeyTask.URL()),
		bundle.RelationKeySpaceId:   domain.String(spaceId),
		bundle.RelationKeyUniqueKey: domain.String(bundle.TypeKeyTask.URL()),
	}, {
		bundle.RelationKeyId:              domain.String(bundle.TypeKeySet.URL()),
		bundle.RelationKeySpaceId:         domain.String(spaceId),
		bundle.RelationKeyDefaultTypeId:   domain.String(bundle.TypeKeySet.URL()),
		bundle.RelationKeyDefaultViewType: domain.Int64(int64(model.BlockContentDataviewView_Gallery)),
	}})

	for _, from := range []model.ObjectTypeLayout{
		model.ObjectType_basic,
		model.ObjectType_note,
		model.ObjectType_todo,
		model.ObjectType_collection,
		model.ObjectType_tag,
	} {
		t.Run(fmt.Sprintf("convert from %s to set", from.String()), func(t *testing.T) {
			// given
			st := state.NewDoc(root, map[string]simple.Block{
				root: simple.New(&model.Block{Id: root, ChildrenIds: []string{}}),
			}).NewState()
			st.SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeySpaceId: domain.String(spaceId),
				bundle.RelationKeySetOf:   domain.StringList([]string{bundle.TypeKeyTask.URL()}),
				bundle.RelationKeyType:    domain.String(bundle.TypeKeySet.URL()),
			}))

			lc := layoutConverter{objectStore: store}

			// when
			err := lc.Convert(st, from, model.ObjectType_set)

			// then
			assert.NoError(t, err)
			dvb := st.Get(template.DataviewBlockId)
			assert.NotNil(t, dvb)
			dv := dvb.Model().GetDataview()
			require.NotNil(t, dv)
			assert.NotEmpty(t, dv.RelationLinks)
			assert.Len(t, dv.Views, 1)
			assert.Equal(t, bundle.TypeKeySet.URL(), dv.Views[0].DefaultObjectTypeId)
			assert.Equal(t, model.BlockContentDataviewView_Gallery, dv.Views[0].Type)
		})
	}
}
