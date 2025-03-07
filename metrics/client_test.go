package metrics

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/cheggaaa/mb/v3"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fastjson"

	"github.com/anyproto/anytype-heart/metrics/anymetry"
	"github.com/anyproto/anytype-heart/metrics/anymetry/mock_anymetry"
)

type testEvent struct {
	baseInfo
}

func (t testEvent) GetBackend() anymetry.MetricsBackend {
	return inhouse
}

func (t testEvent) MarshalFastJson(arena *fastjson.Arena) anymetry.JsonEvent {
	event, _ := setupProperties(arena, "TestEvent")
	return event
}

type testAppInfoProvider struct {
}

func (t testAppInfoProvider) GetAppVersion() string {
	return "AppVersion"
}

func (t testAppInfoProvider) GetStartVersion() string {
	return "StartVersion"
}

func (t testAppInfoProvider) GetDeviceId() string {
	return "DeviceId"
}

func (t testAppInfoProvider) GetPlatform() string {
	return "Platform"
}

func (t testAppInfoProvider) GetUserId() string {
	return "UserId"
}

func TestClient_SendEvents(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	telemetry := &mock_anymetry.MockService{}
	// telemetry.EXPECT().SendEvents(mock.Anything, mock.Anything).Return(nil)
	mutex := sync.Mutex{}
	var events []anymetry.Event
	telemetry.On("SendEvents", mock.Anything, mock.Anything).
		Return(nil).
		Run(func(args mock.Arguments) {
			mutex.Lock()
			events = args.Get(0).([]anymetry.Event)
			mutex.Unlock()
		})

	c := &client{
		aggregatableMap:  make(map[string]SamplableEvent),
		aggregatableChan: make(chan SamplableEvent, bufferSize),
		ctx:              ctx,
		cancel:           cancel,
		batcher:          mb.New[anymetry.Event](0),
		telemetry:        telemetry,
	}

	sendingQueueLimitMin = 2
	go c.startSendingBatchMessages(&testAppInfoProvider{})

	c.send(&testEvent{})
	time.Sleep(100 * time.Millisecond)

	require.Equal(t, 1, c.batcher.Len())
	telemetry.AssertNotCalled(t, "SendEvents", mock.Anything, mock.Anything)

	c.send(&testEvent{})
	time.Sleep(100 * time.Millisecond)

	mutex.Lock()
	require.Equal(t, 0, c.batcher.Len())
	require.Equal(t, 2, len(events))
	mutex.Unlock()

	require.True(t, events[0].GetTimestamp() > 0)
	telemetry.AssertCalled(t, "SendEvents", mock.Anything, mock.Anything)
}
