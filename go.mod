module github.com/anytypeio/go-anytype-library

go 1.13

require (
	github.com/anytypeio/go-slip21 v0.0.0-20200218204727-e2e51e20ab51
	github.com/disintegration/imaging v1.6.0
	github.com/gogo/protobuf v1.3.1
	github.com/golang/protobuf v1.3.2
	github.com/h2non/filetype v1.0.12
	github.com/hsanjuan/ipfs-lite v0.1.8
	github.com/ipfs/go-cid v0.0.5
	github.com/ipfs/go-datastore v0.3.1
	github.com/ipfs/go-ds-badger v0.2.0
	github.com/ipfs/go-ipfs-files v0.0.4
	github.com/ipfs/go-ipld-cbor v0.0.3
	github.com/ipfs/go-ipld-format v0.0.2
	github.com/ipfs/go-log v1.0.0
	github.com/ipfs/go-merkledag v0.2.3
	github.com/ipfs/go-path v0.0.7
	github.com/ipfs/go-unixfs v0.2.2
	github.com/ipfs/interface-go-ipfs-core v0.2.3
	github.com/jsummers/gobmp v0.0.0-20151104160322-e2ba15ffa76e
	github.com/libp2p/go-libp2p v0.4.2
	github.com/libp2p/go-libp2p-connmgr v0.1.1
	github.com/libp2p/go-libp2p-core v0.3.0
	github.com/libp2p/go-libp2p-kad-dht v0.3.0
	github.com/libp2p/go-libp2p-peerstore v0.1.4
	github.com/mr-tron/base58 v1.1.3
	github.com/multiformats/go-base32 v0.0.3
	github.com/multiformats/go-multiaddr v0.2.0
	github.com/multiformats/go-multihash v0.0.13
	github.com/rwcarlsen/goexif v0.0.0-20190401172101-9e8deecbddbd
	github.com/stretchr/testify v1.4.0
	github.com/textileio/go-textile v0.7.8-0.20200102164400-98b263e32c0c
	github.com/textileio/go-threads v0.1.12
	github.com/tyler-smith/go-bip39 v1.0.0
	github.com/xeipuuv/gojsonschema v1.2.0
	google.golang.org/grpc v1.25.1
)

replace github.com/textileio/go-textile => github.com/anytypeio/go-textile v0.7.8-0.20200217213349-f936f40b6472

replace github.com/libp2p/go-eventbus => github.com/libp2p/go-eventbus v0.1.0

replace github.com/mattbaird/jsonpatch => github.com/requilence/jsonpatch v0.0.0-20190628193028-ccadf8ccb170
