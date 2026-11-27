package server

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/cmd/anytype-mcp/contract"
	"github.com/anyproto/anytype-heart/core/api/wrapper"
)

// TestForwarderContract pins the numbers and names cmd/anytype-mcp mirrors
// from this delivery. The forwarder can't import this package (it must
// stay small), so it keeps its own copy in cmd/anytype-mcp/contract; a
// change here that breaks the pair fails this test. When it does, change
// the contract package too and raise the forwarder's Rev.
func TestForwarderContract(t *testing.T) {
	assert.Equal(t, mcpProtocolVersions, contract.HeartVersions, "protocol versions the HTTP delivery answers")
	for _, v := range mcpProtocolVersions {
		assert.Contains(t, contract.ClientVersions, v, "the forwarder must offer every version heart answers")
	}
	assert.Equal(t, mcpSessionHeader, contract.SessionHeader)
	assert.Equal(t, mcpProtocolHeader, contract.ProtocolHeader)
	assert.Equal(t, wrapper.MCPMaxMessageBytes, contract.MaxMessageBytes, "one inbound message")
	assert.LessOrEqual(t, contract.Workers, mcpMaxCallsPerKey, "the forwarder must not run more calls than heart admits per key")
	assert.LessOrEqual(t, contract.Workers, mcpMaxQueue, "nor more than one session queues")
	assert.Greater(t, contract.ExchangeTimeout, mcpCallTimeout, "the forwarder must outwait heart's own call timeout")
	assert.Equal(t, []string{string(wrapper.TierSmall), string(wrapper.TierLarge), string(wrapper.TierFull)}, contract.Tiers)
}
