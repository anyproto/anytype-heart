package change

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/globalsign/mgo/bson"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestStateBuildCases(t *testing.T) {
	require.NoError(t, filepath.Walk("./testcases", func(path string, info os.FileInfo, err error) error {
		if filepath.Ext(info.Name()) == ".yml" {
			t.Run(strings.ReplaceAll(info.Name(), ".yml", ""), func(t *testing.T) {
				doTestCaseFile(t, path)
			})
		}
		return nil
	}))
}

func Test_Issue605(t *testing.T) {
	data, err := ioutil.ReadFile("./testdata/605_snapshot.pb")
	require.NoError(t, err)
	var base = &model.SmartBlockSnapshotBase{}
	require.NoError(t, base.Unmarshal(data))
	data, err = ioutil.ReadFile("./testdata/605_change.pb")
	require.NoError(t, err)
	var change = &pb.Change{}
	require.NoError(t, change.Unmarshal(data))
	blocks := make(map[string]simple.Block)
	for _, b := range base.Blocks {
		blocks[b.Id] = simple.New(b)
	}
	d := state.NewDoc("", blocks).(*state.State)
	s := d.NewState()
	s.ApplyChangeIgnoreErr(change.Content...)
	assert.NoError(t, s.Validate())
}

func Test_Issue605Tree(t *testing.T) {
	data, err := ioutil.ReadFile("./testdata/605_tree.pb")
	require.NoError(t, err)
	var changeSet map[string][]byte
	require.NoError(t, gob.NewDecoder(bytes.NewReader(data)).Decode(&changeSet))
	sb := NewTestSmartBlock()
	sb.changes = make(map[string]*core.SmartblockRecord)
	for k, v := range changeSet {
		sb.changes[k] = &core.SmartblockRecord{Payload: v}
	}
	t.Log("changes:", len(sb.changes))
	sb.logs = append(sb.logs, core.SmartblockLog{
		ID:   "one",
		Head: "bafyreidatuo2ooxzyao56ic2fm5ybyidheywbt4hgdubr3ow3lyvw7t37e",
	})

	tree, _, e := BuildTree(sb)
	require.NoError(t, e)
	var cnt int
	tree.Iterate(tree.RootId(), func(c *Change) (isContinue bool) {
		cnt++
		return true
	})
	assert.Equal(t, cnt, tree.Len())
	//gv, _ := tree.Graphviz()
	//ioutil.WriteFile("/home/che/gv.txt", []byte(gv), 0777)

	root := tree.Root()
	if root == nil || root.GetSnapshot() == nil {
		require.NoError(t, fmt.Errorf("root missing or not a snapshot"))
	}
	doc := state.NewDocFromSnapshot("bafyba6akblqzapgdmlu3swkrsb62mgrvgdv2g72eqvtufuis4vxhgcgl", root.GetSnapshot()).(*state.State)
	doc.SetChangeId(root.Id)
	st, err := BuildStateSimpleCRDT(doc, tree)
	if err != nil {
		return
	}
	if _, _, err = state.ApplyState(st, true); err != nil {
		return
	}
	t.Log(st.String())
}

type TestCase struct {
	Name     string
	Init     *DocStruct
	Changes  []*TestChange
	Expected *DocStruct
}

type DocStruct struct {
	Id    string
	Child []*DocStruct
}

type TestChange struct {
	Type  string
	Error bool
	Data  *TestChangeContent
}

type TestChangeContent struct {
	unmarshal func(interface{}) error
}

func (t *TestChangeContent) UnmarshalYAML(unmarshal func(interface{}) error) error {
	t.unmarshal = unmarshal
	return nil
}

func (t *TestChangeContent) GetChange(tp string) *pb.ChangeContent {
	var value pb.IsChangeContentValue
	switch tp {
	case "move":
		bm := &pb.ChangeBlockMove{}
		t.unmarshal(&bm)
		value = &pb.ChangeContentValueOfBlockMove{
			BlockMove: bm,
		}
	case "create":
		bc := &pb.ChangeBlockCreate{}
		t.unmarshal(&bc)
		value = &pb.ChangeContentValueOfBlockCreate{
			BlockCreate: bc,
		}
	}
	return &pb.ChangeContent{
		Value: value,
	}
}

func doTestCaseFile(t *testing.T, filename string) {
	data, err := ioutil.ReadFile(filename)
	require.NoError(t, err)
	var cases []*TestCase
	err = yaml.Unmarshal(data, &cases)
	require.NoError(t, err)
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			doTestCase(t, tc)
		})
	}
}

func doTestCase(t *testing.T, tc *TestCase) {
	d := state.NewDoc("root", nil).(*state.State)
	if tc.Init.Id == "" {
		tc.Init.Id = "root"
	}
	var fillStruct func(s *state.State, d *DocStruct)
	fillStruct = func(s *state.State, d *DocStruct) {
		var cont model.IsBlockContent
		if d.Id == "/column/" {
			d.Id = bson.NewObjectId().Hex()
			cont = &model.BlockContentOfLayout{
				Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_Column},
			}
		}
		if d.Id == "/row/" {
			d.Id = bson.NewObjectId().Hex()
			cont = &model.BlockContentOfLayout{
				Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_Row},
			}
		}
		b := &model.Block{Id: d.Id, Content: cont}
		for _, ch := range d.Child {
			fillStruct(s, ch)
			b.ChildrenIds = append(b.ChildrenIds, ch.Id)
		}
		s.Add(simple.New(b))
	}
	fillStruct(d, tc.Init)

	for i, ch := range tc.Changes {
		s := d.NewState()
		chc := ch.Data.GetChange(ch.Type)
		err := s.ApplyChange(chc)
		if !ch.Error {
			require.NoError(t, err, fmt.Sprintf("index: %d; change: %s", i, chc.String()))
		} else {
			require.Error(t, err, fmt.Sprintf("index: %d; change: %s", i, chc.String()))
		}

		_, _, err = state.ApplyState(s, false)
		require.NoError(t, err)
	}

	exp := state.NewDoc("root", nil).(*state.State)
	if tc.Expected.Id == "" {
		tc.Expected.Id = "root"
	}
	fillStruct(exp, tc.Expected)
	assert.Equal(t, stateToTestString(exp), stateToTestString(d))
}

func stateToTestString(s *state.State) string {
	var buf = bytes.NewBuffer(nil)
	var writeBlock func(id string, l int)
	writeBlock = func(id string, l int) {
		b := s.Pick(id)
		buf.WriteString(strings.Repeat("\t", l))
		if b == nil {
			buf.WriteString(id)
			buf.WriteString(" MISSING")
		} else {
			id := b.Model().Id
			if layout := b.Model().GetLayout(); layout != nil {
				id = "/" + strings.ToLower(layout.Style.String()) + "/"
			}
			buf.WriteString(id)
		}
		buf.WriteString("\n")
		if b != nil {
			for _, cid := range b.Model().ChildrenIds {
				writeBlock(cid, l+1)
			}
		}
	}
	writeBlock(s.RootId(), 0)
	return buf.String()
}
