package keyvalueservice

import (
	"bytes"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeKeyValuePair(t *testing.T) {
	for i, tc := range []struct {
		key           string
		value         []byte
		isEncodingErr bool
	}{
		{
			key:   "",
			value: []byte(nil),
		},
		{
			key:   "",
			value: []byte("value"),
		},
		{
			key:   "key",
			value: []byte(nil),
		},
		{
			key:   "key",
			value: []byte("value"),
		},
		{
			key:   string(make([]byte, math.MaxUint16)),
			value: []byte("value"),
		},
		{
			key:           string(make([]byte, math.MaxUint16+1)),
			value:         []byte("value"),
			isEncodingErr: true,
		},
	} {
		t.Run(fmt.Sprintf("case %d", i+1), func(t *testing.T) {
			encoded, err := encodeKeyValuePair(tc.key, tc.value)
			if tc.isEncodingErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			decodedKey, decodedValue, err := decodeKeyValuePair(encoded)
			require.NoError(t, err)

			assert.True(t, tc.key == decodedKey)
			assert.True(t, bytes.Equal(tc.value, decodedValue))
		})

	}
}
