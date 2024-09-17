package localdiscovery

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_testSelfConnection(t *testing.T) {
	err := testSelfConnection("127.0.0.1")
	require.NoError(t, err)

	err = testSelfConnection("11.11.11.11")
	require.Error(t, err)
}
