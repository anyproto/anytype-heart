module github.com/anytypeio/go-anytype-library

go 1.14

require (
	github.com/anytypeio/go-slip10 v0.0.0-20200330112030-a352ca8495e4
	github.com/anytypeio/go-slip21 v0.0.0-20200218204727-e2e51e20ab51
	github.com/dgtony/collections v0.1.3
	github.com/disintegration/imaging v1.6.0
	github.com/gobwas/glob v0.2.3
	github.com/gogo/protobuf v1.3.1
	github.com/gogo/status v1.1.0
	github.com/h2non/filetype v1.0.12
	github.com/hsanjuan/ipfs-lite v1.1.15
	github.com/ipfs/go-cid v0.0.7
	github.com/ipfs/go-datastore v0.4.4
	github.com/ipfs/go-ds-badger v0.2.4
	github.com/ipfs/go-ipfs-blockstore v1.0.1
	github.com/ipfs/go-ipfs-files v0.0.8
	github.com/ipfs/go-ipld-cbor v0.0.4
	github.com/ipfs/go-ipld-format v0.2.0
	github.com/ipfs/go-log/v2 v2.1.2-0.20200810212702-264b187bb04f
	github.com/ipfs/go-merkledag v0.3.2
	github.com/ipfs/go-path v0.0.7
	github.com/ipfs/go-unixfs v0.2.4
	github.com/ipfs/interface-go-ipfs-core v0.3.0
	github.com/jsummers/gobmp v0.0.0-20151104160322-e2ba15ffa76e
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/libp2p/go-libp2p v0.10.3
	github.com/libp2p/go-libp2p-connmgr v0.2.4
	github.com/libp2p/go-libp2p-core v0.6.1
	github.com/libp2p/go-libp2p-kad-dht v0.8.3
	github.com/libp2p/go-libp2p-peerstore v0.2.6
	github.com/libp2p/go-libp2p-pubsub v0.3.1 // indirect
	github.com/mr-tron/base58 v1.2.0
	github.com/multiformats/go-base32 v0.0.3
	github.com/multiformats/go-multiaddr v0.2.2
	github.com/multiformats/go-multihash v0.0.14
	github.com/rwcarlsen/goexif v0.0.0-20190401172101-9e8deecbddbd
	github.com/santhosh-tekuri/jsonschema/v2 v2.2.0
	github.com/stretchr/testify v1.6.1
	github.com/textileio/go-threads v0.1.23-forked
	github.com/tyler-smith/go-bip39 v1.0.0
	github.com/xeipuuv/gojsonschema v1.2.0
	go.uber.org/zap v1.15.0
	golang.org/x/net v0.0.0-20200519113804-d87ec0cfa476 // indirect
	google.golang.org/grpc v1.31.0
	gopkg.in/Graylog2/go-gelf.v2 v2.0.0-20180125164251-1832d8546a9f
)

replace github.com/textileio/go-threads => github.com/anytypeio/go-threads v0.1.24-0.20200831040109-0d95d73fbdba

replace gopkg.in/Graylog2/go-gelf.v2 => github.com/anytypeio/go-gelf v0.0.0-20200813115635-198b2af80f88

replace github.com/ipfs/go-log/v2 => github.com/anytypeio/go-log/v2 v2.1.2-0.20200810212702-264b187bb04f
