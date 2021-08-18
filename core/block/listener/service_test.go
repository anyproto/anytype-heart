package listener

import (
	"context"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/stretchr/testify/assert"
)

func TestListener_ReportChange(t *testing.T) {
	l := New()

	var calls int

	var expId = "testId"
	var expState = state.NewDoc("testId", map[string]simple.Block{
		"testId": simple.New(&model.Block{Id: "testId"}),
	}).(*state.State)
	var expCount = 2
	for i := 0; i < expCount; i++ {
		l.OnWholeChange(func(ctx context.Context, id string, s *state.State) error {
			assert.Equal(t, expId, id)
			assert.Equal(t, expState.StringDebug(), s.StringDebug())
			calls++
			return nil
		})
	}

	l.ReportChange(context.Background(), expId, expState)

	assert.Equal(t, expCount, calls)
}
