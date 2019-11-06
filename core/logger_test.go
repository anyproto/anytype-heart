package core

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pb"
	logger "github.com/ipfs/go-log"
	"github.com/stretchr/testify/require"
)

func TestLog(t *testing.T) {
	mw := Middleware{}
	file, err := ioutil.TempFile("", "testlog")
	require.NoError(t, err)
	file.Close()

	os.Setenv("GOLOG_FILE", file.Name())
	logger.SetupLogging()
	logger.SetDebugLogging()
	for level, levelText := range map[pb.RpcLogSendRequestLevel]string{
		pb.RpcLogSendRequest_ERROR:   "[31mERROR",
		pb.RpcLogSendRequest_WARNING: "[33mWARNI",
	} {
		text := fmt.Sprintf("test_log_%s", time.Now().String())
		resp := mw.LogSend(&pb.RpcLogSendRequest{Message: text, Level: level})
		require.Equal(t, pb.RpcLogSendResponseError_NULL, resp.Error.Code, "LogSendResponse contains error: %+v", resp.Error)

		b, err := ioutil.ReadFile(file.Name())
		require.NoError(t, err)

		require.Contains(t, string(b), text)
		require.Contains(t, string(b), levelText)
	}

}
