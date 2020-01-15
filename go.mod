module github.com/anytypeio/go-anytype-library

go 1.12

require (
	github.com/gogo/protobuf v1.3.1
	github.com/golang/protobuf v1.3.1
	github.com/ipfs/go-ipfs v0.4.22-0.20190718080458-55afc478ec02
	github.com/ipfs/go-ipfs-config v0.0.6
	github.com/ipfs/go-ipfs-files v0.0.3
	github.com/ipfs/go-ipld-format v0.0.2
	github.com/ipfs/go-log v0.0.1
	github.com/ipfs/go-path v0.0.7
	github.com/ipfs/interface-go-ipfs-core v0.1.0
	github.com/libp2p/go-libp2p-core v0.2.2
	github.com/libp2p/go-libp2p-crypto v0.1.0
	github.com/mr-tron/base58 v1.1.2
	github.com/multiformats/go-multihash v0.0.5
	github.com/satori/go.uuid v1.2.0
	github.com/segmentio/ksuid v1.0.2
	github.com/textileio/go-textile v0.7.2-0.20190907000013-95a885123536
	github.com/whyrusleeping/go-logging v0.0.0-20170515211332-0457bb6b88fc
)

replace github.com/textileio/go-textile => github.com/anytypeio/go-textile v0.6.10-0.20191224183538-ba056fbef614

replace github.com/libp2p/go-eventbus => github.com/libp2p/go-eventbus v0.1.0

replace github.com/mattbaird/jsonpatch => github.com/requilence/jsonpatch v0.0.0-20190628193028-ccadf8ccb170
