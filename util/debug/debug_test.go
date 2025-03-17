package debug

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test(t *testing.T) {
	t.Run("goroutine found", func(t *testing.T) {
		dump := ParseGoroutinesDump(stackTrace, "core.(*Middleware).AccountSelect")
		require.Equal(t, "runnable crypto/hmac.(*hmac).Reset() /opt/homebrew/opt/go/libexec/src/crypto/hmac/hmac.go:79", dump)
	})
}

const stackTrace = `
goroutine 169 [select]:
google.golang.org/grpc/internal/transport.(*serverHandlerTransport).HandleStreams.func1()
	/Users/somebody/go/pkg/mod/google.golang.org/grpc@v1.65.0/internal/transport/handler_server.go:385 +0xcc
created by google.golang.org/grpc/internal/transport.(*serverHandlerTransport).HandleStreams in goroutine 168
	/Users/somebody/go/pkg/mod/google.golang.org/grpc@v1.65.0/internal/transport/handler_server.go:384 +0x1e0

goroutine 88 [select]:
github.com/anyproto/anytype-heart/metrics.SharedLongMethodsInterceptor.func1()
	/Users/somebody/anytype-heart/metrics/interceptors.go:79 +0xf0
created by github.com/anyproto/anytype-heart/metrics.SharedLongMethodsInterceptor in goroutine 171
	/Users/somebody/anytype-heart/metrics/interceptors.go:76 +0x20c

goroutine 171 [runnable]:
crypto/hmac.(*hmac).Reset(0x14000866120)
	/opt/homebrew/opt/go/libexec/src/crypto/hmac/hmac.go:79 +0x4e4
golang.org/x/crypto/pbkdf2.Key({0x14000e30000, 0x4f, 0x50}, {0x1400071e338, 0x8, 0x8}, 0x800, 0x40, 0x10820e7d0)
	/Users/somebody/go/pkg/mod/golang.org/x/crypto@v0.25.0/pbkdf2/pbkdf2.go:67 +0x348
github.com/tyler-smith/go-bip39.NewSeed({0x140004c2000, 0x4f}, {0x0, 0x0})
	/Users/somebody/go/pkg/mod/github.com/tyler-smith/go-bip39@v1.1.0/bip39.go:269 +0xd4
github.com/tyler-smith/go-bip39.NewSeedWithErrorChecking({0x140004c2000, 0x4f}, {0x0, 0x0})
	/Users/somebody/go/pkg/mod/github.com/tyler-smith/go-bip39@v1.1.0/bip39.go:263 +0xec
github.com/anyproto/any-sync/util/crypto.Mnemonic.Seed({0x140004c2000, 0x4f})
	/Users/somebody/go/pkg/mod/github.com/anyproto/any-sync@v0.4.21/util/crypto/mnemonic.go:150 +0x50
github.com/anyproto/any-sync/util/crypto.Mnemonic.deriveForPath({0x140004c2000, 0x4f}, 0x1, 0x0, {0x10720cdd0, 0xa})
	/Users/somebody/go/pkg/mod/github.com/anyproto/any-sync@v0.4.21/util/crypto/mnemonic.go:102 +0x70
github.com/anyproto/any-sync/util/crypto.Mnemonic.DeriveKeys({0x140004c2000, 0x4f}, 0x0)
	/Users/somebody/go/pkg/mod/github.com/anyproto/any-sync@v0.4.21/util/crypto/mnemonic.go:129 +0x90
github.com/anyproto/anytype-heart/pkg/lib/core.WalletAccountAt({0x140004c2000, 0x4f}, 0x0)
	/Users/somebody/anytype-heart/pkg/lib/core/wallet.go:23 +0x7c
github.com/anyproto/anytype-heart/core/application.(*Service).start(0x140003ebb80, {0x108244fc8, 0x14000a53e90}, {0x140001ac0f0, 0x30}, {0x140001ac240, 0x21}, 0x0, 0x0, 0x0, ...)
	/Users/somebody/anytype-heart/core/application/account_select.go:88 +0x21c
github.com/anyproto/anytype-heart/core/application.(*Service).AccountSelect(0x140003ebb80, {0x108244fc8, 0x14000887140}, 0x140004c4000)
	/Users/somebody/anytype-heart/core/application/account_select.go:75 +0x6fc
github.com/anyproto/anytype-heart/core.(*Middleware).AccountSelect(0x14000186ce8, {0x108244fc8, 0x14000887140}, 0x140004c4000)
	/Users/somebody/anytype-heart/core/account.go:53 +0x60
github.com/anyproto/anytype-heart/pb/service._ClientCommands_AccountSelect_Handler.func1({0x108244fc8, 0x14000887140}, {0x10816f480, 0x140004c4000})
	/Users/somebody/anytype-heart/pb/service/service.pb.go:4462 +0xb4
main.appendInterceptor.func1({0x108244fc8, 0x14000887140}, {0x10816f480, 0x140004c4000}, 0x140001263c0, 0x1400067c120)
	/Users/somebody/anytype-heart/cmd/grpcserver/grpc.go:264 +0x110
github.com/grpc-ecosystem/go-grpc-middleware.ChainUnaryServer.func2.1({0x108244fc8, 0x14000887140}, {0x10816f480, 0x140004c4000})
	/Users/somebody/go/pkg/mod/github.com/grpc-ecosystem/go-grpc-middleware@v1.4.0/chain.go:48 +0xa8
github.com/anyproto/anytype-heart/metrics.SharedLongMethodsInterceptor({0x108244fc8, 0x14000887140}, {0x10816f480, 0x140004c4000}, {0x1072553da, 0xd}, 0x140004c4140)
	/Users/somebody/anytype-heart/metrics/interceptors.go:92 +0x278
github.com/anyproto/anytype-heart/metrics.LongMethodsInterceptor({0x108244fc8, 0x14000886300}, {0x10816f480, 0x140004c4000}, 0x140001263c0, 0x140004c4140)
	/Users/somebody/anytype-heart/metrics/interceptors.go:61 +0x88
github.com/grpc-ecosystem/go-grpc-middleware.ChainUnaryServer.func2.1({0x108244fc8, 0x14000886300}, {0x10816f480, 0x140004c4000})
	/Users/somebody/go/pkg/mod/github.com/grpc-ecosystem/go-grpc-middleware@v1.4.0/chain.go:48 +0xa8
github.com/anyproto/anytype-heart/core.(*Middleware).Authorize(0x14000186ce8, {0x108244fc8, 0x14000886300}, {0x10816f480, 0x140004c4000}, 0x140001263c0, 0x140004c4180)
	/Users/somebody/anytype-heart/core/auth_debug.go:13 +0x64
main.main.func2({0x108244fc8, 0x14000886300}, {0x10816f480, 0x140004c4000}, 0x140001263c0, 0x140004c4180)
	/Users/somebody/anytype-heart/cmd/grpcserver/grpc.go:112 +0x70
github.com/grpc-ecosystem/go-grpc-middleware.ChainUnaryServer.func2.1({0x108244fc8, 0x14000886300}, {0x10816f480, 0x140004c4000})
	/Users/somebody/go/pkg/mod/github.com/grpc-ecosystem/go-grpc-middleware@v1.4.0/chain.go:48 +0xa8
github.com/anyproto/anytype-heart/metrics.SharedTraceInterceptor({0x108244fc8, 0x14000886300}, {0x10816f480, 0x140004c4000}, {0x1072553da, 0xd}, 0x140004c41c0)
	/Users/somebody/anytype-heart/metrics/interceptors.go:54 +0x80
github.com/anyproto/anytype-heart/metrics.UnaryTraceInterceptor({0x108244fc8, 0x14000886300}, {0x10816f480, 0x140004c4000}, 0x140001263c0, 0x140004c41c0)
	/Users/somebody/anytype-heart/metrics/interceptors.go:44 +0x88
github.com/grpc-ecosystem/go-grpc-middleware.ChainUnaryServer.func2({0x108244fc8, 0x14000886300}, {0x10816f480, 0x140004c4000}, 0x140001263c0, 0x1400067c120)
	/Users/somebody/go/pkg/mod/github.com/grpc-ecosystem/go-grpc-middleware@v1.4.0/chain.go:53 +0x1d4
github.com/anyproto/anytype-heart/pb/service._ClientCommands_AccountSelect_Handler({0x10820aaa0, 0x14000186ce8}, {0x108244fc8, 0x14000886300}, 0x140007e8080, 0x1400073bef0)
	/Users/somebody/anytype-heart/pb/service/service.pb.go:4464 +0x1e4
google.golang.org/grpc.(*Server).processUnaryRPC(0x140001fbc00, {0x108244fc8, 0x14000a53da0}, {0x108253530, 0x1400018f040}, 0x14000441d40, 0x140007f60c0, 0x1096ebe60, 0x0)
	/Users/somebody/go/pkg/mod/google.golang.org/grpc@v1.65.0/server.go:1379 +0x13a4
google.golang.org/grpc.(*Server).handleStream(0x140001fbc00, {0x108253530, 0x1400018f040}, 0x14000441d40)
	/Users/somebody/go/pkg/mod/google.golang.org/grpc@v1.65.0/server.go:1790 +0xd0c
google.golang.org/grpc.(*Server).serveStreams.func2.1()
	/Users/somebody/go/pkg/mod/google.golang.org/grpc@v1.65.0/server.go:1029 +0x144
created by google.golang.org/grpc.(*Server).serveStreams.func2 in goroutine 168
	/Users/somebody/go/pkg/mod/google.golang.org/grpc@v1.65.0/server.go:1040 +0x1c8
`
