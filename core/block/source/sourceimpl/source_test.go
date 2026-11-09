package sourceimpl

import (
	"fmt"
	"os"
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree/mock_objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacedomain"
)

func Test_snapshotChance(t *testing.T) {
	if os.Getenv("ANYTYPE_TEST_SNAPSHOT_CHANCE") == "" {
		t.Skip()
		return
	}
	for i := 0; i <= 500; i++ {
		for s := 0; s <= 10000; s++ {
			if snapshotChance(s) {
				fmt.Println(s)
				break
			}
		}
	}
	fmt.Println()
	// here is an example of distribution histogram
	// https://docs.google.com/spreadsheets/d/1xgH7fUxno5Rm-0VEaSD4LsTHeGeUXQFmHsOm29M6paI
}

func Test_snapshotChance2(t *testing.T) {
	if os.Getenv("ANYTYPE_TEST_SNAPSHOT_CHANCE") == "" {
		t.Skip()
		return
	}
	for s := 0; s <= 10000; s++ {
		total := 0
		for i := 0; i <= 50000; i++ {
			if snapshotChance(s) {
				total++
			}
		}
		fmt.Printf("%d\t%.5f\n", s, float64(total)/50000)
	}

	// here is an example of distribution histogram
	// https://docs.google.com/spreadsheets/d/1xgH7fUxno5Rm-0VEaSD4LsTHeGeUXQFmHsOm29M6paI
}

type stubChangeReceiver struct {
	doc state.Doc
}

func (r *stubChangeReceiver) StateAppend(f func(d state.Doc) (s *state.State, changes []*pb.ChangeContent, err error)) error {
	_, _, err := f(r.doc)
	return err
}

func (r *stubChangeReceiver) StateRebuild(d state.Doc) error {
	return nil
}

func TestTreeSource_Update_ChangesSinceSnapshot(t *testing.T) {
	newChange := func(id string, prev string, snapshot *pb.ChangeSnapshot) *objecttree.Change {
		_, pub, err := crypto.GenerateRandomEd25519KeyPair()
		require.NoError(t, err)
		return &objecttree.Change{
			Id:          id,
			PreviousIds: []string{prev},
			Identity:    pub,
			Timestamp:   1,
			Model:       &pb.Change{Snapshot: snapshot},
		}
	}

	newFixture := func(t *testing.T, changesSinceSnapshot int, batch []*objecttree.Change) *treeSource {
		ctrl := gomock.NewController(t)
		tree := mock_objecttree.NewMockObjectTree(ctrl)

		payload, err := (&model.ObjectChangePayload{SmartBlockType: model.SmartBlockType_Page}).Marshal()
		require.NoError(t, err)
		rootChange, err := (&treechangeproto.RootChange{
			ChangeType:    spacedomain.ChangeType,
			ChangePayload: payload,
		}).MarshalVT()
		require.NoError(t, err)
		rawRoot, err := (&treechangeproto.RawTreeChange{Payload: rootChange}).MarshalVT()
		require.NoError(t, err)

		tree.EXPECT().Header().Return(&treechangeproto.RawTreeChangeWithId{RawChange: rawRoot, Id: "treeId"}).AnyTimes()
		tree.EXPECT().Id().Return("treeId").AnyTimes()
		tree.EXPECT().Root().Return(&objecttree.Change{Id: "snapshotId"}).AnyTimes()
		tree.EXPECT().IterateFrom("head0", gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ string, _ objecttree.ChangeConvertFunc, f objecttree.ChangeIterateFunc) error {
				for _, ch := range batch {
					if !f(ch) {
						return nil
					}
				}
				return nil
			}).AnyTimes()

		doc := state.NewDoc("treeId", nil)
		doc.(*state.State).SetChangeId("head0")

		return &treeSource{
			ObjectTree:           tree,
			id:                   "treeId",
			spaceID:              "space1",
			receiver:             &stubChangeReceiver{doc: doc},
			changesSinceSnapshot: changesSinceSnapshot,
		}
	}

	t.Run("batch with a snapshot change resets the counter", func(t *testing.T) {
		// given
		batch := []*objecttree.Change{
			newChange("head0", "treeId", nil), // start change, already applied
			newChange("c1", "head0", nil),
			newChange("c2", "c1", &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{}}),
			newChange("c3", "c2", nil),
		}
		src := newFixture(t, 5, batch)

		// when
		require.NoError(t, src.Update(src.ObjectTree))

		// then: only one change was applied after the snapshot
		assert.Equal(t, 1, src.changesSinceSnapshot)
	})

	t.Run("batch without snapshot accumulates the counter", func(t *testing.T) {
		// given
		batch := []*objecttree.Change{
			newChange("head0", "treeId", nil),
			newChange("c1", "head0", nil),
			newChange("c2", "c1", nil),
		}
		src := newFixture(t, 5, batch)

		// when
		require.NoError(t, src.Update(src.ObjectTree))

		// then
		assert.Equal(t, 7, src.changesSinceSnapshot)
	})
}

func TestSource_CheckChangeSize(t *testing.T) {
	t.Run("big change", func(t *testing.T) {
		// given
		c := &pb.Change{Content: []*pb.ChangeContent{{&pb.ChangeContentValueOfRelationAdd{RelationAdd: &pb.ChangeRelationAdd{
			RelationLinks: []*model.RelationLink{{Key: bundle.RelationKeyName.String()}}},
		}}}}
		data, _ := c.Marshal()

		// when
		err := checkChangeSize(data, len(data)-1)

		// then
		assert.ErrorIs(t, err, source.ErrBigChangeSize)
	})

	t.Run("small change", func(t *testing.T) {
		// given
		c := &pb.Change{Content: []*pb.ChangeContent{{&pb.ChangeContentValueOfRelationAdd{RelationAdd: &pb.ChangeRelationAdd{
			RelationLinks: []*model.RelationLink{{Key: bundle.RelationKeyId.String()}}},
		}}}}
		data, _ := c.Marshal()

		// when
		err := checkChangeSize(data, len(data)+1)

		// then
		assert.NoError(t, err)
	})
}
