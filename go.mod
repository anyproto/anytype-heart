module github.com/anyproto/anytype-heart

go 1.24.0

toolchain go1.24.3

require (
	github.com/JohannesKaufmann/html-to-markdown v1.4.0
	github.com/PuerkitoBio/goquery v1.10.2
	github.com/VividCortex/ewma v1.2.0
	github.com/adrium/goheif v0.0.0-20230113233934-ca402e77a786
	github.com/anyproto/any-store v0.3.5
	github.com/anyproto/any-sync v0.9.9
	github.com/anyproto/anytype-publish-server/publishclient v0.0.0-20250716122732-cdcfe3a126bb
	github.com/anyproto/anytype-push-server/pushclient v0.0.0-20250801122506-553f6c085a23
	github.com/anyproto/go-chash v0.1.0
	github.com/anyproto/go-naturaldate/v2 v2.0.2-0.20230524105841-9829cfd13438
	github.com/anyproto/go-slip10 v1.0.0
	github.com/anyproto/lexid v0.0.6
	github.com/anyproto/tantivy-go v1.0.4
	github.com/araddon/dateparse v0.0.0-20210429162001-6b43995a97de
	github.com/avast/retry-go/v4 v4.6.1
	github.com/chai2010/webp v1.4.0
	github.com/cheggaaa/mb/v3 v3.0.2
	github.com/dave/jennifer v1.7.1
	github.com/davecgh/go-spew v1.1.1
	github.com/dgraph-io/badger/v4 v4.2.0
	github.com/dhowden/tag v0.0.0-20240417053706-3d75831295e8
	github.com/didip/tollbooth/v8 v8.0.1
	github.com/dsoprea/go-exif/v3 v3.0.1
	github.com/dsoprea/go-jpeg-image-structure/v2 v2.0.0-20221012074422-4f3f7e934102
	github.com/ethereum/go-ethereum v1.13.15
	github.com/gabriel-vasile/mimetype v1.4.9
	github.com/gin-gonic/gin v1.10.0
	github.com/globalsign/mgo v0.0.0-20181015135952-eeefdecb41b8
	github.com/go-chi/chi/v5 v5.2.1
	github.com/go-shiori/go-readability v0.0.0-20241012063810-92284fa8a71f
	github.com/goccy/go-graphviz v0.2.9
	github.com/gofrs/flock v0.12.1
	github.com/gogo/protobuf v1.3.2
	github.com/gogo/status v1.1.1
	github.com/golang-jwt/jwt v3.2.2+incompatible
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da
	github.com/golang/snappy v1.0.0
	github.com/google/uuid v1.6.0
	github.com/gosimple/slug v1.15.0
	github.com/gosimple/unidecode v1.0.1
	github.com/grokify/html-strip-tags-go v0.1.0
	github.com/grpc-ecosystem/go-grpc-middleware v1.4.0
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0
	github.com/grpc-ecosystem/grpc-opentracing v0.0.0-20180507213350-8e809c8a8645
	github.com/hashicorp/go-multierror v1.1.1
	github.com/hashicorp/golang-lru/v2 v2.0.7
	github.com/hashicorp/yamux v0.1.2
	github.com/hbagdi/go-unsplash v0.0.0-20230414214043-474fc02c9119
	github.com/huandu/skiplist v1.2.1
	github.com/iancoleman/strcase v0.3.0
	github.com/improbable-eng/grpc-web v0.15.0
	github.com/ipfs/boxo v0.33.1
	github.com/ipfs/go-block-format v0.2.2
	github.com/ipfs/go-cid v0.5.0
	github.com/ipfs/go-datastore v0.8.3
	github.com/ipfs/go-ds-flatfs v0.5.5
	github.com/ipfs/go-ipld-format v0.6.2
	github.com/ipfs/go-log v1.0.5
	github.com/joho/godotenv v1.5.1
	github.com/jsummers/gobmp v0.0.0-20230614200233-a9de23ed2e25
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/klauspost/compress v1.18.0
	github.com/kovidgoyal/imaging v1.6.4
	github.com/libp2p/zeroconf/v2 v2.2.0
	github.com/logrusorgru/aurora v2.0.3+incompatible
	github.com/magiconair/properties v1.8.9
	github.com/matishsiao/goInfo v0.0.0-20240924010139-10388a85396f
	github.com/mattn/go-sqlite3 v1.14.22
	github.com/mb0/diff v0.0.0-20131118162322-d8d9a906c24d
	github.com/microcosm-cc/bluemonday v1.0.27
	github.com/miolini/datacounter v1.0.3
	github.com/mr-tron/base58 v1.2.0
	github.com/multiformats/go-base32 v0.1.0
	github.com/multiformats/go-multiaddr-dns v0.4.1
	github.com/multiformats/go-multibase v0.2.0
	github.com/multiformats/go-multihash v0.2.3
	github.com/oov/psd v0.0.0-20220121172623-5db5eafcecbb
	github.com/opentracing/opentracing-go v1.2.0
	github.com/otiai10/copy v1.14.1
	github.com/otiai10/opengraph/v2 v2.1.0
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.23.0
	github.com/pseudomuto/protoc-gen-doc v1.5.1
	github.com/quic-go/quic-go v0.54.0
	github.com/rwcarlsen/goexif v0.0.0-20190401172101-9e8deecbddbd
	github.com/samber/lo v1.49.1
	github.com/sasha-s/go-deadlock v0.3.5
	github.com/shirou/gopsutil/v3 v3.24.5
	github.com/srwiley/oksvg v0.0.0-20221011165216-be6e8873101c
	github.com/srwiley/rasterx v0.0.0-20220730225603-2ab79fcdd4ef
	github.com/stretchr/testify v1.11.1
	github.com/swaggo/swag/v2 v2.0.0-rc4
	github.com/uber/jaeger-client-go v2.30.0+incompatible
	github.com/valyala/fastjson v1.6.4
	github.com/vektra/mockery/v2 v2.47.0
	github.com/xeipuuv/gojsonschema v1.2.0
	github.com/yuin/goldmark v1.7.8
	github.com/zeebo/blake3 v0.2.4
	go.abhg.dev/goldmark/wikilink v0.6.0
	go.uber.org/atomic v1.11.0
	go.uber.org/mock v0.5.2
	go.uber.org/multierr v1.11.0
	go.uber.org/zap v1.27.0
	golang.org/x/exp v0.0.0-20250819193227-8b4c13bb791b
	golang.org/x/image v0.27.0
	golang.org/x/mobile v0.0.0-20250218173827-cd096645fcd3
	golang.org/x/net v0.43.0
	golang.org/x/oauth2 v0.30.0
	golang.org/x/sys v0.36.0
	golang.org/x/text v0.28.0
	google.golang.org/grpc v1.73.0
	google.golang.org/protobuf v1.36.6
	gopkg.in/Graylog2/go-gelf.v2 v2.0.0-20180125164251-1832d8546a9f
	gopkg.in/natefinch/lumberjack.v2 v2.2.1
	gopkg.in/yaml.v3 v3.0.1
	storj.io/drpc v0.0.34
	zombiezen.com/go/sqlite v1.4.2
)

require (
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/HdrHistogram/hdrhistogram-go v1.1.2 // indirect
	github.com/KyleBanks/depth v1.2.1 // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/semver v1.5.0 // indirect
	github.com/Masterminds/sprig v2.22.0+incompatible // indirect
	github.com/alecthomas/units v0.0.0-20240927000941-0f3dac36c52b // indirect
	github.com/alexbrainman/goissue34681 v0.0.0-20191006012335-3fc7a47baff5 // indirect
	github.com/andybalholm/cascadia v1.3.3 // indirect
	github.com/anyproto/go-slip21 v1.0.0 // indirect
	github.com/anyproto/go-sqlite v1.4.2-any // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/btcsuite/btcd v0.24.2 // indirect
	github.com/btcsuite/btcd/btcec/v2 v2.3.5 // indirect
	github.com/btcsuite/btcd/btcutil v1.1.6 // indirect
	github.com/btcsuite/btcd/chaincfg/chainhash v1.1.0 // indirect
	github.com/btcsuite/btcutil v1.0.3-0.20220129005943-27c39e0ab4f9 // indirect
	github.com/bytedance/sonic v1.12.3 // indirect
	github.com/bytedance/sonic/loader v0.2.0 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cespare/xxhash v1.1.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/chigopher/pathlib v0.19.1 // indirect
	github.com/cloudwego/base64x v0.1.4 // indirect
	github.com/cloudwego/iasm v0.2.0 // indirect
	github.com/crackcomm/go-gitignore v0.0.0-20241020182519-7843d2ba8fdf // indirect
	github.com/davidlazar/go-crypto v0.0.0-20200604182044-b73af7476f6c // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.0 // indirect
	github.com/desertbit/timer v0.0.0-20180107155436-c41aec40b27f // indirect
	github.com/dgraph-io/ristretto v0.1.1 // indirect
	github.com/disintegration/imaging v1.6.2 // indirect
	github.com/dsoprea/go-iptc v0.0.0-20200609062250-162ae6b44feb // indirect
	github.com/dsoprea/go-logging v0.0.0-20200710184922-b02d349568dd // indirect
	github.com/dsoprea/go-photoshop-info-format v0.0.0-20200609050348-3db9b63b202c // indirect
	github.com/dsoprea/go-utility/v2 v2.0.0-20221003172846-a3e1774ef349 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/envoyproxy/protoc-gen-validate v1.2.1 // indirect
	github.com/flopp/go-findfont v0.1.0 // indirect
	github.com/fogleman/gg v1.3.0 // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/gammazero/deque v1.0.0 // indirect
	github.com/gin-contrib/sse v0.1.0 // indirect
	github.com/go-errors/errors v1.4.2 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-openapi/jsonpointer v0.19.6 // indirect
	github.com/go-openapi/jsonreference v0.20.2 // indirect
	github.com/go-openapi/spec v0.20.9 // indirect
	github.com/go-openapi/swag v0.22.3 // indirect
	github.com/go-pkgz/expirable-cache/v3 v3.0.0 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.22.1 // indirect
	github.com/go-shiori/dom v0.0.0-20230515143342-73569d674e1c // indirect
	github.com/go-xmlfmt/xmlfmt v0.0.0-20191208150333-d5b6f63a941b // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/goccy/go-json v0.10.3 // indirect
	github.com/gogo/googleapis v1.3.1 // indirect
	github.com/gogs/chardet v0.0.0-20211120154057-b7413eaefb8f // indirect
	github.com/golang/freetype v0.0.0-20170609003504-e2365dfdc4a0 // indirect
	github.com/golang/geo v0.0.0-20210211234256-740aa86cb551 // indirect
	github.com/golang/glog v1.2.4 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/flatbuffers v1.12.1 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/gopherjs/gopherjs v0.0.0-20190430165422-3e4dfb77656c // indirect
	github.com/gorilla/css v1.0.1 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/holiman/uint256 v1.2.4 // indirect
	github.com/huandu/xstrings v1.4.0 // indirect
	github.com/imdario/mergo v0.3.16 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/ipfs/bbloom v0.0.4 // indirect
	github.com/ipfs/go-bitfield v1.1.0 // indirect
	github.com/ipfs/go-ipld-legacy v0.2.2 // indirect
	github.com/ipfs/go-log/v2 v2.8.1 // indirect
	github.com/ipfs/go-metrics-interface v0.3.0 // indirect
	github.com/ipld/go-codec-dagpb v1.7.0 // indirect
	github.com/ipld/go-ipld-prime v0.21.0 // indirect
	github.com/jbenet/go-temp-err-catcher v0.1.0 // indirect
	github.com/jinzhu/copier v0.3.5 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/libp2p/go-buffer-pool v0.1.0 // indirect
	github.com/libp2p/go-libp2p v0.42.1 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/miekg/dns v1.1.67 // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/multiformats/go-base36 v0.2.0 // indirect
	github.com/multiformats/go-multiaddr v0.16.0 // indirect
	github.com/multiformats/go-multicodec v0.9.2 // indirect
	github.com/multiformats/go-multistream v0.6.1 // indirect
	github.com/multiformats/go-varint v0.0.7 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mwitkow/go-proto-validators v0.3.2 // indirect
	github.com/ncruces/go-strftime v0.1.9 // indirect
	github.com/otiai10/mint v1.6.3 // indirect
	github.com/pelletier/go-toml/v2 v2.2.2 // indirect
	github.com/petermattis/goid v0.0.0-20240813172612-4fcff4a6cae7 // indirect
	github.com/planetscale/vtprotobuf v0.6.1-0.20250313105119-ba97887b0a25 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/polydawn/refmt v0.89.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.65.0 // indirect
	github.com/prometheus/procfs v0.17.0 // indirect
	github.com/pseudomuto/protokit v0.2.1 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/rs/cors v1.11.0 // indirect
	github.com/rs/zerolog v1.29.0 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/spf13/afero v1.10.0 // indirect
	github.com/spf13/cast v1.5.0 // indirect
	github.com/spf13/cobra v1.7.0 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/spf13/viper v1.15.0 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/subosito/gotenv v1.4.2 // indirect
	github.com/sv-tools/openapi v0.2.1 // indirect
	github.com/tetratelabs/wazero v1.8.1 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	github.com/tyler-smith/go-bip39 v1.1.0 // indirect
	github.com/uber/jaeger-lib v2.4.1+incompatible // indirect
	github.com/ugorji/go/codec v1.2.12 // indirect
	github.com/whyrusleeping/chunker v0.0.0-20181014151217-fe64bd25879f // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20180127040702-4e3ac2762d5f // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	github.com/zeebo/errs v1.4.0 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/otel v1.37.0 // indirect
	go.opentelemetry.io/otel/metric v1.37.0 // indirect
	go.opentelemetry.io/otel/trace v1.37.0 // indirect
	golang.org/x/arch v0.8.0 // indirect
	golang.org/x/crypto v0.41.0 // indirect
	golang.org/x/mod v0.27.0 // indirect
	golang.org/x/sync v0.16.0 // indirect
	golang.org/x/term v0.34.0 // indirect
	golang.org/x/time v0.12.0 // indirect
	golang.org/x/tools v0.36.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250603155806-513f23925822 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250603155806-513f23925822 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	lukechampine.com/blake3 v1.4.1 // indirect
	modernc.org/libc v1.66.8 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
	modernc.org/sqlite v1.37.1 // indirect
	nhooyr.io/websocket v1.8.7 // indirect
)

replace github.com/ipfs/go-ds-flatfs => github.com/anyproto/go-ds-flatfs v0.0.0-20250828183910-d49f5b2d567f

replace github.com/dgraph-io/badger/v4 => github.com/anyproto/badger/v4 v4.2.1-0.20240110160636-80743fa3d580

replace github.com/dgraph-io/ristretto => github.com/anyproto/ristretto v0.1.2-0.20240221153107-2b23839cc50c

replace github.com/libp2p/zeroconf/v2 => github.com/anyproto/zeroconf/v2 v2.2.1-0.20240228113933-f90a5cc4439d

replace github.com/JohannesKaufmann/html-to-markdown => github.com/anyproto/html-to-markdown v0.0.0-20231025221133-830bf0a6f139

replace github.com/ipfs/go-log/v2 => github.com/anyproto/go-log/v2 v2.1.2-0.20220721095711-bcf09ff293b2

replace gopkg.in/Graylog2/go-gelf.v2 => github.com/anyproto/go-gelf v0.0.0-20210418191311-774bd5b016e7

replace github.com/araddon/dateparse => github.com/mehanizm/dateparse v0.0.0-20210806203422-f82c8742c9f8 // use a fork to support dd.mm.yyyy date format

replace github.com/multiformats/go-multiaddr => github.com/anyproto/go-multiaddr v0.8.1-0.20250307125826-51ba58e2ebc7

replace github.com/gogo/protobuf => github.com/anyproto/protobuf v1.3.3-0.20240201225420-6e325cf0ac38

replace google.golang.org/genproto/googleapis/rpc => google.golang.org/genproto/googleapis/rpc v0.0.0-20241021214115-324edc3d5d38

replace github.com/btcsuite/btcutil => github.com/btcsuite/btcd/btcutil v1.1.5

replace github.com/dsoprea/go-jpeg-image-structure/v2 => github.com/dchesterton/go-jpeg-image-structure/v2 v2.0.0-20240318203529-c3eea088bd38
