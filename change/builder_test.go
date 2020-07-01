package change

import (
	"context"
	"fmt"
	"testing"

	"github.com/anytypeio/go-anytype-library/core"
	smartblock2 "github.com/anytypeio/go-anytype-library/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/proto"
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
		ch.PreviousDetailsIds = []string{prevDetIds}
		if withDet {
			ch.Content = detailsContent
		}
		return ch
	}
)

func TestStateBuilder_Build(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		_, _, err := BuildTree(newTestSmartBlock())
		assert.Equal(t, ErrEmpty, err)
	})
	t.Run("linear - one snapshot", func(t *testing.T) {
		sb := newTestSmartBlock()
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
		sb := newTestSmartBlock()
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
		sb := newTestSmartBlock()
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
		sb := newTestSmartBlock()
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
		sb := newTestSmartBlock()
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
		sb := newTestSmartBlock()
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
		sb := newTestSmartBlock()
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
		sb := newTestSmartBlock()
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
		require.Error(t, err)
	})
}

func TestBuildDetailsTree(t *testing.T) {
	sb := newTestSmartBlock()

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
	tr, _, err := BuildDetailsTree(sb)
	require.NoError(t, err)
	assert.Equal(t, 3, tr.Len())
	assert.Equal(t, "->s0->c2->c4", tr.String())
}

func newTestSmartBlock() *smartblock {
	return &smartblock{
		changes: make(map[string]*core.SmartblockRecord),
	}
}

type smartblock struct {
	logs    []core.SmartblockLog
	changes map[string]*core.SmartblockRecord
}

func (s *smartblock) BaseSchema() core.SmartBlockSchema {
	panic("implement me")
}

func (s *smartblock) AddChanges(logId string, chs ...*Change) *smartblock {
	var id string
	for _, ch := range chs {
		pl, _ := ch.Change.Marshal()
		s.changes[ch.Id] = &core.SmartblockRecord{
			ID:      ch.Id,
			Payload: pl,
		}
		id = ch.Id
	}
	for i, l := range s.logs {
		if l.ID == logId {
			s.logs[i].Head = id
			return s
		}
	}
	s.logs = append(s.logs, core.SmartblockLog{
		ID:   logId,
		Head: id,
	})
	return s
}

func (s *smartblock) ID() string {
	return "id"
}

func (s *smartblock) Type() smartblock2.SmartBlockType {
	return smartblock2.SmartBlockTypePage
}

func (s *smartblock) Creator() (string, error) {
	return "", nil
}

func (s *smartblock) GetLogs() ([]core.SmartblockLog, error) {
	return s.logs, nil
}

func (s *smartblock) GetRecord(ctx context.Context, recordID string) (*core.SmartblockRecord, error) {
	if data, ok := s.changes[recordID]; ok {
		return data, nil
	}
	return nil, fmt.Errorf("record %v not found", recordID)
}

func (s *smartblock) PushRecord(payload proto.Message) (id string, err error) {
	panic("implement me")
}

func (s *smartblock) SubscribeForRecords(ch chan core.SmartblockRecordWithLogID) (cancel func(), err error) {
	panic("implement me")
}

func (s *smartblock) SubscribeClientEvents(event chan<- proto.Message) (cancelFunc func(), err error) {
	panic("implement me")
}

func (s *smartblock) PublishClientEvent(event proto.Message) error {
	panic("implement me")
}
