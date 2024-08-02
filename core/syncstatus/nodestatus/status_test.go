package nodestatus

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNodeStatus(t *testing.T) {
	st := NewNodeStatus()
	st.SetNodesStatus("spaceId", Online)
	require.Equal(t, Online, st.GetNodeStatus("spaceId"))
	st.SetNodesStatus("spaceId", ConnectionError)
	require.Equal(t, ConnectionError, st.GetNodeStatus("spaceId"))
}
