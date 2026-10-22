package objectcreator

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

// An imported type carrying `uninstalled` restores hidden — unless the
// destination already holds that type LIVE, in which case the live one
// wins: reinstalling or hiding a type the user has since restored would be
// wrong, so the incoming flag is dropped before the state is reset.
func TestKeepLiveTypeInstalled(t *testing.T) {
	incoming := func() *state.State {
		st := state.NewDoc("t", nil).NewState()
		st.SetDetail(bundle.RelationKeyName, domain.String("Removed"))
		st.SetDetail(bundle.RelationKeyIsUninstalled, domain.Bool(true))
		return st
	}
	t.Run("a live destination type stays live", func(t *testing.T) {
		existing := domain.NewDetails()
		existing.SetString(bundle.RelationKeyName, "Removed")
		st := incoming()
		keepLiveTypeInstalled(existing, st)
		assert.False(t, st.Details().Has(bundle.RelationKeyIsUninstalled))
	})
	t.Run("a destination type the user also removed stays removed", func(t *testing.T) {
		existing := domain.NewDetails()
		existing.SetBool(bundle.RelationKeyIsUninstalled, true)
		st := incoming()
		keepLiveTypeInstalled(existing, st)
		assert.True(t, st.Details().GetBool(bundle.RelationKeyIsUninstalled))
	})
	t.Run("no existing details means a new object: the flag is kept", func(t *testing.T) {
		st := incoming()
		keepLiveTypeInstalled(nil, st)
		assert.True(t, st.Details().GetBool(bundle.RelationKeyIsUninstalled))
	})
}
