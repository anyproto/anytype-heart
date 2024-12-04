package converter

import (
	"fmt"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	root    = "root"
	spaceId = "space"
)

func TestLayoutConverter_Convert(t *testing.T) {
	store := objectstore.NewStoreFixture(t)
	store.AddObjects(t, spaceId, []spaceindex.TestObject{{
		bundle.RelationKeyId:        pbtypes.String(bundle.TypeKeyTask.URL()),
		bundle.RelationKeySpaceId:   pbtypes.String(spaceId),
		bundle.RelationKeyUniqueKey: pbtypes.String(bundle.TypeKeyTask.URL()),
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
			st.SetDetails(&types.Struct{
				Fields: map[string]*types.Value{
					bundle.RelationKeySpaceId.String(): pbtypes.String(spaceId),
					bundle.RelationKeySetOf.String():   pbtypes.StringList([]string{bundle.TypeKeyTask.URL()}),
				},
			})

			lc := layoutConverter{objectStore: store}

			// when
			err := lc.Convert(st, from, model.ObjectType_set)

			// then
			assert.NoError(t, err)
			dvb := st.Get(template.DataviewBlockId)
			assert.NotNil(t, dvb)
			dv := dvb.Model().GetDataview()
			require.NotNil(t, dv)
			assert.NotEmpty(t, dv.Views)
			assert.NotEmpty(t, dv.RelationLinks)
		})
	}
}
