package builtintemplate

import (
	"encoding/hex"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestState(rootId string) *state.State {
	st := state.NewDoc(rootId, nil).(*state.State)
	template.InitTemplate(st, template.WithTitle)
	st.AddFileKeys(&pb.ChangeFileKeys{
		Hash: "test",
		Keys: map[string]string{"testKey": "testValue"},
	})
	st.SetDetails(&types.Struct{
		Fields: map[string]*types.Value{
			"key": pbtypes.String("value"),
		},
	})
	st.SetExtraRelation(&model.Relation{
		Key: "testRel",
	})
	st.SetObjectType("testObjType")
	return st
}

func TestStateToBytes(t *testing.T) {
	data, err := StateToBytes(newTestState("root"))
	require.NoError(t, err)
	assert.NotEmpty(t, data)
}

func TestBytesToState(t *testing.T) {
	data, err := StateToBytes(newTestState("root"))
	require.NoError(t, err)

	st, err := BytesToState(data)
	require.NoError(t, err)

	orig := newTestState("root")
	assert.Equal(t, orig.RootId(), st.RootId())
	assert.Equal(t, orig.String(), st.String())
	assert.Equal(t, orig.Details(), st.Details())
	assert.Equal(t, orig.ExtraRelations(), st.ExtraRelations())
}

func TestGenerate(t *testing.T) {
	var states = []*state.State{newTestState("one"), newTestState("two"), newTestState("three")}
	filename := "./testgen/test.gen.go"
	defer os.Remove(filename)
	require.NoError(t, Generate(filename, "main", states))
	out, err := exec.Command("go", "run", "./testgen").CombinedOutput()
	require.NoError(t, err)
	outHex := strings.Split(strings.Trim(string(out), "\n"), "\n")
	assert.Equal(t, len(states), len(outHex))

	for i, oh := range outHex {
		data, err := hex.DecodeString(oh)
		require.NoError(t, err)
		st, err := BytesToState(data)
		require.NoError(t, err)
		assert.Equal(t, states[i].RootId(), st.RootId())
	}

}
