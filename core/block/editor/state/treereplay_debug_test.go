package state

import (
	"bytes"
	"encoding/json"
	"os"
	"sort"
	"testing"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
)

// TestReplayTreeDumpShadow replays a debugtree JSON dump (the files logged by
// the parentIndex shadow sweep / debugtree export) in shadow-verification mode
// and asserts zero index mismatches, plus structural equality between the
// indexed and plain replays. Skipped unless ANYTYPE_TREE_DUMP points to a
// dump file:
//
//	ANYTYPE_TREE_DUMP=/path/tree_XXX.json go test ./core/block/editor/state/ -run TestReplayTreeDumpShadow -v
func TestReplayTreeDumpShadow(t *testing.T) {
	dumpPath := os.Getenv("ANYTYPE_TREE_DUMP")
	if dumpPath == "" {
		t.Skip("set ANYTYPE_TREE_DUMP to a debugtree json dump to run")
	}
	raw, err := os.ReadFile(dumpPath)
	require.NoError(t, err)

	var dump struct {
		Id      string `json:"id"`
		Changes []struct {
			Id     string          `json:"id"`
			Ord    int             `json:"ord"`
			Change json.RawMessage `json:"change"`
		} `json:"changes"`
	}
	require.NoError(t, json.Unmarshal(raw, &dump))
	sort.Slice(dump.Changes, func(i, j int) bool { return dump.Changes[i].Ord < dump.Changes[j].Ord })

	t.Logf("dump %s: %d changes", dump.Id, len(dump.Changes))

	// each run must unmarshal its own copy: changeBlockCreate wraps the
	// change's block models by reference, so sharing parsed changes between
	// replicas aliases their states and corrupts the comparison
	run := func(indexed bool) *State {
		unmarshaler := jsonpb.Unmarshaler{AllowUnknownFields: true}
		d := NewDoc(dump.Id, nil).(*State)
		if indexed {
			d.EnableParentIndex()
		}
		for _, ch := range dump.Changes {
			if ch.Id == dump.Id {
				continue // the tree root change is not applied (see BuildState)
			}
			var model pb.Change
			require.NoError(t, unmarshaler.Unmarshal(bytes.NewReader(ch.Change), &model), "change %s", ch.Id)
			d.SetChangeId(ch.Id)
			d.ApplyChangeIgnoreErr(model.Content...)
		}
		return d
	}

	prev := parentIndexDebug
	parentIndexDebug = true
	defer func() { parentIndexDebug = prev }()

	plain := run(false)
	indexed := run(true)

	require.NotNil(t, indexed.parentIdx)
	t.Logf("shadow: %d lookups, %d mismatches, %d fallbacks",
		indexed.parentIdx.debugLookups, indexed.parentIdx.debugMismatches, indexed.parentIdx.debugFallbacks)
	assert.Zero(t, indexed.parentIdx.debugMismatches)
	assert.Equal(t, plain.String(), indexed.String())
}
