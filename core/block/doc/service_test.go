package doc

import (
	"context"
	"testing"
	"time"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/recordsbatcher"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_ReportChange(t *testing.T) {
	l := New()
	defer l.Close()

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

func TestService_WakeupLoop(t *testing.T) {
	dh := &testDocInfoHandler{
		wakeupIds: make(chan string),
	}
	rb := recordsbatcher.New()
	a := new(app.App)
	a.Register(rb).Register(dh).Register(New())
	require.NoError(t, a.Start())
	defer a.Close()

	recId := func(id string) core.SmartblockRecordWithThreadID {
		return core.SmartblockRecordWithThreadID{ThreadID: id}
	}

	require.NoError(t, rb.Add(recId("1"), recId("2"), recId("2"), recId("1"), recId("3")))

	var result []string
	for i := 0; i < 3; i++ {
		select {
		case id := <-dh.wakeupIds:
			result = append(result, id)
		case <-time.After(time.Second / 4):
			t.Errorf("timeout")
		}
	}
	assert.Equal(t, []string{"1", "2", "3"}, result)
}

type testDocInfoHandler struct {
	wakeupIds chan string
}

func (t *testDocInfoHandler) GetDocInfo(ctx context.Context, id string) (info DocInfo, err error) {
	return
}

func (t *testDocInfoHandler) Wakeup(id string) (err error) {
	t.wakeupIds <- id
	return
}

func (t *testDocInfoHandler) Init(a *app.App) (err error) {
	return nil
}

func (t *testDocInfoHandler) Name() (name string) {
	return "blockService"
}
