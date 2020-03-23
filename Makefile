setup:
	go mod download

fmt:
	echo 'Formatting with prettier...'
	npx prettier --write "./**" 2> /dev/null || true
	echo 'Formatting with goimports...'
	goimports -w -l `find . -type f -name '*.go' -not -path './vendor/*'`

lint:
	echo 'Linting with prettier...'
	npx prettier --check "./**" 2> /dev/null || true
	echo 'Linting with golint...'
	golint `go list ./... | grep -v /vendor/`

protos:
	$(eval P_TIMESTAMP := Mgoogle/protobuf/timestamp.proto=github.com/gogo/protobuf/types)
	$(eval P_STRUCT := Mgoogle/protobuf/struct.proto=github.com/gogo/protobuf/types)
	$(eval PKGMAP := $$(P_TIMESTAMP),$$(P_STRUCT))
	GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 protoc --gogofaster_out=$(PKGMAP):./pb/ pb/model/protos/*.proto; mv pb/github.com/anytypeio/go-anytype-library/pb/model/*.go pb/model/; rm -rf pb/github.com
	GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 protoc --gogofaster_out=$(PKGMAP):./pb/ pb/storage/protos/*.proto; mv pb/github.com/anytypeio/go-anytype-library/pb/storage/*.go pb/storage/; rm -rf pb/github.com
	GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 protoc --gogofaster_out=$(PKGMAP):./pb/ pb/lsmodel/protos/*.proto; mv pb/github.com/anytypeio/go-anytype-library/pb/lsmodel/*.go pb/lsmodel/; rm -rf pb/github.com
