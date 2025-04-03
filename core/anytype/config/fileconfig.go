package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
)

var ErrInvalidConfigFormat = errors.New("invalid config format")

// GetFileConfig - returns data from config file, if file doesn't exist returns same cfg struct
func GetFileConfig(configPath string, cfg *ConfigPersistent) error {
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
				log.Warn("failed to decode config file: %s", err)
				return ErrInvalidConfigFormat
			}
		}
	}

	return nil
}

// ModifyJsonFileConfig - allows to directly read and modify json config file
// used before the bootstrap of the app
func ModifyJsonFileConfig(configPath string, modifier func(cfg *ConfigPersistent) (isModified bool)) error {
	var cfg ConfigPersistent
	// do not open for write, because in most cases we don't modify the config
	err := GetFileConfig(configPath, &cfg)
	// tolerate damaged or invalid config file -
	if err != nil && !errors.Is(err, ErrInvalidConfigFormat) {
		return fmt.Errorf("failed to get old config: %w", err)
	}

	modified := modifier(&cfg)
	if !modified {
		return nil
	}

	cfgFile, err := os.OpenFile(configPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0640)
	if err != nil {
		return fmt.Errorf("failed to open cfg file for updating: %w", err)
	}
	defer cfgFile.Close()

	err = json.NewEncoder(cfgFile).Encode(&cfg)
	if err != nil {
		return fmt.Errorf("failed to save data to the config file: %w", err)
	}

	return nil
}

func writeJsonConfig(configPath string, cfg *ConfigPersistent) error {
	cfgFile, err := os.OpenFile(configPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0640)
	if err != nil {
		return fmt.Errorf("failed to open cfg file for updating: %w", err)
	}
	defer cfgFile.Close()

	err = json.NewEncoder(cfgFile).Encode(&cfg)
	if err != nil {
		return fmt.Errorf("failed to save data to the config file: %w", err)
	}

	return nil
}
