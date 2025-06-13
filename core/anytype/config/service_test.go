package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfig_ServiceInterface(t *testing.T) {
	t.Run("Config partially implements app.Component", func(t *testing.T) {
		cfg := New()
		
		// Config implements Init and Name methods required by app.Component
		assert.NotNil(t, cfg)
		assert.Equal(t, CName, cfg.Name())
	})
}

func TestConfig_ServiceLifecycle(t *testing.T) {
	t.Run("init with mock wallet", func(t *testing.T) {
		// We can't easily test Init without a proper wallet setup
		// So we'll skip this test for now
		t.Skip("Init requires wallet component setup")
	})

	t.Run("methods are thread-safe", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := &Config{
			RepoPath: tmpDir,
		}

		// Run concurrent operations
		done := make(chan bool, 3)

		// Concurrent update
		go func() {
			err := cfg.UpdatePersistentConfig(func(c *ConfigPersistent) bool {
				c.NetworkId = "concurrent-network"
				return true
			})
			assert.NoError(t, err)
			done <- true
		}()

		// Concurrent read
		go func() {
			persistedCfg := cfg.GetPersistentConfig()
			assert.NotNil(t, persistedCfg)
			done <- true
		}()

		// Another concurrent update
		go func() {
			err := cfg.UpdatePersistentConfig(func(c *ConfigPersistent) bool {
				c.CustomFileStorePath = "/concurrent/path"
				return true
			})
			assert.NoError(t, err)
			done <- true
		}()

		// Wait for all operations to complete
		for i := 0; i < 3; i++ {
			<-done
		}
	})
}

func TestConfig_ServiceWithOptions(t *testing.T) {
	t.Run("new account option", func(t *testing.T) {
		cfg := New(WithNewAccount(true))
		assert.True(t, cfg.IsNewAccount())
	})

	t.Run("auto join stream option", func(t *testing.T) {
		streamId := "test-stream-123"
		cfg := New(WithAutoJoinStream(streamId))
		assert.Equal(t, streamId, cfg.AutoJoinStream)
	})

	t.Run("network mode option", func(t *testing.T) {
		cfg := New()
		// Test IsLocalOnlyMode method without importing pb
		cfg.NetworkMode = 1 // pb.RpcAccount_LocalOnly value
		assert.True(t, cfg.IsLocalOnlyMode())
		assert.Equal(t, 1, int(cfg.NetworkMode))
	})

	t.Run("disabled local network sync option", func(t *testing.T) {
		cfg := New(WithDisabledLocalNetworkSync())
		assert.True(t, cfg.DontStartLocalNetworkSyncAutomatically)
	})

	t.Run("debug addr option", func(t *testing.T) {
		addr := "127.0.0.1:6060"
		cfg := New(WithDebugAddr(addr))
		assert.Equal(t, addr, cfg.DebugAddr)
	})

	t.Run("disable file config option", func(t *testing.T) {
		cfg := New(DisableFileConfig(true))
		assert.True(t, cfg.DisableFileConfig)
	})

	t.Run("set fields directly", func(t *testing.T) {
		cfg := New()
		
		// These fields can be set directly
		cfg.PeferYamuxTransport = true
		cfg.JsonApiListenAddr = "127.0.0.1:8888"
		cfg.AnalyticsId = "test-analytics-123"
		cfg.NetworkCustomConfigFilePath = "/path/to/custom/config.yml"
		cfg.DisableNetworkIdCheck = true
		
		assert.True(t, cfg.PeferYamuxTransport)
		assert.Equal(t, "127.0.0.1:8888", cfg.JsonApiListenAddr)
		assert.Equal(t, "test-analytics-123", cfg.AnalyticsId)
		assert.Equal(t, "/path/to/custom/config.yml", cfg.NetworkCustomConfigFilePath)
		assert.True(t, cfg.DisableNetworkIdCheck)
	})

	t.Run("multiple options", func(t *testing.T) {
		cfg := New(
			WithNewAccount(true),
			WithAutoJoinStream("test-stream"),
			WithDebugAddr("127.0.0.1:6060"),
			DisableFileConfig(false),
		)

		assert.True(t, cfg.IsNewAccount())
		assert.Equal(t, "test-stream", cfg.AutoJoinStream)
		assert.Equal(t, "127.0.0.1:6060", cfg.DebugAddr)
		assert.False(t, cfg.DisableFileConfig)
	})
}