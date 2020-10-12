package change

import (
	"encoding/json"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	newSnapshot = func(id, snapshotId string, heads map[string]string, prevIds ...string) *Change {
		return &Change{
			Id: id,
			Change: &pb.Change{
				PreviousIds:    prevIds,
				LastSnapshotId: snapshotId,
				Snapshot: &pb.ChangeSnapshot{
					LogHeads: heads,
				},
			},
		}
	}
	newChange = func(id, snapshotId string, prevIds ...string) *Change {
		return &Change{
			Id: id,
			Change: &pb.Change{
				PreviousIds:    prevIds,
				LastSnapshotId: snapshotId,
				Content:        []*pb.ChangeContent{},
			},
		}
	}
	detailsContent   = []*pb.ChangeContent{{Value: &pb.ChangeContentValueOfDetailsSet{&pb.ChangeDetailsSet{}}}}
	newDetailsChange = func(id, snapshotId string, prevIds string, prevDetIds string, withDet bool) *Change {
		ch := newChange(id, snapshotId, prevIds)
		ch.PreviousMetaIds = []string{prevDetIds}
		if withDet {
			ch.Content = detailsContent
		}
		return ch
	}
)

func TestStateBuilder_Build(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		_, _, err := BuildTree(NewTestSmartBlock())
		assert.Equal(t, ErrEmpty, err)
	})
	t.Run("linear - one snapshot", func(t *testing.T) {
		sb := NewTestSmartBlock()
		sb.AddChanges(
			"a",
			newSnapshot("s0", "", nil),
		)
		b := new(stateBuilder)
		err := b.Build(sb)
		require.NoError(t, err)
		require.NotNil(t, b.tree)
		assert.Equal(t, "s0", b.tree.RootId())
		assert.Equal(t, 1, b.tree.Len())
		assert.Equal(t, []string{"s0"}, b.tree.headIds)
	})
	t.Run("linear - one log", func(t *testing.T) {
		sb := NewTestSmartBlock()
		sb.AddChanges(
			"a",
			newSnapshot("s0", "", nil),
			newChange("c0", "s0", "s0"),
		)
		b := new(stateBuilder)
		err := b.Build(sb)
		require.NoError(t, err)
		require.NotNil(t, b.tree)
		assert.Equal(t, "s0", b.tree.RootId())
		assert.Equal(t, 2, b.tree.Len())
		assert.Equal(t, []string{"c0"}, b.tree.headIds)
	})
	t.Run("linear - two logs - one snapshot", func(t *testing.T) {
		sb := NewTestSmartBlock()
		sb.AddChanges(
			"a",
			newSnapshot("s0", "", nil),
			newChange("c0", "s0", "s0"),
		)
		sb.AddChanges(
			"b",
			newChange("c1", "s0", "c0"),
			newChange("c2", "s0", "c1"),
		)
		b := new(stateBuilder)
		err := b.Build(sb)
		require.NoError(t, err)
		require.NotNil(t, b.tree)
		assert.Equal(t, "s0", b.tree.RootId())
		assert.Equal(t, 4, b.tree.Len())
		assert.Equal(t, []string{"c2"}, b.tree.headIds)
	})
	t.Run("linear - two logs - two snapshots", func(t *testing.T) {
		sb := NewTestSmartBlock()
		sb.AddChanges(
			"a",
			newSnapshot("s0", "", nil),
			newChange("c0", "s0", "s0"),
		)
		sb.AddChanges(
			"b",
			newChange("c1", "s0", "c0"),
			newChange("c2", "s0", "c1"),
			newSnapshot("s1", "s0", map[string]string{"a": "c0", "b": "c2"}, "c2"),
			newChange("c3", "s1", "s1"),
		)
		b := new(stateBuilder)
		err := b.Build(sb)
		require.NoError(t, err)
		require.NotNil(t, b.tree)
		assert.Equal(t, "s1", b.tree.RootId())
		assert.Equal(t, 2, b.tree.Len())
		assert.Equal(t, []string{"c3"}, b.tree.headIds)
	})
	t.Run("split brains", func(t *testing.T) {
		sb := NewTestSmartBlock()
		sb.AddChanges(
			"a",
			newSnapshot("s0", "", nil),
			newChange("c0", "s0", "s0"),
		)
		sb.AddChanges(
			"b",
			newChange("c1", "s0", "c0"),
			newChange("c2", "s0", "c1"),
			newSnapshot("s1", "s0", map[string]string{"a": "c0", "b": "c2"}, "c2"),
			newChange("c3", "s1", "s1"),
		)
		sb.AddChanges(
			"c",
			newChange("c1.1", "s0", "c0"),
			newChange("c2.2", "s0", "c1.1"),
			newSnapshot("s1.1", "s0", map[string]string{"a": "c0", "c": "c2.2"}, "c2.2"),
			newChange("c3.3", "s1.1", "s1.1"),
		)
		b := new(stateBuilder)
		err := b.Build(sb)
		require.NoError(t, err)
		require.NotNil(t, b.tree)
		assert.Equal(t, "s0", b.tree.RootId())
		assert.Equal(t, 10, b.tree.Len())
		assert.Equal(t, []string{"c3", "c3.3"}, b.tree.headIds)
	})
	t.Run("clue brains", func(t *testing.T) {
		sb := NewTestSmartBlock()
		sb.AddChanges(
			"a",
			newSnapshot("s0", "", nil),
			newChange("c0", "s0", "s0"),
		)
		sb.AddChanges(
			"b",
			newChange("c1", "s0", "c0"),
			newChange("c2", "s0", "c1"),
			newSnapshot("s1", "s0", map[string]string{"a": "c0", "b": "c2"}, "c2"),
			newChange("c3", "s1", "s1"),
		)
		sb.AddChanges(
			"c",
			newChange("c1.1", "s0", "c0"),
			newChange("c2.2", "s0", "c1.1"),
			newSnapshot("s1.1", "s0", map[string]string{"a": "c0", "c": "c2.2"}, "c2.2"),
			newChange("c3.3", "s1.1", "s1.1"),
		)
		sb.AddChanges(
			"a",
			newSnapshot("s2", "s0", map[string]string{"a": "c0", "b": "c3", "c": "c3.3"}, "c3", "c3.3"),
			newChange("c4", "s2", "s2"),
		)
		b := new(stateBuilder)
		err := b.Build(sb)
		require.NoError(t, err)
		require.NotNil(t, b.tree)
		assert.Equal(t, "s2", b.tree.RootId())
		assert.Equal(t, 2, b.tree.Len())
		assert.Equal(t, []string{"c4"}, b.tree.headIds)
	})
	t.Run("invalid logs", func(t *testing.T) {
		sb := NewTestSmartBlock()
		sb.AddChanges(
			"a",
			newSnapshot("s0", "", nil),
		)
		sb.AddChanges(
			"b",
			newChange("c1", "s0", "s0"),
		)
		sb.changes["c1"] = &core.SmartblockRecord{
			ID:      "c1",
			Payload: []byte("invalid pb"),
		}
		b := new(stateBuilder)
		err := b.Build(sb)
		require.NoError(t, err)
		require.NotNil(t, b.tree)
		assert.Equal(t, "s0", b.tree.RootId())
		assert.Equal(t, 1, b.tree.Len())
		assert.Equal(t, []string{"s0"}, b.tree.headIds)
	})
}

func TestStateBuilder_findCommonSnapshot(t *testing.T) {
	t.Run("error for empty", func(t *testing.T) {
		b := new(stateBuilder)
		_, err := b.findCommonSnapshot(nil)
		require.Error(t, err)
	})
	t.Run("one snapshot", func(t *testing.T) {
		b := new(stateBuilder)
		id, err := b.findCommonSnapshot([]string{"one"})
		require.NoError(t, err)
		assert.Equal(t, "one", id)
	})
	t.Run("common parent", func(t *testing.T) {
		sb := NewTestSmartBlock()
		sb.AddChanges(
			"a",
			newSnapshot("s0.1", "", nil),
			newSnapshot("s0", "s0.1", nil, "s0.1"),
		)
		sb.AddChanges(
			"b",
			newSnapshot("s1.1", "s0", nil, "s0"),
			newSnapshot("s2.1", "s1.1", nil, "s1.1"),
			newSnapshot("s3.1", "s2.1", nil, "s2.1"),
		)
		sb.AddChanges(
			"c",
			newSnapshot("s1.2", "s0", nil, "s0"),
		)
		sb.AddChanges(
			"d",
			newSnapshot("s1.3", "s0", nil, "s0"),
		)
		sb.AddChanges(
			"e",
			newSnapshot("s1.4", "s1.3", nil, "s1.3"),
			newSnapshot("s2.4", "s1.1", nil, "s1.4"),
		)
		sb.AddChanges(
			"f",
			newSnapshot("s1.5", "s2.4", nil, "s2.4"),
		)
		b := new(stateBuilder)
		err := b.Build(sb)
		require.NoError(t, err)
		assert.Equal(t, "s0", b.tree.RootId())
	})
	t.Run("abs split", func(t *testing.T) {
		sb := NewTestSmartBlock()
		sb.AddChanges(
			"a",
			newSnapshot("s0.1", "", nil),
		)
		sb.AddChanges(
			"b",
			newSnapshot("s1.1", "", nil),
		)
		b := new(stateBuilder)
		err := b.Build(sb)
		require.NoError(t, err)
		assert.Equal(t, "s0.1", b.tree.RootId())
	})
}

func TestBuildDetailsTree(t *testing.T) {
	sb := NewTestSmartBlock()

	sb.AddChanges(
		"a",
		newSnapshot("s0", "", nil),
		newDetailsChange("c0", "s0", "s0", "s0", false),
		newDetailsChange("c1", "s0", "c0", "s0", false),
		newDetailsChange("c2", "s0", "c1", "s0", true),
		newDetailsChange("c3", "s0", "c2", "c2", false),
		newDetailsChange("c4", "s0", "c3", "c2", true),
		newDetailsChange("c5", "s0", "c4", "c4", false),
		newDetailsChange("c6", "s0", "c5", "c4", false),
	)
	tr, _, err := BuildMetaTree(sb)
	require.NoError(t, err)
	assert.Equal(t, 3, tr.Len())
	assert.Equal(t, "s0->c2->c4-|", tr.String())
}

func TestBuildTreeBefore(t *testing.T) {
	t.Run("linear", func(t *testing.T) {
		sb := NewTestSmartBlock()
		sb.AddChanges(
			"a",
			newSnapshot("s0", "", nil),
			newChange("c0", "s0", "s0"),
			newSnapshot("s1", "s0", nil, "c0"),
			newChange("c1", "s1", "s1"),
		)
		tr, err := BuildTreeBefore(sb, "c1", true)
		require.NoError(t, err)
		require.NotNil(t, tr)
		assert.Equal(t, "s1", tr.RootId())
		assert.Equal(t, 2, tr.Len())
		tr, err = BuildTreeBefore(sb, "c0", true)
		require.NoError(t, err)
		require.NotNil(t, tr)
		assert.Equal(t, "s0", tr.RootId())
		assert.Equal(t, 2, tr.Len())
	})
}

func TestBuildTree_Issue639(t *testing.T) {
	var vers []struct {
		Id          string   `json:"id"`
		PreviousIds []string `json:"previousidsList"`
	}
	require.NoError(t, json.Unmarshal([]byte(issue639data), &vers))
	var changes []*Change
	for i := range vers {
		c := &Change{
			Id: vers[i].Id,
			Change: &pb.Change{
				PreviousIds: vers[i].PreviousIds,
			},
		}
		changes = append(changes, c)
	}

	tr := new(Tree)
	tr.Add(changes[len(changes)-1])
	tr.AddFast(changes...)
	//t.Log(tr.Graphviz())
	var lastId string
	tr.Iterate(tr.RootId(), func(c *Change) (isContinue bool) {
		lastId = c.Id
		return true
	})
	assert.Equal(t, "bafyreiaoz4fzp7vnk3nyi5au6teahxpwttv3eflcmsl26yjxyl4aoztfbi", lastId)
}

var issue639data = `[
      {
         "id": "bafyreiemkngls23b7g4qlvkhlutx2bbqf5h3vduyqi27oszikfhdm4tgwa",
         "previousidsList": [
            "bafyreifdv2rkiz67l4gd2qmycvtynvwsku55g27ixmljo6swdqvl5ncw4m"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597346852,
         "groupid": 0
      },
      {
         "id": "bafyreiaoz4fzp7vnk3nyi5au6teahxpwttv3eflcmsl26yjxyl4aoztfbi",
         "previousidsList": [
            "bafyreicsiy6bqe4xm3gx5qxfpqs6dbuokmj6z7xfr52g3htv4p45urhati"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661886,
         "groupid": 0
      },
      {
         "id": "bafyreicsiy6bqe4xm3gx5qxfpqs6dbuokmj6z7xfr52g3htv4p45urhati",
         "previousidsList": [
            "bafyreickrfznvtawkchk6u7553ad4negijx67fbm3ugj6g5r33igp7bqne"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661883,
         "groupid": 0
      },
      {
         "id": "bafyreickrfznvtawkchk6u7553ad4negijx67fbm3ugj6g5r33igp7bqne",
         "previousidsList": [
            "bafyreidlf2b3n6rypqdo22wqikyoiduksyg5mnnmk4j4tpcfambtmb6y34"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661883,
         "groupid": 0
      },
      {
         "id": "bafyreidlf2b3n6rypqdo22wqikyoiduksyg5mnnmk4j4tpcfambtmb6y34",
         "previousidsList": [
            "bafyreihrmuzs3tejo7dtm74yqpytdilizkhw37o5efqdfvyjpzomn2vfuu"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661882,
         "groupid": 0
      },
      {
         "id": "bafyreihrmuzs3tejo7dtm74yqpytdilizkhw37o5efqdfvyjpzomn2vfuu",
         "previousidsList": [
            "bafyreicc22h253rrqohsbgag5pnxwcwoveaafqbufiae5cdnniofpa4kym"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661881,
         "groupid": 0
      },
      {
         "id": "bafyreicc22h253rrqohsbgag5pnxwcwoveaafqbufiae5cdnniofpa4kym",
         "previousidsList": [
            "bafyreicbupmkyj2immelaykwfm6riwazshyyqyd3s6wr52xu66biaruuqq"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661881,
         "groupid": 0
      },
      {
         "id": "bafyreicbupmkyj2immelaykwfm6riwazshyyqyd3s6wr52xu66biaruuqq",
         "previousidsList": [
            "bafyreighflykpquuuwo5jxvgfk7tugbb2qaeacnev2bdpksooyqm3sfjku"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661879,
         "groupid": 0
      },
      {
         "id": "bafyreighflykpquuuwo5jxvgfk7tugbb2qaeacnev2bdpksooyqm3sfjku",
         "previousidsList": [
            "bafyreie7slmh65hvu7yoszwciksysu4tmqvtlsa7zxeq3lc34qupks3bgm"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661879,
         "groupid": 0
      },
      {
         "id": "bafyreie7slmh65hvu7yoszwciksysu4tmqvtlsa7zxeq3lc34qupks3bgm",
         "previousidsList": [
            "bafyreie4a4ha52pe5shppg43braxjvlijs5g7sgovapslabbe2rpvhmivu"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661878,
         "groupid": 0
      },
      {
         "id": "bafyreie4a4ha52pe5shppg43braxjvlijs5g7sgovapslabbe2rpvhmivu",
         "previousidsList": [
            "bafyreihwrorjqfkou2jszgc7gev7dmmedt22boqlc7z4gnlmaigrlepszq"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661878,
         "groupid": 0
      },
      {
         "id": "bafyreihwrorjqfkou2jszgc7gev7dmmedt22boqlc7z4gnlmaigrlepszq",
         "previousidsList": [
            "bafyreiabp5dzgkpxjzl7osxeavoxuj3v52mkmdqnndvy624ap63xpj6jcq"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661877,
         "groupid": 0
      },
      {
         "id": "bafyreiabp5dzgkpxjzl7osxeavoxuj3v52mkmdqnndvy624ap63xpj6jcq",
         "previousidsList": [
            "bafyreiaeoh7633zwqmsbmax5ukny46b3dcwc45mbyzaagg2i23eoilcbpi"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661875,
         "groupid": 0
      },
      {
         "id": "bafyreiaeoh7633zwqmsbmax5ukny46b3dcwc45mbyzaagg2i23eoilcbpi",
         "previousidsList": [
            "bafyreickiyalcyafwvx7rzuuqhtlbk5nn4ev6d3y2s2txgdmyyqytyzzya"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661875,
         "groupid": 0
      },
      {
         "id": "bafyreickiyalcyafwvx7rzuuqhtlbk5nn4ev6d3y2s2txgdmyyqytyzzya",
         "previousidsList": [
            "bafyreid2dvsjx4z74hbp4tf4l5spyu6ktpuhkij2odkweco344zv3cdx54"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661874,
         "groupid": 0
      },
      {
         "id": "bafyreid2dvsjx4z74hbp4tf4l5spyu6ktpuhkij2odkweco344zv3cdx54",
         "previousidsList": [
            "bafyreibp52knm5mv7mzsvqdvnce5pis65wgfm3okhw6zrxhbqmbwwl3xpy"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661874,
         "groupid": 0
      },
      {
         "id": "bafyreibp52knm5mv7mzsvqdvnce5pis65wgfm3okhw6zrxhbqmbwwl3xpy",
         "previousidsList": [
            "bafyreiha2dxnhgfqctqysj7hwnwda5vvjdfpn7r3wa4bgh44w5p26zoagq"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661872,
         "groupid": 0
      },
      {
         "id": "bafyreiha2dxnhgfqctqysj7hwnwda5vvjdfpn7r3wa4bgh44w5p26zoagq",
         "previousidsList": [
            "bafyreihy47nfi27ql6cmmmugm6ba5ma4h47k6xnyat735bzsiw2kubq7zy"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661871,
         "groupid": 0
      },
      {
         "id": "bafyreihy47nfi27ql6cmmmugm6ba5ma4h47k6xnyat735bzsiw2kubq7zy",
         "previousidsList": [
            "bafyreibk4oxduncxp5yhhxbtop3mtfrjsp446mtnqresz3tpp5ex2kyapi"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661867,
         "groupid": 0
      },
      {
         "id": "bafyreibk4oxduncxp5yhhxbtop3mtfrjsp446mtnqresz3tpp5ex2kyapi",
         "previousidsList": [
            "bafyreigphxvnjd2oj4t3f7spemh774kifuos6j32smduwhvx3nqz5is2du"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661867,
         "groupid": 0
      },
      {
         "id": "bafyreigphxvnjd2oj4t3f7spemh774kifuos6j32smduwhvx3nqz5is2du",
         "previousidsList": [
            "bafyreid6wqekmrzfqnnjhnc6elp5biqjhxs3ficeizv6o2k6op6cvpftzi"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661864,
         "groupid": 0
      },
      {
         "id": "bafyreid6wqekmrzfqnnjhnc6elp5biqjhxs3ficeizv6o2k6op6cvpftzi",
         "previousidsList": [
            "bafyreigsfpvqpor6r2dayzxblmbwalqa5kqxnoxpz3x552oiqop625xt6m"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661863,
         "groupid": 0
      },
      {
         "id": "bafyreigsfpvqpor6r2dayzxblmbwalqa5kqxnoxpz3x552oiqop625xt6m",
         "previousidsList": [
            "bafyreihppzog2dgbnqurlfktmzcy76t6hmzov3odcicr7rqvnehzq7nkku"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661862,
         "groupid": 0
      },
      {
         "id": "bafyreihppzog2dgbnqurlfktmzcy76t6hmzov3odcicr7rqvnehzq7nkku",
         "previousidsList": [
            "bafyreiej3ntfihr6nh43kd455eogtkezpnba5hnu7zi7gixkgrzpega43a"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661862,
         "groupid": 0
      },
      {
         "id": "bafyreiej3ntfihr6nh43kd455eogtkezpnba5hnu7zi7gixkgrzpega43a",
         "previousidsList": [
            "bafyreieep4jsce4bauxfacog6f5ogncoksvbh4cccmgjifdfngkhacbfea"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661861,
         "groupid": 0
      },
      {
         "id": "bafyreieep4jsce4bauxfacog6f5ogncoksvbh4cccmgjifdfngkhacbfea",
         "previousidsList": [
            "bafyreibrirmihpgyyr4jg4kpj7joa4ekqt3etkmbkgmdqsywbvxc7h5nwi"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661861,
         "groupid": 0
      },
      {
         "id": "bafyreibrirmihpgyyr4jg4kpj7joa4ekqt3etkmbkgmdqsywbvxc7h5nwi",
         "previousidsList": [
            "bafyreifuva7ucycc4g7hgc2gtrceomfzoakatcv2hgr3q4ozqjpourmyaa"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661861,
         "groupid": 0
      },
      {
         "id": "bafyreifuva7ucycc4g7hgc2gtrceomfzoakatcv2hgr3q4ozqjpourmyaa",
         "previousidsList": [
            "bafyreicnsy53ekz4orhkuh4dfvpe6hdf5ejfiajmbijbgaqddcfymz7aeu"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661861,
         "groupid": 0
      },
      {
         "id": "bafyreicnsy53ekz4orhkuh4dfvpe6hdf5ejfiajmbijbgaqddcfymz7aeu",
         "previousidsList": [
            "bafyreib4j5tkrkachlc2hwg6s4hothylqy5w3wndel7s5dlaynd76lfvx4"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661858,
         "groupid": 0
      },
      {
         "id": "bafyreib4j5tkrkachlc2hwg6s4hothylqy5w3wndel7s5dlaynd76lfvx4",
         "previousidsList": [
            "bafyreihu6jv2pyhx4mzthh6rbfazx5zrhbevw2wp2j5foydgzk2ghd2gqy"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661857,
         "groupid": 0
      },
      {
         "id": "bafyreihu6jv2pyhx4mzthh6rbfazx5zrhbevw2wp2j5foydgzk2ghd2gqy",
         "previousidsList": [
            "bafyreihoobt6sjyqqccbwh6hglqmlx2gukr4uvdmjpip3hlmibdslj2lbe"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661857,
         "groupid": 0
      },
      {
         "id": "bafyreihoobt6sjyqqccbwh6hglqmlx2gukr4uvdmjpip3hlmibdslj2lbe",
         "previousidsList": [
            "bafyreigixgjqxdxwfem6k3rcwuj4vwra3h6g5sa4q7ixgrb34uk5k3dnku"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661856,
         "groupid": 0
      },
      {
         "id": "bafyreigixgjqxdxwfem6k3rcwuj4vwra3h6g5sa4q7ixgrb34uk5k3dnku",
         "previousidsList": [
            "bafyreiagvamqsulkp7c2fjlvo7s7i5pzdkktsgkb56i5zfsm2nbit5bdqy"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661856,
         "groupid": 0
      },
      {
         "id": "bafyreiagvamqsulkp7c2fjlvo7s7i5pzdkktsgkb56i5zfsm2nbit5bdqy",
         "previousidsList": [
            "bafyreif45npzsqk7vagv4wok6vfzkdir3csbz6ba4hrj5soyk5pplb6vpy"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661856,
         "groupid": 0
      },
      {
         "id": "bafyreif45npzsqk7vagv4wok6vfzkdir3csbz6ba4hrj5soyk5pplb6vpy",
         "previousidsList": [
            "bafyreiefocg6bje5xepvhxjjf7qpd4vn3yensducpvlzkk2332uqbizo2e"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661855,
         "groupid": 0
      },
      {
         "id": "bafyreiefocg6bje5xepvhxjjf7qpd4vn3yensducpvlzkk2332uqbizo2e",
         "previousidsList": [
            "bafyreianjolz3xhzwhp7zlyfjdf3euwitxykqk6iutliacq7vn5cavyere"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661855,
         "groupid": 0
      },
      {
         "id": "bafyreianjolz3xhzwhp7zlyfjdf3euwitxykqk6iutliacq7vn5cavyere",
         "previousidsList": [
            "bafyreieipd7lhftthbnoqflzisv3qwnadzhvbeafrylat32opbheoairey"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661854,
         "groupid": 0
      },
      {
         "id": "bafyreieipd7lhftthbnoqflzisv3qwnadzhvbeafrylat32opbheoairey",
         "previousidsList": [
            "bafyreiabbe27yobelkpwwcoyjgkar6ukhhrctjmxnfjqzvl2gmgoq5bzrm"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661852,
         "groupid": 0
      },
      {
         "id": "bafyreiabbe27yobelkpwwcoyjgkar6ukhhrctjmxnfjqzvl2gmgoq5bzrm",
         "previousidsList": [
            "bafyreibrthqxhvizofui76iowfbzrgp3guevctmdp2vbcbo3n7y6pa6jxa"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661852,
         "groupid": 0
      },
      {
         "id": "bafyreibrthqxhvizofui76iowfbzrgp3guevctmdp2vbcbo3n7y6pa6jxa",
         "previousidsList": [
            "bafyreifhxu7erdyksmozqzdpqesfk5s44jvojuhuxxvqckjnexi4lzwem4"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661851,
         "groupid": 0
      },
      {
         "id": "bafyreifhxu7erdyksmozqzdpqesfk5s44jvojuhuxxvqckjnexi4lzwem4",
         "previousidsList": [
            "bafyreiafo4skscrsemyfsgecqd2bm3gh4plzs7pfw2fyuh52g2miguqa2u"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661851,
         "groupid": 0
      },
      {
         "id": "bafyreiafo4skscrsemyfsgecqd2bm3gh4plzs7pfw2fyuh52g2miguqa2u",
         "previousidsList": [
            "bafyreidboi5v4wvcsyx4x7tditit7vusucvofnb3gs3jzsgdzn5qkrtfaq"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661850,
         "groupid": 0
      },
      {
         "id": "bafyreidboi5v4wvcsyx4x7tditit7vusucvofnb3gs3jzsgdzn5qkrtfaq",
         "previousidsList": [
            "bafyreifpnqyegldbhzjxmsyhajhd6nvcmx5sjgcqkrogdmypjahoxyyn4m"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661850,
         "groupid": 0
      },
      {
         "id": "bafyreifpnqyegldbhzjxmsyhajhd6nvcmx5sjgcqkrogdmypjahoxyyn4m",
         "previousidsList": [
            "bafyreibwocmahj47jch7tqn4jr3n7hb7p65r73mwotxc5mlguobx375qyy"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661849,
         "groupid": 0
      },
      {
         "id": "bafyreibwocmahj47jch7tqn4jr3n7hb7p65r73mwotxc5mlguobx375qyy",
         "previousidsList": [
            "bafyreidecljo45bwkx23i3xuparrgbqxlp67asiv2tuik6urkcep72kpri"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661849,
         "groupid": 0
      },
      {
         "id": "bafyreidecljo45bwkx23i3xuparrgbqxlp67asiv2tuik6urkcep72kpri",
         "previousidsList": [
            "bafyreig2aketl7tiyttblhtnrwblxjh7bmiwmoj4ka3pcfkex5yptclyeu"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661849,
         "groupid": 0
      },
      {
         "id": "bafyreig2aketl7tiyttblhtnrwblxjh7bmiwmoj4ka3pcfkex5yptclyeu",
         "previousidsList": [
            "bafyreieizic3fykto46nrgwtswllaibrtxwxuabhycrvj3ofobat7wbhhm"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661847,
         "groupid": 0
      },
      {
         "id": "bafyreieizic3fykto46nrgwtswllaibrtxwxuabhycrvj3ofobat7wbhhm",
         "previousidsList": [
            "bafyreihk6kax7tk7mxrmqmwya2agvffgj5fd3jvgwqi4rji44heaxym2t4"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1599661847,
         "groupid": 0
      },
      {
         "id": "bafyreihk6kax7tk7mxrmqmwya2agvffgj5fd3jvgwqi4rji44heaxym2t4",
         "previousidsList": [
            "bafyreiaaq2ruuo47qad6zzimgi6ntmnzdyz2jndywc4jpejppt27aasrni"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597916075,
         "groupid": 1
      },
      {
         "id": "bafyreiaaq2ruuo47qad6zzimgi6ntmnzdyz2jndywc4jpejppt27aasrni",
         "previousidsList": [
            "bafyreihznpxbbcdhkkuaj67wjsyjleejreekn6cvuosrnp4b6trj4sxtky"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597916075,
         "groupid": 1
      },
      {
         "id": "bafyreihznpxbbcdhkkuaj67wjsyjleejreekn6cvuosrnp4b6trj4sxtky",
         "previousidsList": [
            "bafyreide7tzbqbu34b6fwkxqzax7y3wohpgjschl5gyoer7tq7ckqxxdzu"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597916062,
         "groupid": 1
      },
      {
         "id": "bafyreide7tzbqbu34b6fwkxqzax7y3wohpgjschl5gyoer7tq7ckqxxdzu",
         "previousidsList": [
            "bafyreiaf7nwxeit2ncpzjgnnjp2lhqslgnfhvaov4s32r7m2v7fwzu667e"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597916062,
         "groupid": 1
      },
      {
         "id": "bafyreiaf7nwxeit2ncpzjgnnjp2lhqslgnfhvaov4s32r7m2v7fwzu667e",
         "previousidsList": [
            "bafyreidcsycsrosa7roa22pmwd7fd6slw7twaxsqdmgogo33xhwrx7pnrm"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597916062,
         "groupid": 1
      },
      {
         "id": "bafyreidcsycsrosa7roa22pmwd7fd6slw7twaxsqdmgogo33xhwrx7pnrm",
         "previousidsList": [
            "bafyreigfkm5lilq3vsglpv6cfvon3jqoumwv47pemykir2gdhou7n3l7am"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597916060,
         "groupid": 1
      },
      {
         "id": "bafyreigfkm5lilq3vsglpv6cfvon3jqoumwv47pemykir2gdhou7n3l7am",
         "previousidsList": [
            "bafyreiatz2sixf6jhmfrjuw2mva5mumwwsgmcu5lxlredmrulb6jtb66mu"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597916059,
         "groupid": 1
      },
      {
         "id": "bafyreiatz2sixf6jhmfrjuw2mva5mumwwsgmcu5lxlredmrulb6jtb66mu",
         "previousidsList": [
            "bafyreihzdhbtzx2oxw24leiaqexq7jlx5per7rooqwuh6nrqb5ys6sznlu"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597916059,
         "groupid": 1
      },
      {
         "id": "bafyreihzdhbtzx2oxw24leiaqexq7jlx5per7rooqwuh6nrqb5ys6sznlu",
         "previousidsList": [
            "bafyreidg2xh4ksncb6ofhvt3mh2oahkhnhnhqueknxgk35ud53mpyxk33q"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597916059,
         "groupid": 1
      },
      {
         "id": "bafyreidg2xh4ksncb6ofhvt3mh2oahkhnhnhqueknxgk35ud53mpyxk33q",
         "previousidsList": [
            "bafyreiarvp63so2yxxyp4lvopolddmaugljxf2bp4wp75o2h455wywgbwm"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597916058,
         "groupid": 1
      },
      {
         "id": "bafyreiarvp63so2yxxyp4lvopolddmaugljxf2bp4wp75o2h455wywgbwm",
         "previousidsList": [
            "bafyreibwdpt3mhh2vv5kqx5vy563zoyacsfkqs3q2wxktm6a2z6iqayt6u"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597916058,
         "groupid": 1
      },
      {
         "id": "bafyreibwdpt3mhh2vv5kqx5vy563zoyacsfkqs3q2wxktm6a2z6iqayt6u",
         "previousidsList": [
            "bafyreigguozy43qxfgmnoct6k22g4nnpbcl7dacdl2msop32qrimhfo6yy"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597916058,
         "groupid": 1
      },
      {
         "id": "bafyreigguozy43qxfgmnoct6k22g4nnpbcl7dacdl2msop32qrimhfo6yy",
         "previousidsList": [
            "bafyreifbzz6clxx6t2ly75rtsihltiqyad63jhzovbl5pbjyumgncws4yy"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597916058,
         "groupid": 1
      },
      {
         "id": "bafyreifbzz6clxx6t2ly75rtsihltiqyad63jhzovbl5pbjyumgncws4yy",
         "previousidsList": [
            "bafyreigdan5afpbctbxhvn4kqz3cgctqfn7qhisppoatqzxm7kvg2u4gcm"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597916057,
         "groupid": 1
      },
      {
         "id": "bafyreigdan5afpbctbxhvn4kqz3cgctqfn7qhisppoatqzxm7kvg2u4gcm",
         "previousidsList": [
            "bafyreia542dpvisz7bnjtw63tl3ep2wzqkwq32equwetdz2ycvom5jz6fi"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597916056,
         "groupid": 1
      },
      {
         "id": "bafyreia542dpvisz7bnjtw63tl3ep2wzqkwq32equwetdz2ycvom5jz6fi",
         "previousidsList": [
            "bafyreibtdmltd5jttzih6eabfp7cxhn34w6ibowm3haxtgubqposmml5qq"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597916056,
         "groupid": 1
      },
      {
         "id": "bafyreibtdmltd5jttzih6eabfp7cxhn34w6ibowm3haxtgubqposmml5qq",
         "previousidsList": [
            "bafyreibk6yl2yndqec2yitvy6k67onei6a2b3romj5oemhvzs5ottasmri"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597916056,
         "groupid": 1
      },
      {
         "id": "bafyreibk6yl2yndqec2yitvy6k67onei6a2b3romj5oemhvzs5ottasmri",
         "previousidsList": [
            "bafyreidm7pio2kh5i2vcofl3mgoltubx2fxuj46arftappnhq3ayodcp3m"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597916056,
         "groupid": 1
      },
      {
         "id": "bafyreidm7pio2kh5i2vcofl3mgoltubx2fxuj46arftappnhq3ayodcp3m",
         "previousidsList": [
            "bafyreigoms5fhcxtjx6zmnq3cyhivqq4zs4klemm27iuto7treblewd3di"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597916055,
         "groupid": 1
      },
      {
         "id": "bafyreigoms5fhcxtjx6zmnq3cyhivqq4zs4klemm27iuto7treblewd3di",
         "previousidsList": [
            "bafyreihlgirn5xylm4vi5a45f4cmudtlj7dg3e26frvybgnnpy7mtb4d6m"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597916055,
         "groupid": 1
      },
      {
         "id": "bafyreihlgirn5xylm4vi5a45f4cmudtlj7dg3e26frvybgnnpy7mtb4d6m",
         "previousidsList": [
            "bafyreiaffbik7d75xwc57tfxygpjzcns5cgc6hxkbkgsy5incs37sxda74"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597916055,
         "groupid": 1
      },
      {
         "id": "bafyreiaffbik7d75xwc57tfxygpjzcns5cgc6hxkbkgsy5incs37sxda74",
         "previousidsList": [
            "bafyreicm2wjsowpuwt7l7ukogbse3q2alasjl27rs3mmsfdjq4n577rkfi"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597916054,
         "groupid": 1
      },
      {
         "id": "bafyreicm2wjsowpuwt7l7ukogbse3q2alasjl27rs3mmsfdjq4n577rkfi",
         "previousidsList": [
            "bafyreibuqir5x2hjw2acgla4dd3tre2h6ohbif2mrn4rjrv3v6o2ekrsya"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597916054,
         "groupid": 1
      },
      {
         "id": "bafyreibuqir5x2hjw2acgla4dd3tre2h6ohbif2mrn4rjrv3v6o2ekrsya",
         "previousidsList": [
            "bafyreicwgwn3kxrf7b7snbvi7jvdgcme37nbxdn2vbdwbqfllvu3e7up3u"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597916054,
         "groupid": 1
      },
      {
         "id": "bafyreicwgwn3kxrf7b7snbvi7jvdgcme37nbxdn2vbdwbqfllvu3e7up3u",
         "previousidsList": [
            "bafyreiezkcrp7otnycae2a6l7uzsuz7dl4sjyfksq3u2t42it4vcghz7my"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597916053,
         "groupid": 1
      },
      {
         "id": "bafyreiezkcrp7otnycae2a6l7uzsuz7dl4sjyfksq3u2t42it4vcghz7my",
         "previousidsList": [
            "bafyreielz7i7rfo5o22hwo5kdhdbsobsxlutpczd2a4aiwfizayxikdcme"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597916052,
         "groupid": 1
      },
      {
         "id": "bafyreielz7i7rfo5o22hwo5kdhdbsobsxlutpczd2a4aiwfizayxikdcme",
         "previousidsList": [
            "bafyreicyvs4mqxgeq6dspkuu22aetokevh6y76sgfxggjmjycftza6x444"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597916052,
         "groupid": 1
      },
      {
         "id": "bafyreicyvs4mqxgeq6dspkuu22aetokevh6y76sgfxggjmjycftza6x444",
         "previousidsList": [
            "bafyreibfwrof3pqoomqnvbqivdy23om5cdvyo74hjw4eo5ex2jjafpxbp4"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597916051,
         "groupid": 1
      },
      {
         "id": "bafyreibfwrof3pqoomqnvbqivdy23om5cdvyo74hjw4eo5ex2jjafpxbp4",
         "previousidsList": [
            "bafyreiajfucao3nj42phrgi7ewfqyfajwbjsl4vcfwwjwcugcvm4tcb32a"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597916051,
         "groupid": 1
      },
      {
         "id": "bafyreiajfucao3nj42phrgi7ewfqyfajwbjsl4vcfwwjwcugcvm4tcb32a",
         "previousidsList": [
            "bafyreiesxaepszlf3qz7rqa35xemsndgwq2qljlmx77cgr2bzvmiu7hy6m"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597916051,
         "groupid": 1
      },
      {
         "id": "bafyreiesxaepszlf3qz7rqa35xemsndgwq2qljlmx77cgr2bzvmiu7hy6m",
         "previousidsList": [
            "bafyreiemkngls23b7g4qlvkhlutx2bbqf5h3vduyqi27oszikfhdm4tgwa",
            "bafyreiffkmn7w5ugxdinor7jz6zufp5ocg3wgdck5x7eq73purxzyo5h2a"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597916050,
         "groupid": 1
      },
      {
         "id": "bafyreiffkmn7w5ugxdinor7jz6zufp5ocg3wgdck5x7eq73purxzyo5h2a",
         "previousidsList": [
            "bafyreifjcbekttlizfq7ahlsi33whyhoulqynnfus5nm54yneomev62ouy"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597341932,
         "groupid": 2
      },
      {
         "id": "bafyreifjcbekttlizfq7ahlsi33whyhoulqynnfus5nm54yneomev62ouy",
         "previousidsList": [
            "bafyreigr5g2avbncwk6yhpviazdz5qyur6aepoedkj4p46eqovvflsrgum"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597341922,
         "groupid": 2
      },
      {
         "id": "bafyreigr5g2avbncwk6yhpviazdz5qyur6aepoedkj4p46eqovvflsrgum",
         "previousidsList": [
            "bafyreidwhcektqdgoblkwqzvhqa6g2vx7jujckfne64vy42ziwemvi3ll4"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597341921,
         "groupid": 2
      },
      {
         "id": "bafyreidwhcektqdgoblkwqzvhqa6g2vx7jujckfne64vy42ziwemvi3ll4",
         "previousidsList": [
            "bafyreieq2h4bb4peoy3rizwarfcd2saxki3kyafikxvymhq3iguef4wh4q"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597341278,
         "groupid": 3
      },
      {
         "id": "bafyreieq2h4bb4peoy3rizwarfcd2saxki3kyafikxvymhq3iguef4wh4q",
         "previousidsList": [
            "bafyreihe3llptaluzd4r6mzjqtgtjojg6bmnqpa5jc5rum3qhequfho2fu"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597341276,
         "groupid": 3
      },
      {
         "id": "bafyreihe3llptaluzd4r6mzjqtgtjojg6bmnqpa5jc5rum3qhequfho2fu",
         "previousidsList": [
            "bafyreidep5dwkocoz6zapf2tcdhq45s6hga7jwmxursg6v3jhsf3gk5zmq"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597341276,
         "groupid": 3
      },
      {
         "id": "bafyreidep5dwkocoz6zapf2tcdhq45s6hga7jwmxursg6v3jhsf3gk5zmq",
         "previousidsList": [
            "bafyreifvmz6sxoixr3ou6pfbcdeeoctbh2qjl2mgnmihdfc34kac4d2n6y"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597341275,
         "groupid": 3
      },
      {
         "id": "bafyreifvmz6sxoixr3ou6pfbcdeeoctbh2qjl2mgnmihdfc34kac4d2n6y",
         "previousidsList": [
            "bafyreiexsiikz7gazizf2clvacz4abgyqhswcitecxc6hvlcgexvfnsq4i"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597341199,
         "groupid": 3
      },
      {
         "id": "bafyreiexsiikz7gazizf2clvacz4abgyqhswcitecxc6hvlcgexvfnsq4i",
         "previousidsList": [
            "bafyreieccf6bbiu4hifs4aziyow46kjfnwuhoauzkrodlt2q4p7u6tf7ce"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597341183,
         "groupid": 3
      },
      {
         "id": "bafyreieccf6bbiu4hifs4aziyow46kjfnwuhoauzkrodlt2q4p7u6tf7ce",
         "previousidsList": [
            "bafyreiaarx6teqwo3hgvsqizuzig26gdhjecp3tgxca7l5yd2rhdgo7raq"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597341182,
         "groupid": 3
      },
      {
         "id": "bafyreiaarx6teqwo3hgvsqizuzig26gdhjecp3tgxca7l5yd2rhdgo7raq",
         "previousidsList": [
            "bafyreigqvu76v5buhano4cvwr5wqsxxakpimsh67h57mfjddv2k53pnadu"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597341182,
         "groupid": 3
      },
      {
         "id": "bafyreigqvu76v5buhano4cvwr5wqsxxakpimsh67h57mfjddv2k53pnadu",
         "previousidsList": [
            "bafyreienz6xuxm4gbm7qk4et2hck7g62ogtc2fu7v6ohwx5nkpnb67mq6y"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597341182,
         "groupid": 3
      },
      {
         "id": "bafyreienz6xuxm4gbm7qk4et2hck7g62ogtc2fu7v6ohwx5nkpnb67mq6y",
         "previousidsList": [
            "bafyreid33hymm3hrt37a3gn66eobqdzotothfszsiofdcgnemx3ztwu6ua"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597341182,
         "groupid": 3
      },
      {
         "id": "bafyreid33hymm3hrt37a3gn66eobqdzotothfszsiofdcgnemx3ztwu6ua",
         "previousidsList": [
            "bafyreigmusc2zqt57w2n3xbo7sf5liyxp5se64fupg7wyo6dawzzy4n2hy"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597341182,
         "groupid": 3
      },
      {
         "id": "bafyreigmusc2zqt57w2n3xbo7sf5liyxp5se64fupg7wyo6dawzzy4n2hy",
         "previousidsList": [
            "bafyreiairiotfvmkflysp4vlhiqvxw5ahugpdy3xe4qozxemhptpuoeb2i"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597341181,
         "groupid": 3
      },
      {
         "id": "bafyreiairiotfvmkflysp4vlhiqvxw5ahugpdy3xe4qozxemhptpuoeb2i",
         "previousidsList": [
            "bafyreihx2aq3nzsjnxfe6xfflc2kfsgbrviirppv2chijbcyuxtfa7ayai"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597341181,
         "groupid": 3
      },
      {
         "id": "bafyreihx2aq3nzsjnxfe6xfflc2kfsgbrviirppv2chijbcyuxtfa7ayai",
         "previousidsList": [
            "bafyreibdv73llyccbdsoqwm46qvycvixt2ygqw5kwqazupyapa37bdiezi"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597341181,
         "groupid": 3
      },
      {
         "id": "bafyreibdv73llyccbdsoqwm46qvycvixt2ygqw5kwqazupyapa37bdiezi",
         "previousidsList": [
            "bafyreib27ycqzkitjnmzo7hje45i63hhlrfoi2nh3woq7bhh4m7q6yc42q"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597341180,
         "groupid": 3
      },
      {
         "id": "bafyreib27ycqzkitjnmzo7hje45i63hhlrfoi2nh3woq7bhh4m7q6yc42q",
         "previousidsList": [
            "bafyreifdv2rkiz67l4gd2qmycvtynvwsku55g27ixmljo6swdqvl5ncw4m"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597341180,
         "groupid": 3
      },
      {
         "id": "bafyreifdv2rkiz67l4gd2qmycvtynvwsku55g27ixmljo6swdqvl5ncw4m",
         "previousidsList": [
            "bafyreiegfe5wky5ltxk5xypaygydlzahzuclco3nidpkxmhbyxryxvmuqy"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597339108,
         "groupid": 4
      },
      {
         "id": "bafyreiegfe5wky5ltxk5xypaygydlzahzuclco3nidpkxmhbyxryxvmuqy",
         "previousidsList": [
            "bafyreihnlzryonowi6rlgmrvt63qqpbwlss56wrkw2nxp5ianlnido4ocy"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597339047,
         "groupid": 4
      },
      {
         "id": "bafyreihnlzryonowi6rlgmrvt63qqpbwlss56wrkw2nxp5ianlnido4ocy",
         "previousidsList": [
            "bafyreih5s7vsiwjeovqheharnludbylykbyjmxo4aegok2bofitk5p7iz4"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597339046,
         "groupid": 4
      },
      {
         "id": "bafyreih5s7vsiwjeovqheharnludbylykbyjmxo4aegok2bofitk5p7iz4",
         "previousidsList": [
            "bafyreiha7ab5ptpjvnq3kzm6gz7l6mpcxh3sset2qrjkn6z34y76o3tq2a"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597339035,
         "groupid": 4
      },
      {
         "id": "bafyreiha7ab5ptpjvnq3kzm6gz7l6mpcxh3sset2qrjkn6z34y76o3tq2a",
         "previousidsList": [
            "bafyreihc7tyipmvmxcig4o3v2bsskrsclmyq34svqaapoycauugjq4uikq"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597339035,
         "groupid": 4
      },
      {
         "id": "bafyreihc7tyipmvmxcig4o3v2bsskrsclmyq34svqaapoycauugjq4uikq",
         "previousidsList": [
            "bafyreibhauudeqxcdyb5kpjvlmxmgt55ue5yu5cwzxsaj5qgmoube3fey4"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597339035,
         "groupid": 4
      },
      {
         "id": "bafyreibhauudeqxcdyb5kpjvlmxmgt55ue5yu5cwzxsaj5qgmoube3fey4",
         "previousidsList": [
            "bafyreiagkgqv5bixixp5fkxjjhz75esxqtfvsqs6eaom3ajok57xvwo4jy"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597339033,
         "groupid": 4
      },
      {
         "id": "bafyreiagkgqv5bixixp5fkxjjhz75esxqtfvsqs6eaom3ajok57xvwo4jy",
         "previousidsList": [
            "bafyreibukvez5asqmvow7lkxc7ptxbcxq74wanhjo3t77x2otyfscakabq"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597339032,
         "groupid": 4
      },
      {
         "id": "bafyreibukvez5asqmvow7lkxc7ptxbcxq74wanhjo3t77x2otyfscakabq",
         "previousidsList": [
            "bafyreighjj2xqahjacfwwgwqxu2c55xnud4uiwpifx2hqwecglrcqbrriy"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597339032,
         "groupid": 4
      },
      {
         "id": "bafyreighjj2xqahjacfwwgwqxu2c55xnud4uiwpifx2hqwecglrcqbrriy",
         "previousidsList": [
            "bafyreiassybyk2npiyrqx7iag6b7xjduhzq6eoowmj6uzh576h3j6bc4em"
         ],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597339031,
         "groupid": 4
      },
      {
         "id": "bafyreiassybyk2npiyrqx7iag6b7xjduhzq6eoowmj6uzh576h3j6bc4em",
         "previousidsList": [],
         "authorid": "bafkrcmumctbpm4y7xasy2wwt5wv2fckfcriclbyvqbiowvxsolglcs2j",
         "authorname": "Shared acc++",
         "time": 1597327788,
         "groupid": 5
      }
   ]`
