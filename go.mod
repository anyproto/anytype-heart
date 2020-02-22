module github.com/anytypeio/go-anytype-middleware

go 1.13

require (
	github.com/PuerkitoBio/goquery v1.5.0 // indirect
	github.com/anytypeio/go-anytype-library v0.0.0-20200218104520-6d2a8493bb45
	github.com/anytypeio/html-to-markdown v0.0.0-20200123120722-1c256e006f13

	github.com/gogo/protobuf v1.3.1
	github.com/golang/mock v1.4.0
	github.com/google/uuid v1.1.1
	github.com/h2non/filetype v1.0.12
	github.com/hashicorp/golang-lru v0.5.4
	github.com/ipfs/go-log v0.0.1
	github.com/lunny/html2md v0.0.0-20181018071239-7d234de44546
	github.com/mauidude/go-readability v0.0.0-20141216012317-2f30b1a346f1
	github.com/microcosm-cc/bluemonday v1.0.2
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826
	github.com/otiai10/opengraph v1.1.0
	github.com/stretchr/testify v1.4.0
	github.com/textileio/go-textile v0.7.8-0.20200102164400-98b263e32c0c
	github.com/yosssi/gohtml v0.0.0-20190915184251-7ff6f235ecaf
	github.com/yuin/goldmark v1.1.22
	golang.org/x/crypto v0.0.0-20191011191535-87dc89f01550 // indirect
	golang.org/x/image v0.0.0-20190802002840-cff245a6509b // indirect
	golang.org/x/text v0.3.2
	golang.org/x/xerrors v0.0.0-20191011141410-1b5146add898 // indirect
	google.golang.org/grpc v1.27.1
)

replace github.com/textileio/go-textile => github.com/anytypeio/go-textile v0.7.8-0.20200217213349-f936f40b6472

replace github.com/libp2p/go-eventbus => github.com/libp2p/go-eventbus v0.1.0
