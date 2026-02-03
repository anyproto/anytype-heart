package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testConfigOmitEmpty struct {
	One string `json:",omitempty"`
	Two int    `json:",omitempty"`
}

type testConfig struct {
	One string
	Two int
}

func TestFileConfig_WriteFileConfig(t *testing.T) {
	t.Run("write and get config omitempty config", func(t *testing.T) {

		confFile := "test_config.json"
		defer os.Remove(confFile)

		err := WriteJsonConfig(confFile, testConfigOmitEmpty{One: "one test"})
		require.NoError(t, err)

		err = WriteJsonConfig(confFile, testConfigOmitEmpty{Two: 2})
		require.NoError(t, err)

		res := testConfigOmitEmpty{}
		err = GetFileConfig(confFile, &res)
		require.NoError(t, err)

		require.EqualValues(t, testConfigOmitEmpty{One: "one test", Two: 2}, res)
	})

	t.Run("write and get without omitempty config", func(t *testing.T) {

		confFile := "test_config2.json"
		defer os.Remove(confFile)

		err := WriteJsonConfig(confFile, testConfig{One: "one test"})
		require.NoError(t, err)

		err = WriteJsonConfig(confFile, testConfig{Two: 2})
		require.NoError(t, err)

		res := testConfig{}
		err = GetFileConfig(confFile, &res)
		require.NoError(t, err)

		require.EqualValues(t, testConfig{Two: 2}, res)
	})
}

func TestWriteConfigSafe(t *testing.T) {
	t.Run("writes config atomically", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")

		cfg := PersistedConfig{
			NetworkId:           "test-network",
			CustomFileStorePath: "/custom/path",
			HostAddr:            "/ip4/0.0.0.0/tcp/4006",
		}

		err := writeConfigSafe(configPath, cfg)
		require.NoError(t, err)

		// Verify file exists
		_, err = os.Stat(configPath)
		require.NoError(t, err)

		// Verify temp file doesn't exist
		_, err = os.Stat(configPath + ".tmp")
		require.True(t, os.IsNotExist(err))

		// Verify content
		data, err := os.ReadFile(configPath)
		require.NoError(t, err)

		var readCfg PersistedConfig
		err = json.Unmarshal(data, &readCfg)
		require.NoError(t, err)
		assert.Equal(t, cfg, readCfg)
	})

	t.Run("overwrites existing config", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")

		// Write first config
		cfg1 := PersistedConfig{NetworkId: "first"}
		err := writeConfigSafe(configPath, cfg1)
		require.NoError(t, err)

		// Write second config
		cfg2 := PersistedConfig{NetworkId: "second"}
		err = writeConfigSafe(configPath, cfg2)
		require.NoError(t, err)

		// Verify second config is stored
		readCfg, err := readPersistedConfig(configPath)
		require.NoError(t, err)
		assert.Equal(t, "second", readCfg.NetworkId)
	})
}

func TestReadPersistedConfig(t *testing.T) {
	t.Run("returns empty config for non-existent file", func(t *testing.T) {
		cfg, err := readPersistedConfig("/non/existent/path/config.json")
		require.NoError(t, err)
		assert.Equal(t, PersistedConfig{}, cfg)
	})

	t.Run("returns empty config for empty file", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")

		// Create empty file
		err := os.WriteFile(configPath, []byte{}, 0640)
		require.NoError(t, err)

		cfg, err := readPersistedConfig(configPath)
		require.NoError(t, err)
		assert.Equal(t, PersistedConfig{}, cfg)
	})

	t.Run("returns error for invalid JSON", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")

		err := os.WriteFile(configPath, []byte("invalid json"), 0640)
		require.NoError(t, err)

		_, err = readPersistedConfig(configPath)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidConfigFormat)
	})

	t.Run("reads valid config", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")

		expectedCfg := PersistedConfig{
			NetworkId:              "test-net",
			CustomFileStorePath:    "/path/to/storage",
			HostAddr:               "/ip4/0.0.0.0/tcp/1234",
			AutoDownloadFiles:      true,
			AutoDownloadOnWifiOnly: true,
		}

		data, err := json.Marshal(expectedCfg)
		require.NoError(t, err)
		err = os.WriteFile(configPath, data, 0640)
		require.NoError(t, err)

		cfg, err := readPersistedConfig(configPath)
		require.NoError(t, err)
		assert.Equal(t, expectedCfg, cfg)
	})
}

func TestBootstrapPersistedConfig(t *testing.T) {
	t.Run("creates config for new account", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")

		cfg := PersistedConfig{NetworkId: "test", CustomFileStorePath: "/custom"}
		err := BootstrapPersistedConfig(configPath, cfg)
		require.NoError(t, err)

		readCfg, err := readPersistedConfig(configPath)
		require.NoError(t, err)
		assert.Equal(t, cfg, readCfg)
	})

	t.Run("returns error if config already exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")

		// Create initial config
		cfg := PersistedConfig{NetworkId: "first"}
		err := BootstrapPersistedConfig(configPath, cfg)
		require.NoError(t, err)

		// Try to bootstrap again - should fail
		cfg2 := PersistedConfig{NetworkId: "second"}
		err = BootstrapPersistedConfig(configPath, cfg2)
		require.ErrorIs(t, err, ErrConfigAlreadyExists)

		// Original config should be unchanged
		readCfg, err := readPersistedConfig(configPath)
		require.NoError(t, err)
		assert.Equal(t, "first", readCfg.NetworkId)
	})
}

func TestConfigSettersWriteOnlyOnChange(t *testing.T) {
	t.Run("SetNetworkId writes only when changed", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")

		cfg := New()
		cfg.RepoPath = tmpDir
		cfg.configPath = configPath

		// First set should write
		err := cfg.SetNetworkId("test-network")
		require.NoError(t, err)
		assert.Equal(t, "test-network", cfg.NetworkId())

		// Get file modification time
		info1, err := os.Stat(configPath)
		require.NoError(t, err)

		// Same value should not write
		err = cfg.SetNetworkId("test-network")
		require.NoError(t, err)

		info2, err := os.Stat(configPath)
		require.NoError(t, err)
		// ModTime should be the same if not written
		assert.Equal(t, info1.ModTime(), info2.ModTime())

		// Different value should write
		err = cfg.SetNetworkId("new-network")
		require.NoError(t, err)
		assert.Equal(t, "new-network", cfg.NetworkId())
	})

	t.Run("SetAutoDownloadSettings writes only when changed", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")

		cfg := New()
		cfg.RepoPath = tmpDir
		cfg.configPath = configPath

		err := cfg.SetAutoDownloadSettings(true, false)
		require.NoError(t, err)
		assert.True(t, cfg.AutoDownloadFiles())
		assert.False(t, cfg.AutoDownloadOnWifiOnly())

		// Same values should not write
		info1, _ := os.Stat(configPath)
		err = cfg.SetAutoDownloadSettings(true, false)
		require.NoError(t, err)
		info2, _ := os.Stat(configPath)
		assert.Equal(t, info1.ModTime(), info2.ModTime())
	})
}

func TestConfigGetters(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	// Create config and set values via setters
	cfg := New()
	cfg.RepoPath = tmpDir
	cfg.configPath = configPath

	err := cfg.SetNetworkId("test-net")
	require.NoError(t, err)
	err = cfg.SetCustomFileStorePath("/custom")
	require.NoError(t, err)
	err = cfg.SetLegacyFileStorePath("/legacy")
	require.NoError(t, err)
	err = cfg.SetHostAddr("/ip4/0.0.0.0/tcp/4006")
	require.NoError(t, err)
	err = cfg.SetAutoDownloadSettings(true, false)
	require.NoError(t, err)

	// Verify getters return correct values
	assert.Equal(t, "test-net", cfg.NetworkId())
	assert.Equal(t, "/custom", cfg.CustomFileStorePath())
	assert.Equal(t, "/legacy", cfg.LegacyFileStorePath())
	assert.Equal(t, "/ip4/0.0.0.0/tcp/4006", cfg.HostAddr())
	assert.True(t, cfg.AutoDownloadFiles())
	assert.False(t, cfg.AutoDownloadOnWifiOnly())
}

func TestConfigDisableFileConfig(t *testing.T) {
	t.Run("setters don't write when DisableFileConfig is true", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")

		cfg := New(DisableFileConfig(true))
		cfg.RepoPath = tmpDir
		cfg.configPath = configPath

		err := cfg.SetNetworkId("test")
		require.NoError(t, err)

		// File should not exist
		_, err = os.Stat(configPath)
		require.True(t, os.IsNotExist(err))
	})
}
