module github.com/anytypeio/go-anytype-library

go 1.12

require github.com/textileio/go-textile v0.7.2-0.20190907000013-95a885123536

require (
	github.com/Microsoft/go-winio v0.4.14
	github.com/davecgh/go-spew v1.1.1
	github.com/gogo/protobuf v1.2.1
	github.com/golang/protobuf v1.3.1
	github.com/ipfs/go-ipfs-config v0.0.6
	github.com/ipfs/go-log v0.0.1
	github.com/libp2p/go-libp2p-crypto v0.1.0
	github.com/mitchellh/go-homedir v1.1.0
	github.com/mr-tron/base58 v1.1.2
	github.com/segmentio/ksuid v1.0.2
	golang.org/x/mobile v0.0.0-20190806162312-597adff16ade
	google.golang.org/appengine v1.4.0 // indirect
	google.golang.org/grpc v1.19.0
)

replace github.com/textileio/go-textile => github.com/anytypeio/go-textile v0.0.0-20190919075542-f424be4eaf88

replace github.com/mattbaird/jsonpatch => github.com/requilence/jsonpatch v0.0.0-20190628193028-ccadf8ccb170
