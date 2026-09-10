package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/anyproto/anytype-heart/util/localorigin"
)

// TestOriginInterceptorCarriesOriginIntoContext is the wiring half of the
// browser-caller guard, and it is the half that was missing: the check in
// Middleware.rejectBrowserCaller reads localorigin.OriginFromContext, but on
// this transport the Origin arrives as gRPC metadata. Without the
// interceptor every origin check on a gRPC method read "" and passed, while
// the unit test of the check itself kept passing because it injected the
// context value by hand.
func TestOriginInterceptorCarriesOriginIntoContext(t *testing.T) {
	tests := []struct {
		name string
		md   metadata.MD
		want string
	}{
		{
			name: "webclipper extension origin reaches the handler",
			md:   metadata.Pairs("origin", "chrome-extension://jbnammhjiplhpjfncnlejjjejghimdkf"),
			want: "chrome-extension://jbnammhjiplhpjfncnlejjjejghimdkf",
		},
		{
			name: "a loopback page origin reaches the handler",
			md:   metadata.Pairs("origin", "http://localhost:3000"),
			want: "http://localhost:3000",
		},
		{
			name: "a native caller sends none",
			md:   metadata.MD{},
			want: "",
		},
		{
			name: "an empty value is not an origin",
			md:   metadata.Pairs("origin", ""),
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			var got string
			handler := func(ctx context.Context, _ interface{}) (interface{}, error) {
				got = localorigin.OriginFromContext(ctx)
				return nil, nil
			}

			// when
			_, err := originInterceptor()(
				metadata.NewIncomingContext(context.Background(), tt.md),
				nil,
				&grpc.UnaryServerInfo{FullMethod: "/anytype.ClientCommands/AccountLocalLinkApproveChallenge"},
				handler,
			)

			// then
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
