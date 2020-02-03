module github.com/anytypeio/go-anytype-middleware

go 1.13

require (
	github.com/PuerkitoBio/goquery v1.5.0 // indirect
	github.com/anytypeio/go-anytype-library v0.0.0-20200203093556-863a2783feeb
	github.com/anytypeio/html-to-markdown v0.0.0-20200123120722-1c256e006f13

	github.com/gogo/protobuf v1.3.1
	github.com/golang/mock v1.3.1
	github.com/ipfs/go-log v0.0.1
	github.com/lunny/html2md v0.0.0-20181018071239-7d234de44546
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826
	github.com/stretchr/testify v1.4.0
	github.com/textileio/go-textile v0.7.8-0.20200102164400-98b263e32c0c
	github.com/yosssi/gohtml v0.0.0-20190915184251-7ff6f235ecaf
	github.com/yuin/goldmark v1.1.22
	golang.org/x/text v0.3.2
	google.golang.org/grpc v1.24.0
	gotest.tools v2.1.0+incompatible
)

replace github.com/textileio/go-textile => github.com/anytypeio/go-textile v0.7.8-0.20200202161814-7f86e00257c2

replace github.com/libp2p/go-eventbus => github.com/libp2p/go-eventbus v0.1.0
