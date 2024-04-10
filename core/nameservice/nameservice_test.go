package nameservice

import (
	"testing"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/stretchr/testify/require"
)

func TestNsNameToFullName(t *testing.T) {
	t.Run("should succeed", func(t *testing.T) {
		out := NsNameToFullName("somename", model.NameserviceNameType_AnyName)
		require.Equal(t, "somename.any", out)

		// by default return no suffix without error
		// in this case other NS methods should check the validity and return an error
		out = NsNameToFullName("tony", 1)
		require.Equal(t, "tony", out)

		out = NsNameToFullName("", model.NameserviceNameType_AnyName)
		require.Equal(t, "", out)
	})
}
