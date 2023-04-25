package config

import (
	"os"
	"testing"

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
