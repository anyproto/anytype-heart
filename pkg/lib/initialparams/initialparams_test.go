package initialparams

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
)

func TestInit(t *testing.T) {
	t.Run("first call stores and returns derived paths", func(t *testing.T) {
		resetForTest(t)
		req := &pb.RpcInitialSetParametersRequest{
			Platform:           "linux",
			Version:            "1.2.3",
			Workdir:            "/tmp/anytype",
			LogLevel:           "*=INFO",
			DoNotSaveLogs:      false,
			DoNotSendTelemetry: true,
		}
		want := Paths{
			Workdir:     "/tmp/anytype",
			Common:      filepath.Join("/tmp/anytype", "common"),
			LogsDir:     filepath.Join("/tmp/anytype", "common", "logs"),
			LogFile:     filepath.Join("/tmp/anytype", "common", "logs", "anytype.log"),
			ProfilesDir: filepath.Join("/tmp/anytype", "common", "profiles"),
		}

		got, created, err := Init(req)

		require.NoError(t, err)
		assert.True(t, created)
		assert.Equal(t, want, got.Paths)
		assert.Equal(t, "linux", got.Platform)
		assert.True(t, got.SaveLogs)
		assert.False(t, got.SendTelemetry)
		assert.Equal(t, got, Get())
	})

	t.Run("second call with the same params is a no-op success", func(t *testing.T) {
		resetForTest(t)
		req := &pb.RpcInitialSetParametersRequest{
			Platform: "linux",
			Version:  "1.2.3",
			Workdir:  "/tmp/anytype",
		}
		first, firstCreated, err := Init(req)
		require.NoError(t, err)
		require.True(t, firstCreated)

		second, secondCreated, err := Init(req)

		require.NoError(t, err)
		assert.False(t, secondCreated)
		assert.Equal(t, first, second)
	})

	t.Run("second call with different params returns ErrAlreadyInitialized", func(t *testing.T) {
		resetForTest(t)
		first, _, err := Init(&pb.RpcInitialSetParametersRequest{Workdir: "/a"})
		require.NoError(t, err)

		second, created, err := Init(&pb.RpcInitialSetParametersRequest{Workdir: "/b"})

		assert.True(t, errors.Is(err, ErrAlreadyInitialized))
		assert.False(t, created)
		assert.Equal(t, first, second)
		assert.Equal(t, "/a", Get().Paths.Workdir)
	})

	t.Run("empty workdir produces empty Paths", func(t *testing.T) {
		resetForTest(t)
		got, created, err := Init(&pb.RpcInitialSetParametersRequest{})

		require.NoError(t, err)
		assert.True(t, created)
		assert.Equal(t, Paths{}, got.Paths)
	})
}

// resetForTest clears the stored Params so a test can exercise Init from a
// clean state. Tests only.
func resetForTest(t *testing.T) {
	t.Helper()
	current.Store(nil)
}
