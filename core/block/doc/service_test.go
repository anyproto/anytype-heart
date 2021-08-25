package doc

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
		l.OnWholeChange(func(ctx context.Context, info DocInfo) error {
			assert.Equal(t, expId, info.Id)
			assert.Equal(t, expState.StringDebug(), info.State.StringDebug())
			calls++
			return nil
		})
	}

	l.ReportChange(context.Background(), DocInfo{Id: expId, State: expState})

	assert.Equal(t, expCount, calls)
}
