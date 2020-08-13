module github.com/anytypeio/go-anytype-middleware

go 1.13

require (
	github.com/JohannesKaufmann/html-to-markdown v0.0.0-00010101000000-000000000000
	github.com/PuerkitoBio/goquery v1.5.1
	github.com/anytypeio/go-anytype-library v0.11.5-0.20200813223945-04fde6c44417
	github.com/cheggaaa/mb v1.0.2
	github.com/globalsign/mgo v0.0.0-20181015135952-eeefdecb41b8
	github.com/gogo/protobuf v1.3.1
	github.com/golang/mock v1.4.4
	github.com/google/martian v2.1.0+incompatible
	github.com/google/uuid v1.1.1
	github.com/h2non/filetype v1.1.0
	github.com/hashicorp/golang-lru v0.5.4
	github.com/improbable-eng/grpc-web v0.12.0
	github.com/magiconair/properties v1.8.0
	github.com/mauidude/go-readability v0.0.0-20141216012317-2f30b1a346f1
	github.com/mb0/diff v0.0.0-20131118162322-d8d9a906c24d
	github.com/microcosm-cc/bluemonday v1.0.3
	github.com/otiai10/opengraph v1.1.2
	github.com/santhosh-tekuri/jsonschema/v2 v2.2.0
	github.com/stretchr/testify v1.6.1
	github.com/yuin/goldmark v1.1.30
	go.uber.org/zap v1.15.0
	golang.org/x/image v0.0.0-20190802002840-cff245a6509b // indirect
	golang.org/x/text v0.3.3
	google.golang.org/grpc v1.31.0-dev.0.20200627230533-68098483a7af
)

replace github.com/JohannesKaufmann/html-to-markdown => github.com/anytypeio/html-to-markdown v0.0.0-20200617145221-2afd2a14bae1

replace github.com/textileio/go-threads => github.com/anytypeio/go-threads v0.1.18-0.20200805204901-9c9a659b6cd9

replace github.com/ipfs/go-log/v2 => github.com/anytypeio/go-log/v2 v2.1.2-0.20200810212702-264b187bb04f
