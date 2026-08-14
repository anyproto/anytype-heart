package sourceimpl

// Wire-level tests for pb.Change.IntegrationKey (APIV2_OBJECT_DELETE.md
// §11.1/§11.4/§15): the param → change copy at build, the ChangeNoSnapshot
// read-path conversion, and the legacy (field-absent) shape.

import (
	"testing"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree/mock_objecttree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/pb"
)

func TestBuildChange_IntegrationKey(t *testing.T) {
	// newSource builds the minimal treeSource buildChange needs: an object
	// tree whose head differs from its id (no forced snapshot), so the built
	// change is the plain content shape and the assertion below is about the
	// param → pb.Change copy alone.
	newSource := func(t *testing.T) *treeSource {
		ctrl := gomock.NewController(t)
		tree := mock_objecttree.NewMockObjectTree(ctrl)
		tree.EXPECT().Heads().Return([]string{"someHead"}).AnyTimes()
		tree.EXPECT().Id().Return("rootId").AnyTimes()
		return &treeSource{ObjectTree: tree, id: "rootId"}
	}
	params := func(key string) source.PushChangeParams {
		return source.PushChangeParams{
			State:          state.NewDoc("rootId", nil).(*state.State),
			Changes:        changeWithSmallTextUpdate().Content,
			Time:           time.Unix(1700000000, 0),
			IntegrationKey: key,
		}
	}

	t.Run("param lands on the pb.Change", func(t *testing.T) {
		// fails if buildChange stops copying PushChangeParams.IntegrationKey —
		// the write half of the provenance record
		c := newSource(t).buildChange(params("claude-desktop"))
		assert.Equal(t, "claude-desktop", c.IntegrationKey)
	})

	t.Run("empty param leaves the field empty", func(t *testing.T) {
		c := newSource(t).buildChange(params(""))
		assert.Equal(t, "", c.IntegrationKey)
	})
}

func TestUnmarshalChange_IntegrationKey(t *testing.T) {
	t.Run("the no-snapshot read path preserves the key", func(t *testing.T) {
		// given: a stamped, snapshot-less change as the wire carries it. The
		// no-snapshot unmarshal decodes into pb.ChangeNoSnapshot and CONVERTS
		// back to pb.Change — this test fails if that conversion (source.go)
		// stops copying IntegrationKey: the value would be written by every
		// creation and then silently dropped on every read.
		c := changeWithSmallTextUpdate()
		c.IntegrationKey = "claude-desktop"
		require.Nil(t, c.Snapshot)
		data, dt, err := MarshalChange(c)
		require.NoError(t, err)

		// when: needSnapshot=false — the path every non-first change takes
		res, err := unmarshalChange(&objecttree.Change{DataType: dt}, data, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "claude-desktop", res.(*pb.Change).IntegrationKey)
	})

	t.Run("snapshot read path preserves the key", func(t *testing.T) {
		c := changeWithBigSnapshot()
		c.IntegrationKey = "claude-desktop"
		data, dt, err := MarshalChange(c)
		require.NoError(t, err)

		res, err := unmarshalChange(&objecttree.Change{DataType: dt}, data, true)

		require.NoError(t, err)
		assert.Equal(t, "claude-desktop", res.(*pb.Change).IntegrationKey)
	})

	t.Run("legacy change without the field reads as empty", func(t *testing.T) {
		// the §8 fail-closed shape: a change written before the field existed
		// must deterministically read as unprovenanced, on both paths
		c := changeWithSmallTextUpdate()
		data, dt, err := MarshalChange(c)
		require.NoError(t, err)
		for _, needSnapshot := range []bool{true, false} {
			res, err := unmarshalChange(&objecttree.Change{DataType: dt}, data, needSnapshot)
			require.NoError(t, err)
			assert.Equal(t, "", res.(*pb.Change).IntegrationKey)
		}
	})
}
