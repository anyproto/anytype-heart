package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFileConfig_ModifyJsonFileConfig(t *testing.T) {
	t.Run("write and get config with multiple modifications", func(t *testing.T) {
		confFile := "test_config.json"
		defer os.Remove(confFile)

		// First modification
		err := ModifyJsonFileConfig(confFile, func(cfg *ConfigPersistent) (isModified bool) {
			cfg.CustomFileStorePath = "/test/path"
			return true
		})
		require.NoError(t, err)

		// Second modification
		err = ModifyJsonFileConfig(confFile, func(cfg *ConfigPersistent) (isModified bool) {
			cfg.NetworkId = "test-network"
			return true
		})
		require.NoError(t, err)

		// Read the config
		var res ConfigPersistent
		err = GetFileConfig(confFile, &res)
		require.NoError(t, err)

		require.Equal(t, "/test/path", res.CustomFileStorePath)
		require.Equal(t, "test-network", res.NetworkId)
	})

	t.Run("no modification when callback returns false", func(t *testing.T) {
		confFile := "test_config2.json"
		defer os.Remove(confFile)

		// Write initial config
		err := ModifyJsonFileConfig(confFile, func(cfg *ConfigPersistent) (isModified bool) {
			cfg.GatewayAddr = "127.0.0.1:8080"
			return true
		})
		require.NoError(t, err)

		// Try to modify but return false
		err = ModifyJsonFileConfig(confFile, func(cfg *ConfigPersistent) (isModified bool) {
			cfg.GatewayAddr = "127.0.0.1:9090"
			return false // Should not write changes
		})
		require.NoError(t, err)

		// Read and verify original value is preserved
		var res ConfigPersistent
		err = GetFileConfig(confFile, &res)
		require.NoError(t, err)

		require.Equal(t, "127.0.0.1:8080", res.GatewayAddr)
	})

	t.Run("handle invalid config file gracefully", func(t *testing.T) {
		confFile := "test_config3.json"
		defer os.Remove(confFile)

		// Write invalid JSON
		err := os.WriteFile(confFile, []byte("{invalid json"), 0644)
		require.NoError(t, err)

		// Should still be able to modify
		err = ModifyJsonFileConfig(confFile, func(cfg *ConfigPersistent) (isModified bool) {
			cfg.NetworkId = "new-network"
			return true
		})
		require.NoError(t, err)

		// Read and verify
		var res ConfigPersistent
		err = GetFileConfig(confFile, &res)
		require.NoError(t, err)
		require.Equal(t, "new-network", res.NetworkId)
	})
}
