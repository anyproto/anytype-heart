package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/cheggaaa/mb/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/valyala/fastjson"

	"github.com/anyproto/anytype-heart/metrics/amplitude"
	"github.com/anyproto/anytype-heart/metrics/amplitude/mock_amplitude"
)

type testEvent struct {
	baseInfo
}

func (t testEvent) GetBackend() amplitude.MetricsBackend {
	return inhouse
}

func (t testEvent) MarshalFastJson(arena *fastjson.Arena) amplitude.JsonEvent {
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
	telemetry := &mock_amplitude.MockService{}
	//telemetry.EXPECT().SendEvents(mock.Anything, mock.Anything).Return(nil)
	var events []amplitude.Event
	telemetry.On("SendEvents", mock.Anything, mock.Anything).
		Return(nil).
		Run(func(args mock.Arguments) {
			events = args.Get(0).([]amplitude.Event)
		})

	c := &client{
		aggregatableMap:  make(map[string]SamplableEvent),
		aggregatableChan: make(chan SamplableEvent, bufferSize),
		ctx:              ctx,
		cancel:           cancel,
		batcher:          mb.New[amplitude.Event](0),
		telemetry:        telemetry,
	}

	sendingQueueLimitMin = 2
	go c.startSendingBatchMessages(&testAppInfoProvider{})

	c.send(&testEvent{})
	time.Sleep(1 * time.Millisecond)

	assert.Equal(t, 1, c.batcher.Len())
	telemetry.AssertNotCalled(t, "SendEvents", mock.Anything, mock.Anything)

	c.send(&testEvent{})
	time.Sleep(1 * time.Millisecond)

	assert.Equal(t, 0, c.batcher.Len())
	assert.Equal(t, 2, len(events))

	assert.True(t, events[0].GetTimestamp() > 0)
	telemetry.AssertCalled(t, "SendEvents", mock.Anything, mock.Anything)
}
