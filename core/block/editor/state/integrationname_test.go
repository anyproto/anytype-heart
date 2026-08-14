package state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// The integration name must be strictly per-apply (APIV2_OBJECT_DELETE.md
// §11.4): if any of these propagation paths ever starts carrying it, every
// later local edit inherits the creation stamp and the DELETE ownership gate
// silently widens. Each subtest names the exact leak it would catch.
func TestState_IntegrationNameIsNotPropagated(t *testing.T) {
	t.Run("set and get on one state", func(t *testing.T) {
		s := NewDoc("root", nil).(*State)
		s.SetIntegrationName("Claude Desktop")
		assert.Equal(t, "Claude Desktop", s.IntegrationName())
	})

	t.Run("NewState does not inherit", func(t *testing.T) {
		// the leak: every derived edit state would carry the creation stamp
		parent := NewDoc("root", nil).(*State)
		parent.SetIntegrationName("Claude Desktop")
		assert.Equal(t, "", parent.NewState().IntegrationName())
	})

	t.Run("Copy does not inherit", func(t *testing.T) {
		// the leak: template/snapshot copies would carry the stamp
		s := NewDoc("root", nil).(*State)
		s.SetIntegrationName("Claude Desktop")
		assert.Equal(t, "", s.Copy().IntegrationName())
	})

	t.Run("apply does not write it onto the parent doc", func(t *testing.T) {
		// the leak the spec calls out: if apply persisted the key on the doc
		// state, the NEXT apply's push would read it back and every subsequent
		// local change would be misattributed
		doc := NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{Id: "root"}),
		}).(*State)
		s := doc.NewState()
		s.SetIntegrationName("Claude Desktop")
		s.Add(simple.New(&model.Block{Id: "child"}))
		require.NoError(t, s.InsertTo("root", model.Block_Inner, "child"))

		_, _, err := ApplyState("space1", s, false)
		require.NoError(t, err)

		assert.Equal(t, "", doc.IntegrationName())
		assert.Equal(t, "", doc.NewState().IntegrationName())
	})
}
