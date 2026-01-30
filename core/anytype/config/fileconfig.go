package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
)

var ErrInvalidConfigFormat = errors.New("failed to decode")

// PersistedConfig contains configuration that is persisted to config.json
type PersistedConfig struct {
	HostAddr               string `json:",omitempty"`
	CustomFileStorePath    string `json:",omitempty"`
	LegacyFileStorePath    string `json:",omitempty"`
	NetworkId              string `json:""` // in case this account was at least once connected to the network on this device, this field will be set to the network id
	AutoDownloadFiles      bool   `json:",omitempty"`
	AutoDownloadOnWifiOnly bool   `json:",omitempty"`
}

// writeConfigSafe writes config to disk using atomic rename for crash safety.
// It writes to a temp file, fsyncs, then renames to the target path.
func writeConfigSafe(configPath string, cfg PersistedConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	tmpPath := configPath + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)
	if err != nil {
		return fmt.Errorf("failed to create temp config file: %w", err)
	}

	_, err = f.Write(data)
	if err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write config data: %w", err)
	}

	err = f.Sync()
	if err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to sync config file: %w", err)
	}

	err = f.Close()
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to close config file: %w", err)
	}

	err = os.Rename(tmpPath, configPath)
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp config file: %w", err)
	}

	return nil
}

// readPersistedConfig reads PersistedConfig from file.
// Returns empty config if file doesn't exist or is empty.
// Returns ErrInvalidConfigFormat if JSON is invalid.
func readPersistedConfig(configPath string) (PersistedConfig, error) {
	var cfg PersistedConfig

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("failed to read config file: %w", err)
	}

	if len(data) == 0 {
		return cfg, nil
	}

	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return cfg, errors.Join(ErrInvalidConfigFormat, err)
	}

	return cfg, nil
}

var ErrConfigAlreadyExists = errors.New("config file already exists")

// BootstrapPersistedConfig creates initial config.json for a new account.
// Returns ErrConfigAlreadyExists if config file already exists.
// This should only be used during account creation to set initial values
// before the Config is initialized.
func BootstrapPersistedConfig(configPath string, cfg PersistedConfig) error {
	if _, err := os.Stat(configPath); err == nil {
		return ErrConfigAlreadyExists
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to check config file: %w", err)
	}
	return writeConfigSafe(configPath, cfg)
}

// Deprecated: GetFileConfig is deprecated. Use Config getters (NetworkId(), CustomFileStorePath(), etc.) instead.
// GetFileConfig - returns data from config file, if file doesn't exist returns same cfg struct
func GetFileConfig(configPath string, cfg interface{}) error {
	if reflect.ValueOf(cfg).Kind() != reflect.Ptr {
		return fmt.Errorf("cfg param must be a pointer type")
	}

	cfgFile, err := os.OpenFile(configPath, os.O_RDONLY, 0655)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	defer cfgFile.Close()

	if !errors.Is(err, os.ErrNotExist) {
		info, err := cfgFile.Stat()
		if err != nil {
			return fmt.Errorf("fail to get info about config file: %w", err)
		}

		if info.Size() > 0 {
			err = json.NewDecoder(cfgFile).Decode(cfg)
			if err != nil {
				return errors.Join(ErrInvalidConfigFormat, err)
			}
		}
	}

	return nil
}

// Deprecated: WriteJsonConfig is deprecated. Use Config setters instead.
// WriteJsonConfig - overwrites params in file only specified params which passed in cfg
// `json:",omitempty"` - is required tag for every field in cfg !!!
func WriteJsonConfig(configPath string, cfg interface{}) error {
	oldCfg := make(map[string]interface{})
	if err := GetFileConfig(configPath, &oldCfg); err != nil {
		return err
	}

	newConfig, err := toMapInterface(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal new config: %w", err)
	}

	for oldKey, oldData := range oldCfg {
		if _, ok := newConfig[oldKey]; !ok {
			newConfig[oldKey] = oldData
		}
	}

	cfgFile, err := os.OpenFile(configPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0640)
	if err != nil {
		return fmt.Errorf("failed to open cfg file for updating: %w", err)
	}
	defer cfgFile.Close()

	err = json.NewEncoder(cfgFile).Encode(newConfig)
	if err != nil {
		return fmt.Errorf("failed to save data to the config file: %w", err)
	}

	return nil
}

func toMapInterface(cfg interface{}) (map[string]interface{}, error) {
	var m map[string]interface{}
	byteData, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(byteData, &m)
	return m, err
}
