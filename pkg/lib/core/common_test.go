package core

import (
	"bytes"
	"context"
	"io/ioutil"
	"testing"
	"time"

	"github.com/anytypeio/go-anytype-library/core/config"
	"github.com/anytypeio/go-anytype-library/wallet"
	ipfslite "github.com/hsanjuan/ipfs-lite"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/pnet"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
)

func Benchmark_ConnectCafe(t *testing.B) {
	r := bytes.NewReader([]byte(ipfsPrivateNetworkKey))
	pnet, err := pnet.DecodeV1PSK(r)
	require.NoError(t, err)

	cafeAddr, err := ma.NewMultiaddr(config.DefaultConfig.CafeP2PAddr)
	require.NoError(t, err)

	cafeAddrInfo, err := peer.AddrInfoFromP2pAddr(cafeAddr)
	require.NoError(t, err)

	for n := 0; n < t.N; n++ {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)

		deviceKP, err := wallet.NewRandomKeypair(wallet.KeypairTypeDevice)
		require.NoError(t, err)

		tmpfile, err := ioutil.TempFile("", "ipfslite1")
		require.NoError(t, err)
		tmpfile.Close()

		m, err := ma.NewMultiaddr("/ip4/0.0.0.0/tcp/0")
		require.NoError(t, err)

		h, dht, err := ipfslite.SetupLibp2p(
			ctx,
			deviceKP,
			pnet,
			[]ma.Multiaddr{m},
			nil,
			ipfslite.Libp2pOptionsExtra...,
		)

		err = h.Connect(ctx, *cafeAddrInfo)
		require.NoError(t, err)
		cancel()
		h.Close()
		dht.Close()
	}
}
