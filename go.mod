module github.com/anytypeio/go-anytype-middleware

go 1.13

require (
	github.com/PuerkitoBio/goquery v1.5.1
	github.com/alecthomas/template v0.0.0-20190718012654-fb15b899a751 // indirect
	github.com/alecthomas/units v0.0.0-20190924025748-f65c72e2690d // indirect
	github.com/anytypeio/go-anytype-library v0.4.1-0.20200331122537-eb9425dab568
	github.com/anytypeio/html-to-markdown v0.0.0-20200221082113-a2021b1b2129

	github.com/gogo/protobuf v1.3.1
	github.com/golang/mock v1.4.1
	github.com/google/uuid v1.1.1
	github.com/h2non/filetype v1.0.12
	github.com/hashicorp/golang-lru v0.5.4
	github.com/ipfs/go-log v1.0.3
	github.com/mauidude/go-readability v0.0.0-20141216012317-2f30b1a346f1
	github.com/microcosm-cc/bluemonday v1.0.2
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826
	github.com/otiai10/opengraph v1.1.1
	github.com/prometheus/common v0.6.0
	github.com/stretchr/testify v1.5.1
	github.com/yosssi/gohtml v0.0.0-20190915184251-7ff6f235ecaf
	github.com/yuin/goldmark v1.1.24
	golang.org/x/image v0.0.0-20190802002840-cff245a6509b // indirect
	golang.org/x/text v0.3.2
	google.golang.org/grpc v1.27.1
)

replace github.com/textileio/go-textile => github.com/anytypeio/go-textile v0.7.8-0.20200217213349-f936f40b6472

replace github.com/libp2p/go-eventbus => github.com/libp2p/go-eventbus v0.1.0
