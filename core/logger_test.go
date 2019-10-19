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

func Test_Log(t *testing.T) {
	mw := Middleware{}
	file, err := ioutil.TempFile("", "testlog")
	require.NoError(t, err)
	file.Close()

	os.Setenv("GOLOG_FILE", file.Name())
	logger.SetupLogging()
	logger.SetDebugLogging()
	for level, levelText := range map[pb.LogRequest_Level]string{
		pb.LogRequest_ERROR:   "[31mERROR",
		pb.LogRequest_WARNING: "[33mWARNI",
	} {
		text := fmt.Sprintf("test_log_%s", time.Now().String())
		resp := mw.Log(&pb.LogRequest{Message: text, Level: level})
		require.Equal(t, pb.LogResponse_Error_NULL, resp.Error.Code, "LogResponse contains error: %+v", resp.Error)

		b, err := ioutil.ReadFile(file.Name())
		require.NoError(t, err)

		require.Contains(t, string(b), text)
		require.Contains(t, string(b), levelText)
	}

}
