package source

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/any-sync/accountservice/mock_accountservice"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree/mock_objecttree"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
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

func TestSource_PushChange(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	initTreeAndSource := func() (ot *mock_objecttree.MockObjectTree, s *source) {
		ot = mock_objecttree.NewMockObjectTree(ctrl)
		ot.EXPECT().Root().Return(&objecttree.Change{Id: ""})
		ot.EXPECT().IterateFrom(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		s = &source{ObjectTree: ot}
		return
	}

	t.Run("big change", func(t *testing.T) {
		//given
		_, s := initTreeAndSource()
		params := PushChangeParams{
			State: state.NewDoc("", nil).NewState(),
			Changes: []*pb.ChangeContent{{&pb.ChangeContentValueOfRelationAdd{RelationAdd: &pb.ChangeRelationAdd{
				RelationLinks: []*model.RelationLink{{Key: strings.Repeat("r", changeSizeLimit)}}},
			}}},
			DoSnapshot: true,
		}

		//when
		_, err := s.PushChange(params)

		//then
		assert.ErrorIs(t, err, ErrBigChangeSize)
	})

	t.Run("big block in snapshot", func(t *testing.T) {
		//given
		_, s := initTreeAndSource()
		blocks := map[string]simple.Block{
			"root": simple.New(&model.Block{
				Id: "root",
				Content: &model.BlockContentOfText{Text: &model.BlockContentText{
					Text: strings.Repeat("b", changeSizeLimit)},
				},
			}),
		}
		params := PushChangeParams{
			State:      state.NewDoc("", blocks).NewState(),
			DoSnapshot: true,
		}

		//when
		_, err := s.PushChange(params)

		//then
		assert.ErrorIs(t, err, ErrBigChangeSize)
	})

	t.Run("big detail in snapshot", func(t *testing.T) {
		//given
		_, s := initTreeAndSource()
		st := state.NewDoc("", nil).NewState()
		st.SetDetail("detail", pbtypes.String(strings.Repeat("d", changeSizeLimit)))
		params := PushChangeParams{
			State:      st,
			DoSnapshot: true,
		}

		//when
		_, err := s.PushChange(params)

		//then
		assert.ErrorIs(t, err, ErrBigChangeSize)
	})

	t.Run("change size is under limit", func(t *testing.T) {
		//given
		ot, s := initTreeAndSource()
		ot.EXPECT().AddContent(gomock.Any(), gomock.Any()).Return(objecttree.AddResult{Heads: []string{"root"}}, nil)

		as := mock_accountservice.NewMockService(ctrl)
		as.EXPECT().Account().Return(&accountdata.AccountKeys{SignKey: nil})
		s.accountService = as

		blocks := map[string]simple.Block{
			"root": simple.New(&model.Block{
				Id: "root",
				Content: &model.BlockContentOfText{Text: &model.BlockContentText{
					Text: strings.Repeat("b", changeSizeLimit/4)},
				},
			}),
		}
		st := state.NewDoc("", blocks).NewState()
		st.SetDetail("detail", pbtypes.String(strings.Repeat("d", changeSizeLimit/4)))
		params := PushChangeParams{
			State: st,
			Changes: []*pb.ChangeContent{{&pb.ChangeContentValueOfRelationAdd{RelationAdd: &pb.ChangeRelationAdd{
				RelationLinks: []*model.RelationLink{{Key: strings.Repeat("r", changeSizeLimit/4)}}},
			}}},
			DoSnapshot: true,
		}

		//when
		_, err := s.PushChange(params)

		//then
		assert.NoError(t, err)
	})
}
