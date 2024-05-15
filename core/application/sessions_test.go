package application

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
)

func TestCreateSession(t *testing.T) {
	t.Run("with app key", func(t *testing.T) {
		t.Run("with not initialized app expect error", func(t *testing.T) {
			s := New()

			_, _, err := s.CreateSession(&pb.RpcWalletCreateSessionRequest{
				Auth: &pb.RpcWalletCreateSessionRequestAuthOfAppKey{
					AppKey: "appKey",
				},
			})

			require.Error(t, err)
		})
	})
}
