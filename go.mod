module github.com/anytypeio/go-anytype-middleware

go 1.12

require (
	github.com/anytypeio/go-anytype-library v0.0.0-20191001095643-5552baa6d652
	github.com/davecgh/go-spew v1.1.1
	github.com/gogo/protobuf v1.2.1
	github.com/golang/protobuf v1.3.1
	github.com/ipfs/go-log v0.0.1
	github.com/prometheus/common v0.4.0
	github.com/sirupsen/logrus v1.4.2
	github.com/stretchr/testify v1.3.0
	github.com/textileio/go-textile v0.7.2-0.20190907000013-95a885123536
)

replace github.com/textileio/go-textile => github.com/anytypeio/go-textile v0.0.0-20190924115707-a0dcb5a893ec

replace github.com/libp2p/go-eventbus => github.com/libp2p/go-eventbus v0.1.0
