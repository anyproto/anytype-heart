package device

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/space/spacecore/peermanager"
)

// The peer manager cannot import this package (import cycle via the devices
// service), so it looks the networkState component up by a copied name. If
// the constants drift apart, connectivity-driven peer rebuild silently turns
// off — this test makes the drift loud.
func TestPeermanagerDeviceCNameInSync(t *testing.T) {
	assert.Equal(t, CName, peermanager.DeviceNetworkStateCName)
}
