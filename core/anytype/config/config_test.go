package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_UpdatePersistentConfig(t *testing.T) {
	t.Run("successful update", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := &Config{
			RepoPath: tmpDir,
		}

		err := cfg.UpdatePersistentConfig(func(c *ConfigPersistent) bool {
			c.CustomFileStorePath = "/new/path"
			c.NetworkId = "test-network"
			return true
		})
		require.NoError(t, err)

		// Verify in-memory update
		assert.Equal(t, "/new/path", cfg.CustomFileStorePath)
		assert.Equal(t, "test-network", cfg.NetworkId)

		// Verify file was written
		var savedCfg ConfigPersistent
		err = GetFileConfig(cfg.GetConfigPath(), &savedCfg)
		require.NoError(t, err)
		assert.Equal(t, "/new/path", savedCfg.CustomFileStorePath)
		assert.Equal(t, "test-network", savedCfg.NetworkId)
	})

	t.Run("no update when callback returns false", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := &Config{
			RepoPath: tmpDir,
			ConfigPersistent: ConfigPersistent{
				CustomFileStorePath: "/original/path",
			},
		}

		// Write initial config
		err := writeJsonConfig(cfg.GetConfigPath(), &cfg.ConfigPersistent)
		require.NoError(t, err)

		// Try to update but return false
		err = cfg.UpdatePersistentConfig(func(c *ConfigPersistent) bool {
			// This change should not be persisted
			return false
		})
		require.NoError(t, err)

		// Verify nothing changed
		assert.Equal(t, "/original/path", cfg.CustomFileStorePath)

		// Verify file wasn't updated
		var savedCfg ConfigPersistent
		err = GetFileConfig(cfg.GetConfigPath(), &savedCfg)
		require.NoError(t, err)
		assert.Equal(t, "/original/path", savedCfg.CustomFileStorePath)
	})

	t.Run("update persists even if file write fails", func(t *testing.T) {
		cfg := &Config{
			RepoPath: "/invalid/path/that/does/not/exist",
		}

		err := cfg.UpdatePersistentConfig(func(c *ConfigPersistent) bool {
			c.NetworkId = "test-network"
			return true
		})
		
		// File write should fail but in-memory update should succeed
		assert.Error(t, err)
		assert.Equal(t, "test-network", cfg.NetworkId)
	})
}

func TestConfig_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &Config{
		RepoPath: tmpDir,
	}

	const numGoroutines = 100
	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2)

	// Concurrent writers
	for i := 0; i < numGoroutines; i++ {
		go func(i int) {
			defer wg.Done()
			err := cfg.UpdatePersistentConfig(func(c *ConfigPersistent) bool {
				c.NetworkId = "network-" + string(rune(i))
				return true
			})
			assert.NoError(t, err)
		}(i)
	}

	// Concurrent readers
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			_ = cfg.GetPersistentConfig()
		}()
	}

	wg.Wait()
	
	// Verify final state is consistent
	persistedCfg := cfg.GetPersistentConfig()
	assert.NotEmpty(t, persistedCfg.NetworkId)
	assert.Equal(t, cfg.NetworkId, persistedCfg.NetworkId)
}

func TestConfig_GetPersistentConfig(t *testing.T) {
	cfg := &Config{
		ConfigPersistent: ConfigPersistent{
			GatewayAddr:         "127.0.0.1:8080",
			CustomFileStorePath: "/custom/path",
			NetworkId:           "test-network",
		},
	}

	// Test that GetPersistentConfig returns a copy
	persistedCfg := cfg.GetPersistentConfig()
	assert.Equal(t, cfg.ConfigPersistent, persistedCfg)

	// Modify the returned copy
	persistedCfg.NetworkId = "modified"
	
	// Original should remain unchanged
	assert.Equal(t, "test-network", cfg.NetworkId)
}

func TestConfig_DefaultValues(t *testing.T) {
	// Test that DefaultConfig has the expected values
	assert.Equal(t, GatewayDefaultListenAddr, DefaultConfig.GatewayAddr)
	assert.Empty(t, DefaultConfig.CustomFileStorePath)
	assert.Empty(t, DefaultConfig.NetworkId)
}

func TestConfig_ResetStoredNetworkId(t *testing.T) {
	t.Run("reset when network id exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := &Config{
			RepoPath: tmpDir,
			ConfigPersistent: ConfigPersistent{
				NetworkId: "test-network",
			},
		}

		err := cfg.ResetStoredNetworkId()
		require.NoError(t, err)
		assert.Empty(t, cfg.NetworkId)

		// Verify file was updated
		var savedCfg ConfigPersistent
		err = GetFileConfig(cfg.GetConfigPath(), &savedCfg)
		require.NoError(t, err)
		assert.Empty(t, savedCfg.NetworkId)
	})

	t.Run("no-op when network id is already empty", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := &Config{
			RepoPath: tmpDir,
		}

		// Ensure no file exists initially
		_, err := os.Stat(cfg.GetConfigPath())
		assert.True(t, os.IsNotExist(err))

		err = cfg.ResetStoredNetworkId()
		require.NoError(t, err)

		// Verify no file was created
		_, err = os.Stat(cfg.GetConfigPath())
		assert.True(t, os.IsNotExist(err))
	})
}

func TestConfig_PersistAccountNetworkId(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &Config{
		RepoPath: tmpDir,
		ConfigPersistent: ConfigPersistent{
			NetworkId: "test-network",
		},
	}

	err := cfg.PersistAccountNetworkId()
	require.NoError(t, err)

	// Verify file was written
	var savedCfg ConfigPersistent
	err = GetFileConfig(cfg.GetConfigPath(), &savedCfg)
	require.NoError(t, err)
	assert.Equal(t, "test-network", savedCfg.NetworkId)
}

func TestConfig_InitFromFileAndEnv(t *testing.T) {
	t.Run("read existing config file", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, ConfigFileName)

		// Write a config file
		initialCfg := ConfigPersistent{
			GatewayAddr:         "127.0.0.1:9999",
			CustomFileStorePath: "/existing/path",
			// Don't set NetworkId to avoid mismatch with default network
		}
		err := writeJsonConfig(configPath, &initialCfg)
		require.NoError(t, err)

		// Create config and init from file
		cfg := &Config{
			RepoPath: tmpDir,
			DisableFileConfig: false,
		}
		err = cfg.initFromFileAndEnv(tmpDir)
		require.NoError(t, err)

		assert.Equal(t, "127.0.0.1:9999", cfg.GatewayAddr)
		assert.Equal(t, "/existing/path", cfg.CustomFileStorePath)
		// NetworkId will be set from default network config
		assert.NotEmpty(t, cfg.NetworkId)
	})

	t.Run("preserve legacy file store path", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, ConfigFileName)

		// Write a config file without legacy path
		initialCfg := ConfigPersistent{
			GatewayAddr: "127.0.0.1:8080",
		}
		err := writeJsonConfig(configPath, &initialCfg)
		require.NoError(t, err)

		// Create config with legacy path set
		cfg := &Config{
			RepoPath: tmpDir,
			ConfigPersistent: ConfigPersistent{
				LegacyFileStorePath: "/legacy/path",
			},
		}
		err = cfg.initFromFileAndEnv(tmpDir)
		require.NoError(t, err)

		// Legacy path should be preserved
		assert.Equal(t, "/legacy/path", cfg.LegacyFileStorePath)

		// Verify it was written to file
		var savedCfg ConfigPersistent
		err = GetFileConfig(configPath, &savedCfg)
		require.NoError(t, err)
		assert.Equal(t, "/legacy/path", savedCfg.LegacyFileStorePath)
	})
}

func TestConfig_ThreadSafety(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &Config{
		RepoPath: tmpDir,
	}

	// Test concurrent updates don't cause race conditions
	const iterations = 1000
	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < iterations; i++ {
			_ = cfg.UpdatePersistentConfig(func(c *ConfigPersistent) bool {
				c.NetworkId = "network-" + string(rune(i%10))
				return true
			})
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < iterations; i++ {
			_ = cfg.GetPersistentConfig()
		}
		done <- true
	}()

	// Wait for both to complete
	<-done
	<-done

	// Final state should be valid
	finalCfg := cfg.GetPersistentConfig()
	assert.NotEmpty(t, finalCfg.NetworkId)
	assert.Contains(t, finalCfg.NetworkId, "network-")
}