package anymetry

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fastjson"
)

type testEvent struct{}

func (t testEvent) GetBackend() MetricsBackend {
	return 0
}

func (t testEvent) MarshalFastJson(arena *fastjson.Arena) JsonEvent {
	event := arena.NewObject()
	event.Set("event_type", arena.NewString("TestEvent"))
	return event
}

func (t testEvent) SetTimestamp() {}

func (t testEvent) GetTimestamp() int64 {
	return 3
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
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		bodyBytes, _ := io.ReadAll(req.Body)
		value, _ := fastjson.Parse(string(bodyBytes))
		assert.Equal(t, []byte("api_key"), value.GetStringBytes("api_key"))
		event := value.GetArray("events")[0]
		assert.Equal(t, []byte("TestEvent"), event.GetStringBytes("event_type"))
		assert.Equal(t, []byte("AppVersion"), event.GetStringBytes("app_version"))
		assert.Equal(t, []byte("StartVersion"), event.GetStringBytes("start_version"))
		assert.Equal(t, []byte("DeviceId"), event.GetStringBytes("device_id"))
		assert.Equal(t, []byte("Platform"), event.GetStringBytes("platform"))
		assert.Equal(t, []byte("UserId"), event.GetStringBytes("user_id"))
		assert.Equal(t, int64(3), event.GetInt64("time"))
	}))
	// Close the server when test finishes
	defer server.Close()

	client := New("", "api_key", false).(*Client)
	client.client = server.Client()
	client.eventEndpoint = server.URL

	err := client.SendEvents([]Event{&testEvent{}}, &testAppInfoProvider{})
	assert.NoError(t, err)
}
