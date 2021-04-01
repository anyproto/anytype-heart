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
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
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
	st.SetExtraRelation(&pbrelation.Relation{
		Key: "testRel",
	})
	st.SetObjectType("testObjType")
	return st
}

func TestStateToBytes(t *testing.T) {
	data, err := StateToBytes(newTestState("root"))
	require.NoError(t, err)
	assert.NotEmpty(t, data)
	//t.Log(data)
}

func TestBytesToState(t *testing.T) {
	var data = []byte{31, 139, 8, 0, 0, 0, 0, 0, 0, 255, 76, 142, 193, 74, 195, 64, 16, 134, 59, 118, 155, 180, 19, 42, 203, 98, 81, 6, 145, 178, 23, 131, 7, 31, 68, 15, 5, 17, 15, 94, 202, 198, 12, 36, 113, 117, 67, 50, 10, 123, 245, 253, 124, 39, 89, 215, 131, 183, 255, 27, 230, 255, 102, 204, 55, 160, 70, 53, 133, 32, 182, 232, 216, 181, 60, 61, 47, 240, 10, 255, 50, 173, 53, 92, 192, 30, 106, 176, 43, 233, 197, 243, 23, 156, 172, 151, 120, 139, 153, 204, 57, 238, 176, 58, 182, 44, 174, 247, 243, 61, 71, 83, 144, 122, 119, 111, 76, 69, 110, 77, 74, 151, 180, 48, 215, 120, 138, 203, 87, 142, 166, 164, 213, 167, 243, 31, 140, 59, 84, 18, 71, 54, 91, 170, 132, 103, 57, 52, 195, 99, 28, 217, 110, 176, 76, 248, 192, 222, 214, 121, 67, 183, 84, 29, 154, 129, 95, 100, 159, 176, 134, 187, 237, 49, 72, 248, 157, 164, 74, 7, 55, 255, 13, 116, 137, 42, 161, 57, 203, 166, 244, 211, 38, 133, 167, 116, 246, 39, 0, 0, 255, 255, 9, 116, 10, 46, 238, 0, 0, 0}

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
