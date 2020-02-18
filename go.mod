module github.com/anytypeio/go-anytype-library

go 1.13

require (
	github.com/gogo/protobuf v1.3.1
	github.com/golang/protobuf v1.3.2
	github.com/h2non/filetype v1.0.12
	github.com/ipfs/go-ipfs v0.4.22-0.20191002225611-b15edf287df6
	github.com/ipfs/go-ipfs-config v0.2.0
	github.com/ipfs/go-ipfs-files v0.0.4
	github.com/ipfs/go-ipld-format v0.0.2
	github.com/ipfs/go-log v0.0.1
	github.com/ipfs/go-path v0.0.7
	github.com/ipfs/interface-go-ipfs-core v0.2.3
	github.com/libp2p/go-libp2p-core v0.3.0
	github.com/mr-tron/base58 v1.1.3
	github.com/multiformats/go-multihash v0.0.10
	github.com/satori/go.uuid v1.2.0
	github.com/segmentio/ksuid v1.0.2
	github.com/textileio/go-textile v0.7.8-0.20200102164400-98b263e32c0c
	github.com/whyrusleeping/go-logging v0.0.1
)

replace github.com/textileio/go-textile => github.com/anytypeio/go-textile v0.7.8-0.20200217213349-f936f40b6472

replace github.com/libp2p/go-eventbus => github.com/libp2p/go-eventbus v0.1.0

replace github.com/mattbaird/jsonpatch => github.com/requilence/jsonpatch v0.0.0-20190628193028-ccadf8ccb170
