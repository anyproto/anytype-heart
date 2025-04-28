package sourceimpl

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
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
