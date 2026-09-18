package heartboot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOptionsApplyDefaults(t *testing.T) {
	t.Run("empty options get working defaults", func(t *testing.T) {
		// given
		opt := Options{}

		// when
		opt.applyDefaults()

		// then
		assert.Equal(t, "apiv2eval", opt.AccountName)
		assert.Equal(t, "apiv2eval", opt.AppName)
		assert.NotZero(t, opt.BuildTimeout)
		assert.NotZero(t, opt.StartTimeout)
		assert.NotZero(t, opt.AccountTimeout)
		assert.NotZero(t, opt.APITimeout)
		assert.NotZero(t, opt.StopTimeout)
	})

	t.Run("caller values survive", func(t *testing.T) {
		// given
		opt := Options{AppName: "custom", APITimeout: time.Second}

		// when
		opt.applyDefaults()

		// then
		assert.Equal(t, "custom", opt.AppName)
		assert.Equal(t, time.Second, opt.APITimeout)
	})
}

func TestFreePort(t *testing.T) {
	// given/when
	port, err := freePort()

	// then
	require.NoError(t, err)
	require.NotZero(t, port)
	// the port must actually be bindable — the whole point is handing the
	// heart an address it can take
	lis, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	require.NoError(t, err)
	require.NoError(t, lis.Close())
}

func TestLogTailScan(t *testing.T) {
	t.Run("reports the announced grpc address once", func(t *testing.T) {
		// given
		var sink bytes.Buffer
		tail := &logTail{sink: &sink}
		addrCh := make(chan string, 2)
		input := strings.NewReader("mw grpc: dev\n" +
			grpcStartedPrefix + "127.0.0.1:51423\n" +
			grpcStartedPrefix + "127.0.0.1:9999\n")

		// when
		tail.scan(input, addrCh)

		// then
		require.Len(t, addrCh, 1)
		assert.Equal(t, "127.0.0.1:51423", <-addrCh)
		assert.Contains(t, sink.String(), "mw grpc: dev")
	})

	t.Run("quote keeps only the last lines", func(t *testing.T) {
		// given
		tail := &logTail{}
		var lines []string
		for i := 0; i < logTailLines+10; i++ {
			lines = append(lines, fmt.Sprintf("line %d", i))
		}

		// when
		tail.scan(strings.NewReader(strings.Join(lines, "\n")+"\n"), nil)
		quoted := tail.quote()

		// then
		assert.NotContains(t, quoted, "line 0\n")
		assert.Contains(t, quoted, fmt.Sprintf("line %d", logTailLines+9))
	})

	t.Run("quote says so when the heart printed nothing", func(t *testing.T) {
		// given
		tail := &logTail{}

		// when
		quoted := tail.quote()

		// then
		assert.Contains(t, quoted, "printed nothing")
	})
}

func TestFindRepoRoot(t *testing.T) {
	// given/when — the test itself runs inside the repo
	root, err := findRepoRoot()

	// then
	require.NoError(t, err)
	data, err := os.ReadFile(root + "/go.mod")
	require.NoError(t, err)
	assert.Contains(t, string(data), "module github.com/anyproto/anytype-heart")
}

// TestStartEndToEnd is the real thing: it builds the heart, creates an
// account, mints a key and asks the API what spaces exist. It is opt-in
// (APIV2EVAL_HEARTBOOT_E2E=1) because it builds a large binary and starts a
// process — too heavy for a package test run, and it is exactly what a
// developer wants to run by hand after touching the bootstrap.
//
//	APIV2EVAL_HEARTBOOT_E2E=1 go test ./cmd/apiv2eval/heartboot -run EndToEnd -v -timeout 20m
func TestStartEndToEnd(t *testing.T) {
	if os.Getenv("APIV2EVAL_HEARTBOOT_E2E") == "" {
		t.Skip("set APIV2EVAL_HEARTBOOT_E2E=1 to run the full bootstrap")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	started := time.Now()
	heart, err := Start(ctx, Options{
		AccountName: "apiv2eval e2e",
		AppName:     "apiv2eval-e2e",
		BinaryPath:  os.Getenv("APIV2EVAL_HEART_BINARY"),
	})
	require.NoError(t, err)
	defer func() { require.NoError(t, heart.Stop()) }()
	t.Logf("bootstrap took %s — api %s, account %s, data %s",
		time.Since(started).Round(time.Millisecond), heart.APIURL, heart.AccountId, heart.DataDir)

	// the point of the whole exercise: a space list with nothing in it but
	// the account's own default space
	var spaces struct {
		Data []struct {
			Id   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, heart.APIURL+"/v2/spaces?limit=100", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+heart.APIKey)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	require.NoError(t, json.Unmarshal(body, &spaces))

	for _, s := range spaces.Data {
		t.Logf("space %s %q", s.Id, s.Name)
	}
	assert.LessOrEqual(t, len(spaces.Data), 1, "a fresh account must have at most its default space")
}
