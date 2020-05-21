package change

import (
	"context"
	"fmt"
	"testing"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStateBuilder_Build(t *testing.T) {
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
	)
	t.Run("linear - one snapshot", func(t *testing.T) {
		sb := newTestSmartBlock()
		sb.AddChanges(
			"a",
			newSnapshot("s0", "", nil),
		)
		b := new(StateBuilder)
		err := b.Build(sb)
		require.NoError(t, err)
		require.NotNil(t, b.Tree)
		assert.Equal(t, "s0", b.Tree.RootId())
		assert.Equal(t, 1, b.Tree.Len())
		assert.Equal(t, []string{"s0"}, b.Tree.headIds)
	})
	t.Run("linear - one log", func(t *testing.T) {
		sb := newTestSmartBlock()
		sb.AddChanges(
			"a",
			newSnapshot("s0", "", nil),
			newChange("c0", "s0", "s0"),
		)
		b := new(StateBuilder)
		err := b.Build(sb)
		require.NoError(t, err)
		require.NotNil(t, b.Tree)
		assert.Equal(t, "s0", b.Tree.RootId())
		assert.Equal(t, 2, b.Tree.Len())
		assert.Equal(t, []string{"c0"}, b.Tree.headIds)
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
		b := new(StateBuilder)
		err := b.Build(sb)
		require.NoError(t, err)
		require.NotNil(t, b.Tree)
		assert.Equal(t, "s0", b.Tree.RootId())
		assert.Equal(t, 4, b.Tree.Len())
		assert.Equal(t, []string{"c2"}, b.Tree.headIds)
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
		b := new(StateBuilder)
		err := b.Build(sb)
		require.NoError(t, err)
		require.NotNil(t, b.Tree)
		assert.Equal(t, "s1", b.Tree.RootId())
		assert.Equal(t, 2, b.Tree.Len())
		assert.Equal(t, []string{"c3"}, b.Tree.headIds)
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
		b := new(StateBuilder)
		err := b.Build(sb)
		require.NoError(t, err)
		require.NotNil(t, b.Tree)
		assert.Equal(t, "s0", b.Tree.RootId())
		assert.Equal(t, 10, b.Tree.Len())
		assert.Equal(t, []string{"c3", "c3.3"}, b.Tree.headIds)
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
		b := new(StateBuilder)
		err := b.Build(sb)
		require.NoError(t, err)
		require.NotNil(t, b.Tree)
		assert.Equal(t, "s2", b.Tree.RootId())
		assert.Equal(t, 2, b.Tree.Len())
		assert.Equal(t, []string{"c4"}, b.Tree.headIds)
	})
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

func (s *smartblock) Type() core.SmartBlockType {
	return core.SmartBlockTypePage
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

func (s *smartblock) SubscribeForRecords(ch chan core.SmartblockRecord) (cancel func(), err error) {
	panic("implement me")
}

func (s *smartblock) SubscribeClientEvents(event chan<- proto.Message) (cancelFunc func(), err error) {
	panic("implement me")
}

func (s *smartblock) PublishClientEvent(event proto.Message) error {
	panic("implement me")
}
